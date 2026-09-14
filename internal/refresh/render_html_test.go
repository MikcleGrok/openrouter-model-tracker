package refresh

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
)

func TestRenderHTMLGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderHTML(&buf, goldenData()); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	goldenPath := filepath.Join("testdata", "golden.html")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if buf.String() == string(want) {
		return
	}

	gotLines := strings.Split(buf.String(), "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := "", ""
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("render differs from %s at line %d:\n  got:  %q\n  want: %q", goldenPath, i+1, g, w)
		}
	}
	t.Fatalf("render differs from %s but no differing line was found (trailing bytes?)", goldenPath)
}

// stripStyleAndScript removes <style>...</style> and <script>...</script>
// blocks so TestRenderHTMLIsWellFormed can feed the remaining markup to
// encoding/xml without CSS/JS syntax (curly braces, "<", "&&", comparisons)
// being misread as XML.
var styleOrScriptBlock = regexp.MustCompile(`(?s)<(style|script)\b[^>]*>.*?</(style|script)>`)

func stripStyleAndScript(doc string) string {
	return styleOrScriptBlock.ReplaceAllString(doc, "")
}

// TestRenderHTMLIsWellFormed is the HTML analogue of
// TestRenderProducesNoTableBreakingLines: it catches unclosed tags,
// unescaped "&"/"<", and malformed attributes without a second Go
// dependency (no golang.org/x/net/html — see D7.6 in .task/omt-report/plan.md).
// encoding/xml is stricter than HTML5 (it has no notion of an optional
// closing tag or a bare boolean attribute), which is exactly why every void
// element in comparison.html.tmpl is written in XML style
// (<meta charset="utf-8" />) and every boolean-shaped attribute carries an
// explicit value (data-sortable="true").
func TestRenderHTMLIsWellFormed(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderHTML(&buf, goldenData()); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	body := stripStyleAndScript(buf.String())
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = true
	dec.AutoClose = nil
	dec.Entity = map[string]string{"nbsp": "\u00a0"}
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("HTML is not well-formed XML: %v", err)
		}
	}
}

// TestRenderHTMLSortValuesAreParseable checks every data-value attribute in
// the rendered document is either empty (the documented "n/a sorts last"
// marker) or a value strconv.ParseFloat accepts — the inline sort script's
// own numeric comparison depends on this.
func TestRenderHTMLSortValuesAreParseable(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderHTML(&buf, goldenData()); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	re := regexp.MustCompile(`data-value="([^"]*)"`)
	matches := re.FindAllStringSubmatch(buf.String(), -1)
	if len(matches) == 0 {
		t.Fatal("no data-value attributes found in rendered HTML")
	}
	for _, m := range matches {
		v := m[1]
		if v == "" {
			continue
		}
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			t.Errorf("data-value=%q is not empty and not parseable: %v", v, err)
		}
	}
}

// modelSlugs returns the set of slugs a rendered document actually shows,
// used by TestMarkdownAndHTMLCoverTheSameModels for a completeness-parity
// check between the two formats.
func modelSlugsFromRanked(models ...model.Model) map[string]bool {
	out := make(map[string]bool, len(models))
	for _, m := range models {
		out[m.Slug] = true
	}
	return out
}

