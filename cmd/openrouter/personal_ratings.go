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
	// cancel stops the current generation's still-running fetch — called on
	// toggle-off (and defensively before starting a new one) so mashing M
	// on a large catalog can never overlap two live batches: each of
	// fetchPersonalRatings' workers is blocked on a c.GetOwnFeedback call
	// carrying this context, and a cancelled context makes that call (and
	// every subsequent one the worker would otherwise start) fail
	// immediately instead of actually reaching the network.
	cancel context.CancelFunc

	loading bool
	loaded  bool
	err     string

	rows []personalRatingRow
	// baseRank is the base-ranking snapshot the current generation's batch
	// was ordered against (personalRatingsBaseRanking, computed once at
	// toggle-on) — kept around so a rating changed later from the Feedback
	// tab (syncPersonalRatingsAfterSave) can update/insert a row and
	// re-sort without needing a whole new fetch.
	baseRank map[string]int
	// errCount/total describe the most recently completed batch: how many of
	// the total models in the catalog at fetch time failed their
	// GetOwnFeedback call. A model that failed is simply absent from rows —
	// it is never assumed unrated, since that is not what the failure means.
	errCount, total int

	// savedCursor/savedSelectedSlug capture the normal table's cursor
	// position at the moment the mode was toggled ON, so toggling OFF can
	// restore it exactly. This is not the same thing as m.selectedSlug
	// surviving on its own: entering the mode immediately rebuilds with
	// rows still nil, and restoreSelection's own empty-list branch clears
	// m.selectedSlug in that situation — so without this separate copy,
	// toggling off would strand the user wherever the freshly loaded batch
	// happens to put the cursor (its own top row) instead of back where
	// they were browsing.
	savedCursor       int
	savedSelectedSlug string
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
		// Stop the current batch outright — a still-running fetch left
		// alive across a toggle-off is exactly what let mashing M overlap
		// several full batches against the feedback server.
		if m.personalRatings.cancel != nil {
			m.personalRatings.cancel()
			m.personalRatings.cancel = nil
		}
		m.personalRatings.active = false
		// Restore exactly the cursor/selection the normal table had before
		// this mode was entered — see tuiPersonalRatingsState's own doc
		// comment for why this can't just rely on m.selectedSlug having
		// survived on its own.
		m.cursor = m.personalRatings.savedCursor
		m.selectedSlug = m.personalRatings.savedSelectedSlug
		m.rebuild()
		return m, nil
	}
	baseRank, err := m.personalRatingsBaseRanking()
	if err != nil {
		// Matches buildVisible's own convention: a ranking-config error is
		// surfaced, never swallowed into a silently wrong base_position
		// column. The mode is not entered at all.
		m.err = err.Error()
		return m, nil
	}
	if m.personalRatings.cancel != nil {
		// Defensive only — toggling off above already cancels the previous
		// batch, so this path should be unreachable, but two live batches
		// must never coexist regardless of how this state was reached.
		m.personalRatings.cancel()
	}
	parentCtx := m.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	fetchCtx, cancel := context.WithCancel(parentCtx)
	m.personalRatings.generation++
	generation := m.personalRatings.generation
	models := append([]model.Model(nil), m.models...)
	m.personalRatings.savedCursor = m.cursor
	m.personalRatings.savedSelectedSlug = m.selectedSlug
	m.personalRatings.active = true
	m.personalRatings.loading = true
	m.personalRatings.loaded = false
	m.personalRatings.err = ""
	m.personalRatings.rows = nil
	m.personalRatings.baseRank = baseRank
	m.personalRatings.cancel = cancel
	m.personalRatings.errCount, m.personalRatings.total = 0, 0
	m.rebuild()
	return m, m.personalRatingsCmd(fetchCtx, models, baseRank, generation)
}

// personalRatingsBaseRanking computes modelSlug -> 1-based rank within the
// app's own ranking (the same "utility" ordering the main table defaults
// to, under the currently active ranking mode) — the "existing ranking
// tie-breaker" plan.md's readiness criterion calls for. This mirrors
// buildVisible's own compiled-ranking resolution, error surfaced the same
// way, so the two never disagree about what "the app's ranking" means.
func (m tuiModel) personalRatingsBaseRanking() (map[string]int, error) {
	ranked := append([]model.Model(nil), m.models...)
	compiled := m.rankingConfig
	if !m.rankingConfigSet {
		c := ranking.DefaultConfig()
		c.PriceWeight = &m.priceWeight
		compiled, _ = ranking.Compile(c)
	}
	if err := sortTableModelsWithRankingAndConfig(ranked, "utility", false, m.ranking, compiled, m.mixInputWeight, m.mixOutputWeight); err != nil {
		return nil, err
	}
	positions := make(map[string]int, len(ranked))
	for i, row := range ranked {
		positions[row.Slug] = i + 1
	}
	return positions, nil
}

