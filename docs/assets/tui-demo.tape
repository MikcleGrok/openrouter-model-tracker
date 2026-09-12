Output docs/assets/tui-demo.gif
Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1500
Set Height 480
Set Theme "Catppuccin Frappe"
Set Padding 12

Hide
Type "./bin/openrouter tui --data-dir . --filter availability:paid"
Enter
Sleep 2.5s
Show
Sleep 1s

# Cycle the availability filter: paid -> any -> free -> paid. Both the meta
# line ("filter:...") and the row content (free-tier rows, different prices)
# change visibly at every step; done before the name search below so a
# later "free" state can never intersect with search:"claude" into an
# empty list.
Type "p"
Sleep 2s
Type "p"
Sleep 2s
Type "p"
Sleep 2.2s

# Search "claude" and actually apply it with Enter (not Escape, which only
# cancels the draft) -- this narrows the visible list to just the Claude
# family, the least ambiguous possible proof Claude models are tracked here.
Type "/"
Type "claude"
Enter
Sleep 2.5s

# Cycle the score source while the Claude-only search is still active:
# SWE-bench -> Arena -> GPQA Diamond. The Status column header changes
# ("SWE %" -> "Arena Elo" -> "GPQA %") and so do the Claude rows' own
# numbers and their relative order.
Space
Sleep 2.2s
Space
Sleep 2.2s

# Open the help overlay (opens straight on the Hotkeys section), read it,
# close it.
Type "?"
Sleep 2.5s
Escape
Sleep 1s

# Bonus: open Detail on the top (Claude, still GPQA-ranked) row and step
# through two tabs.
Enter
Sleep 1.5s
Type "2"
Sleep 1.3s
Escape
Sleep 0.8s
