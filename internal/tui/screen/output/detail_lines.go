package output

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// DetailBulletMarker is the detail screen's list marker. It is the same
// "- " the help document already uses for its own lists, so a bulleted
// detail block and a bulleted help block read identically; JustifyLines
// and wrapDetailLine both key their hanging indent off exactly this
// prefix.
const DetailBulletMarker = "- "

// detailBulletIndent is the two-column indent every value under a
// "Label:" heading already carries on this screen (see detailProse). A
// bullet keeps it so a list reads as the heading's body, not as a new
// field.
const detailBulletIndent = "  "

// DetailLabels contains the localized chrome of the detail screen. Values in
// DetailDTO are model data; labels are the only text selected by language.
type DetailLabels struct {
	Identity, Pricing, Benchmarks, Provenance, FitNotes                                                 string
	Manufacturer, Provider, License, Tier, ClaudeReference, TaskFit                                     string
	Context, Input, Output, LongContext, LongContextInput, LongContextOutput                            string
	OpenWeights, ReleaseDate, OpenRouterPage, ModelPage, MetadataSource, HuggingFace, Description, Note string
	SWE, Arena, Detail, Scroll, Close, Tokens, PerMTokens                                               string
	Placeholder, ArenaNote, ReleaseNote                                                                 string
}

type DetailLocalizer interface {
	Labels(lang string) DetailLabels
}
type DetailIconProvider interface{ Manufacturer(data DetailDTO) string }
type DetailPriceProvider interface {
	Context(tokens int) string
	Price(value float64) string
	LongContext(data DetailDTO, lang string) (combined, input, output string)
}
type DetailScoreProvider interface {
	SWEBench(data DetailDTO, lang string) []string
	Arena(data DetailDTO, lang string) []string
}

// DetailDTO is the model-independent input to the detail logical-line
// builder. It intentionally contains no application model types.
type DetailDTO struct {
	DisplayName, Slug, Provider, License, Tier, ClaudeRef, OpenWeights           string
	Description, Note, CanonicalSlug, HuggingFaceID, MetadataSourceURL, ModelURL string
	Manufacturer                                                                 string
	Context                                                                      int
	InPerM, OutPerM                                                              float64
	Created                                                                      int64
	TaskFit                                                                      []string
	LongContextPriceLabel, LongContextInLabel, LongContextOutLabel               string
	HasLongContextOverride                                                       bool
	LongContextOverrideInPerM, LongContextOverrideOutPerM                        float64
	LongContextOverrideMinTokens                                                 int
	SWEBlock, ArenaBlock                                                         []string
	PriceHistory, ScoreHistory                                                   []string
}

type detailDefaults struct{}

func (detailDefaults) Labels(lang string) DetailLabels    { return defaultLabels(lang) }
func (detailDefaults) Lines(string, string) []string      { return nil }
func (detailDefaults) Manufacturer(data DetailDTO) string { return data.Manufacturer }
func (detailDefaults) Context(tokens int) string          { return formatContext(tokens) }
func (detailDefaults) Price(value float64) string         { return formatPrice(value) }
func (detailDefaults) LongContext(data DetailDTO, lang string) (string, string, string) {
	if !data.HasLongContextOverride {
		return "", "", ""
	}
	prep := "from"
	if lang == "ru" {
		prep = "от"
	}
	threshold := formatContext(data.LongContextOverrideMinTokens)
	combined := fmt.Sprintf("%s / %s %s %s+", formatPrice(data.LongContextOverrideInPerM), formatPrice(data.LongContextOverrideOutPerM), prep, threshold)
	return combined, fmt.Sprintf("%s %s %s+", formatPrice(data.LongContextOverrideInPerM), prep, threshold), fmt.Sprintf("%s %s %s+", formatPrice(data.LongContextOverrideOutPerM), prep, threshold)
}
func (detailDefaults) SWEBench(data DetailDTO, lang string) []string {
	return append([]string(nil), data.SWEBlock...)
}
func (detailDefaults) Arena(data DetailDTO, lang string) []string {
	return append([]string(nil), data.ArenaBlock...)
}

