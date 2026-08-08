package sources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ArenaURL is the LMArena text leaderboard. Unlike the two SWE-bench sources
// it embeds no single JSON block: the page is a Next.js app whose data
// arrives as a React Server Components "flight" stream, split across numbered
// self.__next_f.push([1,"..."]) chunks. Each chunk's payload is a JSON string
// literal, and a value can be cut in half between two chunks — so the whole
// stream is unescaped and concatenated back together before anything is read
// out of it.
var ArenaURL = "https://arena.ai/leaderboard/text"

// MetricArenaElo is the Bradley-Terry rating the text arena publishes
// (roughly 950–1550). It is a crowd preference score, not a test on real
// pull requests: it is never comparable with MetricSWEBenchVerified and never
// shares a column with it.
const MetricArenaElo = "LMArena Elo"

// arenaEntriesKey is the single anchor this parser relies on inside the
// flight stream: the leaderboard object's own entries array.
const arenaEntriesKey = `"entries":[`

var (
	arenaChunkRe      = regexp.MustCompile(`self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]\)`)
	arenaVoteCutoffRe = regexp.MustCompile(`"voteCutoffISOString":"([^"]*)"`)
)

// arenaEntry declares only the fields we use. The payload also carries rank,
// confidence bounds, prices and licence strings whose presence and type vary
// between rows; encoding/json skips undeclared fields for free.
type arenaEntry struct {
	ModelKey         string  `json:"modelKey"`
	ModelDisplayName string  `json:"modelDisplayName"`
	Rating           float64 `json:"rating"`
}

// arenaFlight unescapes every push payload and concatenates them in document
// order, which is exactly what the browser does with __next_f. A chunk that
// is not a valid JSON string is skipped rather than fatal: one malformed
// chunk elsewhere on the page must not hide the leaderboard.
func arenaFlight(page []byte) string {
	var b strings.Builder
	for _, m := range arenaChunkRe.FindAllSubmatch(page, -1) {
		var chunk string
		if err := json.Unmarshal(m[1], &chunk); err != nil {
			continue
		}
		b.WriteString(chunk)
	}
	return b.String()
}

// arenaArrayEnd returns the index just past the ] that closes the array
// starting at s[start], ignoring brackets that occur inside string literals.
// A plain strings.Index for "]" would cut the array at the first model name
// that happens to contain one.
func arenaArrayEnd(s string, start int) (int, bool) {
	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

// arenaEntries cuts the leaderboard array out of the reassembled flight
// stream and decodes it. Every failure is an error, never a partial result:
// the orchestrator treats an error as "no Arena data this run" and falls back
// to the snapshot, which is strictly better than a silently truncated table.
func arenaEntries(flight string) ([]arenaEntry, error) {
	i := strings.Index(flight, arenaEntriesKey)
	if i < 0 {
		return nil, fmt.Errorf("no %s array in the flight payload", arenaEntriesKey)
	}
	start := i + len(`"entries":`)
	end, ok := arenaArrayEnd(flight, start)
	if !ok {
		return nil, fmt.Errorf("the entries array is not closed")
	}
	var entries []arenaEntry
	if err := json.Unmarshal([]byte(flight[start:end]), &entries); err != nil {
		return nil, fmt.Errorf("decode entries: %w", err)
	}
	return entries, nil
}
