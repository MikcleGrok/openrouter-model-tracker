// Package notes holds every piece of hand-written prose the comparison document
// contains: per-model commentary, the "why this is the favourite" text, the FLI
// company table, the static Claude reference prices and the caveat bullets. The
// generated markdown is a build artefact, so prose edited there would not
// survive the next run — it lives here instead.
package notes

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var taskFitOrder = []string{"implement", "plan", "research", "debug", "audit", "refactor", "test"}

var taskFitKnown = map[string]bool{
	"implement": true, "plan": true, "research": true, "debug": true,
	"audit": true, "refactor": true, "test": true,
}

// NeedsReview is what the renderer prints where a prose key is missing. The
// same slug also lands in the run report.
const NeedsReview = "_нужен обзор_"

const (
	CopyrightCompliant    = "compliant"
	CopyrightNonCompliant = "non_compliant"
	CopyrightUnknown      = "unknown"
)

// defaultNoScoreReason fills the Качество/цена column for a row that has no
// rankable SWE-bench Verified number.
const defaultNoScoreReason = "n/a (no SWE-bench Verified score)"

// ScoreOverride is a manually entered benchmark number for a model no automated
// source covers — typically a vendor-published figure.
type ScoreOverride struct {
	Label    string  `yaml:"label"`
	Value    float64 `yaml:"value"`
	Rankable bool    `yaml:"rankable"`
	Source   string  `yaml:"source"`
}

// ClaudePrice is one row of the static "Цены Claude (справочно)" table.
type ClaudePrice struct {
	Model   string `yaml:"model"`
	In      string `yaml:"in"`
	Out     string `yaml:"out"`
	Context string `yaml:"context"`
	Note    string `yaml:"note"`
}

// ClaudeTokens is one row of the static Claude block of the $10 table.
type ClaudeTokens struct {
	Model string `yaml:"model"`
	In    string `yaml:"in"`
	Out   string `yaml:"out"`
	Mixed string `yaml:"mixed"`
}

// Company is one row of the FLI safety-grade table.
type Company struct {
	Name    string `yaml:"name"`
	Grade   string `yaml:"grade"`
	Comment string `yaml:"comment"`
}

type modelNote struct {
	Display       string         `yaml:"display"`
	Owner         string         `yaml:"owner"`
	OpenWeights   string         `yaml:"open_weights"`
	ClaudeRef     string         `yaml:"claude_ref"`
	Note          string         `yaml:"note"`
	NoScoreReason string         `yaml:"no_score_reason"`
	Score         *ScoreOverride `yaml:"score"`
	TaskFit       []string       `yaml:"task_fit"`
	Copyright     string         `yaml:"copyright"`
}

type file struct {
	UpdatedNote     string                       `yaml:"updated_note"`
	FableVerdict    string                       `yaml:"fable_verdict"`
	ClaudeNote      string                       `yaml:"claude_note"`
	Sections        map[string]string            `yaml:"sections"`
	ClaudePrices    []ClaudePrice                `yaml:"claude_prices"`
	ClaudeTokens    []ClaudeTokens               `yaml:"claude_tokens"`
	Companies       []Company                    `yaml:"companies"`
	Caveats         []string                     `yaml:"caveats"`
	FavoriteReasons map[string]map[string]string `yaml:"favorite_reasons"`
	Models          map[string]modelNote         `yaml:"models"`
	TaskFit         map[string][]string          `yaml:"task_fit"`
}

// Notes is the parsed notes.yaml.
type Notes struct {
	f file
}

