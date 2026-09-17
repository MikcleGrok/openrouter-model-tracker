package main

import (
	"fmt"
	"strings"
	"time"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
)

// feedbackTabLines builds the Feedback tab's logical lines for row, in the
// same style detail_lines.go already uses ("-- Section --" headings,
// "Label: value" rows, "  - item" bullets) so the shared tuiStyleDetailLine
// pass colours it identically to every other tab. It never mixes network
// calls into rendering — this is a pure function of m.feedbackClient/
// m.feedback/m.lang/row — matching brief 8.3's "не выполнять HTTP прямо в
// Update или View".
//
// base_position is deliberately never shown here: this feedback server has
// no access to the app's own benchmark ranking (Task 4's reviewed design)
// and always reports it PositionUnranked, so displaying it would present a
// meaningless placeholder as if it were real data. The app's existing
// ranking is already visible elsewhere on the detail screen (Identity/
// Pricing/Benchmarks tabs and the main table itself).
func (m tuiModel) feedbackTabLines(row model.Model) []string {
	l := feedbackLabelsForLang(m.lang)
	if m.feedbackClient == nil {
		return []string{
			l.heading,
			l.disabledLine,
			"  " + l.disabledHint,
		}
	}
	if m.feedback.editing {
		return m.feedbackEditLines(row, l)
	}
	return m.feedbackViewLines(row, l)
}

// feedbackViewLines renders the read-only summary: loading skeleton, a load
// error with a retry hint, or the loaded summary itself.
func (m tuiModel) feedbackViewLines(row model.Model, l feedbackLabels) []string {
	st := m.feedback
	if st.slug != row.Slug || (!st.loading && !st.loaded && st.loadErr == "") {
		return []string{l.heading, l.loadingLine}
	}
	if st.loading {
		return []string{l.heading, l.loadingLine}
	}
	if st.loadErr != "" {
		return []string{l.heading, st.loadErr, "  " + l.retryHint}
	}

	summary := st.summary
	lines := []string{l.heading}
	lines = append(lines, l.myRatingField+feedbackRatingText(summaryMineOverall(summary), l))
	lines = append(lines, l.communityRatingField+feedbackAggregateOverallText(summary.Community, l))
	lines = append(lines, l.myPositionField+feedbackPositionText(summary.PersonalPosition, l))
	lines = append(lines, "  "+l.positionExplain)
	if st.savedFlash {
		lines = append(lines, "", l.savedLine)
	}

	lines = append(lines, "", l.mySectionHeading)
	lines = append(lines, l.overallField+feedbackRatingText(summaryMineOverall(summary), l))
	for _, key := range feedbackSkillKeys {
		rating := 0
		if summary.Mine != nil {
			rating = mineSkillRating(summary.Mine.Skills, key)
		}
		lines = append(lines, "  "+feedbackSkillLabel(key, m.lang)+": "+feedbackRatingText(rating, l))
	}
	if summary.Mine != nil {
		lines = append(lines, l.updatedField+feedbackTimeStringOrPlaceholder(summary.Mine.UpdatedAt, l))
		lines = append(lines, l.reviewHeading)
		lines = append(lines, feedbackReviewDisplayLines(summary.Mine.Review, l)...)
	} else {
		lines = append(lines, l.updatedField+l.never)
	}

	lines = append(lines, "", l.communitySectionHeading)
	lines = append(lines, l.overallField+feedbackAggregateOverallText(summary.Community, l))
	lines = append(lines, "  "+l.distributionField+feedbackDistributionText(distributionOf(summary.Community), l))
	for _, key := range feedbackSkillKeys {
		lines = append(lines, "  "+feedbackSkillLabel(key, m.lang)+": "+feedbackSkillAggregateText(skillsOf(summary.Community), key, l))
	}
	lines = append(lines, l.asOfField+feedbackTimeStringOrPlaceholder(computedAtOf(summary.Community), l))

	if summary.Others != nil {
		lines = append(lines, "", l.othersSectionHeading)
		lines = append(lines, l.overallField+feedbackAggregateOverallText(summary.Others, l))
		lines = append(lines, "  "+l.distributionField+feedbackDistributionText(distributionOf(summary.Others), l))
	}

	lines = append(lines, "", feedbackActionHints(m.feedback, l))
	return lines
}

