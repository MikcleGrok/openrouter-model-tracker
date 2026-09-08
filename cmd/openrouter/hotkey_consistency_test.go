package main

// Cross-surface hotkey-consistency test (guide-tools 02-tui.md: "the set of
// categories, their order, and the distribution of entries MUST match on
// every help surface of a program, and this MUST be covered by a test that
// compares surfaces against each other, not by review"). This project has
// exactly two surfaces that enumerate hotkeys: the TUI's own F1 help
// overlay (tuiHelpSectionHotkeysBody / tuiHelpSectionHotkeysBodyRU in
// tui.go) and man/openrouter.1's HOTKEYS section; `--help` never lists
// hotkeys at all, so it is out of scope.
//
// This file lives in package main specifically so it can read the
// unexported tuiHelpSectionHotkeysBody*/tuiHelpSectionFiltersBody/
// tuiHelpSectionDetailBody constants directly. The man page is read from
// its relative repository path -- `go test` runs with the package
// directory as its working directory, so this path is stable without
// needing //go:embed (which cannot reach outside its own package
// directory anyway).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hotkeyEntry is one parsed row from an overlay help-section body: the raw
// Key column exactly as written ("j / k"), the short action label, and the
// description. Only Keys is compared against the man page; Action and
// Description are documentation prose, not part of the taxonomy this test
// enforces.
type hotkeyEntry struct {
	Keys        string
	Action      string
	Description string
}

type hotkeyCategory struct {
	Name    string
	Entries []hotkeyEntry
}

// tabMarker is the literal two-character sequence `\t` (backslash, t) that
// tui.go's help-section bodies use as their column separator. These bodies
// are Go raw string literals (backtick-delimited), inside which `\t` is
// never an escape sequence and never becomes an actual tab byte -- it stays
// two literal characters; tuiFormatHelpLine (tui.go) itself splits on this
// same literal marker at render time, not on a real tab. Every parser below
// MUST split/match on this literal marker, never on "\t" (an interpreted
// string literal, which is one real tab byte and never appears in these
// constants at all).
const tabMarker = `\t`

// parseOverlayHotkeyCategories parses a tuiHelpSectionHotkeysBody-shaped
// constant into its ordered categories and entries.
//
// Line shapes in these constants: the first line is the section's own
// title (e.g. "Hotkeys") and is skipped; a line starting with a tab is an
// entry ("\tKey\tAction\tDescription") belonging to whichever category was
// most recently opened; any other non-empty line is a candidate category
// header. A candidate header is accepted as a category only when the next
// non-blank line is a tab-prefixed entry -- this is what correctly
// excludes trailing prose such as "No task-fit classification is shown as
// n/a." (itself followed by a blank line and then another bare line, never
// a tab-entry) from being mistaken for a zero-entry category.
func parseOverlayHotkeyCategories(body string) []hotkeyCategory {
	lines := strings.Split(body, "\n")
	var categories []hotkeyCategory
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, tabMarker) {
			if len(categories) == 0 {
				continue
			}
			parts := strings.SplitN(line, tabMarker, 4)
			if len(parts) != 4 {
				continue
			}
			cur := &categories[len(categories)-1]
			cur.Entries = append(cur.Entries, hotkeyEntry{
				Keys:        parts[1],
				Action:      parts[2],
				Description: parts[3],
			})
			continue
		}
		isHeader := false
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "" {
				continue
			}
			isHeader = strings.HasPrefix(lines[j], tabMarker)
			break
		}
		if isHeader {
			categories = append(categories, hotkeyCategory{Name: line})
		}
	}
	return categories
}

// splitKeys splits an entry's raw Keys field into its individual physical
// key tokens. Within the Hotkeys taxonomy every multi-key field separates
// keys with " / " (space-slash-space: "j / k", "Enter / Right",
// "Esc / Left / h", "x / Ctrl-C") -- the separator MUST include the
// surrounding spaces, because the bare "/" character is itself a real,
// single-character key (the search/filter key) that appears alone with no
// spaces around it; splitting on a bare "/" would destroy that entry.
func splitKeys(raw string) []string {
	if !strings.Contains(raw, " / ") {
		return []string{raw}
	}
	parts := strings.Split(raw, " / ")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keys = append(keys, p)
		}
	}
	return keys
}