// personalRatingsCmd fetches GetOwnFeedback for every model in models,
// bounded by personalRatingsConcurrency concurrent requests, and returns one
// aggregated tuiPersonalRatingsMsg — matching feedback.go's
// feedbackSummaryCmd shape (a tea.Cmd closing over the client/ctx, returning
// exactly one tea.Msg), just batched over many models instead of one. ctx is
// the per-toggle cancellable context togglePersonalRatings derives — every
// GetOwnFeedback call started under it aborts as soon as it is cancelled.
func (m tuiModel) personalRatingsCmd(ctx context.Context, models []model.Model, baseRank map[string]int, generation uint64) tea.Cmd {
	c := m.feedbackClient
	return func() tea.Msg {
		rows, errCount, firstErr := fetchPersonalRatings(ctx, c, models, baseRank)
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

// personalRatingsEffectiveRows re-resolves the last loaded batch's rows
// against the CURRENT catalog (m.models) by slug: a model still present
// gets its live catalog data (price, quality, name, ...) instead of the
// frozen snapshot taken at fetch time, and a model removed from the catalog
// entirely — e.g. by the app's own periodic background refresh — is
// dropped from the view outright rather than kept showing stale.
//
// The rating value itself (and the order it produced) is left exactly as
// the last fetch computed it: a periodic refresh never re-fetches the whole
// batch on its own. This matches the Feedback tab's own existing behavior
// across a background refresh — a model's loaded rating is never silently
// re-fetched there either (see feedback.go); only the underlying model row
// is live, via the shared m.visible/detailRow plumbing, which is exactly
// what this method reproduces for this view. Re-fetching every rated
// model's GetOwnFeedback on every refresh tick was rejected as needless
// extra load against the feedback server for values that do not change on
// their own — see syncPersonalRatingsAfterSave for the one case a rating
// actually does change while this view is open.
func (m tuiModel) personalRatingsEffectiveRows() []personalRatingRow {
	if len(m.personalRatings.rows) == 0 {
		return nil
	}
	live := make(map[string]model.Model, len(m.models))
	for _, row := range m.models {
		live[row.Slug] = row
	}
	result := make([]personalRatingRow, 0, len(m.personalRatings.rows))
	for _, row := range m.personalRatings.rows {
		current, ok := live[row.model.Slug]
		if !ok {
			continue
		}
		row.model = current
		result = append(result, row)
	}
	return result
}

// personalRatingsFindModel looks up slug in models by exact match — used by
// syncPersonalRatingsAfterSave to resolve a newly-rated model's own catalog
// row when adding it to the list for the first time.
func personalRatingsFindModel(models []model.Model, slug string) (model.Model, bool) {
	for _, row := range models {
		if row.Slug == slug {
			return row, true
		}
	}
	return model.Model{}, false
}

// syncPersonalRatingsAfterSave keeps an already-loaded "My ratings" batch
// consistent with a rating just changed from inside the Feedback tab
// (open My-ratings, Enter into a model's detail, change the rating, Esc
// back — the personal-ratings mode stays active the whole time, since the
// detail overlay sits on top of it): an already-listed model's changed
// rating is updated in place, a model rated for the first time is added,
// and the whole set is re-sorted — all without the user having to manually
// toggle M off and back on.
//
// A no-op before the mode has ever been toggled on this session
// (baseRank is nil then, so there is no batch to keep in sync and no base
// ranking to place a newly-added row against) or when summary carries no
// rating at all (should not happen for a successful save — the edit form
// requires Overall 1-5 before it will even dispatch one — guarded
// defensively regardless).
func (m tuiModel) syncPersonalRatingsAfterSave(slug string, summary feedbackclient.Summary) tuiModel {
	if summary.Mine == nil || m.personalRatings.baseRank == nil {
		return m
	}
	rows := m.personalRatings.rows
	updated := false
	for i := range rows {
		if rows[i].model.Slug == slug {
			rows[i].overall = summary.Mine.Overall
			updated = true
			break
		}
	}
	if !updated {
		row, ok := personalRatingsFindModel(m.models, slug)
		if !ok {
			return m
		}
		position, hasPosition := m.personalRatings.baseRank[slug]
		rows = append(rows, personalRatingRow{model: row, overall: summary.Mine.Overall, basePosition: position, hasBasePosition: hasPosition})
	}
	sortPersonalRatingRows(rows)
	m.personalRatings.rows = rows
	if m.personalRatings.active {
		m.rebuild()
	}
	return m
}
