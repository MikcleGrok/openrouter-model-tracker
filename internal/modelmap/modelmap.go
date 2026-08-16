// Package modelmap parses model-map.tsv, the hand-maintained table that says
// which OpenRouter slugs the document tracks and, for each benchmark source,
// the exact name that source uses for that model. Nothing here guesses or
// normalises names: a wrong match would silently attribute a score to the wrong
// product, which is the single failure mode this whole design exists to avoid.
package modelmap

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/tier"
)

// Entry is one tracked model.
type Entry struct {
	Slug  string
	Tier  string
	Names map[string]string

	// Variants marks a source id (a key that also appears in Names) whose
	// mapped identity has been explicitly confirmed, by a human editing this
	// file, to measure a genuine variant or checkpoint different from the
	// OpenRouter product — not the same model under a different spelling.
	// It is set by the "<source>!variant=<name>" token form (e.g.
	// "vals!variant=some/other-checkpoint"): the mapping still names the
	// exact key to fetch, but classifyIdentity must not trust it as
	// exact_product the way an unflagged mapping is trusted by default.
	Variants map[string]bool
}

// Load reads and validates the tab-separated map at path.
func Load(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("modelmap: %w", err)
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		if trimmed := strings.TrimSpace(raw); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		fields := strings.Split(raw, "\t")
		e := Entry{Slug: strings.TrimSpace(fields[0]), Names: map[string]string{}}
		if e.Slug == "" {
			return nil, fmt.Errorf("modelmap: %s:%d: empty slug in column 1", path, lineNo)
		}
		for _, tok := range fields[1:] {
			tok = strings.TrimSpace(tok)
			if tok == "" || strings.HasPrefix(tok, "#") {
				continue
			}
			key, value, ok := strings.Cut(tok, "=")
			if !ok {
				return nil, fmt.Errorf("modelmap: %s:%d: token %q is not key=value", path, lineNo, tok)
			}
			key = strings.TrimSpace(key)
			variant := false
			if stripped, hasMarker := strings.CutSuffix(key, "!variant"); hasMarker {
				key, variant = stripped, true
			}
			// The marker qualifies a source id; on its own it names nothing to
			// fetch and nothing to flag, and storing it under the empty key
			// would silently add a source no fetcher ever asks for — the same
			// class of quiet mis-mapping a "tier!variant=" token is already
			// rejected for.
			if key == "" {
				if variant {
					return nil, fmt.Errorf("modelmap: %s:%d: token %q carries a !variant marker with no source id (want <source>!variant=<name>)", path, lineNo, tok)
				}
				return nil, fmt.Errorf("modelmap: %s:%d: token %q has an empty key", path, lineNo, tok)
			}
			if key == "tier" {
				if variant {
					return nil, fmt.Errorf("modelmap: %s:%d: tier= cannot carry a !variant marker", path, lineNo)
				}
				if !tier.IsValid(value) {
					return nil, fmt.Errorf("modelmap: %s:%d: unknown tier %q (want %s)", path, lineNo, value, tier.ValuesString())
				}
				e.Tier = value
				continue
			}
			e.Names[key] = value
			if variant {
				if e.Variants == nil {
					e.Variants = map[string]bool{}
				}
				e.Variants[key] = true
			}
		}
		if e.Tier == "" {
			return nil, fmt.Errorf("modelmap: %s:%d: %s has no tier= token", path, lineNo, e.Slug)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("modelmap: %s: %w", path, err)
	}
	return entries, nil
}

// Slugs returns the tracked slugs in file order.
func Slugs(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Slug)
	}
	return out
}

// NamesFor returns the slug -> exact-name map for one source id, skipping every
// entry that does not declare a name for it.
func NamesFor(entries []Entry, source string) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		if name, ok := e.Names[source]; ok && name != "" {
			out[e.Slug] = name
		}
	}
	return out
}
