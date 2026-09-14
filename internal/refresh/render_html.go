package refresh

import (
	_ "embed"
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricing"
)

//go:embed comparison.html.tmpl
var comparisonHTMLTemplate string

var anchorNonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// anchorID turns a slug into a URL/CSS-safe id that never starts with a
// digit: openai/gpt-5.6-luna -> m-openai-gpt-5-6-luna. It is deterministic
// and collision-tested against every tracked slug by
// TestAnchorIDsAreUniqueAcrossTrackedSlugs.
func anchorID(slug string) string {
	s := anchorNonAlnum.ReplaceAllString(strings.ToLower(strings.TrimSpace(slug)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "x"
	}
	return "m-" + s
}

// tierClass is the presentation class for a manual tier: the left-edge
// accent colour a tier/free table row and its tier-section heading carry.
// Anything other than the four known tiers (an unmapped row has no tier at
// all) gets the neutral "tier-none".
func tierClass(tier string) string {
	switch tier {
	case "opus", "sonnet", "haiku", "free":
		return "tier-" + tier
	default:
		return "tier-none"
	}
}

// isTierAnchored reports whether a model with this tier appears in one of
// the four tier/free tables and therefore has an "m-<slug>" row id to link
// to. Cross-links (the Ranked table's model-name link, the appendix's
// back-link) must check this before emitting an href — an unmapped model
// (tier "") has no anchor anywhere in the document, and linking to one
// would be a dangling href that TestRenderHTMLInternalLinksResolve catches.
func isTierAnchored(tier string) bool {
	return tierClass(tier) != "tier-none"
}

// tierRank is the manual tier's sort key for the Ranked table's Claude
// column: the column shows text ("&gt;≈ Opus 5"), but what a reader
// sorting by it actually wants is the tier ladder that text encodes.
func tierRank(tier string) string {
	switch tier {
	case "opus":
		return "3"
	case "sonnet":
		return "2"
	case "haiku":
		return "1"
	case "free":
		return "0"
	default:
		return ""
	}
}

// numVal/ctxVal/optVal format a raw number for a sortable table's
// "data-value" attribute, kept apart from the displayed string (which can
// carry a currency sign, a unit suffix, or provenance text that does not
// compare correctly as a string). numVal/ctxVal always carry a value — a
// free model's $0.00 is a real zero, not "no value". optVal is "" when ok is
// false, so the inline sort script's "no data-value sorts last" rule (see
// comparison.html.tmpl) applies uniformly to every "n/a" cell — the same
// contract docs/reference.md documents for the CLI table's own Q/P column.
func numVal(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func ctxVal(n int) string     { return strconv.Itoa(n) }
func optVal(v float64, ok bool) string {
	if !ok {
		return ""
	}
	return numVal(v)
}

// scoreRefHTML is scoreRefMarkdown's HTML twin: the same "проверка"-level
// provenance pointer (D3's three-level hierarchy), as a real <a> instead of
// a Markdown link — a literal "[vals.ai](url)" would show up as text in an
// HTML cell, which is exactly what TestRenderHTMLIsWellFormed and
// TestE2E_ReportHTML both check never happens. Every dynamic piece is
// escaped explicitly before assembly, since the result is handed to the
// template as pre-trusted template.HTML.
func scoreRefHTML(info *model.ScoreInfo) htmltemplate.HTML {
	if info == nil || info.SourceURL == "" {
		return ""
	}
	name, ok := scoreSourceDisplayNames[info.SourceFamily]
	if !ok {
		name = "источник"
	}
	checked := info.Checked
	if checked == "" {
		checked = "n/a"
	}
	return htmltemplate.HTML(fmt.Sprintf(
		` · <a href="%s" target="_blank" rel="noopener noreferrer">%s</a>, %s`,
		htmltemplate.HTMLEscapeString(info.SourceURL), htmltemplate.HTMLEscapeString(name), htmltemplate.HTMLEscapeString(checked),
	))
}

// inlineHTML renders curated notes.yaml prose for an HTML cell. It escapes
// the whole string first — the one step that makes it impossible for prose
// to inject markup or break out of its cell, regardless of what the second
// step does — then turns paired `backtick`-spans into <code>...</code>,
// notes.yaml's only inline-markup convention (verified by grep: 12
// backtick spans, zero Markdown links, in the live notes.yaml). It is
// total: an odd number of backticks leaves the trailing one as a literal
// character instead of opening a <code> span with no closing tag, so a
// malformed or merely-odd note can never break the page.
func inlineHTML(s string) htmltemplate.HTML {
	escaped := htmltemplate.HTMLEscapeString(s)
	parts := strings.Split(escaped, "`")
	oddTotal := (len(parts)-1)%2 == 1
	var b strings.Builder
	for i, part := range parts {
		isCodeContent := i%2 == 1
		isLast := i == len(parts)-1
		switch {
		case isCodeContent && oddTotal && isLast:
			b.WriteString("`")
			b.WriteString(part)
		case isCodeContent:
			b.WriteString("<code>")
			b.WriteString(part)
			b.WriteString("</code>")
		default:
			b.WriteString(part)
		}
	}
	return htmltemplate.HTML(b.String())
}

var htmlTmpl = htmltemplate.Must(htmltemplate.New("comparison-html").Funcs(htmltemplate.FuncMap{
	"price":      pricing.FormatDollar,
	"ctx":        pricing.FormatContext,
	"tok":        pricing.FormatTokens10,
	"provenance": model.FormatScoreProvenance,
	"marker":     model.ScoreSourceMarker,
	"taskfit":    taskFitLine,
	"claude":     ClaudeEquivalentForSource,
	"scorevalue": scoreValue,
	"anchor":     anchorID,
	"anchored":   isTierAnchored,
	"tierclass":  tierClass,
	"tierrank":   tierRank,
	"numval":     numVal,
	"ctxval":     ctxVal,
	"optval":     optVal,
	"scoreref":   scoreRefHTML,
	"inline":     inlineHTML,
}).Parse(comparisonHTMLTemplate))

// RenderHTML writes the whole document as one self-contained HTML file.
// Like Render it never reads the previous file: the document is a build
// artefact, regenerated from scratch every run. It reuses the exact same
// RenderData Render does — there is no second data pipeline, only a second
// set of pure formatters registered above.
func RenderHTML(w io.Writer, data RenderData) error {
	if err := htmlTmpl.Execute(w, data); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	return nil
}