// normalizeKeySpec canonicalizes an overlay Keys field into the same
// "key1 / key2" shape manEntryKeyStrings produces from parsed man-page
// tokens, so ordered key lists compare equal regardless of which surface
// wrote them.
func normalizeKeySpec(raw string) string {
	return strings.Join(splitKeys(raw), " / ")
}

func overlayCategoryNames(cats []hotkeyCategory) []string {
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return names
}

func overlayEntryKeyStrings(entries []hotkeyEntry) []string {
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = normalizeKeySpec(e.Keys)
	}
	return keys
}

// --- man page-side parsing -------------------------------------------------

// manEntry is one ".TP" block's parsed key tokens, in the order the
// ".B"/".BR" line lists them.
type manEntry struct {
	Keys []string
}

type manCategory struct {
	Name    string
	Entries []manEntry
}

var manKeyMacroRe = regexp.MustCompile(`^\.(B|BR)\s+(.*)$`)

// tokenizeRoffArgs splits a roff macro's argument text the way troff
// itself does: whitespace-separated, except that a double-quoted run is
// one argument (its quotes are stripped, its interior -- including spaces
// -- kept verbatim). This project's man page never needs nested quotes or
// backslash-escaped quotes in macro arguments, so this tokenizer does not
// handle them.
func tokenizeRoffArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inQuotes := false
	flush := func() {
		if cur.Len() > 0 {
			args = append(args, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return args
}

// unescapeRoffKey strips the roff escapes this man page uses inside key
// names (\- for a literal hyphen, e.g. "Ctrl\-C").
func unescapeRoffKey(s string) string {
	return strings.ReplaceAll(s, `\-`, "-")
}

// parseManHotkeyKeys extracts the bold (actual key) tokens from one
// ".B"/".BR" line's already-tokenized arguments. ".B" takes one or more
// bold arguments (this page always uses exactly one). ".BR" alternates
// bold/roman starting with bold, so even-indexed (0-based) arguments are
// keys and odd-indexed ones are roman separator text (", ", " / ", " or ",
// ...) that carries no key information.
func parseManHotkeyKeys(macro string, args []string) []string {
	var keys []string
	if macro == "B" {
		for _, a := range args {
			keys = append(keys, unescapeRoffKey(a))
		}
		return keys
	}
	for i, a := range args {
		if i%2 == 0 {
			keys = append(keys, unescapeRoffKey(a))
		}
	}
	return keys
}

// parseManHotkeysSection parses the ".SH HOTKEYS" ... next-".SH" block of
// the man page into its ordered categories and entries. A ".SS <name>"
// line starts a category (name is the rest of the line, with surrounding
// double quotes stripped if present -- both quoted and unquoted .SS
// arguments are used in this file). A ".TP" line starts a new entry; the
// line immediately after it is expected to be its ".B"/".BR" key line.
// Everything else (plain description prose) is ignored: this test compares
// only the taxonomy (categories, order, keys), never description wording.
func parseManHotkeysSection(manPage string) []manCategory {
	lines := strings.Split(manPage, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if line == ".SH HOTKEYS" {
			start = i + 1
			continue
		}
		if start != -1 && i > start && strings.HasPrefix(line, ".SH ") {
			end = i
			break
		}
	}
	if start == -1 {
		return nil
	}
	var categories []manCategory
	pendingEntry := false
	for i := start; i < end; i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, ".SS"):
			name := strings.TrimSpace(strings.TrimPrefix(line, ".SS"))
			name = strings.Trim(name, `"`)
			categories = append(categories, manCategory{Name: name})
			pendingEntry = false
		case line == ".TP":
			pendingEntry = true
		case pendingEntry:
			pendingEntry = false
			if len(categories) == 0 {
				continue
			}
			m := manKeyMacroRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			keys := parseManHotkeyKeys(m[1], tokenizeRoffArgs(m[2]))
			cur := &categories[len(categories)-1]
			cur.Entries = append(cur.Entries, manEntry{Keys: keys})
		}
	}
	return categories
}

func manCategoryNames(cats []manCategory) []string {
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return names
}

