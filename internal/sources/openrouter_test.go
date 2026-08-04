package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
)

// serveFixture starts a test server returning the given testdata file and
// returns a cache client pointed at a fresh temp dir.
func serveFixture(t *testing.T, fixture string) (*httptest.Server, *httpcache.Client) {
	t.Helper()
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, httpcache.New(t.TempDir(), time.Hour)
}

func TestLookupPrices(t *testing.T) {
	srv, c := serveFixture(t, "testdata/openrouter-models.json")
	old := CatalogURL
	CatalogURL = srv.URL
	t.Cleanup(func() { CatalogURL = old })

	got, err := LookupPrices(context.Background(), c, []string{
		"openai/gpt-5.6-luna",
		"qwen/qwen3.8-max",
		"nvidia/nemotron-3-ultra-550b-a55b:free",
		"x-ai/grok-4.1-fast",
	})
	if err != nil {
		t.Fatalf("LookupPrices: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4 (one per requested slug)", len(got))
	}

	luna := got["openai/gpt-5.6-luna"]
	if !luna.Found || luna.InPerM != 0.5 || luna.OutPerM != 3 || luna.Context != 1000000 || luna.Free {
		t.Errorf("luna = %+v, want in=0.5 out=3 ctx=1000000 free=false found=true", luna)
	}
	if !luna.HasOverride || luna.OverrideMinTokens != 272000 || luna.OverrideInPerM != 1 || luna.OverrideOutPerM != 4 {
		t.Errorf("luna override = %+v, want hasOverride=true minTokens=272000 in=1 out=4 (cache-price override fields must be ignored)", luna)
	}

	qwen := got["qwen/qwen3.8-max"]
	if !qwen.Found || qwen.InPerM != 2 || qwen.OutPerM != 6 || qwen.Context != 262144 {
		t.Errorf("qwen = %+v, want in=2 out=6 ctx=262144 (cache-price fields must be ignored)", qwen)
	}
	if !qwen.HasOverride || qwen.OverrideMinTokens != 64000 || qwen.OverrideInPerM != 2.5 || qwen.OverrideOutPerM != 7 {
		t.Errorf("qwen override = %+v, want the override with the SMALLEST min_prompt_tokens (64000) to win, regardless of array order", qwen)
	}

	free := got["nvidia/nemotron-3-ultra-550b-a55b:free"]
	if !free.Found || !free.Free || free.InPerM != 0 || free.OutPerM != 0 {
		t.Errorf("free = %+v, want free=true with zero prices", free)
	}
	if free.HasOverride {
		t.Errorf("free = %+v, want HasOverride=false — the fixture has no overrides field for this model", free)
	}

	gone := got["x-ai/grok-4.1-fast"]
	if gone.Found {
		t.Errorf("gone = %+v, want Found=false for a slug absent from the catalogue", gone)
	}
	if gone.Slug != "x-ai/grok-4.1-fast" {
		t.Errorf("gone.Slug = %q, want the requested slug echoed back", gone.Slug)
	}
}

func TestCatalogSlugs(t *testing.T) {
	srv, c := serveFixture(t, "testdata/openrouter-models.json")
	old := CatalogURL
	CatalogURL = srv.URL
	t.Cleanup(func() { CatalogURL = old })

	got, err := CatalogSlugs(context.Background(), c)
	if err != nil {
		t.Fatalf("CatalogSlugs: %v", err)
	}
	want := []string{
		"nvidia/nemotron-3-ultra-550b-a55b:free",
		"openai/gpt-5.6-luna",
		"qwen/qwen3.8-max",
		"some/other-model",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d slugs, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slug[%d] = %q, want %q (result must be sorted)", i, got[i], want[i])
		}
	}
}

func TestFetchCatalogFailsLoudlyOnGarbage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	old := CatalogURL
	CatalogURL = srv.URL
	t.Cleanup(func() { CatalogURL = old })

	if _, err := CatalogSlugs(context.Background(), httpcache.New(t.TempDir(), time.Hour)); err == nil {
		t.Fatal("CatalogSlugs returned nil error on an empty catalogue, want an error so the orchestrator falls back to the snapshot")
	}
}
