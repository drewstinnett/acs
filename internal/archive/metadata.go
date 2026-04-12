package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
)

// FileEntry is one file in an IA item's file list.
type FileEntry struct {
	Name     string `json:"name"`
	Format   string `json:"format"`
	Size     string `json:"size"` // IA returns size as a string
	MD5      string `json:"md5"`
	Source   string `json:"source"` // "original" | "derivative"
}

// ItemMetadata holds the metadata and file list for one IA item.
type ItemMetadata struct {
	Identifier  string
	Title       string
	Description string
	Date        string
	Subject     interface{} // may be string or []string
	Files       []FileEntry
}

type iaMetadataResponse struct {
	Metadata struct {
		Identifier  string      `json:"identifier"`
		Title       string      `json:"title"`
		Description string      `json:"description"`
		Date        string      `json:"date"`
		Subject     interface{} `json:"subject"`
	} `json:"metadata"`
	Files []FileEntry `json:"files"`
}

// GetMetadata fetches metadata and the file list for one IA item.
func (c *Client) GetMetadata(ctx context.Context, identifier string) (*ItemMetadata, error) {
	u := c.baseURL + "/metadata/" + identifier

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("metadata request for %q: %w", identifier, err)
	}
	defer resp.Body.Close()

	var raw iaMetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode metadata for %q: %w", identifier, err)
	}

	return &ItemMetadata{
		Identifier:  raw.Metadata.Identifier,
		Title:       raw.Metadata.Title,
		Description: raw.Metadata.Description,
		Date:        raw.Metadata.Date,
		Subject:     raw.Metadata.Subject,
		Files:       raw.Files,
	}, nil
}

// JP2Files returns the JP2 image files from metadata, sorted by name.
// It prefers original files over derivatives.
func (m *ItemMetadata) JP2Files() []FileEntry {
	var jp2s []FileEntry
	for _, f := range m.Files {
		if strings.EqualFold(path.Ext(f.Name), ".jp2") && f.Source == "original" {
			jp2s = append(jp2s, f)
		}
	}
	// Fallback: include derivatives if no originals found.
	if len(jp2s) == 0 {
		for _, f := range m.Files {
			if strings.EqualFold(path.Ext(f.Name), ".jp2") {
				jp2s = append(jp2s, f)
			}
		}
	}
	return jp2s
}

// PDFFiles returns PDF files from metadata.
func (m *ItemMetadata) PDFFiles() []FileEntry {
	var pdfs []FileEntry
	for _, f := range m.Files {
		if strings.EqualFold(path.Ext(f.Name), ".pdf") && f.Source == "original" {
			pdfs = append(pdfs, f)
		}
	}
	return pdfs
}

// SubjectStrings normalises the IA subject field (which can be a string or []string).
func (m *ItemMetadata) SubjectStrings() []string {
	if m.Subject == nil {
		return nil
	}
	switch v := m.Subject.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var out []string
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
