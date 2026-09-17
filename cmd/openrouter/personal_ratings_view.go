package main

import (
	"fmt"
	"strings"
)

// personalRatingsLabels is the "My ratings" view's own localized chrome,
// mirroring feedbackLabels's role for the Feedback tab (feedback_labels.go).
// heading is the primary label the plan calls for ("Мои оценки"/"My
// ratings"); subheading carries the plan's alternate synonym ("Персональный
// рейтинг"/"Personal ranking") as a short explanatory suffix on the title
// line, rather than a second, competing primary label.
type personalRatingsLabels struct {
	heading, subheading                            string
	columnRank, columnName, columnMine, columnBase string
	loadingLine, readyLine, emptyLine              string
	partialErrorFormat                             string
	// hints is this view's own footer hotkey line — kept here rather than
	// routed through m.t (tui.go's English-keyed translation table) since
	// that table only ever covers the normal list's own literal English
	// strings; a string absent from it silently falls back to English
	// inside an otherwise fully Russian view.
	hints string
}

func personalRatingsLabelsForLang(lang string) personalRatingsLabels {
	if lang == "ru" {
		return personalRatingsLabels{
			heading:            "Мои оценки",
			subheading:         "персональный рейтинг",
			columnRank:         "#",
			columnName:         "Модель",
			columnMine:         "Моя оценка",
			columnBase:         "Базовая позиция",
			loadingLine:        "Загрузка ваших оценок...",
			readyLine:          "готово",
			emptyLine:          "Вы ещё не оценили ни одну модель.",
			partialErrorFormat: "не удалось получить %d из %d оценок; показаны только успешно загруженные модели.",
			hints:              "↑↓ навигация · Enter детали · M назад к моделям · x выход",
		}
	}
	return personalRatingsLabels{
		heading:            "My ratings",
		subheading:         "personal ranking",
		columnRank:         "#",
		columnName:         "Model",
		columnMine:         "My rating",
		columnBase:         "Base position",
		loadingLine:        "Loading your ratings...",
		readyLine:          "ready",
		emptyLine:          "You have not rated any models yet.",
		partialErrorFormat: "%d of %d ratings could not be loaded; showing the models that loaded successfully.",
		hints:              "↑↓ navigate · Enter details · M back to models · x quit",
	}
}

