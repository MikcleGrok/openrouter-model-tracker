// Package httpcache is a tiny GET-only HTTP client that caches successful
// responses on disk for a TTL. It never serves stale content: falling back to
// the previous run's values is the orchestrator's job, via the snapshot.
package httpcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Client caches GET responses under dir for ttl.
type Client struct {
	dir    string
	ttl    time.Duration
	http   *http.Client
	force  bool
	mu     *sync.Mutex
	rename func(string, string) error
}

// Options controls one cache client without changing the compatible Get API.
type Options struct {
	Force bool
}

// Result contains a cached body and the timestamp of the successful network
// fetch that produced it. A nil timestamp means the cache entry is legacy or
// its metadata is unavailable.
type Result struct {
	Body             []byte
	NetworkFetchedAt *time.Time
}

type metadata struct {
	NetworkFetchedAt string `json:"network_fetched_at"`
	BodySHA256       string `json:"body_sha256"`
}

const maxResponseBytes = 4 << 20

// New returns a Client caching into dir with the given freshness window.
func New(dir string, ttl time.Duration) *Client {
	return NewWithTimeout(dir, ttl, 30*time.Second)
}

func NewWithTimeout(dir string, ttl, timeout time.Duration) *Client {
	return &Client{dir: dir, ttl: ttl, http: &http.Client{Timeout: timeout}, mu: &sync.Mutex{}, rename: os.Rename}
}

// WithOptions returns a client sharing the same cache and HTTP transport.
func (c *Client) WithOptions(opts Options) *Client {
	clone := *c
	clone.force = opts.Force
	return &clone
}

func (c *Client) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".bin")
}

func (c *Client) metadataPath(url string) string {
	return c.path(url)[:len(c.path(url))-len(".bin")] + ".meta.json"
}

// Get returns the body of url, from the disk cache when it is younger than the
// TTL and from the network otherwise.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	result, err := c.GetWithMetadata(ctx, url)
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

