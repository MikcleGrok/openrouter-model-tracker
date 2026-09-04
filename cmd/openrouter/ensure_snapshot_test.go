package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// TestEnsureLocalSnapshotNoopWhenSnapshotExists proves ensureLocalSnapshot
// never touches the network when a snapshot is already on disk: opts here
// points at a DataDir with no model-map.tsv/notes.yaml at all, so a
// refresh.Run attempt would fail immediately. A nil return is only possible
// if that call was never made.
func TestEnsureLocalSnapshotNoopWhenSnapshotExists(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, refresh.SnapshotFileName), []byte(`{"models":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := refresh.Options{DataDir: filepath.Join(t.TempDir(), "does-not-exist"), OutputPath: filepath.Join(root, "output.md")}
	var out bytes.Buffer
	if err := ensureLocalSnapshot(context.Background(), &out, root, opts); err != nil {
		t.Fatalf("ensureLocalSnapshot with an existing snapshot = %v, want nil (and no refresh attempt)", err)
	}
	if out.Len() != 0 {
		t.Fatalf("ensureLocalSnapshot wrote %q while the snapshot already existed; it should stay silent", out.String())
	}
}

// TestEnsureLocalSnapshotErrorsWithoutOutputWhenMissing covers the case
// where there is nothing to fetch with: no local snapshot, and no
// --output/default_output configured to run a refresh against. This must
// fail fast, with an actionable message, and without attempting the
// network — proven the same way as above: opts.DataDir points nowhere, so a
// refresh.Run call would fail differently (and not with this message) if it
// were ever reached.
func TestEnsureLocalSnapshotErrorsWithoutOutputWhenMissing(t *testing.T) {
	root := t.TempDir()
	opts := refresh.Options{DataDir: filepath.Join(t.TempDir(), "does-not-exist")}
	var out bytes.Buffer
	err := ensureLocalSnapshot(context.Background(), &out, root, opts)
	if err == nil {
		t.Fatal("ensureLocalSnapshot with no snapshot and no OutputPath = nil, want an error")
	}
	if !strings.Contains(err.Error(), "no local snapshot yet") || !strings.Contains(err.Error(), "openrouter refresh") {
		t.Fatalf("error = %v, want a message pointing at 'openrouter refresh'", err)
	}
	if out.Len() != 0 {
		t.Fatalf("ensureLocalSnapshot wrote %q before failing; it should not announce a refresh it never starts", out.String())
	}
}

// TestEnsureLocalSnapshotFetchesOnceWhenMissing exercises the real
// auto-refresh path end-to-end: no local snapshot, but opts.OutputPath is
// configured, so ensureLocalSnapshot must call the real refresh.Run and
// leave a snapshot behind. The four upstream sources refresh.Run's live
// deps hit (internal/sources' CatalogURL, ValsSWEBenchURL, SWEBenchURL,
// ArenaURL) are package-level vars maintained specifically so tests can
// redirect them to an httptest server instead of the real network — see
// internal/sources/*_test.go for the same pattern. The three score-source
// endpoints are pointed at a server returning an empty body: each Fetch
// function treats "expected content block not found" as a soft per-source
// failure (a warning, not a hard error — see internal/refresh's own
// fallback tests), so the run still completes and produces a snapshot with
// price data but no scores.
func TestEnsureLocalSnapshotFetchesOnceWhenMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model-map.tsv"), []byte("demo/high\ttier=sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("models:\n  demo/high:\n    display: Demo High\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"demo/high","name":"Demo High","context_length":128000,"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	t.Cleanup(catalog.Close)
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(empty.Close)

	oldCatalog, oldVals, oldSWEBench, oldArena := sources.CatalogURL, sources.ValsSWEBenchURL, sources.SWEBenchURL, sources.ArenaURL
	sources.CatalogURL = catalog.URL
	sources.ValsSWEBenchURL = empty.URL
	sources.SWEBenchURL = empty.URL
	sources.ArenaURL = empty.URL
	t.Cleanup(func() {
		sources.CatalogURL, sources.ValsSWEBenchURL, sources.SWEBenchURL, sources.ArenaURL = oldCatalog, oldVals, oldSWEBench, oldArena
	})

	opts := refresh.Options{DataDir: root, OutputPath: filepath.Join(root, "output.md")}
	var out bytes.Buffer
	if err := ensureLocalSnapshot(context.Background(), &out, root, opts); err != nil {
		t.Fatalf("ensureLocalSnapshot with a configured output = %v, want a successful auto-refresh", err)
	}
	if !strings.Contains(out.String(), "refresh") {
		t.Errorf("ensureLocalSnapshot did not announce the initial refresh:\n%s", out.String())
	}
	if _, err := os.Stat(refresh.SnapshotPath(root)); err != nil {
		t.Fatalf("refresh.SnapshotPath(root) after auto-refresh: %v, want the snapshot to exist", err)
	}

	// A second call must be a no-op now that the snapshot exists — proven
	// the same way TestEnsureLocalSnapshotNoopWhenSnapshotExists proves it:
	// point the servers at handlers that fail the test if hit again.
	catalog.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("refresh.Run was called again even though the snapshot now exists")
	})
	out.Reset()
	if err := ensureLocalSnapshot(context.Background(), &out, root, opts); err != nil {
		t.Fatalf("ensureLocalSnapshot on a second call = %v, want nil", err)
	}
	if out.Len() != 0 {
		t.Fatalf("ensureLocalSnapshot re-announced a refresh on the second call: %q", out.String())
	}
}
