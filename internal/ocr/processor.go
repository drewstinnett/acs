package ocr

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/drewstinnett/acs/internal/store"
)

// Options controls OCR processing behaviour.
type Options struct {
	Workers  int
	Lang     string
	KeepPNG  bool
	Rerun    bool
	ItemID   string
	DataDir  string
}

// Processor orchestrates JP2→PNG conversion and Tesseract OCR for pages.
type Processor struct {
	db   *store.DB
	opts Options
	log  *slog.Logger
}

// NewProcessor creates a new Processor.
func NewProcessor(db *store.DB, opts Options, log *slog.Logger) *Processor {
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if log == nil {
		log = slog.Default()
	}
	return &Processor{db: db, opts: opts, log: log}
}

// Run processes all pages that need OCR.
func (p *Processor) Run(ctx context.Context) error {
	status := "pending"
	if p.opts.Rerun {
		status = "complete" // rerun completed pages
	}

	var pages []store.Page
	var err error

	if p.opts.Rerun {
		// Collect both pending and complete.
		pending, e1 := p.db.ListPagesByOCRStatus(p.opts.ItemID, "pending")
		complete, e2 := p.db.ListPagesByOCRStatus(p.opts.ItemID, "complete")
		if e1 != nil {
			return e1
		}
		if e2 != nil {
			return e2
		}
		pages = append(pending, complete...)
	} else {
		pages, err = p.db.ListPagesByOCRStatus(p.opts.ItemID, status)
		if err != nil {
			return err
		}
		// Also pick up error pages for retry.
		errPages, _ := p.db.ListPagesByOCRStatus(p.opts.ItemID, "error")
		pages = append(pages, errPages...)
	}

	total := len(pages)
	if total == 0 {
		p.log.Info("no pages to OCR")
		return nil
	}
	p.log.Info("starting OCR", "pages", total, "workers", p.opts.Workers)

	sem := make(chan struct{}, p.opts.Workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for _, pg := range pages {
		select {
		case <-ctx.Done():
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(pg store.Page) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := p.processPage(ctx, pg); err != nil {
				p.log.Error("OCR failed", "page_id", pg.ID, "err", err)
			}

			mu.Lock()
			done++
			fmt.Fprintf(os.Stderr, "\rOCR progress: %d/%d (%d%%)   ",
				done, total, done*100/total)
			mu.Unlock()
		}(pg)
	}
	wg.Wait()
	fmt.Fprintln(os.Stderr)

	// Update OCR counts for affected items.
	items := uniqueItemIDs(pages)
	for _, itemID := range items {
		if err := p.db.UpdateItemOCRCounts(itemID); err != nil {
			p.log.Warn("failed to update OCR counts", "item_id", itemID, "err", err)
		}
	}

	return nil
}

func (p *Processor) processPage(ctx context.Context, pg store.Page) error {
	if pg.LocalPath == "" {
		return fmt.Errorf("page %d has no local_path (not downloaded)", pg.ID)
	}
	if _, err := os.Stat(pg.LocalPath); err != nil {
		return fmt.Errorf("local file missing: %w", err)
	}

	// Convert JP2 → PNG.
	pngPath := pg.PNGPath
	if pngPath == "" {
		ext := filepath.Ext(pg.LocalPath)
		pngPath = pg.LocalPath[:len(pg.LocalPath)-len(ext)] + ".png"
	}

	if _, err := os.Stat(pngPath); err != nil {
		p.log.Debug("converting JP2 to PNG", "page_id", pg.ID, "src", pg.LocalPath)
		converted, err := ConvertJP2ToPNG(ctx, pg.LocalPath, pngPath)
		if err != nil {
			_ = p.db.SetPageOCRError(pg.ID, err.Error())
			return err
		}
		pngPath = converted
	}

	// Run Tesseract.
	p.log.Debug("running OCR", "page_id", pg.ID, "png", pngPath)
	text, conf, err := RunTesseract(ctx, pngPath, p.opts.Lang)
	if err != nil {
		_ = p.db.SetPageOCRError(pg.ID, err.Error())
		if !p.opts.KeepPNG {
			_ = os.Remove(pngPath)
		}
		return err
	}

	if !p.opts.KeepPNG {
		_ = os.Remove(pngPath)
		pngPath = "" // don't cache if not keeping
	}

	if err := p.db.SetPageOCR(pg.ID, text, conf, pngPath); err != nil {
		return fmt.Errorf("store OCR result: %w", err)
	}

	p.log.Debug("OCR complete", "page_id", pg.ID, "confidence", conf)
	return nil
}

func uniqueItemIDs(pages []store.Page) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pages {
		if !seen[p.ItemID] {
			seen[p.ItemID] = true
			out = append(out, p.ItemID)
		}
	}
	return out
}
