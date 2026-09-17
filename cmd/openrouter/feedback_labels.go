package main

import (
	"fmt"
	"time"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
)

// feedbackLabels is the Feedback tab's own localized chrome, mirroring
// internal/tui/screen/output.DetailLabels's role for the other tabs — kept
// here instead, since the Feedback tab's content never flows through that
// package's DetailLines at all (see feedback_view.go's feedbackTabLines).
type feedbackLabels struct {
	lang string

	heading                                                  string
	disabledLine, disabledHint                               string
	loadingLine                                              string
	retryHint, saveRetryHint                                 string
	savedLine                                                string
	myRatingField, communityRatingField, myPositionField     string
	positionExplain                                          string
	mySectionHeading, communitySectionHeading                string
	othersSectionHeading                                     string
	overallField, distributionField, updatedField, asOfField string
	reviewHeading                                            string
	placeholder, never                                       string
	notRated, noRatingsYet, notRankedYet, ineligibleValue    string
	editHeading, editHint, editKeysHint                      string
	editActionHint, othersActionHint                         string
	reviewFieldFormat                                        string
	savingLine                                               string
}

func feedbackLabelsForLang(lang string) feedbackLabels {
	if lang == "ru" {
		return feedbackLabels{
			lang:                    "ru",
			heading:                 "-- Отзывы --",
			disabledLine:            "Отзывы выключены.",
			disabledHint:            "Включите feedback.enabled: true (и token_file/identity_file) в config.yaml, запустите feedback-server и `feedback init`, чтобы оценивать модели и видеть оценки сообщества здесь.",
			loadingLine:             "Загрузка отзывов...",
			retryHint:               "Нажмите r для повтора.",
			saveRetryHint:           "Черновик сохранён — нажмите Ctrl+S для повтора.",
			savedLine:               "Сохранено.",
			myRatingField:           "Моя оценка: ",
			communityRatingField:    "Оценка сообщества: ",
			myPositionField:         "Моя позиция: ",
			positionExplain:         "Личная позиция считается только среди моделей, которые вы сами оценили; она не меняет строки таблицы и не влияет на личную позицию другой identity.",
			mySectionHeading:        "-- Моя оценка --",
			communitySectionHeading: "-- Оценка сообщества --",
			othersSectionHeading:    "-- Сообщество без моего голоса --",
			overallField:            "  Общая: ",
			distributionField:       "Распределение: ",
			updatedField:            "  Обновлено: ",
			asOfField:               "  По состоянию на: ",
			reviewHeading:           "  Отзыв:",
			placeholder:             "н/д",
			never:                   "никогда",
			notRated:                "не оценено",
			noRatingsYet:            "оценок пока нет",
			notRankedYet:            "не ранжировано",
			ineligibleValue:         "недоступно (мало оценок сообщества)",
			editHeading:             "-- Отзывы: оценить модель --",
			editHint:                "Редактирование вашей оценки.",
			editKeysHint:            "Tab/Up/Down — поля · Left/Right или 1-5 — оценка · 0/Backspace — очистить · Ctrl+S — сохранить · Esc — отмена",
			editActionHint:          "e редактировать/оценить",
			othersActionHint:        "o сообщество без моего голоса",
			reviewFieldFormat:       "  Отзыв (%d/%d симв., %d/%d строк):",
			savingLine:              "Сохранение...",
		}
	}
	return feedbackLabels{
		lang:                    "",
		heading:                 "-- Feedback --",
		disabledLine:            "Feedback is disabled.",
		disabledHint:            "Set feedback.enabled: true (plus token_file/identity_file) in config.yaml, run feedback-server and `feedback init`, to rate models and see community ratings here.",
		loadingLine:             "Loading feedback...",
		retryHint:               "Press r to retry.",
		saveRetryHint:           "Draft kept — press Ctrl+S to retry.",
		savedLine:               "Saved.",
		myRatingField:           "My rating: ",
		communityRatingField:    "Community rating: ",
		myPositionField:         "My position: ",
		positionExplain:         "Personal position ranks this model only among the ones you have rated yourself; it never changes the main table rows or another identity's own personal position.",
		mySectionHeading:        "-- My rating --",
		communitySectionHeading: "-- Community rating --",
		othersSectionHeading:    "-- Community (excluding my vote) --",
		overallField:            "  Overall: ",
		distributionField:       "Distribution: ",
		updatedField:            "  Updated: ",
		asOfField:               "  As of: ",
		reviewHeading:           "  Review:",
		placeholder:             "n/a",
		never:                   "never",
		notRated:                "not rated",
		noRatingsYet:            "no ratings yet",
		notRankedYet:            "not ranked yet",
		ineligibleValue:         "not available (too few community ratings)",
		editHeading:             "-- Feedback: rate this model --",
		editHint:                "Editing your rating.",
		editKeysHint:            "Tab/Up/Down move fields · Left/Right or 1-5 set a rating · 0/Backspace clears it · Ctrl+S save · Esc cancel",
		editActionHint:          "e edit/rate this model",
		othersActionHint:        "o community excluding my vote",
		reviewFieldFormat:       "  Review (%d/%d chars, %d/%d lines):",
		savingLine:              "Saving...",
	}
}

