PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS ia_items (
    id                   TEXT PRIMARY KEY,
    title                TEXT NOT NULL DEFAULT '',
    description          TEXT NOT NULL DEFAULT '',
    date                 TEXT,
    year                 INTEGER,
    subject              TEXT,
    metadata_fetched_at  TEXT,
    download_status      TEXT NOT NULL DEFAULT 'pending'
                         CHECK(download_status IN ('pending','partial','complete','error')),
    download_error       TEXT,
    ocr_status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK(ocr_status IN ('pending','partial','complete','error')),
    page_count           INTEGER NOT NULL DEFAULT 0,
    ocr_page_count       INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_ia_items_year       ON ia_items(year);
CREATE INDEX IF NOT EXISTS idx_ia_items_dl_status  ON ia_items(download_status);
CREATE INDEX IF NOT EXISTS idx_ia_items_ocr_status ON ia_items(ocr_status);

CREATE TABLE IF NOT EXISTS pages (
    id              INTEGER PRIMARY KEY,
    item_id         TEXT NOT NULL REFERENCES ia_items(id) ON DELETE CASCADE,
    ia_filename     TEXT NOT NULL,
    ia_filesize     INTEGER,
    ia_md5          TEXT,
    local_path      TEXT,
    png_path        TEXT,
    page_seq        INTEGER NOT NULL DEFAULT 0,
    download_status TEXT NOT NULL DEFAULT 'pending'
                    CHECK(download_status IN ('pending','downloading','complete','error')),
    download_error  TEXT,
    downloaded_at   TEXT,
    ocr_status      TEXT NOT NULL DEFAULT 'pending'
                    CHECK(ocr_status IN ('pending','running','complete','error','skipped')),
    ocr_text        TEXT,
    ocr_confidence  REAL,
    ocr_error       TEXT,
    ocr_run_at      TEXT,
    corrected_text  TEXT,
    corrected_at    TEXT,
    UNIQUE(item_id, ia_filename)
);

CREATE INDEX IF NOT EXISTS idx_pages_item_id    ON pages(item_id);
CREATE INDEX IF NOT EXISTS idx_pages_ocr_status ON pages(ocr_status);
CREATE INDEX IF NOT EXISTS idx_pages_dl_status  ON pages(download_status);
CREATE INDEX IF NOT EXISTS idx_pages_seq        ON pages(item_id, page_seq);

CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
    ocr_text,
    corrected_text,
    content='pages',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS pages_ai AFTER INSERT ON pages BEGIN
    INSERT INTO pages_fts(rowid, ocr_text, corrected_text)
    VALUES (new.id, new.ocr_text, new.corrected_text);
END;

CREATE TRIGGER IF NOT EXISTS pages_ad AFTER DELETE ON pages BEGIN
    INSERT INTO pages_fts(pages_fts, rowid, ocr_text, corrected_text)
    VALUES ('delete', old.id, old.ocr_text, old.corrected_text);
END;

CREATE TRIGGER IF NOT EXISTS pages_au AFTER UPDATE ON pages
WHEN new.ocr_text IS NOT old.ocr_text OR new.corrected_text IS NOT old.corrected_text
BEGIN
    INSERT INTO pages_fts(pages_fts, rowid, ocr_text, corrected_text)
    VALUES ('delete', old.id, old.ocr_text, old.corrected_text);
    INSERT INTO pages_fts(rowid, ocr_text, corrected_text)
    VALUES (new.id, new.ocr_text, new.corrected_text);
END;

CREATE TABLE IF NOT EXISTS saved_searches (
    id         INTEGER PRIMARY KEY,
    query      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
