Local patch of `github.com/charmbracelet/bubbletea@v1.3.10`.

Two files differ from upstream: `tea_init.go` and `standard_renderer.go`,
and one file is added: `painted_width.go`. Everything else is verbatim
upstream.

## `tea_init.go` — bounding the startup terminal query

Its `init()` calls `lipgloss.HasDarkBackground()`, which sends an OSC 11 /
CSI 6n terminal query and blocks reading the reply. Upstream bounds a single
byte read via termenv's `OSCTimeout` (5s), but not the whole two-query
exchange, so a terminal that never answers, or answers in a way termenv's
reader does not expect, can block this `init()` — and therefore the whole
program, since `init()` runs before `main()` and before any TUI frame can
render — indefinitely, with the terminal showing nothing.

This patch runs the same call in a goroutine behind a 2-second deadline,
so program startup is always bounded regardless of what the terminal does.

Remove this replace + directory once upstream bubbletea v2 drops the
workaround (its own doc comment says so) or ships an equivalent bound.

## `painted_width.go` — measuring a line the way the terminal paints it

`flush()` appends `ansi.EraseLineRight` to a painted line only when it
believes the line is narrower than the terminal. The guard matters: on a line
that fills the terminal the cursor ends up in the last cell with wrap pending,
so an erase issued there wipes the character just painted instead of clearing
empty space. Upstream measures with `ansi.StringWidth`.

That measure is wrong for a small set of runes, and wrong in the direction the
guard cannot tolerate. A terminal's cell width is not decided by Unicode alone:
when the primary monospace face has no glyph for a character, the terminal
substitutes another font. macOS Terminal.app and iTerm2 both fall back to an
emoji face for the bare "circled Latin letter" glyphs of the Enclosed
Alphanumerics block — `Ⓩ`, `Ⓝ`, `ⓧ`, this repo's manufacturer icons for z.ai,
NVIDIA and Xiaomi — and paint them across two cells. Those characters carry
neither variation-selector-16 nor the Emoji_Presentation property, so
East_Asian_Width says Ambiguous and every width table, `ansi` and
`mattn/go-runewidth` alike, reports 1. No library can be "fixed" here; the
answer is a property of the font stack, not of the character.

A line those icons fill to the terminal edge is therefore measured one cell
short, `flush()` believes it has room to spare, and the erase wipes the last
painted cell of every such row. The host repository's layout code measures
through `internal/termwidth`, which applies the override, so without this patch
the renderer and the layout that feeds it disagree by exactly one column on
those rows.

`painted_width.go` adds `paintedLineWidth`, and `flush()`'s two erase-line
decisions call it in place of `ansi.StringWidth`. Nothing else in the renderer
changes: truncation still goes through `ansi.Truncate`, which is deliberate —
it only ever under-truncates here, and the layout already caps the line.

The rune list duplicates `internal/termwidth` because this directory is its own
Go module, reached through a `replace` directive, and importing the host
repository's `internal/` package would invert that dependency. `internal/termwidth`
is the authoritative list; the two must stay in sync.

Regression test: `TestRendererTreatsBareCircledIconLineAsFullWidth` in
`cmd/openrouter/icon_width_test.go`, which drives a real `tea.Program` painting
full-width icon lines and asserts no erase-line-right is appended to them.

## `standard_renderer.go` — no torn frame on window resize

Upstream applies a `WindowSizeMsg` to the renderer in `handleMessages`:
under the renderer's mutex it assigns `r.width`/`r.height` and calls
`repaint()`. The content laid out for that new size only arrives later, in
`write()`, under a *second* acquisition of the same mutex. Between the two,
`tea.go`'s event loop runs `model.Update(msg)` — arbitrary application code,
with no lock held:

```
handleMessages(msg)          // r.width/r.height = new size, repaint()  [lock #1]
model.Update(msg)            // unbounded app work, no lock held
renderer.write(model.View()) // r.buf = content for the new size        [lock #2]
```

The renderer's own ticker goroutine (`listen()`) calls `flush()` on a
~16.67ms timer under that same mutex. A tick landing in the gap paints the
*previous* frame's `r.buf` against the *new* `r.width`/`r.height`: pre-resize
content, truncated and cropped to the post-resize geometry, at the wrong
screen positions. `repaint()` in lock #1 makes this guaranteed rather than
rare — clearing `lastRender` is exactly what defeats `flush()`'s own
"nothing changed" early-out, so a dirty buffer is certain to be painted
instead of skipped. The artifact is a single frame and self-heals on the next
flush, but a drag-resize delivers many `SIGWINCH` events and hits it
repeatedly.

This patch stages the size instead of applying it. `handleMessages` records
it in a new `pendingResize` field; `write()` calls `applyPendingResize()`
under the lock it already takes, adopting the dimensions and the repaint
together with the content computed at those dimensions. The renderer
therefore only ever exposes a `(dimensions, content)` pair that was actually
produced together — the race is closed by construction rather than by
changing what `flush()` emits from a given state.

Two properties keep the staging bounded. `write()` is called once per
message, at the end of the same event-loop iteration that staged the size, so
a staged resize outlives exactly one `Update`+`View` and never spans two
messages. And `insertTop`/`insertBottom` — the only readers of `r.height`
besides `flush()` — are driven from `handleMessages` for message types that
stage nothing, so they always observe `pendingResize == nil`.

Locking across `model.Update` instead was rejected: it would hold the render
mutex for the whole duration of arbitrary application code, blocking the
ticker and every other renderer operation for as long as an app wants to
take.

The *first* `WindowSizeMsg` is deliberately not special-cased, even though
the renderer starts at `r.width == r.height == 0`. If a tick lands in that
one startup gap, upstream paints the construction-size view truncated and
cropped to the real terminal, whereas this fork paints it unbounded. The two
render nearly identically when the model was constructed at the terminal's
real size — which this repo's own `runTUIWithRankingConfigCompiled` does via
`tuiTerminalSize(out)` — though minor differences exist: at `r.width == 0`,
`flush()` never appends `ansi.EraseLineRight` to lines (it only does so when
`paintedLineWidth(line) < r.width`), and at `r.height == 0`, `flush()` does
not truncate `newLines` to the last `r.height` lines. Both differences appear
only in this one startup frame (before adoption) and are invisible with
`tea.WithAltScreen()` (which clears the screen). Keeping one uniform rule was
preferred over a branch whose only justification is that cropping stale
content beats wrapping it.

Regression test: `TestRendererNeverPaintsPreResizeContentAtNewDimensions` in
`cmd/openrouter/renderer_resize_test.go`, which drives a real `tea.Program`
with `Update` parked mid-resize and asserts that nothing painted in that
window carries the pre-resize layout.

Upstream issue for this race is not filed; re-check whether bubbletea v2's
renderer rework makes the patch unnecessary when dropping the `tea_init.go`
one.
