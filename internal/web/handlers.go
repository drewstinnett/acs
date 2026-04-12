package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/drewstinnett/acs/internal/store"
)

// staticFS embedded in server.go

type Server struct {
	db      *store.DB
	dataDir string
	log     *slog.Logger
	mux     *http.ServeMux
}

func NewServer(db *store.DB, dataDir string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{db: db, dataDir: dataDir, log: log, mux: http.NewServeMux()}
	s.registerRoutes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /_status", s.handleStatus)
	s.mux.HandleFunc("GET /items", s.handleItems)
	s.mux.HandleFunc("GET /items/{id}", s.handleItem)
	s.mux.HandleFunc("GET /pages/{id}", s.handlePage)
	s.mux.HandleFunc("GET /pages/{id}/image", s.handlePageImage)
	s.mux.HandleFunc("GET /pages/{id}/thumb", s.handlePageThumb)
	s.mux.HandleFunc("POST /pages/{id}/correction", s.handlePageCorrection)
	s.mux.HandleFunc("GET /search", s.handleSearch)
	s.mux.HandleFunc("GET /", s.handleIndex)
}

// ── Index ─────────────────────────────────────────────────────────────────

type indexData struct {
	Query      string
	Year       string
	ItemFilter string
	Items      []store.Item
	Results    []store.SearchResult
	Stats      store.Stats
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query().Get("q")
	year := r.URL.Query().Get("year")
	itemFilter := r.URL.Query().Get("item")

	items, _ := s.db.ListItems()
	stats, _ := s.db.GetStats()

	data := indexData{
		Query: q, Year: year, ItemFilter: itemFilter,
		Items: items, Stats: stats,
	}

	if q != "" {
		results, err := s.db.Search(q, itemFilter, 50, 0)
		if err != nil {
			s.log.Error("search error", "err", err)
		}
		data.Results = results
	}

	s.render(w, "index.html", data)
}

// ── Search (HTMX fragment) ────────────────────────────────────────────────

type searchData struct {
	Query      string
	Year       string
	ItemFilter string
	Results    []store.SearchResult
	Items      []store.Item
	Stats      store.Stats
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	itemFilter := r.URL.Query().Get("item")

	var results []store.SearchResult
	if q != "" {
		var err error
		results, err = s.db.Search(q, itemFilter, 50, 0)
		if err != nil {
			s.log.Error("search error", "err", err)
		}
	}

	items, _ := s.db.ListItems()
	stats, _ := s.db.GetStats()

	data := searchData{
		Query: q, ItemFilter: itemFilter,
		Results: results, Items: items, Stats: stats,
	}

	// HTMX requests get just the fragment; full requests get the full page.
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, "results.html", data)
	} else {
		s.render(w, "index.html", indexData{
			Query: q, ItemFilter: itemFilter,
			Items: items, Stats: stats, Results: results,
		})
	}
}

// ── Items list ────────────────────────────────────────────────────────────

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.ListItems()
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, "items.html", map[string]any{"Items": items})
}

// ── Item detail ───────────────────────────────────────────────────────────

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := s.db.GetItem(id)
	if err != nil || item == nil {
		http.NotFound(w, r)
		return
	}
	pages, err := s.db.ListPages(id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, "item.html", map[string]any{"Item": item, "Pages": pages})
}

// ── Page viewer ───────────────────────────────────────────────────────────

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, err := s.db.GetPage(id)
	if err != nil || page == nil {
		http.NotFound(w, r)
		return
	}
	prevID, nextID, _ := s.db.AdjacentPages(id)

	item, _ := s.db.GetItem(page.ItemID)
	itemTitle := page.ItemID
	if item != nil && item.Title != "" {
		itemTitle = item.Title
	}

	s.render(w, "page.html", map[string]any{
		"Page":      page,
		"PrevID":    prevID,
		"NextID":    nextID,
		"ItemTitle": itemTitle,
	})
}

// ── Image serving ─────────────────────────────────────────────────────────

func (s *Server) handlePageImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, err := s.db.GetPage(id)
	if err != nil || page == nil || page.LocalPath == "" {
		http.NotFound(w, r)
		return
	}

	pngPath, err := s.ensurePNG(r.Context(), page)
	if err != nil {
		s.log.Error("PNG conversion failed", "page_id", id, "err", err)
		// Fall back to serving the JP2 directly if conversion fails.
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, page.LocalPath)
		return
	}

	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFile(w, r, pngPath)
}

func (s *Server) handlePageThumb(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page, err := s.db.GetPage(id)
	if err != nil || page == nil || page.LocalPath == "" {
		http.NotFound(w, r)
		return
	}

	// Thumbnail path derived from local_path.
	ext := filepath.Ext(page.LocalPath)
	thumbPath := page.LocalPath[:len(page.LocalPath)-len(ext)] + "_thumb.jpg"

	if _, err := os.Stat(thumbPath); err != nil {
		// Generate thumbnail via ImageMagick.
		cmd := exec.CommandContext(r.Context(), "convert",
			"-thumbnail", "200x",
			page.LocalPath,
			thumbPath,
		)
		if err := cmd.Run(); err != nil {
			http.NotFound(w, r)
			return
		}
	}

	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

// ensurePNG returns a path to a PNG version of the page image, converting on demand.
func (s *Server) ensurePNG(ctx context.Context, page *store.Page) (string, error) {
	if page.PNGPath != "" {
		if _, err := os.Stat(page.PNGPath); err == nil {
			return page.PNGPath, nil
		}
	}

	ext := filepath.Ext(page.LocalPath)
	pngPath := page.LocalPath[:len(page.LocalPath)-len(ext)] + ".png"

	if _, err := os.Stat(pngPath); err == nil {
		_ = s.db.SetPagePNGPath(page.ID, pngPath)
		return pngPath, nil
	}

	// Convert JP2 → PNG.
	cmd := exec.CommandContext(ctx, "convert",
		"-density", "150", // lower resolution for web viewing
		"-depth", "8",
		page.LocalPath,
		pngPath,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert: %w: %s", err, stderr.String())
	}

	_ = s.db.SetPagePNGPath(page.ID, pngPath)
	return pngPath, nil
}

// ── Correction ────────────────────────────────────────────────────────────

func (s *Server) handlePageCorrection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "parse form", http.StatusBadRequest)
		return
	}
	text := r.FormValue("text")

	if err := s.db.SetPageCorrectedText(id, text); err != nil {
		s.serverError(w, err)
		return
	}

	// Return updated text panel (HTMX outerHTML swap).
	page, err := s.db.GetPage(id)
	if err != nil || page == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	prevID, nextID, _ := s.db.AdjacentPages(id)
	item, _ := s.db.GetItem(page.ItemID)
	itemTitle := page.ItemID
	if item != nil && item.Title != "" {
		itemTitle = item.Title
	}

	s.render(w, "page.html", map[string]any{
		"Page":      page,
		"PrevID":    prevID,
		"NextID":    nextID,
		"ItemTitle": itemTitle,
	})
}

// ── Status ────────────────────────────────────────────────────────────────

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	b, err := renderTemplate(name, data)
	if err != nil {
		s.log.Error("template error", "template", name, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.log.Error("server error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