// tuiPersonalRatingsBaseView renders the "My ratings" list mode in place of
// the normal table (baseView delegates here whenever m.personalRatings.active
// is true): only models this identity has rated, ordered by that rating,
// each row showing its own rating and its base_position for context — see
// personal_ratings.go for the ordering itself. It reuses m.cursor/m.visible
// exactly like the normal list, so navigation, detail drilldown (Enter) and
// the universal quit/close keys keep working unchanged; only rendering
// differs here.
func tuiPersonalRatingsBaseView(m tuiModel) string {
	pl := personalRatingsLabelsForLang(m.lang)
	titleText := pl.heading + " (" + pl.subheading + ")"
	if m.width >= 80 {
		titleText += "  " + m.freshnessLine()
	}
	rows := m.personalRatingsEffectiveRows()
	title := truncateTable(titleText, m.width)
	meta := truncateTable(plainTableText(fmt.Sprintf("%s: %d", pl.columnName, len(rows))), m.width)
	lines := []string{tuiTitleStyle.Render(title), tuiMetaStyle.Render(meta)}

	rankWidth, identityWidth, mineWidth, baseWidth := personalRatingsColumnWidths(m.width)
	header := "  " + strings.Join([]string{
		padTableCell(truncateTable(pl.columnRank, rankWidth), rankWidth),
		padTableCell(truncateTable(pl.columnName, identityWidth), identityWidth),
		padTableCell(truncateTable(pl.columnMine, mineWidth), mineWidth),
		padTableCell(truncateTable(pl.columnBase, baseWidth), baseWidth),
	}, " | ")
	lines = append(lines, tuiHeaderStyle.Render(truncateTable(header, m.width)))

	status := personalRatingsStatusLine(m, pl)
	statusLine := tuiStatusStyle.Render(truncateTable(plainTableText(status), m.width))
	if m.personalRatings.err != "" {
		statusLine = tuiErrorStyle.Render(truncateTable(plainTableText(status), m.width))
	}
	hintsLine := tuiHintStyle.Render(truncateTable(pl.hints, m.width))

	rowsBudget := m.height - 6
	if rowsBudget > 0 {
		start := max(0, min(m.cursor-max(1, rowsBudget)/2, max(0, len(rows)-1)))
		end := min(len(rows), start+rowsBudget)
		for i := start; i < end; i++ {
			line := personalRatingsRowText(m, i, rows[i], i == m.cursor, rankWidth, identityWidth, mineWidth, baseWidth)
			if i == m.cursor {
				line = tuiSelectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
		if len(rows) == 0 && !m.personalRatings.loading && m.personalRatings.loaded && m.personalRatings.errCount == 0 {
			lines = append(lines, "  "+pl.emptyLine)
		}
	}

	if m.height >= 6 {
		footer := []string{statusLine, hintsLine}
		if len(lines)+1+len(footer) <= m.height {
			lines = append(lines, "")
		}
		lines = append(lines, footer...)
		return strings.Join(lines, "\n")
	}
	return m.compactView(lines, statusLine, hintsLine, "")
}

// personalRatingsStatusLine reports loading/error/partial-error/ready in
// that priority order — a full-batch failure (every request errored) takes
// precedence over the ready line, but a partial failure (some models loaded)
// is reported without hiding the rows that did load.
func personalRatingsStatusLine(m tuiModel, pl personalRatingsLabels) string {
	switch {
	case m.personalRatings.loading:
		return pl.loadingLine
	case m.personalRatings.err != "":
		return m.personalRatings.err
	case m.personalRatings.errCount > 0:
		return fmt.Sprintf(pl.partialErrorFormat, m.personalRatings.errCount, m.personalRatings.total)
	default:
		return pl.readyLine
	}
}

// personalRatingsColumnWidths fixes the rank/mine/base column widths and
// gives everything else to the identity column, shared by both the header
// and every row so the two stay aligned. The "  "/"> " prefix and the three
// " | " separators are accounted for identically to how the normal table's
// own tuiCellWidths reserves its own fixed overhead.
func personalRatingsColumnWidths(width int) (rank, identity, mine, base int) {
	rank, mine, base = 4, 9, 16
	fixed := rank + mine + base + len(" | ")*3 + len("> ")
	identity = max(10, width-fixed)
	return
}

func personalRatingsRowText(m tuiModel, index int, row personalRatingRow, selected bool, rankWidth, identityWidth, mineWidth, baseWidth int) string {
	l := feedbackLabelsForLang(m.lang)
	identity := plainTableText(modelIdentityWithIconsAndGaps(row.model, m.icons, m.iconGaps, m.iconGap))
	rank := fmt.Sprintf("%d.", index+1)
	mine := feedbackRatingText(row.overall, l)
	base := personalRatingsBasePositionText(row, l)
	prefix := "  "
	if selected {
		prefix = "> "
	}
	cells := []string{
		padTableCell(truncateTable(rank, rankWidth), rankWidth),
		padTableCell(truncateTable(identity, identityWidth), identityWidth),
		padTableCell(truncateTable(mine, mineWidth), mineWidth),
		padTableCell(truncateTable(base, baseWidth), baseWidth),
	}
	return truncateTable(prefix+strings.Join(cells, " | "), m.width)
}

// personalRatingsBasePositionText renders a row's base_position, never a
// fabricated number for a model absent from the app's own base ranking.
func personalRatingsBasePositionText(row personalRatingRow, l feedbackLabels) string {
	if !row.hasBasePosition {
		return l.notRankedYet
	}
	return fmt.Sprintf("#%d", row.basePosition)
}