// GetWithMetadata is Get with the network-fetch timestamp attached when known.
func (c *Client) GetWithMetadata(ctx context.Context, url string) (Result, error) {
	c.mu.Lock()
	p := c.path(url)
	if !c.force {
		if result, ok := c.readFresh(url, p); ok {
			c.mu.Unlock()
			return result, nil
		}
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("httpcache: build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "openrouter-model-tracker/1.0 (+local)")

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("httpcache: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("httpcache: GET %s: status %d", url, resp.StatusCode)
	}
	if resp.ContentLength > maxResponseBytes {
		return Result{}, fmt.Errorf("httpcache: GET %s: response exceeds %d bytes", url, maxResponseBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("httpcache: read %s: %w", url, err)
	}
	if int64(len(body)) > maxResponseBytes {
		return Result{}, fmt.Errorf("httpcache: GET %s: response exceeds %d bytes", url, maxResponseBytes)
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("httpcache: create cache dir: %w", err)
	}
	fetchedAt := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.writeEntry(url, body, fetchedAt); err != nil {
		return Result{}, err
	}
	return Result{Body: body, NetworkFetchedAt: &fetchedAt}, nil
}

func (c *Client) readFresh(url, bodyPath string) (Result, bool) {
	st, err := os.Stat(bodyPath)
	if err != nil || time.Since(st.ModTime()) >= c.ttl {
		return Result{}, false
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return Result{}, false
	}
	metadataBody, err := os.ReadFile(c.metadataPath(url))
	if err != nil {
		return Result{}, false
	}
	var m metadata
	if json.Unmarshal(metadataBody, &m) != nil || m.NetworkFetchedAt == "" || m.BodySHA256 == "" {
		return Result{}, false
	}
	fetchedAt, err := time.Parse(time.RFC3339Nano, m.NetworkFetchedAt)
	if err != nil {
		return Result{}, false
	}
	digest := sha256.Sum256(body)
	if !strings.EqualFold(m.BodySHA256, hex.EncodeToString(digest[:])) {
		return Result{}, false
	}
	return Result{Body: body, NetworkFetchedAt: &fetchedAt}, true
}

func (c *Client) writeEntry(url string, body []byte, fetchedAt time.Time) error {
	bodyTmp, err := os.CreateTemp(c.dir, ".httpcache-body-*")
	if err != nil {
		return fmt.Errorf("httpcache: create body temp: %w", err)
	}
	bodyTmpName := bodyTmp.Name()
	defer os.Remove(bodyTmpName)
	if err := bodyTmp.Chmod(0o644); err != nil {
		bodyTmp.Close()
		return fmt.Errorf("httpcache: chmod body temp: %w", err)
	}
	if _, err := bodyTmp.Write(body); err != nil {
		bodyTmp.Close()
		return fmt.Errorf("httpcache: write cache entry: %w", err)
	}
	if err := bodyTmp.Close(); err != nil {
		return fmt.Errorf("httpcache: close body temp: %w", err)
	}
	metaTmp, err := os.CreateTemp(c.dir, ".httpcache-meta-*")
	if err != nil {
		return fmt.Errorf("httpcache: create metadata temp: %w", err)
	}
	metaTmpName := metaTmp.Name()
	defer os.Remove(metaTmpName)
	if err := metaTmp.Chmod(0o644); err != nil {
		metaTmp.Close()
		return fmt.Errorf("httpcache: chmod metadata temp: %w", err)
	}
	digest := sha256.Sum256(body)
	meta, err := json.Marshal(metadata{NetworkFetchedAt: fetchedAt.Format(time.RFC3339Nano), BodySHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		metaTmp.Close()
		return err
	}
	if _, err := metaTmp.Write(meta); err != nil {
		metaTmp.Close()
		return fmt.Errorf("httpcache: write metadata: %w", err)
	}
	if err := metaTmp.Close(); err != nil {
		return fmt.Errorf("httpcache: close metadata temp: %w", err)
	}
	bodyPath, metadataPath := c.path(url), c.metadataPath(url)
	bodyBackup, metadataBackup := bodyPath+".backup", metadataPath+".backup"
	removeBackup := func() error {
		return errors.Join(removeIfExists(bodyBackup), removeIfExists(metadataBackup))
	}
	if err := removeBackup(); err != nil {
		return fmt.Errorf("httpcache: remove old backups: %w", err)
	}
	removeBackup()
	bodyHadOriginal, metadataHadOriginal := false, false
	if err := c.rename(bodyPath, bodyBackup); err == nil {
		bodyHadOriginal = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("httpcache: backup cache entry: %w", err)
	}
	if err := c.rename(metadataPath, metadataBackup); err == nil {
		metadataHadOriginal = true
	} else if !os.IsNotExist(err) {
		restoreErr := error(nil)
		if bodyHadOriginal {
			restoreErr = c.rename(bodyBackup, bodyPath)
		}
		return fmt.Errorf("httpcache: backup metadata: %w", errors.Join(err, restoreErr))
	}
	rollback := func() error {
		errs := []error{removeIfExists(bodyPath), removeIfExists(metadataPath)}
		if bodyHadOriginal {
			errs = append(errs, c.rename(bodyBackup, bodyPath))
		}
		if metadataHadOriginal {
			errs = append(errs, c.rename(metadataBackup, metadataPath))
		}
		errs = append(errs, removeBackup())
		return errors.Join(errs...)
	}
	if err := c.rename(bodyTmpName, bodyPath); err != nil {
		return fmt.Errorf("httpcache: publish cache entry: %w", errors.Join(err, rollback()))
	}
	if err := c.rename(metaTmpName, metadataPath); err != nil {
		return fmt.Errorf("httpcache: publish metadata: %w", errors.Join(err, rollback()))
	}
	if err := removeBackup(); err != nil {
		return fmt.Errorf("httpcache: cleanup backups: %w", err)
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// NetworkFetchedAt reads metadata without consulting or changing the body cache.
func (c *Client) NetworkFetchedAt(url string) *time.Time { return c.readMetadata(url) }

func (c *Client) readMetadata(url string) *time.Time {
	b, err := os.ReadFile(c.metadataPath(url))
	if err != nil {
		return nil
	}
	var m metadata
	if json.Unmarshal(b, &m) != nil || m.NetworkFetchedAt == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, m.NetworkFetchedAt)
	if err != nil {
		return nil
	}
	return &t
}
