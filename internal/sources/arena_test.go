package sources

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestArenaFlightJoinsChunks(t *testing.T) {
	page, err := os.ReadFile("testdata/arena.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	flight := arenaFlight(page)
	if !strings.Contains(flight, `"modelKey":"hy3-tencent-cloud-text"`) {
		t.Fatalf("arenaFlight did not rejoin a key split across two chunks:\n%s", flight)
	}
}

func TestArenaEntries(t *testing.T) {
	page, err := os.ReadFile("testdata/arena.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	entries, err := arenaEntries(arenaFlight(page))
	if err != nil {
		t.Fatalf("arenaEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}
	if entries[1].ModelKey != "hy3-tencent-cloud-text" || entries[1].ModelDisplayName != "hy3" || entries[1].Rating != 1453 {
		t.Errorf("entries[1] = %+v, want the rejoined Tencent row", entries[1])
	}
}

func TestArenaEntriesRejectsUnclosedArray(t *testing.T) {
	page, err := os.ReadFile("testdata/arena-truncated.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if _, err := arenaEntries(arenaFlight(page)); err == nil {
		t.Fatal("want an error for an unclosed entries array, so the orchestrator falls back to the snapshot")
	}
}

func TestArenaEntriesRejectsPageWithoutEntries(t *testing.T) {
	if _, err := arenaEntries("nothing to see here"); err == nil {
		t.Fatal("want an error when the flight payload has no entries array")
	}
}

func TestArenaArrayEndIgnoresBracketsInsideStrings(t *testing.T) {
	const in = `[{"name":"a]b[c","v":1},{"name":"\"]\"","v":2}] tail`
	end, ok := arenaArrayEnd(in, 0)
	if !ok {
		t.Fatal("arenaArrayEnd did not find the closing bracket")
	}
	if in[:end] != `[{"name":"a]b[c","v":1},{"name":"\"]\"","v":2}]` {
		t.Errorf("arenaArrayEnd cut at %d: %q", end, in[:end])
	}
}

func TestFetchArenaElo(t *testing.T) {
	srv, c := serveFixture(t, "testdata/arena.html")
	old := ArenaURL
	ArenaURL = srv.URL
	t.Cleanup(func() { ArenaURL = old })

	rows, err := FetchArenaElo(context.Background(), c, map[string]string{
		"openai/gpt-5.6-sol":      "gpt-5.6-sol-xhigh-text",
		"tencent/hy3":             "hy3-tencent-cloud-text",
		"openai/gpt-oss-20b:free": "gpt-oss-20b",
		"minimax/minimax-m3":      "minimax-m3", // такой записи на лидерборде нет
	})
	if err != nil {
		t.Fatalf("FetchArenaElo: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (a tracked slug with no arena row is silently absent): %+v", len(rows), rows)
	}

	byslug := map[string]ScoreRow{}
	for _, r := range rows {
		byslug[r.Slug] = r
	}
	hy3 := byslug["tencent/hy3"]
	if hy3.Value != 1453 {
		t.Errorf("hy3.Value = %v, want the raw Elo 1453, not a normalised number", hy3.Value)
	}
	if hy3.Metric != "LMArena Elo" {
		t.Errorf("hy3.Metric = %q, want %q", hy3.Metric, "LMArena Elo")
	}
	if hy3.VariantMeasured != "hy3" {
		t.Errorf("hy3.VariantMeasured = %q, want the arena display name", hy3.VariantMeasured)
	}
	if hy3.Checked != "2026-08-06" {
		t.Errorf("hy3.Checked = %q, want the vote cutoff date", hy3.Checked)
	}
	if hy3.SourceURL != srv.URL {
		t.Errorf("hy3.SourceURL = %q, want %q", hy3.SourceURL, srv.URL)
	}
	if hy3.SourceFamily != "arena" || hy3.ConfiguredIdentity != "hy3-tencent-cloud-text" || hy3.CanonicalID != "hy3-tencent-cloud-text" {
		t.Errorf("hy3 identity = %+v, want the configured Arena namespace identity", hy3)
	}
	if hy3.Provider != "Tencent" || hy3.License != "Apache-2.0" || hy3.ModelURL != "https://arena.ai/models/hy3" || hy3.MetadataSourceURL != srv.URL {
		t.Errorf("hy3 metadata = %+v, want provider/license/link and the Arena source URL", hy3)
	}
	if rows[0].Slug > rows[1].Slug || rows[1].Slug > rows[2].Slug {
		t.Errorf("rows are not sorted by slug: %+v", rows)
	}
}

func TestFetchArenaEloErrorsWhenChunksAreGone(t *testing.T) {
	srv, c := serveFixture(t, "testdata/openrouter-models.json") // страница без __next_f
	old := ArenaURL
	ArenaURL = srv.URL
	t.Cleanup(func() { ArenaURL = old })

	if _, err := FetchArenaElo(context.Background(), c, map[string]string{"a/b": "x"}); err == nil {
		t.Fatal("want an error when the page carries no RSC chunks, so the orchestrator falls back to the snapshot")
	}
}

func TestFetchArenaEloErrorsOnTruncatedPayload(t *testing.T) {
	srv, c := serveFixture(t, "testdata/arena-truncated.html")
	old := ArenaURL
	ArenaURL = srv.URL
	t.Cleanup(func() { ArenaURL = old })

	if _, err := FetchArenaElo(context.Background(), c, map[string]string{"a/b": "x"}); err == nil {
		t.Fatal("want an error for a half-delivered leaderboard, never a partial table")
	}
}

// TestFetchArenaEloErrorsWhenNoRequestedNameMatches guards against a parsing
// regression that finds a real, well-formed entries array — just not the
// leaderboard's own (e.g. "entries":[ now matches some other array earlier
// in the page first). arenaEntries would happily decode it, and if none of
// its rows carry a modelKey we asked for, the naive result is an empty slice
// with a nil error — indistinguishable from "none of these models are on
// Arena today". That would silently bypass applyArenaFallback in
// internal/refresh/run.go (which only engages on an error) and wipe every
// Arena score from the snapshot. When names is non-empty but nothing
// matched, FetchArenaElo must return an error instead.
func TestFetchArenaEloErrorsWhenNoRequestedNameMatches(t *testing.T) {
	srv, c := serveFixture(t, "testdata/arena.html")
	old := ArenaURL
	ArenaURL = srv.URL
	t.Cleanup(func() { ArenaURL = old })

	_, err := FetchArenaElo(context.Background(), c, map[string]string{
		"some/model": "not-on-this-leaderboard",
	})
	if err == nil {
		t.Fatal("want an error when none of the requested names matched a real, well-formed entries array, so the orchestrator falls back to the snapshot instead of silently wiping every Arena score")
	}
}

// TestFetchArenaEloDoesNotErrorWhenNoNamesRequested guards the other half of
// the fix: an empty names map is a legitimate "nothing was requested" case
// (e.g. no model-map.tsv row declares arena= at all), not a failure, and
// must not be turned into a spurious error.
func TestFetchArenaEloDoesNotErrorWhenNoNamesRequested(t *testing.T) {
	srv, c := serveFixture(t, "testdata/arena.html")
	old := ArenaURL
	ArenaURL = srv.URL
	t.Cleanup(func() { ArenaURL = old })

	rows, err := FetchArenaElo(context.Background(), c, map[string]string{})
	if err != nil {
		t.Fatalf("FetchArenaElo with no requested names: %v, want no error", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}
