// Feedback TUI integration (plan step 6, .superpowers/sdd/plan/task-6-brief.md).
// This file owns everything specific to the Feedback detail tab: the async
// tea.Cmd/tea.Msg plumbing against internal/feedback/client, the edit-form
// state machine and its own keymap context. It never touches HTTP directly
// from Update/View — only from inside a tea.Cmd closure — and it never
// changes the general Detail/Viewport mechanism: feedbackTabLines (in
// feedback_view.go) hands back plain logical lines the same way
// detail_lines.go does, and tui.go's detailLinesForTab splices them in for
// one tab exactly like every other tab's lines.
package main

import (
	"context"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sboborikin/openrouter-model-tracker/internal/config"
	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

// feedbackSkillKeys mirrors internal/feedback.AllowedSkills() — duplicated
// here rather than imported, matching internal/feedback/client's own doc.go
// "Deliberately decoupled" convention: cmd/openrouter is the TUI binary and
// must not gain a dependency on the feedback server's domain package for a
// four-item constant. Keep in sync with internal/feedback/skills.go by hand;
// a skill key the server does not recognize is simply rejected by its own
// validation on save, the same as any other client of this API.
var feedbackSkillKeys = []string{"reasoning", "coding", "instruction_following", "long_context"}

// feedbackSkillLabel returns the display label for one of feedbackSkillKeys,
// localized. An unrecognized key (should not happen given the fixed list
// above) falls back to the raw key so nothing ever renders blank.
func feedbackSkillLabel(key, lang string) string {
	labels := map[string][2]string{
		"reasoning":             {"Reasoning", "Рассуждение"},
		"coding":                {"Coding", "Код"},
		"instruction_following": {"Instruction following", "Следование инструкциям"},
		"long_context":          {"Long context", "Длинный контекст"},
	}
	pair, ok := labels[key]
	if !ok {
		return key
	}
	if lang == "ru" {
		return pair[1]
	}
	return pair[0]
}

// Feedback edit-form focus fields, in tab order: the overall rating, then
// each of feedbackSkillKeys in order, then the free-text review.
const (
	feedbackFocusOverall = iota
	feedbackFocusSkillFirst
)

var feedbackFocusReview = feedbackFocusSkillFirst + len(feedbackSkillKeys)
var feedbackFieldCount = feedbackFocusReview + 1

// tuiFeedbackState is the Feedback tab's per-session state: which model it
// was last loaded/edited for, the last known-good summary, and (while
// editing) the user's in-progress draft. Exactly one model's feedback is
// ever shown at a time, matching how the detail overlay itself only ever
// shows one row — see tuiModel.feedback.
type tuiFeedbackState struct {
	// slug and reqSeq identify which model and which in-flight request this
	// state belongs to. reqSeq is ONE monotonic counter shared by every
	// GetSummary and PutFeedback dispatch for this tab — never two separate
	// counters — specifically so a save can never be mistaken for stale (or
	// vice versa) by racing against the wrong sequence space: startFeedbackLoad
	// bumps it and replaces the whole struct (so a model switch always moves
	// it forward, even across the reset), and startFeedbackSave bumps the
	// very same counter rather than a private one. A GetSummary/PutFeedback
	// response is applied only when both its slug and its reqSeq still match
	// — see applyFeedbackSummaryMsg/applyFeedbackSaveMsg. This is what keeps
	// a slow response for a previously-viewed model, or a superseded earlier
	// attempt for the same model, from ever repainting current state.
	slug   string
	reqSeq uint64

	loading bool
	loaded  bool
	loadErr string

	summary feedbackclient.Summary

	editing       bool
	focus         int
	draftOverall  int
	draftSkills   [4]int
	draftReview   string
	validationErr string

	saving  bool
	saveErr string
	// savedFlash is set true right after a save completes successfully and
	// cleared the next time editing (re)starts — a lightweight "Saved" state
	// (brief 8.1's loading/saved/offline/validation-error state list)
	// without a separate timer or extra message type.
	savedFlash bool
	// othersRequested is true once the user has explicitly asked to see the
	// community aggregate excluding their own vote (Summary.Others) — brief
	// 8.2 step 2's "others запрашивать только явным действием": it is never
	// fetched as part of the normal GetSummary call that fires on opening
	// the tab, only in response to feedback.go's "others" action.
	othersRequested bool
}

// tuiFeedbackSummaryMsg is the result of a GetSummary tea.Cmd.
type tuiFeedbackSummaryMsg struct {
	slug          string
	reqSeq        uint64
	includeOthers bool
	summary       feedbackclient.Summary
	err           error
}

// tuiFeedbackSaveMsg is the result of a PutFeedback tea.Cmd.
type tuiFeedbackSaveMsg struct {
	slug    string
	reqSeq  uint64
	summary feedbackclient.Summary
	err     error
}

// feedbackSummaryCmd returns a tea.Cmd calling GetSummary for slug. The
// client's own configured RequestTimeout already bounds the call (applied
// inside feedbackclient.Client.do on top of ctx), so no separate timeout is
// added here — matching brief 8.2 step 2's "с context/timeout вызывает
// GetFeedbackSummary". includeOthers is false for every automatic load and
// true only for the explicit "others" action — see tuiFeedbackState's own
// othersRequested doc comment. reqSeq is the shared load/save counter — see
// tuiFeedbackState's own doc comment.
func (m tuiModel) feedbackSummaryCmd(slug string, reqSeq uint64, includeOthers bool) tea.Cmd {
	c, ctx := m.feedbackClient, m.ctx
	return func() tea.Msg {
		reqCtx := ctx
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		summary, err := c.GetSummary(reqCtx, slug, includeOthers)
		return tuiFeedbackSummaryMsg{slug: slug, reqSeq: reqSeq, includeOthers: includeOthers, summary: summary, err: err}
	}
}

// feedbackSaveCmd returns a tea.Cmd calling PutFeedback once for slug — a
// single idempotent PUT per brief 8.2 step 5, never retried automatically.
// reqSeq is the shared load/save counter — see tuiFeedbackState's own doc
// comment for why this must never be a private save-only counter.
func (m tuiModel) feedbackSaveCmd(slug string, reqSeq uint64, req feedbackclient.FeedbackRequest) tea.Cmd {
	c, ctx := m.feedbackClient, m.ctx
	return func() tea.Msg {
		reqCtx := ctx
		if reqCtx == nil {
			reqCtx = context.Background()
		}
		summary, err := c.PutFeedback(reqCtx, slug, req)
		return tuiFeedbackSaveMsg{slug: slug, reqSeq: reqSeq, summary: summary, err: err}
	}
}

// applyFeedbackSummaryMsg applies a GetSummary result, discarding it outright
// if it no longer matches the feedback state's current slug/reqSeq (a stale
// response for a model the user has since left, or superseded by a newer
// request — load or save — for the same model, see startFeedbackLoad).
func (m tuiModel) applyFeedbackSummaryMsg(msg tuiFeedbackSummaryMsg) tuiModel {
	if msg.slug != m.feedback.slug || msg.reqSeq != m.feedback.reqSeq {
		return m
	}
	m.feedback.loading = false
	if msg.err != nil {
		m.feedback.loadErr = feedbackErrorMessage(msg.err, m.lang)
		return m
	}
	m.feedback.loadErr = ""
	m.feedback.loaded = true
	m.feedback.summary = msg.summary
	if msg.includeOthers {
		m.feedback.othersRequested = true
	}
	if !m.feedback.editing {
		m.feedback.draftOverall, m.feedback.draftSkills, m.feedback.draftReview = feedbackDraftFromSummary(msg.summary)
	}
	return m
}

// applyFeedbackSaveMsg applies a PutFeedback result, guarded by slug/reqSeq
// the same way applyFeedbackSummaryMsg is — against the SAME shared counter,
// not a separate save-only one, so a save superseded by an intervening
// model-switch-and-back (which bumps reqSeq via startFeedbackLoad) or by a
// second save for the same model is correctly seen as stale rather than
// silently overwriting the newer attempt's outcome. On error the draft
// (overall/skills/review) is left exactly as the user entered it and editing
// stays open — brief 8.2 step 6: "оставить введённый draft, показать retry,
// не считать сохранение успешным". On success the local state is replaced by
// the server's own response, per step 5.
func (m tuiModel) applyFeedbackSaveMsg(msg tuiFeedbackSaveMsg) tuiModel {
	if msg.slug != m.feedback.slug || msg.reqSeq != m.feedback.reqSeq {
		return m
	}
	m.feedback.saving = false
	if msg.err != nil {
		m.feedback.saveErr = feedbackErrorMessage(msg.err, m.lang)
		return m
	}
	m.feedback.saveErr = ""
	m.feedback.validationErr = ""
	m.feedback.loaded = true
	m.feedback.loadErr = ""
	m.feedback.summary = msg.summary
	m.feedback.editing = false
	m.feedback.savedFlash = true
	m.feedback.draftOverall, m.feedback.draftSkills, m.feedback.draftReview = feedbackDraftFromSummary(msg.summary)
	return m
}

// feedbackDraftFromSummary extracts a fresh edit draft from a Summary's own
// "mine" — nil (never rated) becomes an all-zero, empty draft, never a fake
// score.
func feedbackDraftFromSummary(summary feedbackclient.Summary) (overall int, skills [4]int, review string) {
	if summary.Mine == nil {
		return 0, [4]int{}, ""
	}
	overall = summary.Mine.Overall
	for _, rated := range summary.Mine.Skills {
		for i, key := range feedbackSkillKeys {
			if rated.Key == key {
				skills[i] = rated.Rating
			}
		}
	}
	review = summary.Mine.Review
	return overall, skills, review
}

// feedbackErrorMessage turns a typed internal/feedback/client error into a
// short, localized, human-readable line — distinguishing offline/timeout
// from a rejected submission from an unexpected server error, per brief
// 8.1's loading/saved/offline/validation-error state list.
func feedbackErrorMessage(err error, lang string) string {
	ru := lang == "ru"
	switch {
	case feedbackclient.IsTimeout(err):
		if ru {
			return "Сервер отзывов не отвечает (таймаут)"
		}
		return "Feedback server timed out"
	case feedbackclient.IsNetworkError(err):
		if ru {
			return "Нет соединения с сервером отзывов (offline)"
		}
		return "Feedback server unreachable (offline)"
	case feedbackclient.IsCredentialError(err):
		if ru {
			return "Проблема с локальными учётными данными отзывов"
		}
		return "Feedback credentials problem"
	case feedbackclient.IsUnauthorized(err), feedbackclient.IsForbidden(err):
		if ru {
			return "Сервер отзывов отклонил учётные данные"
		}
		return "Feedback server rejected the credentials"
	case feedbackclient.IsRateLimited(err):
		if ru {
			return "Сервер отзывов ограничил частоту запросов, попробуйте позже"
		}
		return "Feedback server rate-limited this request; try again later"
	case feedbackclient.IsServiceUnavailable(err):
		if ru {
			return "Сервер отзывов временно недоступен"
		}
		return "Feedback server temporarily unavailable"
	case feedbackclient.IsValidationError(err):
		// *APIError.Error() embeds the server's own message string
		// (errorResponseDTO's "error" field) verbatim — server-controlled
		// text that must go through the same terminal-safe sanitizer as
		// review text before it is ever interpolated into a rendered line,
		// not the older, narrower normalizePlainLine the shared Detail
		// pipeline applies (that one does not strip C1 controls or bidi
		// overrides, only 7-bit escapes and bytes <0x20/0x7f).
		if ru {
			return "Сервер отклонил отзыв: " + sanitizeFeedbackReviewText(err.Error())
		}
		return "Feedback server rejected the submission: " + sanitizeFeedbackReviewText(err.Error())
	default:
		// Same reasoning as above: err.Error() here can also be an
		// *APIError carrying server-controlled text (any error shape not
		// matched by a more specific case above).
		if ru {
			return "Ошибка отзывов: " + sanitizeFeedbackReviewText(err.Error())
		}
		return "Feedback error: " + sanitizeFeedbackReviewText(err.Error())
	}
}

// startFeedbackLoad resets the feedback state for row and dispatches a fresh
// GetSummary. Bumping seq (rather than starting a parallel counter) is what
// lets a superseded in-flight request for the very same slug — e.g. the user
// pressed retry twice — be told apart from the one whose result should
// actually apply.
func (m tuiModel) startFeedbackLoad(row model.Model, includeOthers bool) (tuiModel, tea.Cmd) {
	nextSeq := m.feedback.reqSeq + 1
	m.feedback = tuiFeedbackState{slug: row.Slug, reqSeq: nextSeq, loading: true}
	return m, m.feedbackSummaryCmd(row.Slug, nextSeq, includeOthers)
}

// ensureFeedbackSummaryLoaded is called on every keypress while the detail
// overlay's Feedback tab is active (see tui.go's key()). It is a no-op
// whenever feedback is disabled, or a load for the current model is already
// in flight, already succeeded, or already failed (a failed load waits for
// an explicit retry rather than silently re-firing on the next keystroke).
func (m tuiModel) ensureFeedbackSummaryLoaded(row model.Model) (tuiModel, tea.Cmd) {
	if m.feedbackClient == nil {
		return m, nil
	}
	if m.feedback.slug == row.Slug && (m.feedback.loading || m.feedback.loaded || m.feedback.loadErr != "") {
		return m, nil
	}
	return m.startFeedbackLoad(row, false)
}

// feedbackViewKey handles the Feedback tab's own actions while in read-only
// (non-editing) view mode: entering the edit form, retrying a failed load,
// and explicitly asking to see the community aggregate excluding the user's
// own vote. handled is false for every other key, which then falls through
// to the shared detail scroll/tab switch in tui.go's key(), unchanged.
func (m tuiModel) feedbackViewKey(originalKey string, row model.Model) (tuiModel, tea.Cmd, bool) {
	if m.feedbackClient == nil {
		return m, nil, false
	}
	if m.keyMatches("feedback", "edit", originalKey) {
		if m.feedback.saving {
			return m, nil, true
		}
		m.feedback.editing = true
		m.feedback.focus = feedbackFocusOverall
		m.feedback.validationErr = ""
		m.feedback.saveErr = ""
		m.feedback.savedFlash = false
		return m, nil, true
	}
	if m.keyMatches("feedback", "retry", originalKey) {
		if m.feedback.loading || m.feedback.loadErr == "" {
			return m, nil, true
		}
		next, cmd := m.startFeedbackLoad(row, false)
		return next, cmd, true
	}
	if m.keyMatches("feedback", "others", originalKey) {
		if m.feedback.loading || !m.feedback.loaded || m.feedback.othersRequested {
			return m, nil, true
		}
		next, cmd := m.startFeedbackLoad(row, true)
		return next, cmd, true
	}
	return m, nil, false
}

// feedbackEditKey handles every key while the edit form owns the keyboard
// (m.feedback.editing) — mirroring how the settings/filter/columns overlays
// already own the keyboard fully while open, rather than sharing it with the
// detail screen's own tab/scroll bindings.
func (m tuiModel) feedbackEditKey(key, originalKey string, runes []rune) (tuiModel, tea.Cmd) {
	if m.keyMatches("feedback", "cancel", originalKey) {
		m.feedback.editing = false
		m.feedback.validationErr = ""
		m.feedback.saveErr = ""
		// Discard the abandoned draft back to the last known-good summary —
		// re-entering edit afterwards starts fresh from server truth, never
		// silently resuming a cancelled edit.
		m.feedback.draftOverall, m.feedback.draftSkills, m.feedback.draftReview = feedbackDraftFromSummary(m.feedback.summary)
		return m, nil
	}
	if m.keyMatches("feedback", "save", originalKey) {
		return m.startFeedbackSave()
	}
	if m.feedback.saving {
		// brief 8.1's "disabled/save-in-progress state": the form does not
		// accept further edits until the in-flight PUT resolves.
		return m, nil
	}
	if m.feedback.focus == feedbackFocusReview {
		m.feedback = m.feedback.reviewKey(key, runes)
		return m, nil
	}
	switch key {
	case "up", "shift+tab":
		m.feedback.focus = (m.feedback.focus - 1 + feedbackFieldCount) % feedbackFieldCount
	case "down", "tab":
		m.feedback.focus = (m.feedback.focus + 1) % feedbackFieldCount
	case "left":
		m.feedback.adjustFocused(-1)
	case "right":
		m.feedback.adjustFocused(1)
	case "1", "2", "3", "4", "5":
		m.feedback.setFocused(int(key[0] - '0'))
	case "0", "backspace":
		m.feedback.setFocused(0)
	}
	return m, nil
}

// reviewKey applies one keypress to the free-text review field: it is the
// only field where printable runes are literal input rather than a rating
// shortcut or a focus move.
func (f tuiFeedbackState) reviewKey(key string, runes []rune) tuiFeedbackState {
	switch key {
	case "up", "shift+tab":
		f.focus = feedbackFocusReview - 1
	case "down", "tab":
		f.focus = feedbackFocusOverall
	case "backspace":
		_, n := utf8.DecodeLastRuneInString(f.draftReview)
		if n > 0 {
			f.draftReview = f.draftReview[:len(f.draftReview)-n]
		}
	case "enter":
		if utf8.RuneCountInString(f.draftReview) < feedbackReviewMaxRunes {
			f.draftReview += "\n"
		}
	default:
		if len(runes) > 0 && utf8.RuneCountInString(f.draftReview)+len(runes) <= feedbackReviewMaxRunes {
			f.draftReview += string(runes)
		}
	}
	return f
}

func clampFeedbackRating(value int) int {
	if value < 0 {
		return 0
	}
	if value > 5 {
		return 5
	}
	return value
}

// adjustFocused nudges the currently-focused rating field by delta (Left/
// Right), clamped to 0 ("not rated") through 5. A no-op when the review
// field has focus — that field has no scalar value to nudge.
func (f *tuiFeedbackState) adjustFocused(delta int) {
	switch {
	case f.focus == feedbackFocusOverall:
		f.draftOverall = clampFeedbackRating(f.draftOverall + delta)
	case f.focus >= feedbackFocusSkillFirst && f.focus < feedbackFocusReview:
		i := f.focus - feedbackFocusSkillFirst
		f.draftSkills[i] = clampFeedbackRating(f.draftSkills[i] + delta)
	}
}

// setFocused sets the currently-focused rating field to an absolute value
// (the digit keys 0-5), clamped the same way adjustFocused is.
func (f *tuiFeedbackState) setFocused(value int) {
	value = clampFeedbackRating(value)
	switch {
	case f.focus == feedbackFocusOverall:
		f.draftOverall = value
	case f.focus >= feedbackFocusSkillFirst && f.focus < feedbackFocusReview:
		f.draftSkills[f.focus-feedbackFocusSkillFirst] = value
	}
}

// startFeedbackSave validates the draft locally, and — only when valid —
// sanitizes the review text and dispatches exactly one PutFeedback call
// (brief 8.2 step 5). Overall is the one field the server requires; skills
// and review are optional and simply omitted/empty when unset.
func (m tuiModel) startFeedbackSave() (tuiModel, tea.Cmd) {
	if m.feedbackClient == nil || m.feedback.saving {
		return m, nil
	}
	if m.feedback.draftOverall < 1 || m.feedback.draftOverall > 5 {
		if m.lang == "ru" {
			m.feedback.validationErr = "Поставьте общую оценку 1-5 перед сохранением"
		} else {
			m.feedback.validationErr = "Rate Overall 1-5 before saving"
		}
		return m, nil
	}
	skills := make([]feedbackclient.SkillRating, 0, len(feedbackSkillKeys))
	for i, key := range feedbackSkillKeys {
		if rating := m.feedback.draftSkills[i]; rating >= 1 && rating <= 5 {
			skills = append(skills, feedbackclient.SkillRating{Key: key, Rating: rating})
		}
	}
	// The review text is sanitized before it is ever sent, not only before
	// it is displayed: a hostile paste has no reason to reach the server
	// (and, from there, any future reader) unsanitized just because display
	// sanitization already protects this client's own terminal.
	review := sanitizeFeedbackReviewText(m.feedback.draftReview)
	m.feedback.validationErr = ""
	m.feedback.saveErr = ""
	m.feedback.saving = true
	// Bump the SAME counter startFeedbackLoad bumps — never a private
	// save-only one — so a stale save response can never be mistaken for
	// current just because the user switched models and back in between
	// (see tuiFeedbackState's own reqSeq doc comment).
	m.feedback.reqSeq++
	req := feedbackclient.FeedbackRequest{Overall: m.feedback.draftOverall, Skills: skills, Review: review}
	return m, m.feedbackSaveCmd(m.feedback.slug, m.feedback.reqSeq, req)
}

// withFeedbackClient constructs m.feedbackClient from fc when enabled,
// resolving TokenFile/IdentityFile the same config-relative way
// DataDir/DefaultOutput already are (resolveConfigPath). A disabled section,
// or one that somehow still fails to construct a client despite already
// having passed cfg.Feedback.Validate() during config.Load, simply leaves
// feedback turned off — never a fatal error for the whole TUI.
func (m tuiModel) withFeedbackClient(fc config.FeedbackConfig) tuiModel {
	if !fc.Enabled {
		m.feedbackClient = nil
		return m
	}
	timeout, err := fc.EffectiveRequestTimeout()
	if err != nil {
		m.feedbackClient = nil
		return m
	}
	tokenFile := fc.EffectiveTokenFile()
	identityFile := fc.EffectiveIdentityFile()
	if m.configPath != "" {
		tokenFile = resolveConfigPath(m.configPath, tokenFile)
		identityFile = resolveConfigPath(m.configPath, identityFile)
	}
	client, err := feedbackclient.New(feedbackclient.Config{
		Endpoint:       fc.EffectiveEndpoint(),
		TokenFile:      tokenFile,
		IdentityFile:   identityFile,
		RequestTimeout: timeout,
	})
	if err != nil {
		m.feedbackClient = nil
		return m
	}
	m.feedbackClient = client
	return m
}

// feedbackTimeString formats a timestamp for the Feedback tab in a fixed,
// language-independent shape (this screen otherwise never shows clock
// times, only dates), skipping a zero time entirely.
func feedbackTimeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
