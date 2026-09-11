package httpcache

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestForceBypassesFreshCache(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&hits, 1)
		_, _ = w.Write([]byte(string(rune('0' + count))))
	}))
	defer srv.Close()
	c := New(t.TempDir(), time.Hour)
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	forced := c.WithOptions(Options{Force: true})
	body, err := forced.Get(context.Background(), srv.URL)
	if err != nil || string(body) != "2" || atomic.LoadInt64(&hits) != 2 {
		t.Fatalf("forced Get = %q, %v, hits=%d; want second network response", body, err, hits)
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
	if err != nil || len(entries) != 2 {
		t.Fatalf("cache dir has %d entries (err %v), want body and metadata", len(entries), err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, filepath.Base(c.path(srv.URL))), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := atomic.LoadInt64(hits); got != 2 {
		t.Fatalf("server hits = %d, want 2 (the expired entry must be re-fetched)", got)
	}
}

func TestGetWithMetadataRecordsNetworkFetchAndKeepsItOnHit(t *testing.T) {
	srv, _ := newCountingServer(t, "hello")
	c := New(t.TempDir(), time.Hour)

	first, err := c.GetWithMetadata(context.Background(), srv.URL)
	if err != nil || first.NetworkFetchedAt == nil {
		t.Fatalf("first GetWithMetadata = %+v, %v; want a timestamp", first, err)
	}
	firstAt := *first.NetworkFetchedAt
	second, err := c.GetWithMetadata(context.Background(), srv.URL)
	if err != nil || second.NetworkFetchedAt == nil || !second.NetworkFetchedAt.Equal(firstAt) {
		t.Fatalf("cache hit metadata = %+v, %v; want original timestamp %v", second, err, firstAt)
	}
}

func TestGetWithMetadataLegacyBodyHasUnknownFreshness(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newCountingServer(t, "network")
	c := New(dir, time.Hour)
	if err := os.WriteFile(c.path(srv.URL), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := c.GetWithMetadata(context.Background(), srv.URL)
	if err != nil || string(result.Body) != "network" || result.NetworkFetchedAt == nil {
		t.Fatalf("legacy result = %+v, %v; want a network refresh", result, err)
	}
}

func TestFreshCacheRejectsBodyMetadataMismatch(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newCountingServer(t, "network")
	c := New(dir, time.Hour)
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	bodyInfo, err := os.Stat(c.path(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path(srv.URL), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(c.path(srv.URL), bodyInfo.ModTime(), bodyInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if body, err := c.Get(context.Background(), srv.URL); err != nil || string(body) != "network" || *hits != 2 {
		t.Fatalf("mismatched cache result = %q, %v, hits=%d; want network refresh", body, err, *hits)
	}
}

func TestFreshCacheRejectsBadDigest(t *testing.T) {
	dir := t.TempDir()
	srv, hits := newCountingServer(t, "network")
	c := New(dir, time.Hour)
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	metadataBody, err := json.Marshal(metadata{NetworkFetchedAt: c.NetworkFetchedAt(srv.URL).Format(time.RFC3339Nano), BodySHA256: "not-a-sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.metadataPath(srv.URL), metadataBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), srv.URL); err != nil || *hits != 2 {
		t.Fatalf("bad digest cache result = %v, hits=%d; want network refresh", err, *hits)
	}
}

func TestFailedNetworkFetchDoesNotReplaceMetadata(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newCountingServer(t, "hello")
	c := New(dir, time.Hour)
	if _, err := c.GetWithMetadata(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	want := c.NetworkFetchedAt(srv.URL)
	srv.Close()
	if _, err := c.GetWithMetadata(context.Background(), srv.URL+"/expired"); err == nil {
		t.Fatal("failed network fetch returned nil error")
	}
	if got := c.NetworkFetchedAt(srv.URL); got == nil || want == nil || !got.Equal(*want) {
		t.Fatalf("metadata after failed fetch = %v, want %v", got, want)
	}
}

func TestForceFailurePreservesBodyAndMetadata(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newCountingServer(t, "old")
	c := New(dir, time.Hour)
	if _, err := c.GetWithMetadata(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	wantBody, err := os.ReadFile(c.path(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	wantMeta := c.NetworkFetchedAt(srv.URL)
	srv.Close()
	if _, err := c.WithOptions(Options{Force: true}).Get(context.Background(), srv.URL); err == nil {
		t.Fatal("forced failed fetch returned nil error")
	}
	gotBody, err := os.ReadFile(c.path(srv.URL))
	if err != nil || !bytes.Equal(gotBody, wantBody) {
		t.Fatalf("body after forced failure = %q, %v; want %q", gotBody, err, wantBody)
	}
	if gotMeta := c.NetworkFetchedAt(srv.URL); gotMeta == nil || wantMeta == nil || !gotMeta.Equal(*wantMeta) {
		t.Fatalf("metadata after forced failure = %v, want %v", gotMeta, wantMeta)
	}
}

func TestForceSuccessUpdatesMetadata(t *testing.T) {
	var body atomic.Value
	body.Store("old")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body.Load().(string))) }))
	defer srv.Close()
	c := New(t.TempDir(), time.Hour)
	first, err := c.GetWithMetadata(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body.Store("new")
	second, err := c.WithOptions(Options{Force: true}).GetWithMetadata(context.Background(), srv.URL)
	if err != nil || string(second.Body) != "new" || second.NetworkFetchedAt == nil || !second.NetworkFetchedAt.After(*first.NetworkFetchedAt) {
		t.Fatalf("forced result = %+v, %v; want new body and metadata", second, err)
	}
}

func TestForceMetadataCommitFailureRestoresBodyAndMetadata(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newCountingServer(t, "old")
	c := New(dir, time.Hour)
	if _, err := c.GetWithMetadata(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	wantBody, err := os.ReadFile(c.path(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	wantMetadata, err := os.ReadFile(c.metadataPath(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	forced := c.WithOptions(Options{Force: true})
	failed := false
	forced.rename = func(src, dst string) error {
		if dst == forced.metadataPath(srv.URL) && !failed {
			failed = true
			return os.ErrPermission
		}
		return os.Rename(src, dst)
	}
	if _, err := forced.Get(context.Background(), srv.URL); err == nil {
		t.Fatal("forced metadata commit unexpectedly succeeded")
	}
	gotBody, err := os.ReadFile(c.path(srv.URL))
	if err != nil || !bytes.Equal(gotBody, wantBody) {
		t.Fatalf("body after metadata commit failure = %q, %v; want %q", gotBody, err, wantBody)
	}
	gotMetadata, err := os.ReadFile(c.metadataPath(srv.URL))
	if err != nil || !bytes.Equal(gotMetadata, wantMetadata) {
		t.Fatalf("metadata after commit failure = %q, %v; want %q", gotMetadata, err, wantMetadata)
	}
}

func TestOversizedNetworkFetchDoesNotReplaceMetadata(t *testing.T) {
	dir := t.TempDir()
	good, _ := newCountingServer(t, "hello")
	c := New(dir, time.Hour)
	if _, err := c.GetWithMetadata(context.Background(), good.URL); err != nil {
		t.Fatal(err)
	}
	want := c.NetworkFetchedAt(good.URL)
	osServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4194305")
		_, _ = w.Write([]byte("too large"))
	}))
	defer osServer.Close()
	if _, err := c.GetWithMetadata(context.Background(), osServer.URL); err == nil {
		t.Fatal("oversized response returned nil error")
	}
	if got := c.NetworkFetchedAt(good.URL); got == nil || want == nil || !got.Equal(*want) {
		t.Fatalf("metadata after oversized fetch = %v, want %v", got, want)
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

func TestGetRejectsOversizedResponseWithoutWritingCache(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4194305")
		_, _ = w.Write([]byte("too large"))
	}))
	defer srv.Close()

	if _, err := New(dir, time.Hour).Get(context.Background(), srv.URL); err == nil {
		t.Fatal("Get accepted oversized response")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache entries = %d, want no cache write", len(entries))
	}
}

func TestGetRejectsUnknownLengthOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, maxResponseBytes+1))
	}))
	defer srv.Close()
	if _, err := New(t.TempDir(), time.Hour).Get(context.Background(), srv.URL); err == nil {
		t.Fatal("Get accepted unknown-length oversized response")
	}
}
