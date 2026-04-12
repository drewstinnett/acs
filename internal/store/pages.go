package store

import (
	"database/sql"
	"errors"
	"time"
)

// Page represents one scanned page/image in the database.
type Page struct {
	ID             int64
	ItemID         string
	IAFilename     string
	IAFilesize     int64
	IAMD5          string
	LocalPath      string
	PNGPath        string
	PageSeq        int
	DownloadStatus string
	DownloadError  string
	DownloadedAt   *time.Time
	OCRStatus      string
	OCRText        string
	OCRConfidence  float64
	OCRError       string
	OCRRunAt       *time.Time
	CorrectedText  string
	CorrectedAt    *time.Time
}

// UpsertPage inserts or updates a page record.
// If the page already exists (by item_id + ia_filename), it updates only
// metadata fields and does not touch download/ocr state.
func (d *DB) UpsertPage(p Page) (int64, error) {
	res, err := d.sql.Exec(`
		INSERT INTO pages (item_id, ia_filename, ia_filesize, ia_md5, page_seq)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_id, ia_filename) DO UPDATE SET
			ia_filesize = excluded.ia_filesize,
			ia_md5      = excluded.ia_md5,
			page_seq    = excluded.page_seq
	`, p.ItemID, p.IAFilename, p.IAFilesize, nullStr(p.IAMD5), p.PageSeq)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		// On conflict update, LastInsertId may be 0; fetch the actual id.
		var existing int64
		_ = d.sql.QueryRow(
			`SELECT id FROM pages WHERE item_id=? AND ia_filename=?`, p.ItemID, p.IAFilename,
		).Scan(&existing)
		return existing, nil
	}
	return id, nil
}

// GetPage fetches a page by its primary key.
func (d *DB) GetPage(id int64) (*Page, error) {
	row := d.sql.QueryRow(`
		SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
		       local_path, png_path, page_seq,
		       download_status, download_error, downloaded_at,
		       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
		       corrected_text, corrected_at
		FROM pages WHERE id = ?`, id)
	return scanPage(row)
}

