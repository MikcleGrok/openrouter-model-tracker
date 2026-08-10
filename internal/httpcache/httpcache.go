// Package httpcache is a tiny GET-only HTTP client that caches successful
// responses on disk for a TTL. It never serves stale content: falling back to
// the previous run's values is the orchestrator's job, via the snapshot.
package httpcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client caches GET responses under dir for ttl.
type Client struct {
	dir  string
	ttl  time.Duration
	http *http.Client
}

const maxResponseBytes = 4 << 20

// New returns a Client caching into dir with the given freshness window.
func New(dir string, ttl time.Duration) *Client {
	return NewWithTimeout(dir, ttl, 30*time.Second)
}

func NewWithTimeout(dir string, ttl, timeout time.Duration) *Client {
	return &Client{dir: dir, ttl: ttl, http: &http.Client{Timeout: timeout}}
}

func (c *Client) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".bin")
}

// Get returns the body of url, from the disk cache when it is younger than the
// TTL and from the network otherwise.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	p := c.path(url)
	if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < c.ttl {
		if body, err := os.ReadFile(p); err == nil {
			return body, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("httpcache: build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "openrouter-model-tracker/1.0 (+local)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpcache: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("httpcache: GET %s: status %d", url, resp.StatusCode)
	}
	if resp.ContentLength > maxResponseBytes {
		return nil, fmt.Errorf("httpcache: GET %s: response exceeds %d bytes", url, maxResponseBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("httpcache: read %s: %w", url, err)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, fmt.Errorf("httpcache: GET %s: response exceeds %d bytes", url, maxResponseBytes)
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return nil, fmt.Errorf("httpcache: create cache dir: %w", err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return nil, fmt.Errorf("httpcache: write cache entry: %w", err)
	}
	return body, nil
}