func manEntryKeyStrings(entries []manEntry) []string {
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = strings.Join(e.Keys, " / ")
	}
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readManPage(t *testing.T) string {
	t.Helper()
	manPath := filepath.Join("..", "..", "man", "openrouter.1")
	data, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatalf("reading man page at %s: %v", manPath, err)
	}
	return string(data)
}

// TestHotkeyConsistency is the machine-checkable test 02-tui.md requires
// for cross-surface hotkey documentation: category names, their order, the
// ordered key list within each category, and the "one entry = one action"
// rule are all verified between the TUI overlay and the man page, in both
// languages the overlay ships.
func TestHotkeyConsistency(t *testing.T) {
	manPage := readManPage(t)

	overlayCats := parseOverlayHotkeyCategories(tuiHelpSectionHotkeysBody)
	manCats := parseManHotkeysSection(manPage)

	if len(overlayCats) == 0 {
		t.Fatal("overlay parser found zero Hotkeys categories -- parser or tui.go fixture regressed")
	}
	if len(manCats) == 0 {
		t.Fatal("man-page parser found zero HOTKEYS categories -- parser or man page regressed")
	}

	// Assertion 1: category names and their order match.
	overlayNames := overlayCategoryNames(overlayCats)
	manNames := manCategoryNames(manCats)
	if !equalStrings(overlayNames, manNames) {
		t.Fatalf("hotkey category names/order differ:\n overlay: %v\n man:     %v", overlayNames, manNames)
	}

	for i, oc := range overlayCats {
		mc := manCats[i]

		// Assertion 2: the ordered key list within each category matches.
		overlayKeys := overlayEntryKeyStrings(oc.Entries)
		manKeys := manEntryKeyStrings(mc.Entries)
		if !equalStrings(overlayKeys, manKeys) {
			t.Errorf("category %q: ordered key list differs:\n overlay: %v\n man:     %v", oc.Name, overlayKeys, manKeys)
		}

		// Assertion 3: "one entry describes exactly one action"
		// (02-tui.md), in machine form. Map every physical key in this
		// category to the overlay entry it belongs to, then verify each
		// man-page entry's key set resolves to exactly one overlay entry --
		// this is precisely what catches two overlay actions combined into
		// one man-page row (e.g. a regression re-merging "q" and "r", or
		// "s" and "S"), while passing legitimate multi-key synonyms of a
		// single action (Enter/Right, j/k, Home/g, ...) because those keys
		// already share one overlay entry.
		keyToEntry := map[string]int{}
		for idx, e := range oc.Entries {
			for _, k := range splitKeys(e.Keys) {
				keyToEntry[k] = idx
			}
		}
		for _, me := range mc.Entries {
			seen := map[int]bool{}
			for _, k := range me.Keys {
				idx, ok := keyToEntry[k]
				if !ok {
					t.Errorf("category %q: man page key %q (in entry %v) has no counterpart in the overlay", oc.Name, k, me.Keys)
					continue
				}
				seen[idx] = true
			}
			if len(seen) > 1 {
				t.Errorf("category %q: man entry %v combines keys that are separate actions in the overlay (overlay entry indices %v) -- one row MUST describe exactly one action (02-tui.md)", oc.Name, me.Keys, seen)
			}
		}
	}

	// Assertion 4: the RU overlay has the same categories (same order, RU
	// names) and the same per-category key lists as the EN overlay. The Key
	// column is documented as never translated (tuiHelpSectionHotkeysBodyRU's
	// own doc comment in tui.go), and that holds for every actual keyboard
	// key -- with one deliberate, narrow exception: the "Help search"
	// category's "0 matches" row does not name a key at all, it names the
	// zero-match status string the app itself renders (tui.go's own
	// matchStatus variable renders exactly "0 matches" in English and
	// "0 совпадений" in Russian), so translating it there is locale-accurate
	// app behavior, not drift. enToRUKeyColumn is that one exception, spelled
	// out explicitly rather than guessed at by a heuristic; every other key
	// compares directly, byte for byte, against its EN counterpart. Category
	// *names* differ by language throughout, so they are compared through
	// their own explicit map instead.
	enToRUKeyColumn := map[string]string{
		"0 matches": "0 совпадений",
	}
	translateKeySpecToRU := func(spec string) string {
		parts := strings.Split(spec, " / ")
		for i, p := range parts {
			if ru, ok := enToRUKeyColumn[p]; ok {
				parts[i] = ru
			}
		}
		return strings.Join(parts, " / ")
	}
	enToRUCategory := map[string]string{
		"Navigation":         "Навигация",
		"Data/view":          "Данные/вид",
		"Filters/settings":   "Фильтры/настройки",
		"Task-fit codes":     "Коды task-fit",
		"Refresh and finish": "Обновление и завершение",
		"Help search":        "Поиск в справке",
		"General/help":       "Общее/справка",
	}
	ruCats := parseOverlayHotkeyCategories(tuiHelpSectionHotkeysBodyRU)
	if len(ruCats) != len(overlayCats) {
		t.Fatalf("RU overlay has %d Hotkeys categories, EN overlay has %d", len(ruCats), len(overlayCats))
	}
	for i, oc := range overlayCats {
		rc := ruCats[i]
		wantRU, ok := enToRUCategory[oc.Name]
		if !ok {
			t.Fatalf("no RU category-name mapping registered for EN category %q -- add one to enToRUCategory", oc.Name)
		}
		if rc.Name != wantRU {
			t.Errorf("RU category order/name mismatch at index %d: got %q, want %q (EN: %q)", i, rc.Name, wantRU, oc.Name)
		}
		overlayKeys := overlayEntryKeyStrings(oc.Entries)
		ruKeys := overlayEntryKeyStrings(rc.Entries)
		expectedRUKeys := make([]string, len(overlayKeys))
		for i, k := range overlayKeys {
			expectedRUKeys[i] = translateKeySpecToRU(k)
		}
		if !equalStrings(expectedRUKeys, ruKeys) {
			t.Errorf("category %q/%q: RU key list differs from EN (after the one documented status-text exception):\n EN->expected RU: %v\n actual RU:       %v", oc.Name, rc.Name, expectedRUKeys, ruKeys)
		}
	}
}