// feedbackEditLines renders the rating form: field rows with a focus marker,
// the review field with its visible limit counter, and the current
// save/validation state.
func (m tuiModel) feedbackEditLines(row model.Model, l feedbackLabels) []string {
	st := m.feedback
	lines := []string{l.editHeading, l.editHint, l.editKeysHint, ""}

	marker := func(field int) string {
		if st.focus == field {
			return "> "
		}
		return "  "
	}
	lines = append(lines, marker(feedbackFocusOverall)+l.overallField+feedbackRatingText(st.draftOverall, l))
	for i, key := range feedbackSkillKeys {
		field := feedbackFocusSkillFirst + i
		lines = append(lines, marker(field)+feedbackSkillLabel(key, m.lang)+": "+feedbackRatingText(st.draftSkills[i], l))
	}
	runeCount := len([]rune(st.draftReview))
	lineCount := strings.Count(st.draftReview, "\n") + 1
	lines = append(lines, marker(feedbackFocusReview)+fmt.Sprintf(l.reviewFieldFormat, runeCount, feedbackReviewMaxRunes, lineCount, feedbackReviewMaxLines))
	lines = append(lines, feedbackReviewDisplayLines(st.draftReview, l)...)

	lines = append(lines, "")
	switch {
	case st.saving:
		lines = append(lines, l.savingLine)
	case st.saveErr != "":
		lines = append(lines, st.saveErr, "  "+l.saveRetryHint)
	case st.validationErr != "":
		lines = append(lines, st.validationErr)
	}
	return lines
}

// feedbackActionHints is the one-line reminder of what's available from the
// read-only view: entering the edit form, retrying after a load error (never
// shown once a load has actually succeeded), and revealing the
// community-excluding-me aggregate once (never re-offered after it has
// already been fetched).
func feedbackActionHints(st tuiFeedbackState, l feedbackLabels) string {
	hints := []string{l.editActionHint}
	if !st.othersRequested {
		hints = append(hints, l.othersActionHint)
	}
	return strings.Join(hints, " · ")
}

// feedbackReviewDisplayLines sanitizes review before it is ever rendered
// (brief 8.3's terminal-safe sanitization contract, applied unconditionally
// — including to the user's own in-progress draft, which may hold a raw
// paste) and returns it as one indented logical line so the shared Detail
// pipeline wraps/scrolls it exactly like Description/Note already are.
func feedbackReviewDisplayLines(review string, l feedbackLabels) []string {
	clean := sanitizeFeedbackReviewText(review)
	if strings.TrimSpace(clean) == "" {
		return []string{"  " + l.placeholder}
	}
	return []string{"  " + clean}
}

// --- small Summary/Aggregate accessors: nil-safe field reads that keep the
// render functions above free of repeated nil checks. ---

func summaryMineOverall(summary feedbackclient.Summary) int {
	if summary.Mine == nil {
		return 0
	}
	return summary.Mine.Overall
}

func mineSkillRating(skills []feedbackclient.SkillRating, key string) int {
	for _, s := range skills {
		if s.Key == key {
			return s.Rating
		}
	}
	return 0
}

func distributionOf(agg *feedbackclient.Aggregate) [5]int {
	if agg == nil {
		return [5]int{}
	}
	return agg.Distribution
}

func skillsOf(agg *feedbackclient.Aggregate) []feedbackclient.SkillAggregate {
	if agg == nil {
		return nil
	}
	return agg.Skills
}

func computedAtOf(agg *feedbackclient.Aggregate) time.Time {
	if agg == nil {
		return time.Time{}
	}
	return agg.ComputedAt
}