// DetailLines builds semantic rows. Detail is the only function that turns these rows into physical rows.
func DetailLines(data DetailDTO, now time.Time, lang string, localizer DetailLocalizer, icons DetailIconProvider, prices DetailPriceProvider, scores DetailScoreProvider) []string {
	if localizer == nil {
		localizer = detailDefaults{}
	}
	if icons == nil {
		icons = detailDefaults{}
	}
	if prices == nil {
		prices = detailDefaults{}
	}
	if scores == nil {
		scores = detailDefaults{}
	}
	l := localizer.Labels(lang)
	value := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return l.Placeholder
		}
		return s
	}
	context := l.Placeholder
	if data.Context > 0 {
		context = prices.Context(data.Context)
	}
	lines := []string{value(data.DisplayName) + " (" + value(data.Slug) + ")"}
	lines = append(lines, "", l.Identity)
	manufacturer := l.Manufacturer + value(icons.Manufacturer(data))
	lines = append(lines, manufacturer)
	lines = append(lines, l.Provider+value(data.Provider), l.License+value(data.License), l.Tier+value(data.Tier), l.ClaudeReference+value(data.ClaudeRef))
	lines = append(lines, "", l.Pricing, l.Context+context+l.Tokens, l.Input+prices.Price(data.InPerM)+l.PerMTokens, l.Output+prices.Price(data.OutPerM)+l.PerMTokens)
	if combined, input, output := prices.LongContext(data, lang); combined != "" {
		lines = append(lines, l.LongContext+combined, l.LongContextInput+input, l.LongContextOutput+output)
	}
	lines = append(lines, data.PriceHistory...)
	lines = append(lines, l.OpenWeights+value(data.OpenWeights), "", l.Benchmarks)
	lines = append(lines, scores.SWEBench(data, lang)...)
	lines = append(lines, "")
	lines = append(lines, scores.Arena(data, lang)...)
	lines = append(lines, data.ScoreHistory...)
	lines = append(lines, "", l.Provenance, l.ReleaseDate+releaseDate(data.Created, now, l), l.OpenRouterPage+"https://openrouter.ai/"+value(data.CanonicalSlug))
	if strings.TrimSpace(data.ModelURL) != "" {
		lines = append(lines, l.ModelPage+value(data.ModelURL))
	}
	if strings.TrimSpace(data.MetadataSourceURL) != "" {
		lines = append(lines, l.MetadataSource+value(data.MetadataSourceURL))
	}
	if strings.TrimSpace(data.HuggingFaceID) != "" {
		lines = append(lines, l.HuggingFace+"https://huggingface.co/"+value(data.HuggingFaceID))
	}
	lines = append(lines, l.Description, detailProse(data.Description, l.Placeholder))
	lines = append(lines, "", l.FitNotes, detailHeading(l.TaskFit))
	lines = append(lines, detailBullets(taskFitBullets(data.TaskFit, lang), l.Placeholder)...)
	lines = append(lines, l.Note)
	lines = append(lines, detailBullets(DetailClaims(data.Note), l.Placeholder)...)
	return lines
}

func detailProse(value, placeholder string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return detailBulletIndent + placeholder
	}
	return detailBulletIndent + value
}

// detailHeading turns a field label ("Task fit: ") into the block-heading
// form ("Task fit:") the Description and Note blocks already use, so a
// bulleted block needs no second label in DetailLabels and cannot drift
// out of EN/RU parity with its own inline label.
func detailHeading(label string) string { return strings.TrimRight(label, " ") }

// detailBullets renders one list item per entry, indented under its
// heading. An empty list stays a plain indented placeholder rather than a
// bullet: a placeholder is the absence of items, not an item, and keeping
// it unbulleted is also what lets the caller's styling recognise it.
func detailBullets(items []string, placeholder string) []string {
	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			cleaned = append(cleaned, item)
		}
	}
	if len(cleaned) == 0 {
		return []string{detailBulletIndent + placeholder}
	}
	lines := make([]string, 0, len(cleaned))
	for _, item := range cleaned {
		lines = append(lines, detailBulletIndent+DetailBulletMarker+item)
	}
	return lines
}

// taskFitGloss is one keyword's one-line explanation, in both languages.
type taskFitGloss struct{ en, ru string }

