package refresh

import (
	"sort"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// Report is what one run tells the human. Deciding what to do about any of it —
// adding a model to the map, writing its prose, dropping a retired slug —
// stays a human/LLM job; the tool only detects.
type Report struct {
	NewCandidates  []string
	CatalogAdded   []string
	CatalogRemoved []string
	Retired        []string
	NeedsReview    []string
	NoScore        []string
	NoArenaScore   []string
	ArenaOnly      []string
	Warnings       []string
	PriceChanges   []PriceChange
}

func vendorOf(slug string) string {
	if i := strings.IndexByte(slug, '/'); i > 0 {
		return slug[:i]
	}
	return slug
}

func catalogDelta(previous, current []string) (added, removed []string) {
	oldSet := make(map[string]bool, len(previous))
	newSet := make(map[string]bool, len(current))
	for _, slug := range previous {
		oldSet[slug] = true
	}
	for _, slug := range current {
		newSet[slug] = true
	}
	for slug := range newSet {
		if !oldSet[slug] {
			added = append(added, slug)
		}
	}
	for slug := range oldSet {
		if !newSet[slug] {
			removed = append(removed, slug)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// BuildReport compares the tracked set against the live catalogue and the
// merged rows. An empty catalogue means the catalogue lookup failed, and
// nothing is reported as new: a failed check must never look like a removal.
// Retired is gated on pricesOK instead, since it only needs the price lookup
// to have actually succeeded — the catalogue and price fetches can now fail
// independently.
func BuildReport(entries []modelmap.Entry, catalog []string, prices map[string]sources.PriceInfo, pricesOK bool, models []model.Model) Report {
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
	}
	if pricesOK {
		for _, e := range entries {
			if p, ok := prices[e.Slug]; ok && !p.Found {
				r.Retired = append(r.Retired, e.Slug)
			}
		}
	}

	// The two families are counted apart: an arena= token alone must not make
	// the SWE-bench section shout about a model that never declared a
	// SWE-bench source, and vice versa.
	hasScoreSource := make(map[string]bool, len(entries))
	hasArenaSource := make(map[string]bool, len(entries))
	for _, e := range entries {
		for sourceID := range e.Names {
			switch model.SourceFamily[sourceID] {
			case model.ScoreSourceArena:
				hasArenaSource[e.Slug] = true
			case model.ScoreSourceSWEBench:
				hasScoreSource[e.Slug] = true
			}
		}
	}

	for _, m := range models {
		if m.Note == notes.NeedsReview || m.Owner == notes.NeedsReview || m.OpenWeights == notes.NeedsReview ||
			(m.Free && m.ClaudeRef == notes.NeedsReview) {
			r.NeedsReview = append(r.NeedsReview, m.Slug)
		}
		if m.Score == nil && hasScoreSource[m.Slug] {
			r.NoScore = append(r.NoScore, m.Slug)
		}
		if m.ArenaScore == nil && hasArenaSource[m.Slug] {
			r.NoArenaScore = append(r.NoArenaScore, m.Slug)
		}
		// A row whose only quality signal is a crowd-sourced Elo is exactly
		// where a real coding benchmark is still worth looking for.
		if m.ArenaScore != nil && m.Score == nil {
			r.ArenaOnly = append(r.ArenaOnly, m.Slug)
		}
	}

	sort.Strings(r.NewCandidates)
	sort.Strings(r.Retired)
	sort.Strings(r.NeedsReview)
	sort.Strings(r.NoScore)
	sort.Strings(r.NoArenaScore)
	sort.Strings(r.ArenaOnly)
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
	section(&b, "🆕", "модели, появившиеся после последнего refresh", r.CatalogAdded)
	section(&b, "🗑️", "модели, исчезнувшие после последнего refresh", r.CatalogRemoved)
	section(&b, "➖", "slug'ов больше нет в каталоге OpenRouter", r.Retired)
	section(&b, "📝", "нет прозы в notes.yaml", r.NeedsReview)
	section(&b, "❓", "нет оценки — проверь имя модели на источнике в model-map.tsv", r.NoScore)
	section(&b, "🎯", "нет строки на Arena — проверь arena= в model-map.tsv", r.NoArenaScore)
	section(&b, "🔍", "только Arena-оценка, настоящего coding-бенчмарка нет", r.ArenaOnly)
	if len(r.PriceChanges) > 0 {
		b.WriteString("💰 изменения цен с последнего live-наблюдения:\n")
		for _, change := range r.PriceChanges {
			b.WriteString("    " + change.Slug + ": " + pricehistory.Format(change.Previous) + " → " + pricehistory.Format(change.Current) + "\n")
		}
	}
	if b.Len() == 0 {
		return "✅ Разбирать нечего: карта, проза и источники сошлись.\n"
	}
	return b.String()
}
