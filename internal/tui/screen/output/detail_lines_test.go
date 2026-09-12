package output

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// TestDetailClaimsSplitsOnlyWhereAClaimReallyEnds is the claim splitter's
// contract. Every "keep" case below is a shape this screen's real notes
// contain — decimals, percentages, version numbers, domains, dotted
// abbreviations, parenthesised lists — and splitting inside one of them
// would cut a sentence in half rather than separate two claims.
func TestDetailClaimsSplitsOnlyWhereAClaimReallyEnds(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  []string
	}{
		{"sentence boundary", "Первое утверждение. Второе утверждение.", []string{"Первое утверждение.", "Второе утверждение."}},
		{"semicolon before a number", "в ранжирование пошло 93.4%; 76.8% сохранено здесь.", []string{"в ранжирование пошло 93.4%", "76.8% сохранено здесь."}},
		{"decimal is not a boundary", "Оценка 93.4% подтверждена.", []string{"Оценка 93.4% подтверждена."}},
		{"domain is not a boundary", "Источник — vals.ai и swebench.com.", []string{"Источник — vals.ai и swebench.com."}},
		{"lowercase continuation is not a boundary", "см. arXiv:2511.03929 и далее.", []string{"см. arXiv:2511.03929 и далее."}},
		{"closing bracket travels with its claim", "Проверено (2026-07-22). Дальше.", []string{"Проверено (2026-07-22).", "Дальше."}},
		{"explicit line break always splits", `first paragraph\nsecond paragraph`, []string{"first paragraph", "second paragraph"}},
		{"question and exclamation end a claim", "Почему? Потому. Точно!", []string{"Почему?", "Потому.", "Точно!"}},
		{"single claim without a terminator", "одно утверждение без точки", []string{"одно утверждение без точки"}},
		{"empty", "   ", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := DetailClaims(test.value)
			if len(got) != len(test.want) {
				t.Fatalf("DetailClaims(%q) = %#v, want %#v", test.value, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("DetailClaims(%q)[%d] = %q, want %q", test.value, i, got[i], test.want[i])
				}
			}
		})
	}
}

// TestDetailClaimsNeverLosesOrDuplicatesText guards the property that
// matters more than any single split point: splitting is a presentation
// choice, so every word of the note has to survive it exactly once.
func TestDetailClaimsNeverLosesOrDuplicatesText(t *testing.T) {
	note := "Основное число изменено: 93.4% взяты с vals.ai (ранг #4, продукт совпадает точно). Прошлая версия показывала 76.8%, но источник не отследить; 76.8% сохранено как альтернатива. Расхождение объясняется скаффолдом!"
	claims := DetailClaims(note)
	if len(claims) < 3 {
		t.Fatalf("DetailClaims returned %d claims, want the note split into at least 3: %#v", len(claims), claims)
	}
	want := strings.Join(strings.Fields(strings.ReplaceAll(note, ";", "")), " ")
	got := strings.Join(strings.Fields(strings.Join(claims, " ")), " ")
	if got != want {
		t.Fatalf("splitting changed the text:\n got %q\nwant %q", got, want)
	}
}

// TestDetailLinesRendersFitAndNotesAsLists is the user-visible shape: a
// task-fit list with one item per tag and a note list with one item per
// claim, both under their own heading, none of it a paragraph.
func TestDetailLinesRendersFitAndNotesAsLists(t *testing.T) {
	data := DetailDTO{DisplayName: "Demo", Slug: "demo/model", TaskFit: []string{"implement", "plan", "test"}, Note: "Первое утверждение. Второе утверждение."}
	lines := DetailLines(data, time.Unix(0, 0), "", nil, nil, nil, nil)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "-- Fit and notes --\nTask fit:\n  - implement\n  - plan\n  - test\nNote:\n  - Первое утверждение.\n  - Второе утверждение.") {
		t.Fatalf("fit and notes block is not a list:\n%s", joined)
	}
	if strings.Contains(joined, "implement + plan") {
		t.Fatalf("the joined one-line task fit survived alongside the list:\n%s", joined)
	}
}

// TestDetailLinesKeepsEmptyListsAsBareplaceholders pins the one case that
// is deliberately not a bullet: nothing to list is not a list of one.
func TestDetailLinesKeepsEmptyListsAsBarePlaceholders(t *testing.T) {
	for _, lang := range []string{"", "ru"} {
		lines := DetailLines(DetailDTO{DisplayName: "Demo", Slug: "demo/model"}, time.Unix(0, 0), lang, nil, nil, nil, nil)
		joined := strings.Join(lines, "\n")
		labels := defaultLabels(lang)
		placeholder := labels.Placeholder
		if !strings.Contains(joined, detailHeading(labels.TaskFit)+"\n  "+placeholder) || !strings.Contains(joined, labels.Note+"\n  "+placeholder) {
			t.Fatalf("lang %q: empty lists lost their placeholder:\n%s", lang, joined)
		}
		if strings.Contains(joined, "- "+placeholder) {
			t.Fatalf("lang %q: a placeholder was rendered as a list item:\n%s", lang, joined)
		}
	}
}

// TestDetailWrapsBulletsUnderTheirOwnTextWithinTheViewport covers the
// wrapping rule a list needs and a field row does not: continuation rows
// line up under the item's text, and — the part that would silently lose
// characters if the marker were simply prepended after wrapping — no row
// is wider than the viewport.
func TestDetailWrapsBulletsUnderTheirOwnTextWithinTheViewport(t *testing.T) {
	item := "  " + DetailBulletMarker + strings.Repeat("claim ", 12)
	frame := Detail(DetailData{Width: 24, Height: 12, Lines: []string{"Note:", item}})
	var body []string
	for _, line := range frame.Lines {
		if strings.Contains(line, "claim") {
			body = append(body, line)
		}
	}
	if len(body) < 3 {
		t.Fatalf("long item did not wrap: %#v", frame.Lines)
	}
	if !strings.HasPrefix(body[0], "  - claim") {
		t.Fatalf("first row lost its marker: %q", body[0])
	}
	for i, line := range body {
		if ansi.StringWidth(ansi.Strip(line)) > 24 {
			t.Fatalf("row %d exceeds the viewport: %q", i, line)
		}
		if i > 0 && !strings.HasPrefix(line, "    claim") {
			t.Fatalf("continuation row %d is not aligned under the item text: %q", i, line)
		}
	}
	// A section heading also starts with a dash and must keep being a
	// heading, not an item with a hanging indent.
	heading := Detail(DetailData{Width: 8, Height: 6, Lines: []string{"-- Pricing and more --"}})
	if strings.HasPrefix(heading.Lines[1], "  ") {
		t.Fatalf("a section heading was wrapped as a list item: %#v", heading.Lines)
	}
}
