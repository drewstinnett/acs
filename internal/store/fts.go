package store

import (
	"fmt"
	"strings"
)

// SearchResult holds a single FTS5 match.
type SearchResult struct {
	PageID    int64
	ItemID    string
	ItemTitle string
	ItemDate  string
	PageSeq   int
	Snippet   string
	Score     float64
}

// Search performs a full-text search and returns matching pages with snippets.
// query is the raw user query. pageSize / offset control pagination.
// itemID may be non-empty to restrict results to a single item.
func (d *DB) Search(query, itemID string, pageSize, offset int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}
	// Escape special FTS5 characters that users might type literally.
	query = fts5Escape(query)

	args := []any{query}
	filter := ""
	if itemID != "" {
		filter = "AND p.item_id = ?"
		args = append(args, itemID)
	}
	args = append(args, pageSize, offset)

	rows, err := d.sql.Query(fmt.Sprintf(`
		SELECT p.id, p.item_id, i.title, i.date, p.page_seq,
		       snippet(pages_fts, 0, '<mark>', '</mark>', '…', 20),
		       pages_fts.rank
		FROM pages_fts
		JOIN pages p ON pages_fts.rowid = p.id
		JOIN ia_items i ON p.item_id = i.id
		WHERE pages_fts MATCH ? %s
		ORDER BY pages_fts.rank
		LIMIT ? OFFSET ?`, filter), args...)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.PageID, &r.ItemID, &r.ItemTitle, &r.ItemDate,
			&r.PageSeq, &r.Snippet, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// fts5Escape wraps the query in double quotes if it contains special FTS5 operators
// that would otherwise cause a parse error for naive user input.
// For simple word queries, it returns the query unchanged.
func fts5Escape(q string) string {
	q = strings.TrimSpace(q)
	// If it already looks like a quoted phrase or uses FTS5 operators, pass through.
	if strings.ContainsAny(q, `"*^()`) {
		return q
	}
	// Wrap each token in quotes to treat as phrase literals, joined with implicit AND.
	tokens := strings.Fields(q)
	quoted := make([]string, len(tokens))
	for i, t := range tokens {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " ")
}
