package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
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

// FetchArenaElo returns one row per tracked slug present on the text arena
// leaderboard. names maps a slug to the exact modelKey the site uses, e.g.
// "hy3-tencent-cloud-text"; matching is an exact map lookup and never a
// guess, exactly as for the two SWE-bench sources.
//
// Value is the raw Elo rating. Rescaling it onto the 0–100 range the ranking
// formula is tuned for happens in internal/model, once the whole set is
// known — min-max needs every value at once, and the snapshot must keep the
// raw number so the provenance stays readable.
func FetchArenaElo(ctx context.Context, c *httpcache.Client, names map[string]string) ([]ScoreRow, error) {
	body, err := c.Get(ctx, ArenaURL)
	if err != nil {
		return nil, fmt.Errorf("arena: fetch: %w", err)
	}
	flight := arenaFlight(body)
	if flight == "" {
		return nil, fmt.Errorf("arena: %s: no self.__next_f.push chunk on the page", ArenaURL)
	}
	entries, err := arenaEntries(flight)
	if err != nil {
		return nil, fmt.Errorf("arena: %s: %w", ArenaURL, err)
	}

	checked := ""
	if m := arenaVoteCutoffRe.FindStringSubmatch(flight); m != nil {
		checked, _, _ = strings.Cut(m[1], "T")
	}

	// modelKey -> entry, so matching is an exact map lookup and never a guess.
	byKey := make(map[string]arenaEntry, len(entries))
	for _, e := range entries {
		if e.ModelKey == "" {
			continue
		}
		byKey[e.ModelKey] = e
	}

	out := make([]ScoreRow, 0, len(names))
	for slug, key := range names {
		e, ok := byKey[key]
		if !ok {
			continue // tracked, but not on this leaderboard: report.go tells the human
		}
		out = append(out, ScoreRow{
			Slug:            slug,
			Metric:          MetricArenaElo,
			Value:           e.Rating,
			VariantMeasured: e.ModelDisplayName,
			SourceURL:       ArenaURL,
			Checked:         checked,
		})
	}
	// A specific request (len(names) > 0) that matched nothing is treated as a
	// failure, not as "these 0 models are legitimately absent": if the page's
	// structure ever changes so arenaEntriesKey anchors onto the wrong array
	// first, every entry would fail to match and this would otherwise return
	// an empty slice with a nil error — indistinguishable from a genuine
	// zero-overlap leaderboard, and invisible to applyArenaFallback in
	// internal/refresh/run.go, which only falls back to the snapshot on an
	// error. An empty names map (nothing was requested at all) is left alone.
	if len(names) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("arena: fetched %d entries but none matched the %d requested names", len(entries), len(names))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
