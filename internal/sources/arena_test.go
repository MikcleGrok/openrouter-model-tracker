package sources

import (
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