func TestMarkdownAndHTMLCoverTheSameModels(t *testing.T) {
	data := goldenData()
	var md, html bytes.Buffer
	if err := Render(&md, data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if err := RenderHTML(&html, data); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	luna, sol, nemo := goldenModels()
	want := modelSlugsFromRanked(luna, sol, nemo)
	for slug := range want {
		if !strings.Contains(md.String(), slug) {
			t.Errorf("Markdown output is missing slug %q", slug)
		}
		if !strings.Contains(html.String(), slug) {
			t.Errorf("HTML output is missing slug %q", slug)
		}
	}
}

// TestRenderHTMLEscapesAngleBracketLabels exercises the exact hostile shape
// notes.yaml already contains in spirit ("<≈ Haiku 4.5") plus a literal tag
// and an ampersand, on both Note and ClaudeRef — both routed through
// inlineHTML's explicit escaping in the free-models table — and checks
// neither lets it through: no live <b> tag reaches the page, and the
// special characters show up as their named/numeric entities.
func TestRenderHTMLEscapesAngleBracketLabels(t *testing.T) {
	hostile := `<≈ Haiku 4.5 <b>x</b> & "q"`
	m := model.Model{
		Slug: "hostile/model", DisplayName: "Hostile Model", Tier: "free",
		Context: 1000, Free: true, ScoreLabel: "n/a",
		QualityPriceLabel: "n/a (free)", ClaudeRef: hostile, Note: hostile,
		Owner: "Demo", OpenWeights: "n/a", Copyright: "unknown",
	}
	data := RenderData{FreeModels: []model.Model{m}, FreeNotes: []ModelNote{{Model: m, Note: m.Note}}}
	var buf bytes.Buffer
	if err := RenderHTML(&buf, data); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "<b>x</b>") {
		t.Fatalf("rendered HTML contains a live <b> tag from model prose:\n%s", out)
	}
	if !strings.Contains(out, "&lt;b&gt;x&lt;/b&gt;") {
		t.Fatalf("rendered HTML does not escape the <b> tag as text:\n%s", out)
	}
	if !strings.Contains(out, "&amp;") {
		t.Fatalf("rendered HTML does not escape '&':\n%s", out)
	}
}

// TestInlineHTMLIsTotal checks inlineHTML always produces balanced
// <code>/</code>, <strong>/</strong> and <em>/</em> counts, including on
// deliberately unbalanced backtick/asterisk/underscore input — an unpaired
// delimiter must never open a span with no closing tag.
func TestInlineHTMLIsTotal(t *testing.T) {
	cases := []string{
		"",
		"no backticks here",
		"one `pair` only",
		"two `pairs` in `one` string",
		"trailing unpaired `backtick",
		"leading unpaired backtick` trailing text",
		"`",
		"``",
		"```",
		"a`b`c`d`e`f`g",
		"<already> & \"escaped\" `code`",
		"**bold**",
		"_italic_",
		"one ** here",
		"trailing unpaired **bold",
		"leading unpaired bold** trailing text",
		"**",
		"****",
		"trailing unpaired _italic",
		"leading unpaired italic_ trailing text",
		"_",
		"__",
		"**bold _and italic_ text**",
		"`code with _underscore_`",
		"`code with *asterisk*`",
		"hugging_face_id and reasoning_effort stay literal",
		notes.NeedsReview,
		"a**b**c**d**e**f**g",
		"a_b_c_d_e_f_g",
	}
	for _, in := range cases {
		out := string(inlineHTML(in))
		for _, tag := range []string{"code", "strong", "em"} {
			opens := strings.Count(out, "<"+tag+">")
			closes := strings.Count(out, "</"+tag+">")
			if opens != closes {
				t.Errorf("inlineHTML(%q) = %q, <%s> count %d != </%s> count %d", in, out, tag, opens, tag, closes)
			}
		}
	}
}

