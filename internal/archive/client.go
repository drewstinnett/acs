package archive

import (
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL = "https://archive.org"
	userAgent      = "acs-tool/1.0 (github.com/drewstinnett/acs; historical research)"
)

// Client is a rate-limited HTTP client for the Internet Archive APIs.
type Client struct {
	http    *http.Client
	limiter *rate.Limiter
	baseURL string
	log     *slog.Logger
}

// NewClient creates a new Client with sensible defaults.
func NewClient(log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		http:    &http.Client{Timeout: 60 * time.Second},
		limiter: rate.NewLimiter(rate.Every(2*time.Second), 1),
		baseURL: defaultBaseURL,
		log:     log,
	}
}

// do executes an HTTP request with rate limiting and retry logic.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent)

	const maxRetries = 5
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.limiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("http do: %w", err)
			}
			c.log.Warn("request failed, retrying", "attempt", attempt+1, "err", err)
			sleep(backoff)
			backoff = jitter(backoff * 2)
			// Rebuild the request for retry since body may be consumed.
			req = req.Clone(req.Context())
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusPartialContent:
			return resp, nil
		case http.StatusTooManyRequests:
			resp.Body.Close()
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			c.log.Warn("rate limited by IA", "retry_after", wait, "attempt", attempt+1)
			if wait > 5*time.Minute {
				wait = 5 * time.Minute
			}
			sleep(wait)
			backoff = jitter(backoff * 2)
			req = req.Clone(req.Context())
		case http.StatusServiceUnavailable:
			resp.Body.Close()
			c.log.Warn("IA returned 503, backing off", "wait", backoff, "attempt", attempt+1)
			sleep(backoff)
			backoff = jitter(backoff * 2)
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			req = req.Clone(req.Context())
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, req.URL)
		}
	}
	return nil, fmt.Errorf("max retries exceeded for %s", req.URL)
}

func jitter(d time.Duration) time.Duration {
	extra := time.Duration(rand.Int63n(int64(d / 2)))
	return d + extra
}

func sleep(d time.Duration) {
	time.Sleep(d)
}
