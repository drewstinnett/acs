package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Item represents one Internet Archive item in the database.
type Item struct {
	ID                 string
	Title              string
	Description        string
	Date               string
	Year               int
	Subject            []string
	MetadataFetchedAt  *time.Time
	DownloadStatus     string
	DownloadError      string
	OCRStatus          string
	PageCount          int
	OCRPageCount       int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// UpsertItem inserts or updates an item. It never overwrites download/ocr status columns
// when the item already exists (to avoid stomping on in-progress work).
func (d *DB) UpsertItem(it Item) error {
	subj, err := json.Marshal(it.Subject)
	if err != nil {
		return fmt.Errorf("marshal subject: %w", err)
	}
	_, err = d.sql.Exec(`
		INSERT INTO ia_items (id, title, description, date, year, subject)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title       = excluded.title,
			description = excluded.description,
			date        = excluded.date,
			year        = excluded.year,
			subject     = excluded.subject,
			updated_at  = strftime('%Y-%m-%dT%H:%M:%fZ','now')
	`, it.ID, it.Title, it.Description, it.Date, it.Year, string(subj))
	return err
}

// GetItem retrieves an item by ID.
func (d *DB) GetItem(id string) (*Item, error) {
	row := d.sql.QueryRow(`
		SELECT id, title, description, date, year, subject,
		       metadata_fetched_at, download_status, download_error,
		       ocr_status, page_count, ocr_page_count, created_at, updated_at
		FROM ia_items WHERE id = ?`, id)
	return scanItem(row)
}

// ListItems returns all items ordered by year.
func (d *DB) ListItems() ([]Item, error) {
	rows, err := d.sql.Query(`
		SELECT id, title, description, date, year, subject,
		       metadata_fetched_at, download_status, download_error,
		       ocr_status, page_count, ocr_page_count, created_at, updated_at
		FROM ia_items ORDER BY year, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// ListItemsByDownloadStatus returns items with the given status.
func (d *DB) ListItemsByDownloadStatus(status string) ([]Item, error) {
	rows, err := d.sql.Query(`
		SELECT id, title, description, date, year, subject,
		       metadata_fetched_at, download_status, download_error,
		       ocr_status, page_count, ocr_page_count, created_at, updated_at
		FROM ia_items WHERE download_status = ? ORDER BY year, id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

// SetItemDownloadStatus updates the download status of an item.
func (d *DB) SetItemDownloadStatus(id, status, errMsg string) error {
	_, err := d.sql.Exec(`
		UPDATE ia_items SET download_status = ?, download_error = ?,
		       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`, status, nullStr(errMsg), id)
	return err
}

// SetItemMetadataFetched marks the item's metadata as fetched and updates page_count.
func (d *DB) SetItemMetadataFetched(id string, pageCount int) error {
	_, err := d.sql.Exec(`
		UPDATE ia_items
		SET metadata_fetched_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		    page_count = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`, pageCount, id)
	return err
}

// UpdateItemOCRCounts recalculates and stores the ocr_page_count and ocr_status for an item.
func (d *DB) UpdateItemOCRCounts(id string) error {
	var total, done int
	err := d.sql.QueryRow(
		`SELECT COUNT(*), COUNT(CASE WHEN ocr_status='complete' THEN 1 END) FROM pages WHERE item_id=?`, id,
	).Scan(&total, &done)
	if err != nil {
		return err
	}
	status := "pending"
	if done > 0 && done < total {
		status = "partial"
	} else if total > 0 && done == total {
		status = "complete"
	}
	_, err = d.sql.Exec(`
		UPDATE ia_items SET ocr_page_count=?, ocr_status=?,
		       updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?`, done, status, id)
	return err
}

// Stats holds aggregate counts for the status endpoint.
type Stats struct {
	ItemCount    int
	PageCount    int
	OCRPageCount int
}

// GetStats returns aggregate statistics.
func (d *DB) GetStats() (Stats, error) {
	var s Stats
	err := d.sql.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(page_count),0), COALESCE(SUM(ocr_page_count),0)
		FROM ia_items`).Scan(&s.ItemCount, &s.PageCount, &s.OCRPageCount)
	return s, err
}

// ── helpers ──────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (*Item, error) {
	var it Item
	var subjJSON string
	var metaFetched sql.NullString
	var dlErr sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&it.ID, &it.Title, &it.Description, &it.Date, &it.Year, &subjJSON,
		&metaFetched, &it.DownloadStatus, &dlErr,
		&it.OCRStatus, &it.PageCount, &it.OCRPageCount, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(subjJSON), &it.Subject)
	if metaFetched.Valid {
		t, _ := time.Parse(time.RFC3339Nano, metaFetched.String)
		it.MetadataFetchedAt = &t
	}
	if dlErr.Valid {
		it.DownloadError = dlErr.String
	}
	it.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	it.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &it, nil
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		if it != nil {
			items = append(items, *it)
		}
	}
	return items, rows.Err()
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