// TestInlineHTMLBoldAndItalic checks the exact conversion for **bold** and
// _italic_ spans, including the interactions D3/D7.2 and the review that
// found this bug both called out as the easy-to-get-wrong cases: emphasis
// nested inside bold, a code span's content staying fully literal even when
// it looks like it contains emphasis markers, unmatched delimiters falling
// back to literal text, and the real snake_case collision this feature's
// review turned up in notes.yaml itself (canonical_slug, hugging_face_id,
// reasoning_effort — all in the z-ai/glm-5.2:free note).
func TestInlineHTMLBoldAndItalic(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**bold**", "<strong>bold</strong>"},
		{"italic", "_italic_", "<em>italic</em>"},
		{"needs review fixture", notes.NeedsReview, "<em>нужен обзор</em>"},
		{"needs review with trailing note", notes.NeedsReview + " (нет данных)", "<em>нужен обзор</em> (нет данных)"},
		{"real bold example from notes.yaml", "**да, MIT**", "<strong>да, MIT</strong>"},
		{"unmatched bold, no closing pair", "one ** here", "one ** here"},
		{"trailing unpaired bold", "trailing unpaired **bold", "trailing unpaired **bold"},
		{"leading unpaired bold", "leading unpaired bold** trailing text", "leading unpaired bold** trailing text"},
		{"bold marker alone", "**", "**"},
		{"unmatched italic, no closing pair", "trailing unpaired text _italic", "trailing unpaired text _italic"},
		{"stray italic with no opener candidate", "leading unpaired italic_ trailing text", "leading unpaired italic_ trailing text"},
		{"italic marker alone", "_", "_"},
		{"italic nested inside bold", "**bold _and italic_ text**", "<strong>bold <em>and italic</em> text</strong>"},
		{"code span content is never further interpreted: underscore", "`code with _underscore_`", "<code>code with _underscore_</code>"},
		{"code span content is never further interpreted: asterisk", "`code with **asterisk**`", "<code>code with **asterisk**</code>"},
		{"empty string", "", ""},
		{
			"snake_case identifiers stay literal, single underscore each",
			"canonical_slug and reasoning_effort are not italic",
			"canonical_slug and reasoning_effort are not italic",
		},
		{
			"snake_case identifier with two underscores stays fully literal",
			"hugging_face_id stays literal",
			"hugging_face_id stays literal",
		},
		{
			"real notes.yaml sentence mixing bold, a genuine no-op underscore run, and snake_case",
			`**Бесплатный маршрут** того же продукта: canonical_slug совпадает с hugging_face_id, а reasoning_effort — дефолтный.`,
			`<strong>Бесплатный маршрут</strong> того же продукта: canonical_slug совпадает с hugging_face_id, а reasoning_effort — дефолтный.`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(inlineHTML(tc.in))
			if got != tc.want {
				t.Errorf("inlineHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNotesProseHasNoMarkdownLinks guards inlineHTML's one assumption: that
// notes.yaml's only inline-markup convention is `backtick` spans, never
// Markdown links. If a future edit adds a "[text](url)" it must render
// literally in HTML (inlineHTML does not parse it), which this test catches
// loudly instead of letting a silently-wrong render ship.
func TestNotesProseHasNoMarkdownLinks(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "notes.yaml"))
	if err != nil {
		t.Fatalf("read notes.yaml: %v", err)
	}
	if m := regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`).FindString(string(body)); m != "" {
		t.Fatalf("notes.yaml contains a Markdown link (%q), which inlineHTML does not render as a link — update inlineHTML before this stays true", m)
	}
}

// TestRenderHTMLInternalLinksResolve checks every "#x" href actually has a
// matching id="x", and that ids are unique — run on the full report-shaped
// data (Ranked set) and on the refresh-shaped data (Ranked nil, appendix
// following the input-models order) since the two paths build different
// sets of cross-links.
func TestRenderHTMLInternalLinksResolve(t *testing.T) {
	for _, name := range []string{"report", "refresh"} {
		t.Run(name, func(t *testing.T) {
			data := goldenData()
			if name == "refresh" {
				data.Ranked = nil
				data.GenerationLine = ""
			}
			var buf bytes.Buffer
			if err := RenderHTML(&buf, data); err != nil {
				t.Fatalf("RenderHTML: %v", err)
			}
			assertInternalLinksResolve(t, buf.String())
		})
	}
}

func assertInternalLinksResolve(t *testing.T, doc string) {
	t.Helper()
	ids := map[string]int{}
	for _, m := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(doc, -1) {
		ids[m[1]]++
	}
	for id, n := range ids {
		if n > 1 {
			t.Errorf("id %q appears %d times, want unique", id, n)
		}
	}
	for _, m := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(doc, -1) {
		target := m[1]
		if target == "top" {
			continue // #top is body's own id, always present
		}
		if ids[target] == 0 {
			t.Errorf("href=\"#%s\" has no matching id — dangling internal link", target)
		}
	}
}

// TestAnchorIDsAreUniqueAcrossTrackedSlugs slugifies every real tracked slug
// in model-map.tsv and confirms anchorID produces no collision — the live
// invariant the "TestRenderHTMLInternalLinksResolve"/unique-ids check above
// depends on for the real document, not just the small test fixture.
func TestAnchorIDsAreUniqueAcrossTrackedSlugs(t *testing.T) {
	entries, err := modelmap.Load(filepath.Join("..", "..", "model-map.tsv"))
	if err != nil {
		t.Fatalf("modelmap.Load: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("model-map.tsv has no entries")
	}
	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		id := anchorID(e.Slug)
		if prior, ok := seen[id]; ok && prior != e.Slug {
			t.Errorf("anchorID collision: %q and %q both slugify to %q", prior, e.Slug, id)
		}
		seen[id] = e.Slug
	}
}