// keyTokenDelimRe splits a Filters/Model-Detail Key column on "/", ",",
// " or " and " and " -- unlike the Hotkeys taxonomy itself, these two
// sections' prose mixes "/" with English glue words (tuiHelpSectionDetailBody:
// "Enter or Right", "Up/Down or j/k"), so the strict splitKeys used above
// is not enough here.
var keyTokenDelimRe = regexp.MustCompile(`\s*(?:/|,|\bor\b|\band\b)\s*`)

func tokenizeKeySpec(raw string) []string {
	parts := keyTokenDelimRe.Split(raw, -1)
	tokens := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// overlayFlatKeys collects every physical key token mentioned in any
// tab-prefixed entry line of a help-section body, regardless of category
// structure.
func overlayFlatKeys(body string) map[string]bool {
	keys := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, tabMarker) {
			continue
		}
		parts := strings.SplitN(line, tabMarker, 4)
		if len(parts) != 4 {
			continue
		}
		for _, k := range tokenizeKeySpec(parts[1]) {
			keys[k] = true
		}
	}
	return keys
}

// TestHotkeyNoOrphansInFiltersAndDetail implements this project's own
// resolution of the plan's secondary question: keys mentioned in the
// Filters and Model Detail overlay sections (which repeat some bindings in
// context) are not their own taxonomy and are not required to appear in
// the man page verbatim, but every key they mention MUST already be part
// of the Hotkeys taxonomy -- no "orphan" key documented only in a
// secondary section and nowhere in the canonical hotkey list.
func TestHotkeyNoOrphansInFiltersAndDetail(t *testing.T) {
	taxonomyKeys := map[string]bool{}
	for _, c := range parseOverlayHotkeyCategories(tuiHelpSectionHotkeysBody) {
		for _, e := range c.Entries {
			for _, k := range tokenizeKeySpec(e.Keys) {
				taxonomyKeys[k] = true
			}
		}
	}
	sections := map[string]string{
		"Filters":      tuiHelpSectionFiltersBody,
		"Model Detail": tuiHelpSectionDetailBody,
	}
	for _, name := range []string{"Filters", "Model Detail"} {
		for k := range overlayFlatKeys(sections[name]) {
			if !taxonomyKeys[k] {
				t.Errorf("%s section mentions key %q, which does not appear anywhere in the Hotkeys taxonomy", name, k)
			}
		}
	}
}
