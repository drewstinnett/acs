package archive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// DownloadURL returns the S3-compatible download URL for a file in an IA item.
func (c *Client) DownloadURL(identifier, filename string) string {
	return fmt.Sprintf("https://s3.us.archive.org/%s/%s", identifier, filename)
}

// ProgressFunc is called periodically during a download with bytes written so far and total size.
type ProgressFunc func(written, total int64)

// DownloadFile downloads a file from IA to destPath.
// If destPath already exists and the MD5 matches expectedMD5, the download is skipped.
// progress may be nil.
func (c *Client) DownloadFile(ctx context.Context, identifier, filename, destPath, expectedMD5 string, progress ProgressFunc) error {
	// Check if file already exists and is valid.
	if expectedMD5 != "" {
		if md5Match(destPath, expectedMD5) {
			return nil // already downloaded and verified
		}
	} else if _, err := os.Stat(destPath); err == nil {
		return nil // exists, no md5 to verify
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	dlURL := c.DownloadURL(identifier, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("download %q: %w", filename, err)
	}
	defer resp.Body.Close()

	total, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)

	f, err := os.CreateTemp(filepath.Dir(destPath), ".acs-dl-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() {
		f.Close()
		os.Remove(tmpPath) // no-op if renamed successfully
	}()

	h := md5.New()
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
			h.Write(buf[:n])
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Verify MD5 if provided.
	if expectedMD5 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != expectedMD5 {
			return fmt.Errorf("MD5 mismatch for %q: got %s want %s", filename, got, expectedMD5)
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename to dest: %w", err)
	}
	return nil
}

// md5Match returns true if the file at path exists and its MD5 equals expected.
func md5Match(path, expected string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == expected
}
