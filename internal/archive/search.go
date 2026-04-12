package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// SearchResult is one item returned by the IA search API.
type SearchResult struct {
	Identifier  string   `json:"identifier"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Date        string   `json:"date"`
	Subject     []string `json:"subject"`
	Creator     string   `json:"creator"`
	Language    string   `json:"language"`
}

type iaSearchResponse struct {
	Response struct {
		NumFound int            `json:"numFound"`
		Start    int            `json:"start"`
		Docs     []SearchResult `json:"docs"`
	} `json:"response"`
}

// Search queries the Internet Archive advanced search API.
// It returns up to limit results for the given query.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	params := url.Values{
		"q":          {query},
		"output":     {"json"},
		"rows":       {strconv.Itoa(limit)},
		"fl[]":       {"identifier,title,description,date,subject,creator,language"},
		"sort[]":     {"date asc"},
	}
	u := c.baseURL + "/advancedsearch.php?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	var result iaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	return result.Response.Docs, nil
}
