// "My ratings" / personal-ranking view mode (plan.md readiness criterion,
// line 650/671: a dedicated, navigable action showing only the models the
// current identity has rated, ordered by that rating). This file owns the
// mode's state, the batched GetOwnFeedback fetch (one call per model in the
// currently-loaded catalog, bounded by personalRatingsConcurrency), and the
// client-side ordering — plan.md's own HTTP API never defines a bulk "list
// my ratings" endpoint, so there is nothing server-side to ask for this; see
// internal/feedback/ranking.go's personalPosition, which this mirrors
// (same tie-break: rating desc, then the app's own base ranking, matching
// its own lessByBaseRank exactly). It never touches
// internal/feedback/httpapi or internal/feedback/sqlite, and it never
// fetches or exposes community/others data — only the current identity's
// own "mine", via the existing per-model GetOwnFeedback.
package main

import (
	"context"
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/ranking"
)

// personalRatingsConcurrency bounds how many GetOwnFeedback requests run at
// once when building this view — the catalog can run to a few hundred
// models, and firing them all at once would open that many sockets
// simultaneously for no benefit.
const personalRatingsConcurrency = 8

// personalRatingRow is one rated model in the "My ratings" ordering: the
// model itself, this identity's own overall rating (always 1-5 — a row is
// only ever created for a model GetOwnFeedback actually returned an
// OwnFeedback for), and its position in the app's own base ranking, used
// only as the tie-breaker (basePosition is meaningless when
// hasBasePosition is false — a model absent from the base ranking).
type personalRatingRow struct {
	model           model.Model
	overall         int
	basePosition    int
	hasBasePosition bool
}

// tuiPersonalRatingsState is the "My ratings" view mode's own session state
// — mirrors tuiFeedbackState's shape (see feedback.go) for the same reason:
// a monotonically increasing generation counter guards every batch fetch's
// result against a superseded one, the same role reqSeq plays there.
// generation only ever advances when a fetch is (re)dispatched (toggling
// the mode ON); toggling OFF does not need its own bump — the next toggle ON
// always draws a fresh generation, so a response tagged with an old one is
// discarded whether or not the mode is active when it finally arrives (see
// applyPersonalRatingsMsg).
type tuiPersonalRatingsState struct {
	active     bool
	generation uint64

	loading bool
	loaded  bool
	err     string

	rows []personalRatingRow
	// errCount/total describe the most recently completed batch: how many of
	// the total models in the catalog at fetch time failed their
	// GetOwnFeedback call. A model that failed is simply absent from rows —
	// it is never assumed unrated, since that is not what the failure means.
	errCount, total int
}

// tuiPersonalRatingsMsg is the result of one full personalRatingsCmd batch:
// every model in the catalog snapshot at dispatch time was attempted, rows
// holds only the ones that both succeeded and turned out rated, already
// ordered.
type tuiPersonalRatingsMsg struct {
	generation      uint64
	rows            []personalRatingRow
	errCount, total int
	// err is the first error encountered, if any — only rendered when every
	// single request failed (errCount == total), so a handful of flaky
	// requests among an otherwise-successful batch doesn't replace the real
	// (partial) result with an error banner.
	err error
}

// togglePersonalRatings flips the "My ratings" view mode on or off. Turning
// it on when feedback is disabled (m.feedbackClient == nil) is a no-op that
// surfaces the same disabled explanation the Feedback tab itself shows
// (feedback_view.go), rather than silently doing nothing.
func (m tuiModel) togglePersonalRatings() (tuiModel, tea.Cmd) {
	if m.feedbackClient == nil {
		l := feedbackLabelsForLang(m.lang)
		m.status, m.err = l.disabledLine+" "+l.disabledHint, ""
		return m, nil
	}
	if m.personalRatings.active {
		// m.status/m.err are left untouched — this view never reads or
		// writes them, so the normal table reappears with whatever status it
		// last had before the user toggled into "My ratings", exactly as if
		// this mode had never been entered.
		m.personalRatings.active = false
		m.rebuild()
		return m, nil
	}
	m.personalRatings.generation++
	generation := m.personalRatings.generation
	models := append([]model.Model(nil), m.models...)
	baseRank := m.personalRatingsBaseRanking()
	m.personalRatings.active = true
	m.personalRatings.loading = true
	m.personalRatings.loaded = false
	m.personalRatings.err = ""
	m.personalRatings.rows = nil
	m.personalRatings.errCount, m.personalRatings.total = 0, 0
	m.rebuild()
	return m, m.personalRatingsCmd(models, baseRank, generation)
}

// personalRatingsBaseRanking computes modelSlug -> 1-based rank within the
// app's own ranking (the same "utility" ordering the main table defaults
// to, under the currently active ranking mode) — the "existing ranking
// tie-breaker" plan.md's readiness criterion calls for. This mirrors
// buildVisible's own compiled-ranking resolution so the two never disagree
// about what "the app's ranking" means.
func (m tuiModel) personalRatingsBaseRanking() map[string]int {
	ranked := append([]model.Model(nil), m.models...)
	compiled := m.rankingConfig
	if !m.rankingConfigSet {
		c := ranking.DefaultConfig()
		c.PriceWeight = &m.priceWeight
		compiled, _ = ranking.Compile(c)
	}
	_ = sortTableModelsWithRankingAndConfig(ranked, "utility", false, m.ranking, compiled, m.mixInputWeight, m.mixOutputWeight)
	positions := make(map[string]int, len(ranked))
	for i, row := range ranked {
		positions[row.Slug] = i + 1
	}
	return positions
}

