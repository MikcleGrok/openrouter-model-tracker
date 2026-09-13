Output docs/assets/tui-demo.gif
Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1500
Set Height 820
Set Theme "Catppuccin Frappe"
Set Padding 12

Hide
Type "./bin/openrouter tui --data-dir . --filter availability:paid"
Enter
Sleep 2.5s
Show
Sleep 1s

# 1. Establish the models list: this is a paid-model catalog, default
# "utility" sort. A long establishing pause before anything else changes.
Sleep 3.5s

# 2. Filtering: open the real structured filter editor ("f") and set BOTH
# a quality predicate and a price predicate before applying once with
# Enter, so the row list visibly narrows in one deliberate step.
#   quality>=80 (SWE-bench Verified, the default score source) and
#   input<=1 ($/M tokens) together leave a small, real, non-empty subset
#   of the paid catalog (confirmed by extracting and reading actual GIF
#   frames: 5 rows — gpt-5.6-luna, glm-5.2, glm-5.3-flash, gemini-3.7-flash,
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

# 3. Sorting: two different sorts applied in sequence via the direct-jump
# keys. "g" (bound to the same "home" action as the Home key in the
# main-list context, tui.go:1111-1112) resets the cursor to the top row
# both before AND after each sort keypress: rebuild() re-locates the
# cursor by following the *previously selected model* to its new row
# (confirmed empirically -- pressing "g" only beforehand left the cursor on
# row 1, not row 0, after the second sort, because the model selected by
# the first "g" wasn't top-of-list under the new order), so only the
# post-sort "g" reliably anchors the following Down-trace at row 0. Hold
# long enough to read the "sort:" label change in the meta line
# (tui.go:670/2074) before stepping through the new top few rows.
Type "g"
Sleep 500ms
Type "q"
Sleep 2.5s
Type "g"
Sleep 500ms
Down
Sleep 700ms
Down
Sleep 700ms
Type "g"
Sleep 500ms
Type "r"
Sleep 2.5s
Type "g"
Sleep 500ms
Down
Sleep 700ms
Down
Sleep 700ms

# 4. Detail screen: open it on the top row and step through all 4 tabs in
# order, pausing meaningfully on each. Digit keys are explicit and
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

# 6. Help overlay, last: open it and page through all 6 sections in
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

# Quit cleanly from the bare list with the universal, always-safe quit key.
# Hidden, mirroring the opening Hide/Show: the GIF must end on the TUI's own
# last visible screen, never on the bash prompt that reappears once the
# program exits.
Hide
Type "x"
Sleep 1s
