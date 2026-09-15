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

# ==================== PASS 1: ENGLISH ====================
# Full narrative, English: establish the list -> two sorts -> structured
# filter narrowing to 5 rows -> all 4 Detail tabs -> all 6 Help sections.
# Pass 2 below (after the reset sequence) repeats this exact narrative in
# Russian, structurally unchanged, so the two passes are equally weighted.

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
# 6=input 7=output 8=has-q/p 9=availability 10=copyright_guardrail 11=task_fit.
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

# ==================== RESET: back to Pass 1's starting state ====================
# Pass 1 above ends filtered down to 5 rows, sorted "q/p" -- Pass 2 needs to
# start from the same base state Pass 1 did (423 rows, filter
# "availability:paid", sort "utility") or its own sort/filter steps would be
# a weak, no-op demo narrowing an already-narrow list. Validated empirically
# against extracted GIF frames during investigation (final state matched
# models:423, filter:"availability:paid", sort:utility, cursor row 0, same
# top model highlighted as the tape's true opening screen -- pixel/text-
# identical), not just reasoned from the tape's own logic.
#   "s" - sort cycle (tuiSortKeys, tui.go:53): the last sort Pass 1 leaves in
#     place is "q/p" (set by the direct-jump "r" press in part 2 above,
#     index 7 in tuiSortKeys); one "s" press, (7+1)%9=8, lands exactly on
#     "utility", the same default Pass 1 opened on.
#   "f" - reopen the filter editor; filterDraft re-parses from the current
#     filter string (tui.go:1525-1526), same as part 3's own "f" above.
#   "c" - filterKey's own clear (tui.go:1585-1586) zeroes the ENTIRE draft,
#     Availability (field 9) included -- confirmed empirically pressing "c"
#     on a draft with availability:paid set reverts every field including
#     Availability. Landing here alone would show all 423 paid+free models,
#     not the paid-only catalog Pass 1 opened on, so Availability needs
#     explicitly re-setting below. (The "Paid" checkbox, field 1, is a wholly
#     different concept from the Availability token, field 9 -- field 1 is
#     never touched anywhere in this tape.)
#   Down 9 - filterCursor 0->9 (Availability field; field order tui.go:1532-
#     1596/1950, same as part 3's own comment above: 0=free 1=paid 2=scored
#     3=tier 4=quality 5=context 6=input 7=output 8=has-q/p 9=availability
#     10=copyright_guardrail 11=task_fit).
#   Right 3 - "" -> "any" -> "free" -> "paid" (tuiNextAvailability, tui.go:
#     1759-1767).
#   Enter - applies; candidate == "availability:paid" == config.DefaultFilter,
#     via applyFilterDraft (tui.go:1601-1619), same as part 3's own Enter.
#   "g" - forces cursor to row 0 (tui.go:1111-1112, "home"/"g") -- NEEDED:
#     rebuild()'s cursor-follows-previously-selected-model logic does NOT
#     reliably land on row 0 after unfiltering back to the full list
#     (empirically the cursor landed on row 2 of 5 without this step).
Type "s"
Sleep 1.5s
Type "f"
Sleep 1.5s
Type "c"
Sleep 1s
Down 9
Sleep 1s
Right 3
Sleep 1s
Enter
Sleep 2s
Type "g"
Sleep 1.5s

# ==================== TRANSITION: English -> Russian ====================
# "l" (language_toggle, config.go:190) flips m.lang between "" and "ru"
# GLOBALLY -- every screen, not just overlay-local text -- so from here on
# Pass 2 renders entirely in Russian. This toggle fires here, well after
# Pass 1 has been running for many tens of seconds (not near process
# startup) -- the same placement this tape's previous single-screen RU
# preview used successfully. That placement matters: an isolated short
# probe tape toggling "l" seconds after launch was found to occasionally
# not register in time before the very next keypress (two back-to-back runs
# of the identical unmodified probe tape produced different results --
# English once, Russian once -- a process-startup race, not a language-
# handling bug), whereas the already-shipped baseline GIF's own use of this
# exact late placement (list, then a Detail tab, then a Help section, all
# sampled from the committed GIF) rendered clean, consistent Russian at
# every checkpoint with zero flakiness. Firing the toggle here, after Pass 1
# has fully warmed up the app, is the same regime that evidence covers.
Type "l"
Sleep 3s

# ==================== PASS 2: RUSSIAN (mirrors Pass 1 exactly) ====================
# Same 5-part narrative as Pass 1 above, same keys, same order -- comments
# here point back to Pass 1's own rather than re-deriving the mechanics.

# 1. Establish the models list (mirrors Pass 1 part 1 above).
Sleep 1.2s

# 2. Sorting: same two direct-jump sorts, same "g" pattern (mirrors Pass 1
# part 2 above -- see its comment for the rebuild()/cursor mechanics).
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

# 3. Filtering: same structured filter, quality>=80 and input<=1, same field
# navigation, same resulting 5-row narrowing (mirrors Pass 1 part 3 above).
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

# 4. Detail screen: same 4 tabs in order (mirrors Pass 1 part 4 above).
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
Type "h"
Sleep 1s

# 5. Help overlay: same 6 sections in ascending order (mirrors Pass 1 part 5
# above).
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
Escape
Sleep 1.5s

# Quit cleanly from the bare list, ending on Pass 2's own Russian state --
# no toggle back to English. Hidden, mirroring the opening Hide/Show: the
# GIF must end on the TUI's own last visible screen, never on the bash
# prompt that reappears once the program exits.
Hide
Type "x"
Sleep 1s