// taskFitGlosses gives each task-fit keyword the same one-line explanation
// already authoritative for this taxonomy: the Hotkeys help section's
// "Task-fit codes" table (tuiHelpSectionHotkeysBody / …BodyRU in
// cmd/openrouter/tui.go). Reused verbatim rather than reworded, so the
// Fit & Notes tab and the help document never carry two competing
// definitions of the same keyword; keep the two in sync by hand if either
// changes. The keyword set matches taskFitOrder/taskFitKnown in
// internal/notes/notes.go.
var taskFitGlosses = map[string]taskFitGloss{
	"implement": {"write or change production code.", "написать или изменить продакшен-код."},
	"plan":      {"define scope, steps, and decisions.", "определить объём, шаги и решения."},
	"research":  {"investigate options, evidence, or behavior.", "исследовать варианты, свидетельства или поведение."},
	"debug":     {"find and fix a defect or failure.", "найти и исправить дефект или сбой."},
	"audit":     {"inspect quality, safety, or compliance.", "проверить качество, безопасность или соответствие требованиям."},
	"refactor":  {"improve structure without changing behavior.", "улучшить структуру без изменения поведения."},
	"test":      {"add or improve automated verification.", "добавить или улучшить автоматизированную проверку."},
}

// taskFitBullets pairs each task-fit keyword with its one-line gloss as
// "keyword: gloss." — the same "code: gloss." shape the help document's
// Task-fit codes table already uses for the same pairing. The keyword
// itself is never translated (see tuiDetailTaskFitForLang in
// cmd/openrouter/tui.go); only the gloss that follows it varies with lang.
// A keyword with no known gloss is passed through bare rather than
// dropped — normalizeTaskFit already restricts notes.yaml to the known
// set, so this only guards test fixtures and any future keyword added
// there before its gloss is.
func taskFitBullets(keywords []string, lang string) []string {
	items := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		gloss, ok := taskFitGlosses[keyword]
		if !ok {
			items = append(items, keyword)
			continue
		}
		text := gloss.en
		if lang == "ru" {
			text = gloss.ru
		}
		items = append(items, keyword+": "+text)
	}
	return items
}

// DetailClaims splits note prose into the separate claims it is actually
// made of, so each one can be rendered as its own list item instead of
// disappearing into a paragraph.
//
// A split happens only where a terminator (. ! ? ;) is followed by
// whitespace and then by something that can start a new claim — an
// uppercase letter, a digit, or an opening quote/bracket. That triple
// condition is what keeps the shapes this data really contains
// intact: decimals ("93.4%") and domains ("vals.ai") have no space after
// the dot, and lowercase abbreviations ("см. arXiv:2511.03929", "т.д.
// дальше") continue in lower case. Explicit line breaks, including the
// escaped "\n" this screen's inputs carry, always separate claims.
func DetailClaims(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var claims []string
	for _, paragraph := range splitEscapedLines(value) {
		claims = append(claims, detailParagraphClaims(paragraph)...)
	}
	return claims
}

func detailParagraphClaims(paragraph string) []string {
	runes := []rune(strings.TrimSpace(paragraph))
	if len(runes) == 0 {
		return nil
	}
	var claims []string
	start := 0
	for i := 0; i < len(runes); i++ {
		if !detailClaimTerminator(runes[i]) {
			continue
		}
		end := i + 1
		for end < len(runes) && detailClaimCloser(runes[end]) {
			end++
		}
		if end >= len(runes) || !unicode.IsSpace(runes[end]) {
			continue
		}
		next := end
		for next < len(runes) && unicode.IsSpace(runes[next]) {
			next++
		}
		if next >= len(runes) || !detailClaimOpener(runes[next]) {
			continue
		}
		if claim := detailTrimClaim(string(runes[start:end])); claim != "" {
			claims = append(claims, claim)
		}
		start, i = next, next-1
	}
	if tail := detailTrimClaim(string(runes[start:])); tail != "" {
		claims = append(claims, tail)
	}
	return claims
}

// detailTrimClaim drops a trailing semicolon. It is punctuation that
// joined this claim to the next one, and once the two are separate items
// it has nothing left to join. A full stop is left alone: it ends a
// sentence whether or not the sentence is a list item.
func detailTrimClaim(claim string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(claim), ";"))
}

