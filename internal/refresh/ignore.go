package refresh

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

// loadIgnorePatterns reads ignore-candidates.txt: one path.Match glob per
// line, "#"-prefixed comments and blank lines ignored, same convention as
// model-map.tsv's comment style. A missing file yields no patterns and no
// error — the file is optional.
func loadIgnorePatterns(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ignore-candidates: %w", err)
	}
	var patterns []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, nil
}

// filterIgnored drops every candidate that matches at least one of the
// glob patterns (Go's path.Match syntax).
func filterIgnored(candidates, patterns []string) []string {
	var out []string
	for _, c := range candidates {
		ignored := false
		for _, p := range patterns {
			if ok, _ := path.Match(p, c); ok {
				ignored = true
				break
			}
		}
		if !ignored {
			out = append(out, c)
		}
	}
	return out
}