// feedbackRatingText renders a single 0-5 rating value: 0 always means "not
// rated" (never a fake score), 1-5 render as "N/5".
func feedbackRatingText(value int, l feedbackLabels) string {
	if value <= 0 {
		return l.notRated
	}
	return fmt.Sprintf("%d/5", value)
}

// feedbackAggregateOverallText renders a community/others aggregate's
// overall average and vote count, or "no ratings yet" for a nil aggregate or
// one with zero votes — Aggregate.Average is a pointer specifically so this
// can never confuse "no ratings" with "a real average of 0".
func feedbackAggregateOverallText(agg *feedbackclient.Aggregate, l feedbackLabels) string {
	if agg == nil || agg.Count == 0 || agg.Average == nil {
		return l.noRatingsYet
	}
	count := feedbackVoteCountText(agg.Count, l)
	return fmt.Sprintf("%.1f (%s)", *agg.Average, count)
}

// feedbackVoteCountText pluralizes a vote count in the active language,
// reusing the same one/few/many (Russian) and singular/plural (English)
// helpers the rest of the detail screen already uses.
func feedbackVoteCountText(n int, l feedbackLabels) string {
	if l.lang == "ru" {
		return tuiPlural(n, "голос", "голоса", "голосов")
	}
	return tuiPluralEN(n, "vote")
}

// feedbackSkillAggregateText finds key's aggregate within skills (present
// only for a skill with at least one rating) and renders it the same way
// feedbackAggregateOverallText does, or "no ratings yet" when absent.
func feedbackSkillAggregateText(skills []feedbackclient.SkillAggregate, key string, l feedbackLabels) string {
	for _, s := range skills {
		if s.Key == key {
			return fmt.Sprintf("%.1f (%s)", s.Average, feedbackVoteCountText(s.Count, l))
		}
	}
	return l.noRatingsYet
}

// feedbackDistributionText renders a 1-5 rating histogram as
// "1:a 2:b 3:c 4:d 5:e" — plain digits, no translation needed.
func feedbackDistributionText(d [5]int, l feedbackLabels) string {
	if d == ([5]int{}) {
		return l.noRatingsYet
	}
	return fmt.Sprintf("1:%d 2:%d 3:%d 4:%d 5:%d", d[0], d[1], d[2], d[3], d[4])
}

// feedbackPositionText renders a Position, distinguishing ranked from
// unranked (never computed, or nothing to rank yet) from ineligible (policy
// withholds it) — never presenting a fake number for the latter two.
func feedbackPositionText(p feedbackclient.Position, l feedbackLabels) string {
	switch p.Status {
	case feedbackclient.PositionRanked:
		return fmt.Sprintf("#%d", p.Value)
	case feedbackclient.PositionIneligible:
		return l.ineligibleValue
	default: // PositionUnranked, or an unrecognized future status
		return l.notRankedYet
	}
}

// feedbackTimeStringOrPlaceholder formats t via feedbackTimeString, falling
// back to the localized placeholder for a zero time.
func feedbackTimeStringOrPlaceholder(t time.Time, l feedbackLabels) string {
	if formatted := feedbackTimeString(t); formatted != "" {
		return formatted
	}
	return l.placeholder
}
