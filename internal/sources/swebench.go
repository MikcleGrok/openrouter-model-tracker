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

	// Every submission tagged for a slug is kept, not just the highest: the
	// reported number is the median across all of them (see median below),
	// which blunts the incentive to game this self-submitted leaderboard
	// with one aggressive scaffold. The top-scoring submission is still
	// tracked separately, for the Scaffold provenance string.
	//
	// One scaffold gets one vote, though: the leaderboard can list the same
	// scaffold twice — a resubmission, or a re-verification of an earlier
	// run — and counting both would weight that scaffold double in the
	// median, reopening the very gap the median closes. Duplicates collapse
	// onto the submission that supersedes the other (see supersedes).
	grouped := map[string][]sweResult{}
	byScaffold := map[string]map[string]int{}
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
		key := scaffoldKey(r.Name)
		seen := byScaffold[slug]
		if seen == nil {
			seen = map[string]int{}
			byScaffold[slug] = seen
		}
		// An unnamed submission has no scaffold identity to compare, so it is
		// never treated as a duplicate of another unnamed one.
		if i, dup := seen[key]; dup && key != "" {
			if supersedes(r, grouped[slug][i]) {
				grouped[slug][i] = r
			}
			continue
		}
		seen[key] = len(grouped[slug])
		grouped[slug] = append(grouped[slug], r)
	}

	out := make([]ScoreRow, 0, len(grouped))
	for slug, results := range grouped {
		values := make([]float64, len(results))
		top := results[0]
		for i, r := range results {
			values[i] = r.Resolved
			if r.Resolved > top.Resolved {
				top = r
			}
		}
		scaffold := fmt.Sprintf("single scaffold (%s)", top.Name)
		variant, checked := top.Name, top.Date
		if len(results) > 1 {
			scaffold = fmt.Sprintf("median of %d scaffolds (highest individual: %s)", len(results), top.Name)
			// Value is the median over every scaffold, so it is nobody's
			// individual result. Leaving the top submission's name in
			// VariantMeasured and its date in Checked would put a specific
			// named run on a specific day right next to a number that run did
			// not produce — every renderer prints those three together. So
			// for n > 1 the row names the mapped model instead of one
			// submission, says how the number was formed, and dates itself by
			// the span the aggregated submissions cover. The top scaffold
			// stays visible in Scaffold, where it is explicitly worded as an
			// individual result.
			variant = fmt.Sprintf("%s (median of %d scaffolds)", strings.TrimPrefix(names[slug], modelTagPrefix), len(results))
			checked = checkedSpan(results)
		}
		out = append(out, ScoreRow{
			Slug:               slug,
			SourceFamily:       "swebench",
			ConfiguredIdentity: names[slug],
			Metric:             MetricSWEBenchVerified,
			Value:              median(values),
			Unit:               "%",
			VariantMeasured:    variant,
			SourceURL:          SWEBenchURL,
			Checked:            checked,
			Scaffold:           scaffold,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// scaffoldKey normalises a submission's scaffold name into the identity two
// listings of the same scaffold share. Case and inner whitespace are the only
// things forgiven: anything looser would merge two genuinely different
// scaffolds into one vote, which is the more damaging mistake of the two.
func scaffoldKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// supersedes reports whether a should replace b when both are the same
// scaffold. The later submission wins, because that is what a resubmission or
// a re-verification is; between two submissions of the same date the higher
// score wins, so the choice never depends on the order the page happened to
// list them in.
func supersedes(a, b sweResult) bool {
	if a.Date != b.Date {
		return a.Date > b.Date
	}
	return a.Resolved > b.Resolved
}

// checkedSpan describes when the aggregated submissions were posted: one date
// when they share it, "earliest..latest" otherwise. Undated submissions are
// skipped rather than widening the span to the empty string.
func checkedSpan(results []sweResult) string {
	first, last := "", ""
	for _, r := range results {
		if r.Date == "" {
			continue
		}
		if first == "" || r.Date < first {
			first = r.Date
		}
		if last == "" || r.Date > last {
			last = r.Date
		}
	}
	if first == last {
		return first
	}
	return first + ".." + last
}

// median returns the standard median of values: the middle element for an
// odd count, the average of the two middle elements for an even count. It
// sorts values in place; callers pass a slice they own.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}
