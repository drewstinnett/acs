package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/drewstinnett/acs/internal/archive"
	"github.com/drewstinnett/acs/internal/store"
	"github.com/spf13/cobra"
)

var (
	dlItem    string
	dlWorkers int
	dlFormat  string
	dlDryRun  bool
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download microfilm images from Internet Archive",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDB()
		if err != nil {
			return err
		}
		defer db.Close()

		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return fmt.Errorf("create data dir: %w", err)
		}

		// Determine which items to download.
		var items []store.Item
		if dlItem != "" {
			it, err := db.GetItem(dlItem)
			if err != nil || it == nil {
				log.Info("item not in database, adding", "id", dlItem)
				_ = db.UpsertItem(store.Item{ID: dlItem})
				it = &store.Item{ID: dlItem, DownloadStatus: "pending"}
			}
			items = []store.Item{*it}
		} else {
			pending, err := db.ListItemsByDownloadStatus("pending")
			if err != nil {
				return err
			}
			partial, err := db.ListItemsByDownloadStatus("partial")
			if err != nil {
				return err
			}
			items = append(pending, partial...)
		}

		if len(items) == 0 {
			fmt.Println("No items to download. Use 'acs search <query> --save' to add items.")
			return nil
		}

		client := archive.NewClient(log)

		for _, item := range items {
			if err := downloadItem(cmd.Context(), db, client, item); err != nil {
				log.Error("download failed", "item", item.ID, "err", err)
				_ = db.SetItemDownloadStatus(item.ID, "error", err.Error())
			}
		}
		return nil
	},
}

func downloadItem(ctx context.Context, db *store.DB, client *archive.Client, item store.Item) error {
	log.Info("fetching metadata", "item", item.ID)
	meta, err := client.GetMetadata(ctx, item.ID)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}

	// Pick files based on preferred format.
	var files []archive.FileEntry
	if dlFormat == "pdf" {
		files = meta.PDFFiles()
	}
	if len(files) == 0 {
		files = meta.JP2Files()
	}
	if len(files) == 0 {
		return fmt.Errorf("no downloadable files found for %q", item.ID)
	}

	// Sort by name for stable ordering.
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	log.Info("found files", "item", item.ID, "count", len(files))

	// Upsert pages into DB.
	for seq, f := range files {
		if _, err := db.UpsertPage(store.Page{
			ItemID:     item.ID,
			IAFilename: f.Name,
			IAFilesize: parseSize(f.Size),
			IAMD5:      f.MD5,
			PageSeq:    seq,
		}); err != nil {
			return fmt.Errorf("upsert page: %w", err)
		}
	}
	if err := db.SetItemMetadataFetched(item.ID, len(files)); err != nil {
		log.Warn("set metadata fetched", "err", err)
	}

	if dlDryRun {
		fmt.Printf("[dry-run] Would download %d files for %q\n", len(files), item.ID)
		return nil
	}

	// Fetch pages that still need downloading.
	pendingPages, err := db.ListPagesByDownloadStatus(item.ID, "pending")
	if err != nil {
		return err
	}
	errPages, _ := db.ListPagesByDownloadStatus(item.ID, "error")
	pendingPages = append(pendingPages, errPages...)

	total := len(pendingPages)
	if total == 0 {
		log.Info("all pages already downloaded", "item", item.ID)
		_ = db.SetItemDownloadStatus(item.ID, "complete", "")
		return nil
	}

	log.Info("downloading pages", "item", item.ID, "count", total, "workers", dlWorkers)

	sem := make(chan struct{}, dlWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for _, pg := range pendingPages {
		wg.Add(1)
		sem <- struct{}{}
		go func(pg store.Page) {
			defer wg.Done()
			defer func() { <-sem }()

			destPath := filepath.Join(dataDir, item.ID, pg.IAFilename)

			if err := client.DownloadFile(ctx, item.ID, pg.IAFilename, destPath, pg.IAMD5, nil); err != nil {
				log.Error("download error", "file", pg.IAFilename, "err", err)
				_ = db.SetPageDownloadError(pg.ID, err.Error())
			} else {
				_ = db.SetPageDownloaded(pg.ID, destPath)
			}

			mu.Lock()
			done++
			fmt.Fprintf(os.Stderr, "\r[%s] downloading: %d/%d (%d%%)   ",
				item.ID, done, total, done*100/total)
			mu.Unlock()
		}(pg)
	}
	wg.Wait()
	fmt.Fprintln(os.Stderr)

	// Determine overall download status.
	remaining, _ := db.ListPagesByDownloadStatus(item.ID, "pending")
	errored, _ := db.ListPagesByDownloadStatus(item.ID, "error")

	status := "complete"
	if len(remaining) > 0 || len(errored) > 0 {
		status = "partial"
	}
	return db.SetItemDownloadStatus(item.ID, status, "")
}

func init() {
	downloadCmd.Flags().StringVar(&dlItem, "item", "", "specific IA identifier to download")
	downloadCmd.Flags().IntVar(&dlWorkers, "workers", 2, "parallel download workers")
	downloadCmd.Flags().StringVar(&dlFormat, "format", "jp2", "preferred format: jp2 or pdf")
	downloadCmd.Flags().BoolVar(&dlDryRun, "dry-run", false, "show what would be downloaded without downloading")
}

func parseSize(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