// Load reads notes.yaml. A missing file is an error: this is project data, not
// an optional setting.
func Load(path string) (*Notes, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("notes: %w", err)
	}
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("notes: %s: %w", path, err)
	}
	for slug, m := range f.Models {
		m.Display = normalizeMissingLabels(m.Display)
		m.Owner = normalizeMissingLabels(m.Owner)
		m.OpenWeights = normalizeMissingLabels(m.OpenWeights)
		m.ClaudeRef = normalizeMissingLabels(m.ClaudeRef)
		m.Note = normalizeMissingLabels(m.Note)
		m.NoScoreReason = normalizeMissingLabels(m.NoScoreReason)
		m.Copyright, err = normalizeCopyright(m.Copyright)
		if err != nil {
			return nil, fmt.Errorf("notes: models.%s.copyright: %w", slug, err)
		}
		if m.Score != nil {
			m.Score.Label = normalizeMissingLabels(m.Score.Label)
			m.Score.Source = normalizeMissingLabels(m.Score.Source)
		}
		values := m.TaskFit
		if values == nil {
			values = f.TaskFit[slug]
		}
		fit, err := normalizeTaskFit(values)
		if err != nil {
			return nil, fmt.Errorf("notes: models.%s.task_fit: %w", slug, err)
		}
		m.TaskFit = fit
		f.Models[slug] = m
	}
	for slug, values := range f.TaskFit {
		fit, err := normalizeTaskFit(values)
		if err != nil {
			return nil, fmt.Errorf("notes: task_fit.%s: %w", slug, err)
		}
		f.TaskFit[slug] = fit
	}
	f.UpdatedNote = normalizeMissingLabels(f.UpdatedNote)
	f.FableVerdict = normalizeMissingLabels(f.FableVerdict)
	f.ClaudeNote = normalizeMissingLabels(f.ClaudeNote)
	for i := range f.ClaudePrices {
		f.ClaudePrices[i].Model = normalizeMissingLabels(f.ClaudePrices[i].Model)
		f.ClaudePrices[i].In = normalizeMissingLabels(f.ClaudePrices[i].In)
		f.ClaudePrices[i].Out = normalizeMissingLabels(f.ClaudePrices[i].Out)
		f.ClaudePrices[i].Context = normalizeMissingLabels(f.ClaudePrices[i].Context)
		f.ClaudePrices[i].Note = normalizeMissingLabels(f.ClaudePrices[i].Note)
	}
	for i := range f.ClaudeTokens {
		f.ClaudeTokens[i].Model = normalizeMissingLabels(f.ClaudeTokens[i].Model)
		f.ClaudeTokens[i].In = normalizeMissingLabels(f.ClaudeTokens[i].In)
		f.ClaudeTokens[i].Out = normalizeMissingLabels(f.ClaudeTokens[i].Out)
		f.ClaudeTokens[i].Mixed = normalizeMissingLabels(f.ClaudeTokens[i].Mixed)
	}
	for i := range f.Companies {
		f.Companies[i].Name = normalizeMissingLabels(f.Companies[i].Name)
		f.Companies[i].Grade = normalizeMissingLabels(f.Companies[i].Grade)
		f.Companies[i].Comment = normalizeMissingLabels(f.Companies[i].Comment)
	}
	for key, value := range f.Sections {
		f.Sections[key] = normalizeMissingLabels(value)
	}
	for key, value := range f.FavoriteReasons {
		for slug, reason := range value {
			value[slug] = normalizeMissingLabels(reason)
		}
		f.FavoriteReasons[key] = value
	}
	for i := range f.Caveats {
		f.Caveats[i] = normalizeMissingLabels(f.Caveats[i])
	}
	return &Notes{f: f}, nil
}

func normalizeMissingLabels(value string) string {
	for _, legacy := range []string{"н/д", "Н/Д", "Н/д", "н/Д"} {
		value = strings.ReplaceAll(value, legacy, "n/a")
	}
	switch value {
	case "n/a (оценка не для этого варианта)":
		return "n/a (variant mismatch)"
	case "n/a (нет оценки по SWE-bench Verified)":
		return defaultNoScoreReason
	}
	return value
}

func normalizeTaskFit(values []string) ([]string, error) {
	present := make(map[string]bool, len(values))
	for _, value := range values {
		if !taskFitKnown[value] {
			return nil, fmt.Errorf("unknown keyword %q; allowed values: %s", value, joinTaskFitKeywords())
		}
		present[value] = true
	}
	result := make([]string, 0, len(present))
	for _, value := range taskFitOrder {
		if present[value] {
			result = append(result, value)
		}
	}
	return result, nil
}

func normalizeCopyright(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return CopyrightUnknown, nil
	}
	switch value {
	case CopyrightCompliant, CopyrightNonCompliant, CopyrightUnknown:
		return value, nil
	default:
		return "", fmt.Errorf("unknown value %q; allowed values: %s, %s, %s", value, CopyrightCompliant, CopyrightNonCompliant, CopyrightUnknown)
	}
}

func joinTaskFitKeywords() string { return "implement, plan, research, debug, audit, refactor, test" }

func (n *Notes) model(slug string) modelNote { return n.f.Models[slug] }

func orNeedsReview(s string) string {
	if s == "" {
		return NeedsReview
	}
	return s
}

func isDisplayPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == strings.ToLower(NeedsReview) || normalized == "n/a" || normalized == "n/d" || normalized == "н/д" {
		return true
	}
	return strings.HasPrefix(normalized, "n/a (") || strings.HasPrefix(normalized, "n/d (") || strings.HasPrefix(normalized, "н/д (") || strings.HasPrefix(normalized, strings.ToLower(NeedsReview)+" ") || strings.HasPrefix(normalized, strings.ToLower(NeedsReview)+"(")
}

// ModelNote returns the per-model commentary shown in the Примечание column.
func (n *Notes) ModelNote(slug string) string { return orNeedsReview(n.model(slug).Note) }

// TaskFit returns normalized manual task-fit keywords for a model.
func (n *Notes) TaskFit(slug string) []string {
	values := n.model(slug).TaskFit
	if values == nil {
		values = n.f.TaskFit[slug]
	}
	return append([]string(nil), values...)
}

// DisplayName returns the human-facing model name, falling back to the slug so
// a brand-new row still renders.
func (n *Notes) DisplayName(slug string) string {
	if d := n.model(slug).Display; d != "" && !isDisplayPlaceholder(d) {
		return d
	}
	return slug
}

// Owner returns the "Владелец (FLI)" cell.
func (n *Notes) Owner(slug string) string { return orNeedsReview(n.model(slug).Owner) }

// OpenWeights returns the "Открытые веса" cell.
func (n *Notes) OpenWeights(slug string) string { return orNeedsReview(n.model(slug).OpenWeights) }

// Copyright returns the manual copyright classification from notes.yaml.
// Missing values intentionally mean unknown and never use the catalogue license.
func (n *Notes) Copyright(slug string) string {
	value, err := normalizeCopyright(n.model(slug).Copyright)
	if err != nil {
		return CopyrightUnknown
	}
	return value
}

// ClaudeRef returns the "Ориентир по Claude" cell of the free-models table.
func (n *Notes) ClaudeRef(slug string) string { return orNeedsReview(n.model(slug).ClaudeRef) }

// NoScoreReason returns what to print in Качество/цена when the row has no
// rankable number.
func (n *Notes) NoScoreReason(slug string) string {
	if r := n.model(slug).NoScoreReason; r != "" {
		return englishNoScoreReason(r)
	}
	return defaultNoScoreReason
}

func englishNoScoreReason(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "н/д (оценка не для этого варианта)":
		return "n/a (variant mismatch)"
	case "н/д (нет оценки по SWE-bench Verified)":
		return defaultNoScoreReason
	default:
		return strings.Replace(value, "н/д", "n/a", 1)
	}
}

// ScoreOverride returns a manually entered benchmark number, if any.
func (n *Notes) ScoreOverride(slug string) (ScoreOverride, bool) {
	s := n.model(slug).Score
	if s == nil {
		return ScoreOverride{}, false
	}
	return *s, true
}

// FavoriteReason returns the "Почему фаворит" text for a slug within a tier.
func (n *Notes) FavoriteReason(tier, slug string) string {
	return orNeedsReview(n.f.FavoriteReasons[tier][slug])
}

// Section returns a static prose block by id.
func (n *Notes) Section(id string) string { return orNeedsReview(n.f.Sections[id]) }

// UpdatedNote is the parenthetical after the date on the "Обновлено:" line.
func (n *Notes) UpdatedNote() string { return orNeedsReview(n.f.UpdatedNote) }

// FableVerdict is the text of the "≈ Fable 5" favourites row, which has no model.
func (n *Notes) FableVerdict() string { return orNeedsReview(n.f.FableVerdict) }

// ClaudeNote is the paragraph under the static Claude price table.
func (n *Notes) ClaudeNote() string { return orNeedsReview(n.f.ClaudeNote) }

// ClaudePrices returns the static Claude reference price rows.
func (n *Notes) ClaudePrices() []ClaudePrice { return n.f.ClaudePrices }

// ClaudeTokens returns the static Claude rows of the $10 table.
func (n *Notes) ClaudeTokens() []ClaudeTokens { return n.f.ClaudeTokens }

// Companies returns the FLI safety-grade rows.
func (n *Notes) Companies() []Company { return n.f.Companies }

// Caveats returns the bullets of "На что обратить внимание".
func (n *Notes) Caveats() []string { return n.f.Caveats }
