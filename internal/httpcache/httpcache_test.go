package httpcache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newCountingServer(t *testing.T, body string) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestGetFetchesOnCacheMiss(t *testing.T) {
	srv, hits := newCountingServer(t, "hello")
	c := New(t.TempDir(), time.Hour)

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

func TestGetServesFromCacheWithinTTL(t *testing.T) {
	srv, hits := newCountingServer(t, "hello")
	c := New(t.TempDir(), time.Hour)

	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Fatalf("server hits = %d, want 1 (the second Get must be served from disk)", got)
	}
}

func TestGetRefetchesAfterTTL(t *testing.T) {
	srv, hits := newCountingServer(t, "hello")
	dir := t.TempDir()
	c := New(dir, time.Hour)

	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("first Get: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache dir has %d entries (err %v), want exactly 1", len(entries), err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, entries[0].Name()), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("server hits = %d, want 2 (the expired entry must be re-fetched)", got)
	}
}

func TestGetWithZeroTTLAlwaysFetches(t *testing.T) {
	srv, hits := newCountingServer(t, "hello")
	c := New(t.TempDir(), 0)
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("server hits = %d, want 2 for zero TTL", got)
	}
}

func TestGetWithZeroTimeoutDoesNotApplyDefaultTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	c := NewWithTimeout(t.TempDir(), time.Hour, 0)
	if body, err := c.Get(context.Background(), srv.URL); err != nil || string(body) != "hello" {
		t.Fatalf("Get with explicit zero timeout = %q, %v; zero must mean no client timeout", body, err)
	}
}

func TestGetReturnsErrorOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(t.TempDir(), time.Hour)
	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("Get returned nil error on HTTP 500, want an error")
	}
}