// personalRatingsCmd fetches GetOwnFeedback for every model in models,
// bounded by personalRatingsConcurrency concurrent requests, and returns one
// aggregated tuiPersonalRatingsMsg — matching feedback.go's
// feedbackSummaryCmd shape (a tea.Cmd closing over the client/ctx, returning
// exactly one tea.Msg), just batched over many models instead of one.
func (m tuiModel) personalRatingsCmd(models []model.Model, baseRank map[string]int, generation uint64) tea.Cmd {
	c, ctx := m.feedbackClient, m.ctx
	return func() tea.Msg {
		reqCtx := ctx
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		rows, errCount, firstErr := fetchPersonalRatings(reqCtx, c, models, baseRank)
		sortPersonalRatingRows(rows)
		return tuiPersonalRatingsMsg{generation: generation, rows: rows, errCount: errCount, total: len(models), err: firstErr}
	}
}

// fetchPersonalRatings runs one GetOwnFeedback call per model, at most
// personalRatingsConcurrency at a time, and collects every rated result.
// A model whose call fails is simply excluded — never assumed unrated —
// and counted in errCount.
func fetchPersonalRatings(ctx context.Context, c *feedbackclient.Client, models []model.Model, baseRank map[string]int) ([]personalRatingRow, int, error) {
	type outcome struct {
		row   personalRatingRow
		rated bool
		err   error
	}
	jobs := make(chan model.Model)
	results := make(chan outcome)
	workers := personalRatingsConcurrency
	if workers > len(models) {
		workers = len(models)
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for row := range jobs {
				own, err := c.GetOwnFeedback(ctx, row.Slug)
				if err != nil {
					results <- outcome{err: err}
					continue
				}
				if own.OwnFeedback == nil {
					results <- outcome{}
					continue
				}
				position, hasPosition := baseRank[row.Slug]
				results <- outcome{rated: true, row: personalRatingRow{model: row, overall: own.OwnFeedback.Overall, basePosition: position, hasBasePosition: hasPosition}}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, row := range models {
			jobs <- row
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	var rows []personalRatingRow
	errCount := 0
	var firstErr error
	for res := range results {
		if res.err != nil {
			errCount++
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		if res.rated {
			rows = append(rows, res.row)
		}
	}
	return rows, errCount, firstErr
}

// sortPersonalRatingRows orders rows by Overall descending, ties broken by
// lessByPersonalBaseRank — the client-side mirror of
// internal/feedback/ranking.go's personalPosition (Overall desc, then
// lessByBaseRank).
func sortPersonalRatingRows(rows []personalRatingRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.overall != b.overall {
			return a.overall > b.overall
		}
		return lessByPersonalBaseRank(a, b)
	})
}

// lessByPersonalBaseRank mirrors internal/feedback/ranking.go's
// lessByBaseRank: a model present in the base ranking always beats one
// absent from it; between two present models the lower (better) base rank
// wins; between two absent models, slug order breaks the tie so the result
// is deterministic.
func lessByPersonalBaseRank(a, b personalRatingRow) bool {
	switch {
	case a.hasBasePosition && b.hasBasePosition:
		return a.basePosition < b.basePosition
	case a.hasBasePosition:
		return true
	case b.hasBasePosition:
		return false
	default:
		return a.model.Slug < b.model.Slug
	}
}

// applyPersonalRatingsMsg applies one batch result, discarding it outright
// if it no longer matches the current generation (msg.generation !=
// m.personalRatings.generation) — a superseded fetch from a toggle-off-
// then-on cycle. It is safe to apply even while the mode is currently
// inactive (the generation still matches, e.g. toggled off before this,
// still-current batch's response arrived): the state is updated but
// buildVisible ignores m.personalRatings entirely while inactive, so
// nothing repaints.
func (m tuiModel) applyPersonalRatingsMsg(msg tuiPersonalRatingsMsg) tuiModel {
	if msg.generation != m.personalRatings.generation {
		return m
	}
	m.personalRatings.loading = false
	m.personalRatings.loaded = true
	m.personalRatings.rows = msg.rows
	m.personalRatings.errCount, m.personalRatings.total = msg.errCount, msg.total
	if msg.total > 0 && msg.errCount == msg.total {
		m.personalRatings.err = feedbackErrorMessage(msg.err, m.lang)
	} else {
		m.personalRatings.err = ""
	}
	if m.personalRatings.active {
		m.rebuild()
	}
	return m
}

// personalRatingsVisible projects rows onto the []model.Model shape
// buildVisible/m.visible already expect — the "My ratings" view reuses the
// exact same cursor/navigation/detail-drilldown machinery the normal table
// does, over this smaller, differently-ordered list.
func personalRatingsVisible(rows []personalRatingRow) []model.Model {
	if len(rows) == 0 {
		return nil
	}
	result := make([]model.Model, len(rows))
	for i, row := range rows {
		result[i] = row.model
	}
	return result
}
