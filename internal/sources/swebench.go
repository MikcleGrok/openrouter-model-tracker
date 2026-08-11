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

// SWEBenchURL is the SWE-bench leaderboard page. It embeds its data as raw JSON
// in a <script id="leaderboard-data"> block; there is no separate API.
var SWEBenchURL = "https://www.swebench.com/"

// MetricSWEBenchVerified is the only metric this project ranks by.
const MetricSWEBenchVerified = "SWE-bench Verified"

// modelTagPrefix is how a leaderboard entry names the model it evaluated. The
// entry's own "name" is the agentic scaffold, not the model.
const modelTagPrefix = "Model: "

// ScoreRow is one benchmark number attributed to one OpenRouter slug. Every
// source produces these.
type ScoreRow struct {
	Slug               string
	SourceFamily       string
	ConfiguredIdentity string
	IdentityAmbiguous  bool
	Metric             string
	Value              float64
	Unit               string
	VariantMeasured    string
	SourceURL          string
	Checked            string
	IdentityStatus     string
	Provider           string
	License            string
	ModelURL           string
	MetadataSourceURL  string
	CanonicalID        string
	ReleaseVariant     string
	ModelVariant       string
	Reasoning          string
	Configuration      string
	SampleSize         string
	Uncertainty        string
	Harness            string
	Scaffold           string
}

type sweGroup struct {
	Name    string      `json:"name"`
	Results []sweResult `json:"results"`
}

// sweResult declares only the fields we use. The payload also carries fields
// whose type varies between entries (checked: bool|null|string, cost:
// number|null, site: string|[]string|null); declaring them would break
// unmarshalling, and encoding/json skips undeclared fields for free.
type sweResult struct {
	Name     string   `json:"name"`
	Resolved float64  `json:"resolved"`
	Date     string   `json:"date"`
	Tags     []string `json:"tags"`
}

var sweDataRe = regexp.MustCompile(`(?s)<script[^>]*id="leaderboard-data"[^>]*>(.*?)</script>`)

// FetchSWEBenchVerified returns one row per tracked slug that appears in the
// "Verified" leaderboard group. names maps a slug to the exact tag value on the
// site, including the "Model: " prefix.
func FetchSWEBenchVerified(ctx context.Context, c *httpcache.Client, names map[string]string) ([]ScoreRow, error) {
	body, err := c.Get(ctx, SWEBenchURL)
	if err != nil {
		return nil, fmt.Errorf("swebench: fetch: %w", err)
	}
	m := sweDataRe.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("swebench: no <script id=%q> block at %s", "leaderboard-data", SWEBenchURL)
	}

	var groups []sweGroup
	if err := json.Unmarshal(m[1], &groups); err != nil {
		return nil, fmt.Errorf("swebench: decode leaderboard JSON: %w", err)
	}
	var verified *sweGroup
	for i := range groups {
		if groups[i].Name == "Verified" {
			verified = &groups[i]
			break
		}
	}
	if verified == nil {
		return nil, fmt.Errorf("swebench: leaderboard group %q not found at %s", "Verified", SWEBenchURL)
	}

	// tag value -> slug, so matching is an exact map lookup and never a guess.
	wanted := make(map[string]string, len(names))
	for slug, tag := range names {
		if tag != "" {
			wanted[tag] = slug
		}
	}

	best := map[string]ScoreRow{}
	for _, r := range verified.Results {
		slug := ""
		for _, tag := range r.Tags {
			if !strings.HasPrefix(tag, modelTagPrefix) {
				continue
			}
			slug = wanted[tag] // "" when the tag names a model we do not track
			break
		}
		if slug == "" {
			continue
		}
		if cur, seen := best[slug]; seen && cur.Value >= r.Resolved {
			continue
		}
		best[slug] = ScoreRow{
			Slug:               slug,
			SourceFamily:       "swebench",
			ConfiguredIdentity: names[slug],
			Metric:             MetricSWEBenchVerified,
			Value:              r.Resolved,
			Unit:               "%",
			VariantMeasured:    r.Name,
			SourceURL:          SWEBenchURL,
			Checked:            r.Date,
		}
	}

	out := make([]ScoreRow, 0, len(best))
	for _, row := range best {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
