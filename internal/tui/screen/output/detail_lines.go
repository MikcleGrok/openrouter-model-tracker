package output

import (
	"fmt"
	"strings"
	"time"
)

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
type DetailHistoryProvider interface {
	Lines(slug, lang string) []string
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
	History                                                                      []string
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
func DetailLines(data DetailDTO, now time.Time, lang string, localizer DetailLocalizer, history DetailHistoryProvider, icons DetailIconProvider, prices DetailPriceProvider, scores DetailScoreProvider) []string {
	if localizer == nil {
		localizer = detailDefaults{}
	}
	if history == nil {
		history = detailDefaults{}
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
	lines = append(lines, l.Provider+value(data.Provider), l.License+value(data.License), l.Tier+value(data.Tier), l.ClaudeReference+value(data.ClaudeRef), l.TaskFit+taskFit(data.TaskFit, l.Placeholder))
	lines = append(lines, "", l.Pricing, l.Context+context+l.Tokens, l.Input+prices.Price(data.InPerM)+l.PerMTokens, l.Output+prices.Price(data.OutPerM)+l.PerMTokens)
	if combined, input, output := prices.LongContext(data, lang); combined != "" {
		lines = append(lines, l.LongContext+combined, l.LongContextInput+input, l.LongContextOutput+output)
	}
	if historyLines := append([]string(nil), data.History...); len(historyLines) > 0 {
		lines = append(lines, historyLines...)
	} else if generated := history.Lines(data.Slug, lang); len(generated) > 0 {
		lines = append(lines, generated...)
	}
	lines = append(lines, l.OpenWeights+value(data.OpenWeights), "", l.Benchmarks)
	lines = append(lines, scores.SWEBench(data, lang)...)
	lines = append(lines, "")
	lines = append(lines, scores.Arena(data, lang)...)
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
	lines = append(lines, "", l.FitNotes, l.Note)
	lines = append(lines, detailProse(data.Note, l.Placeholder))
	return lines
}

func detailProse(value, placeholder string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "  " + placeholder
	}
	return "  " + value
}
func taskFit(values []string, placeholder string) string {
	if len(values) == 0 {
		return placeholder
	}
	return strings.Join(values, " + ")
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
