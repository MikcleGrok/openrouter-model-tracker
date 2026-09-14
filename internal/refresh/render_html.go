package refresh

import (
	_ "embed"
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
// to inject markup or break out of its cell, regardless of what the later
// steps do — then converts notes.yaml's three inline-markup conventions on
// the escaped text: paired `backtick`-spans into <code>...</code>, paired
// **double-asterisk**-spans into <strong>...</strong>, and paired
// _underscore_-spans into <em>...</em> (the shape of internal/notes.NeedsReview,
// "_нужен обзор_", and of general Markdown-ish emphasis notes.yaml could grow
// more of). It is total: no input can ever leave a dangling, unclosed tag —
// an unmatched backtick, "**", or "_" always falls back to the literal
// character(s) instead, the same guarantee the original backtick-only
// version had (see splitPaired).
func inlineHTML(s string) htmltemplate.HTML {
	escaped := htmltemplate.HTMLEscapeString(s)
	var b strings.Builder
	for _, span := range splitPaired(escaped, "`") {
		if span.content {
			// A code span's content is never further interpreted as
			// bold/italic markup, matching how a real Markdown parser
			// treats code spans: `code with _underscore_` must render
			// with a literal underscore, not grow an <em>.
			b.WriteString("<code>")
			b.WriteString(span.text)
			b.WriteString("</code>")
			continue
		}
		b.WriteString(boldItalicHTML(span.text))
	}
	return htmltemplate.HTML(b.String())
}

// pairedSpan is one piece of a string split around a repeated delimiter, as
// produced by splitPaired: content pieces sat between a matched pair of
// delimiters and are meant to be wrapped in a tag; the rest is plain
// surrounding text (which may itself carry a delimiter character that never
// found a partner, folded back in literally).
type pairedSpan struct {
	text    string
	content bool
}

// splitPaired splits s on every occurrence of the literal delimiter delim
// and classifies each resulting piece as content (wrap it) or plain text.
// It is total, generalizing inlineHTML's original backtick-only algorithm:
// with an odd number of delimiter occurrences the final one can never close
// anything, so the trailing piece is folded back together with a literal
// copy of that delimiter instead of being reported as unclosed content —
// callers never see a span they'd have to special-case as unmatched.
func splitPaired(s, delim string) []pairedSpan {
	raw := strings.Split(s, delim)
	if len(raw) == 1 {
		return []pairedSpan{{text: s}}
	}
	oddTotal := (len(raw)-1)%2 == 1
	spans := make([]pairedSpan, 0, len(raw))
	for i, part := range raw {
		isContent := i%2 == 1
		isLast := i == len(raw)-1
		if isContent && oddTotal && isLast {
			spans = append(spans, pairedSpan{text: delim + part})
			continue
		}
		spans = append(spans, pairedSpan{text: part, content: isContent})
	}
	return spans
}

// boldItalicHTML converts **bold** and _italic_ spans in text already known
// to contain no backtick code spans (inlineHTML carves those out first and
// never calls back into this function with their content). Bold is
// resolved before italic, and italic is then run separately over every
// resulting piece — both the text that lands inside <strong> and the plain
// text around it — so "**bold _and italic_ text**" nests an <em> inside the
// <strong> instead of the "**" and "_" delimiters fighting over the same
// span.
func boldItalicHTML(s string) string {
	var b strings.Builder
	for _, span := range splitPaired(s, "**") {
		if span.content {
			b.WriteString("<strong>")
			b.WriteString(italicHTML(span.text))
			b.WriteString("</strong>")
			continue
		}
		b.WriteString(italicHTML(span.text))
	}
	return b.String()
}

// italicHTML converts _italic_ spans, with the one refinement that makes it
// safe on real prose: an underscore immediately touching a letter or digit
// on its inside edge never opens or closes a span (isItalicOpener/
// isItalicCloser below). Without that guard, a snake_case identifier
// sitting unquoted in notes.yaml prose — canonical_slug, hugging_face_id,
// and reasoning_effort all appear there today, in the z-ai/glm-5.2:free
// note — would have its middle segment wrongly wrapped in <em>, e.g.
// "hugging<em>face</em>id". notes.yaml's one real italic construct,
// "_нужен обзор_" (internal/notes.NeedsReview), is unaffected: its opening
// "_" is preceded by nothing/whitespace and its closing "_" is followed by
// nothing/whitespace/punctuation, never by a letter or digit. This mirrors
// CommonMark's own restriction on intraword "_" emphasis (the reason
// Markdown reaches for "*" rather than "_" to emphasize mid-word); "**"
// carries no such restriction, so boldItalicHTML above needs no equivalent
// care.
//
// Like splitPaired, this is total: an underscore that never finds a valid
// partner — isItalicOpener or findItalicCloser saying no — is emitted
// exactly as-is, never as half of a dangling tag.
func italicHTML(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.IndexByte(s[i:], '_')
		if idx == -1 {
			b.WriteString(s[i:])
			break
		}
		pos := i + idx
		if !isItalicOpener(s, pos) {
			b.WriteString(s[i : pos+1])
			i = pos + 1
			continue
		}
		closeAt := findItalicCloser(s, pos+1)
		if closeAt == -1 {
			b.WriteString(s[i : pos+1])
			i = pos + 1
			continue
		}
		b.WriteString(s[i:pos])
		b.WriteString("<em>")
		b.WriteString(s[pos+1 : closeAt])
		b.WriteString("</em>")
		i = closeAt + 1
	}
	return b.String()
}

// isItalicOpener reports whether the '_' at byte offset pos in s can open
// an italic span: not immediately preceded by a letter/digit (the
// intraword guard), and not immediately followed by whitespace or the end
// of the string (an opener needs something to actually open onto).
func isItalicOpener(s string, pos int) bool {
	if prev, psize := utf8.DecodeLastRuneInString(s[:pos]); psize != 0 && isWordRune(prev) {
		return false
	}
	next, nsize := utf8.DecodeRuneInString(s[pos+1:])
	if nsize == 0 || unicode.IsSpace(next) {
		return false
	}
	return true
}

// isItalicCloser is isItalicOpener's mirror for the '_' at pos closing a
// span: not immediately followed by a letter/digit (the intraword guard),
// and not immediately preceded by whitespace or the start of the string.
func isItalicCloser(s string, pos int) bool {
	if next, nsize := utf8.DecodeRuneInString(s[pos+1:]); nsize != 0 && isWordRune(next) {
		return false
	}
	prev, psize := utf8.DecodeLastRuneInString(s[:pos])
	if psize == 0 || unicode.IsSpace(prev) {
		return false
	}
	return true
}

// findItalicCloser scans s for the first valid closing '_' at or after
// from, skipping any '_' that fails isItalicCloser — e.g. one that is
// itself just the second half of a snake_case identifier. Returns -1 when
// no valid closer exists, which is how italicHTML leaves an opener that
// never resolves as a plain, literal underscore instead of an open tag.
func findItalicCloser(s string, from int) int {
	for from < len(s) {
		idx := strings.IndexByte(s[from:], '_')
		if idx == -1 {
			return -1
		}
		pos := from + idx
		if isItalicCloser(s, pos) {
			return pos
		}
		from = pos + 1
	}
	return -1
}

// isWordRune is the letter/digit test isItalicOpener/isItalicCloser use for
// the intraword guard — Unicode-aware so it applies equally to notes.yaml's
// Russian prose and to the ASCII snake_case identifiers embedded in it.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

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
