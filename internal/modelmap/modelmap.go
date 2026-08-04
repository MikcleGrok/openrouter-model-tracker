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
)

// Entry is one tracked model.
type Entry struct {
	Slug  string
	Tier  string
	Names map[string]string
}

var validTiers = map[string]bool{"opus": true, "sonnet": true, "haiku": true, "free": true}

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
			if key == "tier" {
				if !validTiers[value] {
					return nil, fmt.Errorf("modelmap: %s:%d: unknown tier %q (want opus|sonnet|haiku|free)", path, lineNo, value)
				}
				e.Tier = value
				continue
			}
			e.Names[key] = value
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