func detailClaimTerminator(r rune) bool { return r == '.' || r == '!' || r == '?' || r == ';' }
func detailClaimCloser(r rune) bool {
	return r == '"' || r == '\'' || r == ')' || r == ']' || r == '»' || r == '”' || r == '*'
}
func detailClaimOpener(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsDigit(r) || r == '«' || r == '"' || r == '“' || r == '('
}
func releaseDate(created int64, now time.Time, labels DetailLabels) string {
	if created <= 0 {
		return labels.Placeholder
	}
	published := time.Unix(created, 0).UTC()
	days := int(now.UTC().Sub(published).Hours() / 24)
	age := "future date"
	if labels.Placeholder == "н/д" {
		age = "дата в будущем"
	}
	if days == 0 {
		age = "today"
		if labels.Placeholder == "н/д" {
			age = "сегодня"
		}
	} else if days > 0 {
		age = englishAge(days)
		if labels.Placeholder == "н/д" {
			age = russianAge(days)
		}
	}
	return published.Format("2006-01-02") + " (" + age + ")" + labels.ReleaseNote
}
func englishAge(days int) string {
	if days < 31 {
		return fmt.Sprintf("%d %s ago", days, pluralEN(days, "day"))
	}
	if days < 365 {
		return fmt.Sprintf("%d %s ago", days/30, pluralEN(days/30, "month"))
	}
	return fmt.Sprintf("%d %s ago", days/365, pluralEN(days/365, "year"))
}
func russianAge(days int) string {
	if days < 31 {
		return pluralRU(days, "день", "дня", "дней") + " назад"
	}
	if days < 365 {
		return pluralRU(days/30, "месяц", "месяца", "месяцев") + " назад"
	}
	return pluralRU(days/365, "год", "года", "лет") + " назад"
}
func pluralEN(value int, unit string) string {
	if value == 1 {
		return unit
	}
	return unit + "s"
}
func pluralRU(value int, one, few, many string) string {
	form := many
	if value%100 < 11 || value%100 > 14 {
		if value%10 == 1 {
			form = one
		} else if value%10 >= 2 && value%10 <= 4 {
			form = few
		}
	}
	return fmt.Sprintf("%d %s", value, form)
}
func defaultLabels(lang string) DetailLabels {
	if lang == "ru" {
		return DetailLabels{Identity: "-- Идентичность --", Pricing: "-- Цены --", Benchmarks: "-- Бенчмарки --", Provenance: "-- Происхождение и метаданные --", FitNotes: "-- Соответствие и заметки --", Manufacturer: "Производитель: ", Provider: "Провайдер: ", License: "Лицензия: ", Tier: "Тир: ", ClaudeReference: "Claude-референс: ", TaskFit: "Task fit: ", Context: "Контекст: ", Input: "Вход: ", Output: "Выход: ", LongContext: "Длинный контекст: ", LongContextInput: "  вход: ", LongContextOutput: "  выход: ", OpenWeights: "Открытые веса: ", ReleaseDate: "Дата релиза: ", OpenRouterPage: "Страница OpenRouter: ", ModelPage: "Страница модели: ", MetadataSource: "Источник метаданных: ", HuggingFace: "Репозиторий HuggingFace: ", Description: "Описание:", Note: "Заметка:", Tokens: " токенов", PerMTokens: " за M токенов", Placeholder: "н/д", ReleaseNote: "; дата создания записи каталога, релиз неизвестен"}
	}
	return DetailLabels{Identity: "-- Identity --", Pricing: "-- Pricing --", Benchmarks: "-- Benchmarks --", Provenance: "-- Provenance and metadata --", FitNotes: "-- Fit and notes --", Manufacturer: "Manufacturer: ", Provider: "Provider: ", License: "License: ", Tier: "Tier: ", ClaudeReference: "Claude reference: ", TaskFit: "Task fit: ", Context: "Context: ", Input: "Input: ", Output: "Output: ", LongContext: "Long context: ", LongContextInput: "  input: ", LongContextOutput: "  output: ", OpenWeights: "Open weights: ", ReleaseDate: "Release date: ", OpenRouterPage: "OpenRouter page: ", ModelPage: "Model page: ", MetadataSource: "Metadata source: ", HuggingFace: "HuggingFace repository: ", Description: "Description:", Note: "Note:", Tokens: " tokens", PerMTokens: " per M tokens", Placeholder: "n/a", ReleaseNote: "; catalogue entry creation date, release date unknown"}
}
func formatContext(tokens int) string  { return fmt.Sprintf("%dK", tokens/1000) }
func formatPrice(value float64) string { return fmt.Sprintf("$%.4g", value) }
