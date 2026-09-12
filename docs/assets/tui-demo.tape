Output docs/assets/tui-demo.gif
Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1500
Set Height 480
Set Theme "Catppuccin Frappe"
Set Padding 12

Hide
Type "cd /Users/sergey/projects/tools/openrouter-model-tracker/.worktree/readme-and-demo-refresh && ./bin/openrouter tui --data-dir . --filter availability:paid"
Enter
Sleep 2.5s
Show

# Cycle the score source: SWE-bench -> Arena -> GPQA Diamond.
Space
Sleep 1.5s
Space
Sleep 1.5s

# Open a model's Detail screen and step through all four consolidated tabs.
Enter
Sleep 1.5s
Type "2"
Sleep 1.3s
Type "3"
Sleep 1.3s
Type "4"
Sleep 1.3s
Type "1"
Sleep 1.3s
Escape
Sleep 1s

# Cycle the availability filter over to free models, then back to paid.
Type "p"
Sleep 1.3s
Type "p"
Sleep 1.5s
Type "p"
Sleep 1s

# Quick substring search.
Type "/"
Type "claude"
Sleep 1.5s
Escape
Sleep 1s

# Manual refresh.
Type "R"
Sleep 2.5s
