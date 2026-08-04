// Package notes holds every piece of hand-written prose the comparison document
// contains: per-model commentary, the "why this is the favourite" text, the FLI
// company table, the static Claude reference prices and the caveat bullets. The
// generated markdown is a build artefact, so prose edited there would not
// survive the next run — it lives here instead.
package notes

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// NeedsReview is what the renderer prints where a prose key is missing. The
// same slug also lands in the run report.
const NeedsReview = "_нужен обзор_"

// defaultNoScoreReason fills the Качество/цена column for a row that has no
// rankable SWE-bench Verified number.
const defaultNoScoreReason = "н/д (нет оценки по SWE-bench Verified)"

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
	return &Notes{f: f}, nil
}

func (n *Notes) model(slug string) modelNote { return n.f.Models[slug] }

func orNeedsReview(s string) string {
	if s == "" {
		return NeedsReview
	}
	return s
}

// ModelNote returns the per-model commentary shown in the Примечание column.
func (n *Notes) ModelNote(slug string) string { return orNeedsReview(n.model(slug).Note) }

// DisplayName returns the human-facing model name, falling back to the slug so
// a brand-new row still renders.
func (n *Notes) DisplayName(slug string) string {
	if d := n.model(slug).Display; d != "" {
		return d
	}
	return slug
}

// Owner returns the "Владелец (FLI)" cell.
func (n *Notes) Owner(slug string) string { return orNeedsReview(n.model(slug).Owner) }

// OpenWeights returns the "Открытые веса" cell.
func (n *Notes) OpenWeights(slug string) string { return orNeedsReview(n.model(slug).OpenWeights) }

// ClaudeRef returns the "Ориентир по Claude" cell of the free-models table.
func (n *Notes) ClaudeRef(slug string) string { return orNeedsReview(n.model(slug).ClaudeRef) }

// NoScoreReason returns what to print in Качество/цена when the row has no
// rankable number.
func (n *Notes) NoScoreReason(slug string) string {
	if r := n.model(slug).NoScoreReason; r != "" {
		return r
	}
	return defaultNoScoreReason
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
func (n *Notes) UpdatedNote() string { return n.f.UpdatedNote }

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
