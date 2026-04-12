package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/drewstinnett/acs/internal/store"
)

//go:embed static_templates/*
var staticTemplateFS embed.FS

var staticFuncMap = template.FuncMap{
	"inc": func(i int) int { return i + 1 },
	"iaImageURL": func(identifier, filename string) string {
		return fmt.Sprintf("https://archive.org/download/%s/%s",
			identifier, url.PathEscape(filename))
	},
	"iaThumbURL": func(identifier, filename string) string {
		encoded := url.PathEscape(identifier) + "%2F" + url.PathEscape(filename)
		return fmt.Sprintf("https://iiif.archive.org/iiif/%s/full/300,/0/default.jpg", encoded)
	},
}

// staticPageTmpl is a single template set that defines all the body/scripts
// snippets plus the base layout. Because each snippet uses a unique name
// (index-body, items-body, etc.) there are no name collisions.
var staticPageTmpl *template.Template

func init() {
	var err error
	staticPageTmpl, err = template.New("base").Funcs(staticFuncMap).ParseFS(staticTemplateFS,
		"static_templates/base.html",
		"static_templates/index.html",
		"static_templates/items.html",
		"static_templates/item.html",
		"static_templates/page.html",
	)
	if err != nil {
		panic(fmt.Sprintf("parse static templates: %v", err))
	}
}

// layoutData is the data passed to the base template.
type layoutData struct {
	Title   string
	Body    template.HTML
	Scripts template.HTML
}

// renderStatic renders a named body template then wraps it in the base layout.
func renderStatic(bodyTmpl, scriptsTmpl, title string, data any) ([]byte, error) {
	// Render body.
	var bodyBuf bytes.Buffer
	if err := staticPageTmpl.ExecuteTemplate(&bodyBuf, bodyTmpl, data); err != nil {
		return nil, fmt.Errorf("render body %s: %w", bodyTmpl, err)
	}

	// Render scripts (may be empty).
	var scriptsBuf bytes.Buffer
	if scriptsTmpl != "" {
		if t := staticPageTmpl.Lookup(scriptsTmpl); t != nil {
			if err := staticPageTmpl.ExecuteTemplate(&scriptsBuf, scriptsTmpl, data); err != nil {
				return nil, fmt.Errorf("render scripts %s: %w", scriptsTmpl, err)
			}
		}
	}

	// Wrap in base layout.
	ld := layoutData{
		Title:   title,
		Body:    template.HTML(bodyBuf.String()),
		Scripts: template.HTML(scriptsBuf.String()),
	}
	var out bytes.Buffer
	if err := staticPageTmpl.ExecuteTemplate(&out, "base", ld); err != nil {
		return nil, fmt.Errorf("render base: %w", err)
	}
	return out.Bytes(), nil
}

// Exporter generates a static HTML site from the database.
type Exporter struct {
	db     *store.DB
	outDir string
	log    *slog.Logger
}

// NewExporter creates a new Exporter.
func NewExporter(db *store.DB, outDir string, log *slog.Logger) *Exporter {
	if log == nil {
		log = slog.Default()
	}
	return &Exporter{db: db, outDir: outDir, log: log}
}

// Export generates the full static site into e.outDir.
func (e *Exporter) Export() error {
	if err := os.MkdirAll(e.outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := e.copyStatic(); err != nil {
		return fmt.Errorf("copy static assets: %w", err)
	}

	stats, _ := e.db.GetStats()

	// Homepage.
	if err := e.writePage(
		"index.html",
		"ACS Archive Browser",
		"index-body", "index-scripts",
		map[string]any{"Stats": stats},
	); err != nil {
		return err
	}
	e.log.Info("exported homepage")

	// Items list.
	items, err := e.db.ListItems()
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}
	if err := e.writePage(
		filepath.Join("items", "index.html"),
		"Items — ACS Archive",
		"items-body", "",
		map[string]any{"Items": items},
	); err != nil {
		return err
	}
	e.log.Info("exported items list", "count", len(items))

	for _, item := range items {
		if err := e.exportItem(item); err != nil {
			e.log.Error("failed to export item", "item", item.ID, "err", err)
		}
	}

	e.log.Info("export complete", "out_dir", e.outDir)
	return nil
}

func (e *Exporter) exportItem(item store.Item) error {
	pages, err := e.db.ListPages(item.ID)
	if err != nil {
		return err
	}

	itemTitle := item.ID
	if item.Title != "" {
		itemTitle = item.Title
	}

	// Item detail.
	if err := e.writePage(
		filepath.Join("items", item.ID, "index.html"),
		itemTitle+" — ACS Archive",
		"item-body", "",
		map[string]any{"Item": &item, "Pages": pages},
	); err != nil {
		return err
	}

	// Per-page viewer.
	for _, pg := range pages {
		prevID, nextID, _ := e.db.AdjacentPages(pg.ID)
		year := ""
		if item.Year > 0 {
			year = fmt.Sprintf("%d", item.Year)
		}

		pageTitle := fmt.Sprintf("Page %d — %s — ACS Archive", pg.PageSeq+1, itemTitle)
		if err := e.writePage(
			filepath.Join("pages", fmt.Sprintf("%d", pg.ID), "index.html"),
			pageTitle,
			"page-body", "",
			map[string]any{
				"Page":      &pg,
				"PrevID":    prevID,
				"NextID":    nextID,
				"ItemTitle": itemTitle,
				"ItemYear":  year,
			},
		); err != nil {
			e.log.Error("failed to export page", "page_id", pg.ID, "err", err)
		}
	}

	e.log.Info("exported item", "id", item.ID, "pages", len(pages))
	return nil
}

// writePage renders a page and writes it to a file.
func (e *Exporter) writePage(relPath, title, bodyTmpl, scriptsTmpl string, data any) error {
	b, err := renderStatic(bodyTmpl, scriptsTmpl, title, data)
	if err != nil {
		return fmt.Errorf("render %s: %w", relPath, err)
	}

	dest := filepath.Join(e.outDir, relPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(dest, b, 0o644)
}

// copyStatic copies embedded static assets into outDir/static/.
func (e *Exporter) copyStatic() error {
	return fs.WalkDir(embeddedStatic, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "static/")
		if rel == "" {
			return nil
		}
		dest := filepath.Join(e.outDir, "static", rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		src, err := embeddedStatic.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		dst, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}