// ListPages returns all pages for an item ordered by sequence.
func (d *DB) ListPages(itemID string) ([]Page, error) {
	rows, err := d.sql.Query(`
		SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
		       local_path, png_path, page_seq,
		       download_status, download_error, downloaded_at,
		       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
		       corrected_text, corrected_at
		FROM pages WHERE item_id = ? ORDER BY page_seq`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPages(rows)
}

// ListPagesByOCRStatus returns pages for all items with a given ocr_status.
// If itemID is non-empty, restricts to that item.
func (d *DB) ListPagesByOCRStatus(itemID, status string) ([]Page, error) {
	var rows *sql.Rows
	var err error
	if itemID != "" {
		rows, err = d.sql.Query(`
			SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
			       local_path, png_path, page_seq,
			       download_status, download_error, downloaded_at,
			       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
			       corrected_text, corrected_at
			FROM pages WHERE item_id=? AND ocr_status=? ORDER BY page_seq`, itemID, status)
	} else {
		rows, err = d.sql.Query(`
			SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
			       local_path, png_path, page_seq,
			       download_status, download_error, downloaded_at,
			       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
			       corrected_text, corrected_at
			FROM pages WHERE ocr_status=? ORDER BY item_id, page_seq`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPages(rows)
}

// ListPagesByDownloadStatus returns pages with the given download status.
func (d *DB) ListPagesByDownloadStatus(itemID, status string) ([]Page, error) {
	var rows *sql.Rows
	var err error
	if itemID != "" {
		rows, err = d.sql.Query(`
			SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
			       local_path, png_path, page_seq,
			       download_status, download_error, downloaded_at,
			       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
			       corrected_text, corrected_at
			FROM pages WHERE item_id=? AND download_status=? ORDER BY page_seq`, itemID, status)
	} else {
		rows, err = d.sql.Query(`
			SELECT id, item_id, ia_filename, ia_filesize, ia_md5,
			       local_path, png_path, page_seq,
			       download_status, download_error, downloaded_at,
			       ocr_status, ocr_text, ocr_confidence, ocr_error, ocr_run_at,
			       corrected_text, corrected_at
			FROM pages WHERE download_status=? ORDER BY item_id, page_seq`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPages(rows)
}

// SetPageDownloaded marks a page as downloaded and records its local path.
func (d *DB) SetPageDownloaded(id int64, localPath string) error {
	_, err := d.sql.Exec(`
		UPDATE pages SET download_status='complete', local_path=?,
		       downloaded_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		       download_error=NULL
		WHERE id=?`, localPath, id)
	return err
}

// SetPageDownloadError records a download failure.
func (d *DB) SetPageDownloadError(id int64, msg string) error {
	_, err := d.sql.Exec(`
		UPDATE pages SET download_status='error', download_error=? WHERE id=?`, msg, id)
	return err
}

// SetPageOCR stores OCR results for a page.
func (d *DB) SetPageOCR(id int64, text string, confidence float64, pngPath string) error {
	_, err := d.sql.Exec(`
		UPDATE pages SET ocr_status='complete', ocr_text=?, ocr_confidence=?,
		       png_path=?, ocr_run_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		       ocr_error=NULL
		WHERE id=?`, text, confidence, nullStr(pngPath), id)
	return err
}

// SetPageOCRError records an OCR failure.
func (d *DB) SetPageOCRError(id int64, msg string) error {
	_, err := d.sql.Exec(`
		UPDATE pages SET ocr_status='error', ocr_error=? WHERE id=?`, msg, id)
	return err
}

// SetPageCorrectedText saves a user's manual correction.
func (d *DB) SetPageCorrectedText(id int64, text string) error {
	_, err := d.sql.Exec(`
		UPDATE pages SET corrected_text=?,
		       corrected_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?`, text, id)
	return err
}

// SetPagePNGPath records the path to a converted PNG.
func (d *DB) SetPagePNGPath(id int64, pngPath string) error {
	_, err := d.sql.Exec(`UPDATE pages SET png_path=? WHERE id=?`, pngPath, id)
	return err
}

// AdjacentPages returns the IDs of the previous and next pages within the same item.
func (d *DB) AdjacentPages(pageID int64) (prevID, nextID int64, err error) {
	var itemID string
	var seq int
	err = d.sql.QueryRow(`SELECT item_id, page_seq FROM pages WHERE id=?`, pageID).Scan(&itemID, &seq)
	if err != nil {
		return 0, 0, err
	}

	_ = d.sql.QueryRow(
		`SELECT id FROM pages WHERE item_id=? AND page_seq<? ORDER BY page_seq DESC LIMIT 1`, itemID, seq,
	).Scan(&prevID)
	_ = d.sql.QueryRow(
		`SELECT id FROM pages WHERE item_id=? AND page_seq>? ORDER BY page_seq ASC LIMIT 1`, itemID, seq,
	).Scan(&nextID)
	return prevID, nextID, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func scanPage(row scanner) (*Page, error) {
	var p Page
	var md5, localPath, pngPath, dlErr, ocrText, ocrErr, corrText sql.NullString
	var ocrConf sql.NullFloat64
	var dlAt, ocrAt, corrAt sql.NullString
	var filesize sql.NullInt64

	err := row.Scan(
		&p.ID, &p.ItemID, &p.IAFilename, &filesize, &md5,
		&localPath, &pngPath, &p.PageSeq,
		&p.DownloadStatus, &dlErr, &dlAt,
		&p.OCRStatus, &ocrText, &ocrConf, &ocrErr, &ocrAt,
		&corrText, &corrAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.IAFilesize = filesize.Int64
	p.IAMD5 = md5.String
	p.LocalPath = localPath.String
	p.PNGPath = pngPath.String
	p.DownloadError = dlErr.String
	p.OCRText = ocrText.String
	p.OCRConfidence = ocrConf.Float64
	p.OCRError = ocrErr.String
	p.CorrectedText = corrText.String

	parseTime := func(ns sql.NullString) *time.Time {
		if !ns.Valid {
			return nil
		}
		t, _ := time.Parse(time.RFC3339Nano, ns.String)
		return &t
	}
	p.DownloadedAt = parseTime(dlAt)
	p.OCRRunAt = parseTime(ocrAt)
	p.CorrectedAt = parseTime(corrAt)
	return &p, nil
}

func scanPages(rows *sql.Rows) ([]Page, error) {
	var pages []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		if p != nil {
			pages = append(pages, *p)
		}
	}
	return pages, rows.Err()
}
