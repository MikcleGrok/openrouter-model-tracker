Output docs/assets/tui-demo.gif
Set FontSize 24
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 2250
Set Height 1230
Set Theme "Catppuccin Frappe"
Set Padding 12

Hide
Type "./bin/openrouter tui --data-dir . --filter availability:paid"
Enter
Sleep 2.5s
Show
Sleep 1s

# 1. Establish the models list: this is a paid-model catalog, default
# "utility" sort. Short pause -- just long enough to register "here is a
# list", not long enough to read as its own inert scene before sorting
# (previously 3.5s, which made sorting feel buried rather than the
# opening act).
Sleep 1.2s

# 2. Sorting: two different sorts applied in sequence via the direct-jump
# keys, against the full unfiltered paid-models list (~423 rows) -- this
# runs BEFORE the filter demo below, so nothing has narrowed the row count
# yet. "g" (bound to the same "home" action as the Home key in the
# main-list context, tui.go:1111-1112) resets the cursor to the top row
# both before AND after each sort keypress: rebuild() re-locates the
# cursor by following the *previously selected model* to its new row
# (confirmed empirically -- pressing "g" only beforehand left the cursor on
# row 1, not row 0, after the second sort, because the model selected by
# the first "g" wasn't top-of-list under the new order), so only the
# post-sort "g" reliably anchors the following Down-trace at row 0.
#
# The sort keypress itself is the whole point, so it gets its own
# unmistakable beat: pause on the pre-sort (utility) top row for long
# enough to register as "the starting order", THEN press the sort key,
# THEN hold on the immediate result before doing anything else (moving
# the cursor, reading the label) -- so a viewer sees list -> keypress ->
# reordered list as one legible beat instead of a blink between two
# already-settled states.
Type "g"
Sleep 1.5s
Type "q"
Sleep 3s
Type "g"
Sleep 500ms
Down
Sleep 700ms
Down
Sleep 700ms
Type "g"
Sleep 1.2s
Type "r"
Sleep 3s
Type "g"
Sleep 500ms
Down
Sleep 700ms
Down
Sleep 700ms

# 3. Filtering: open the real structured filter editor ("f") and set BOTH
# a quality predicate and a price predicate before applying once with
# Enter, so the row list visibly narrows in one deliberate step. Sort
# state persists across a filter being applied -- applyFilterDraft
# (tui.go:1601-1619) and rebuild() (tui.go:411-423) never touch
# m.sortKey -- and filterTableModels' predicate match (tui.go:443-451)
# never depends on the current sort key either, so this filter narrows to
# exactly the same 5-row result below regardless of running after the
# sorting block above instead of before it; only the *display order* of
# the surviving rows (now q/p, the sort left in place by the block above,
# instead of the original "utility" default) and the carried-over cursor
# position differ from the pre-reorder tape.
#   quality>=80 (SWE-bench Verified, the default score source) and
#   input<=1 ($/M tokens) together leave a small, real, non-empty subset
#   of the paid catalog (confirmed by extracting and reading actual GIF
#   frames: 5 rows -- gpt-5.6-luna, glm-5.2, glm-5.3-flash, gemini-3.7-flash,
#   gemini-3.8-flash; no deepseek model made this particular cut).
# Cursor starts at field 0 ("free") on open (openFilterEditor sets
# m.filterCursor = 0, tui.go:1524). Field order (tui.go:1532-1596,
# tui.go:1950): 0=free 1=paid 2=scored 3=tier 4=quality 5=context
# 6=input 7=output 8=has-q/p 9=availability 10=copyright_guardrail.
# The current filter (availability:paid from the launch flag) is already
# parsed into the draft on open, so field 9 needs no touching.
Type "f"
Sleep 2.5s
Down 4
Sleep 800ms
Type "80"
Sleep 1.8s
Down 2
Sleep 800ms
Type "1"
Sleep 1.8s
Enter
Sleep 4s

# 4. Detail screen: open it on the current row and step through all 4 tabs
# in order, pausing meaningfully on each. Digit keys are explicit and
# deliberate (Identity is already the default tab on open, tui.go:1197;
# "1" is pressed anyway to make the walk-through unambiguous).
Enter
Sleep 2.5s
Type "1"
Sleep 3s
Type "2"
Sleep 3s
Type "3"
Sleep 3s
Type "4"
Sleep 3.5s

# Close Detail back to the list. "h" is context-scoped to Detail here (it
# only quits from the bare list, which we are not at).
Type "h"
Sleep 1s

# 5. Help overlay, last: open it and page through all 6 sections in
# ascending order via the direct-jump digit keys, then close cleanly.
Type "?"
Sleep 1.5s
Type "1"
Sleep 3s
Type "2"
Sleep 3s
Type "3"
Sleep 3.5s
Type "4"
Sleep 3.5s
Type "5"
Sleep 3s
Type "6"
Sleep 3.5s

# Close the help overlay. Escape is context-scoped to the overlay here (it
# only quits from the bare list, which we are not at).
Escape
Sleep 1.5s

# 6. Language toggle: "l" (language_toggle, config.go:190) flips m.lang
# between "" and "ru" GLOBALLY -- every screen, not just this overlay's own
# bilingual help text -- so the beat needs to prove that on more than the
# list. First hold on the list long enough to actually read the column
# headers and the bottom hotkey-hint line flip to Russian (mirrors the
# "hold on the immediate result" pacing used for the sort keypresses in
# part 2), then reopen Detail and Help in turn to show the same toggle
# reaching the tab names, section titles and body prose everywhere else.
# "1" is pressed explicitly on each open, same as parts 4 and 5 already
# do, but for two different reasons confirmed by reading the key handler
# (tui.go:1190-1198, 1169-1189): opening Detail via Enter always resets
# detailTab to 0 regardless of what was open before, so its "1" is
# belt-and-suspenders like part 4's own; opening Help via "?" hardcodes
# landing on section index 2 ("Хоткеи"/Hotkeys), never index 0, so its "1"
# is the only thing that puts "Обзор" on screen at all. Only "Обзор" is
# shown (not all 6 RU sections, which would roughly double the GIF for no
# extra proof) since one short, prose-bearing section already demonstrates
# whole-screen translation. Finish by flipping back to English so the GIF
# ends in the same clean English state it did before this part existed,
# ahead of the unchanged quit sequence below.
Type "l"
Sleep 3s

Enter
Sleep 2.5s
Type "1"
Sleep 3s
Type "h"
Sleep 1s

Type "?"
Sleep 1.5s
Type "1"
Sleep 3.5s
Escape
Sleep 1.5s

Type "l"
Sleep 2.5s

# Quit cleanly from the bare list with the universal, always-safe quit key.
# Hidden, mirroring the opening Hide/Show: the GIF must end on the TUI's own
# last visible screen, never on the bash prompt that reappears once the
# program exits.
Hide
Type "x"
Sleep 1s
