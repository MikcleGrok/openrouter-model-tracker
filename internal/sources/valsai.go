package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
)

// ValsSWEBenchURL is the vals.ai SWE-bench leaderboard. The page is built with
// Astro and has no JSON API: the leaderboard data lives in the props attribute
// of the <astro-island> whose component-url names BenchmarkView.
var ValsSWEBenchURL = "https://www.vals.ai/benchmarks/swebench"

var (
	astroIslandRe = regexp.MustCompile(`(?s)<astro-island\b[^>]*>`)
	astroPropsRe  = regexp.MustCompile(`props="([^"]*)"`)
)

// unwrapAstro removes Astro's serialisation wrapper, in which every value is a
// two-element array [typeCode, value]. Only two codes occur in this payload:
// 0 passes a scalar, object or null through, 1 marks an array whose elements
// are themselves wrapped. It also recurses into a bare, not-yet-wrapped map —
// the shape of the top-level JSON object this is always first called on.
func unwrapAstro(v any) any {
	if arr, ok := v.([]any); ok && len(arr) == 2 {
		if code, ok := arr[0].(float64); ok {
			switch code {
			case 0:
				return unwrapAstro(arr[1])
			case 1:
				if lst, ok := arr[1].([]any); ok {
					out := make([]any, len(lst))
					for i, e := range lst {
						out[i] = unwrapAstro(e)
					}
					return out
				}
			}
		}
	}
	if m, ok := v.(map[string]any); ok {
		out := make(map[string]any, len(m))
		for k, vv := range m {
			out[k] = unwrapAstro(vv)
		}
		return out
	}
	return v
}

// benchmarkProps extracts and un-wraps the BenchmarkView props of an Astro page.
func benchmarkProps(page []byte) ([]byte, error) {
	escaped := ""
	for _, tag := range astroIslandRe.FindAllString(string(page), -1) {
		if !strings.Contains(tag, "BenchmarkView") {
			continue
		}
		if m := astroPropsRe.FindStringSubmatch(tag); m != nil {
			escaped = m[1]
			break
		}
	}
	if escaped == "" {
		return nil, fmt.Errorf("no <astro-island> with a BenchmarkView component-url and a props attribute")
	}
	var raw any
	if err := json.Unmarshal([]byte(html.UnescapeString(escaped)), &raw); err != nil {
		return nil, fmt.Errorf("decode props: %w", err)
	}
	plain, err := json.Marshal(unwrapAstro(raw))
	if err != nil {
		return nil, fmt.Errorf("re-encode unwrapped props: %w", err)
	}
	return plain, nil
}

type valsPage struct {
	BenchmarkView struct {
		Default struct {
			Metadata struct {
				Updated string `json:"updated"`
			} `json:"metadata"`
			Tasks map[string]map[string]struct {
				Accuracy float64 `json:"accuracy"`
				Provider string  `json:"provider"`
			} `json:"tasks"`
		} `json:"default"`
	} `json:"benchmarkView"`
}

// FetchValsSWEBench returns one row per tracked slug present in
// benchmarkView.default.tasks.overall. names maps a slug to the exact model key
// vals.ai uses, e.g. "anthropic/claude-opus-5".
func FetchValsSWEBench(ctx context.Context, c *httpcache.Client, names map[string]string) ([]ScoreRow, error) {
	body, err := c.Get(ctx, ValsSWEBenchURL)
	if err != nil {
		return nil, fmt.Errorf("valsai: fetch: %w", err)
	}
	plain, err := benchmarkProps(body)
	if err != nil {
		return nil, fmt.Errorf("valsai: %s: %w", ValsSWEBenchURL, err)
	}
	var page valsPage
	if err := json.Unmarshal(plain, &page); err != nil {
		return nil, fmt.Errorf("valsai: decode benchmarkView: %w", err)
	}
	overall, ok := page.BenchmarkView.Default.Tasks["overall"]
	if !ok {
		return nil, fmt.Errorf("valsai: %s: tasks.overall is missing", ValsSWEBenchURL)
	}

	out := make([]ScoreRow, 0, len(names))
	for slug, key := range names {
		entry, ok := overall[key]
		if !ok {
			continue
		}
		out = append(out, ScoreRow{
			Slug:               slug,
			SourceFamily:       "vals",
			ConfiguredIdentity: key,
			Metric:             MetricSWEBenchVerified,
			Value:              entry.Accuracy,
			Unit:               "%",
			VariantMeasured:    key,
			SourceURL:          ValsSWEBenchURL,
			Checked:            page.BenchmarkView.Default.Metadata.Updated,
			Provider:           entry.Provider,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
