Local patch of `github.com/charmbracelet/bubbletea@v1.3.10`.

Only `tea_init.go` differs from upstream: its `init()` calls
`lipgloss.HasDarkBackground()`, which sends an OSC 11 / CSI 6n terminal
query and blocks reading the reply. Upstream bounds a single byte read via
termenv's `OSCTimeout` (5s), but not the whole two-query exchange, so a
terminal that never answers, or answers in a way termenv's reader does not
expect, can block this `init()` — and therefore the whole program, since
`init()` runs before `main()` and before any TUI frame can render —
indefinitely, with the terminal showing nothing.

This patch runs the same call in a goroutine behind a 2-second deadline,
so program startup is always bounded regardless of what the terminal does.

Remove this replace + directory once upstream bubbletea v2 drops the
workaround (its own doc comment says so) or ships an equivalent bound.
