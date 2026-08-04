package refresh

import (
	"sort"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// Report is what one run tells the human. Deciding what to do about any of it —
// adding a model to the map, writing its prose, dropping a retired slug —
// stays a human/LLM job; the tool only detects.
type Report struct {
	NewCandidates []string
	Retired       []string
	NeedsReview   []string
	NoScore       []string
	Warnings      []string
}

func vendorOf(slug string) string {
	if i := strings.IndexByte(slug, '/'); i > 0 {
		return slug[:i]
	}
	return slug
}

// BuildReport compares the tracked set against the live catalogue and the
// merged rows. An empty catalogue means the lookup failed, and nothing is
// reported as new or retired: a failed check must never look like a removal.
func BuildReport(entries []modelmap.Entry, catalog []string, prices map[string]sources.PriceInfo, models []model.Model) Report {
	var r Report

	tracked := make(map[string]bool, len(entries))
	vendors := make(map[string]bool, len(entries))
	for _, e := range entries {
		tracked[e.Slug] = true
		vendors[vendorOf(e.Slug)] = true
	}

	if len(catalog) > 0 {
		for _, slug := range catalog {
			if !tracked[slug] && vendors[vendorOf(slug)] {
				r.NewCandidates = append(r.NewCandidates, slug)
			}
		}
		for _, e := range entries {
			if p, ok := prices[e.Slug]; ok && !p.Found {
				r.Retired = append(r.Retired, e.Slug)
			}
		}
	}

	for _, m := range models {
		if m.Note == notes.NeedsReview || m.Owner == notes.NeedsReview || m.OpenWeights == notes.NeedsReview ||
			(m.Free && m.ClaudeRef == notes.NeedsReview) {
			r.NeedsReview = append(r.NeedsReview, m.Slug)
		}
		if m.Score == nil {
			r.NoScore = append(r.NoScore, m.Slug)
		}
	}

	sort.Strings(r.NewCandidates)
	sort.Strings(r.Retired)
	sort.Strings(r.NeedsReview)
	sort.Strings(r.NoScore)
	return r
}

func section(b *strings.Builder, icon, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(icon + " " + title + ":\n")
	for _, s := range items {
		b.WriteString("    " + s + "\n")
	}
}

// String renders the report as a scannable list.
func (r Report) String() string {
	var b strings.Builder
	section(&b, "⚠️", "источники, упавшие в этом прогоне", r.Warnings)
	section(&b, "➕", "кандидаты на добавление в model-map.tsv", r.NewCandidates)
	section(&b, "➖", "slug'ов больше нет в каталоге OpenRouter", r.Retired)
	section(&b, "📝", "нет прозы в notes.yaml", r.NeedsReview)
	section(&b, "❓", "нет оценки — проверь имя модели на источнике в model-map.tsv", r.NoScore)
	if b.Len() == 0 {
		return "✅ Разбирать нечего: карта, проза и источники сошлись.\n"
	}
	return b.String()
}
