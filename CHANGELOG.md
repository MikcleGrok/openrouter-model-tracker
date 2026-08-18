# What's New

## [1.14.31]

- Add a runtime Russian translation of the entire interactive TUI, toggled with `l` (mnemonic "language") and persisted to `config.yaml` via a new `tui_language` field. English stays the default — nothing changes for an existing config or a fresh session unless `l` is pressed. `l` is repurposed from a third `open_details` alias (alongside `Enter`/`Right`) to the new `language_toggle` keymap action, checked globally like `x` so it switches the language from the main list or from inside any overlay; its Cyrillic ЙЦУКЕН alias `д` follows automatically.
- Translate the main list view, the Settings/Filter/Columns overlays, the Model Detail overlay's remaining English text (section headers, the `n/a` placeholder → `н/д`, footer), table column headers, and all six F1 help sections (Overview, Score Sources, Hotkeys, Filters, Model Detail, Methodology) including the tab bar and search-mode status line. Literal key names, proper nouns and every literal CLI/filter/formula token are never translated; terminology follows `docs/methodology.md` and `model-map.tsv`'s own header comment throughout.

## [1.14.30]

- Add regression-test coverage auditing every single-key TUI hotkey — every main-context command letter, F1, digits, navigation keys, and the full Cyrillic ЙЦУКЕН alias table from `tuiLayoutAliases` — against every active text-input context (the search box, the F1 help search, and the Filter overlay's numeric fields), following up on the `x`-interception fix in 1.14.27. No dispatch bug was found: `m.inputMode` already gates the whole command surface, not just `x`.
- Pin one genuine finding from that audit: the Filter overlay's four numeric fields (Quality/Context/Input/Output) also accept free-text rune entry, but aren't gated behind `m.inputMode`, so `x` closes the whole overlay and discards an in-progress numeric edit — exactly the way `c` already does. Judged intentional, since no letter is ever valid content in a float-parsed field, and now covered by a dedicated test so a future `key()` change can't silently drift from that precedent.

## [1.14.29]

- Fix status/footer lines rendering flush against the content above them, with no blank line in between — the F1 help overlay's position footer (`Help N-M/T · ...`), its search input line, the Model Detail overlay's position footer (`Detail N-M/T · ...`), the Settings overlay's Status/Error line, and the main table's status/hints/search-input footer all now have a blank separator line before them, matching the Filter overlay's existing spacing.
- Fix the F1 help overlay's footer silently disappearing on a tall terminal or a long section instead of just missing its separator: the viewport now always reserves room for it (and, while searching, for the input line too), instead of relying on the surrounding text to happen to be short enough to leave it room.

## [1.14.28]

- Add a sixth F1 help section, Methodology: a brief but complete synthesis of how every row's data and the ranking itself are built — the three independent kinds of data behind a row, the `model-map.tsv` identity gate and its `!variant` escape hatch, the vals.ai/swebench.com/LMArena three-way split and vals.ai's priority, the exact mixed-utility formula, and why a row with a real number can still be excluded from ranking. Digit `6` and `Right` from Model Detail both reach it, matching the existing five-section navigation.
- Add `docs/methodology.md`: the same story with the extra room a web page allows, grounded in `model-map.tsv`'s header comment, `internal/model/model.go`'s identity-classification code and `internal/sources` package docs. The F1 Methodology section is its terminal-friendly cut, not a different text.
- Add a short "Методология" section to README.md summarising the same principles, linking to `docs/methodology.md` for the full write-up.

## [1.14.27]

- Fix `x` intercepting active text input: typing `x` (or its Cyrillic `ч` alias) while in the search box or the F1 help search no longer quits/closes the surrounding overlay — it inserts the literal character, matching every other letter. Searching for a real `model-map.tsv` slug like `x-ai/grok-4.5` used to quit the whole TUI the moment `x` was typed; it now works. Esc remains the only way to cancel out of active text input.
- Fix the Cyrillic `ч` alias not closing any TUI overlay (Settings, Filter, Columns, Detail, Help) — it only ever quit the bare main view, because the universal `x`-exit check compared the raw keypress instead of the same alias-translated key every other hotkey uses. `ч` now closes overlays exactly like `x` does, for Russian-layout (ЙЦУКЕН) users.
- Fix `x` leaving the Detail overlay's scroll offset stale when it closed it — it now resets the same way Detail's own dedicated close key already did. All overlay close paths now share one `closeOverlay` method, so `x` can no longer drift from an overlay's own close semantics.

## [1.14.26]

- Break the F1 help overlay's dense Overview and Score Sources prose paragraphs into `- ` bulleted, labelled points for scannability — every original sentence kept verbatim (the `model-map.tsv` identity-gate mechanics, the `exact_product`/`variant_mismatch`/`!variant` vocabulary, swebench.com's median-of-scaffolds/one-vote-per-scaffold rule, and LMArena Elo's Bradley-Terry caveat are all unchanged). The Model Detail section was checked and left alone — it was already scannable.
- Rename the program's self-reference in the F1 help content from `openrouter` to `omt` (the page title, the Overview opening line, and the Filters section's CLI example), matching the short `omt` alias the Homebrew formula already installs.

## [1.14.25]

- Retire the separate `?` shortcuts help document: `?` now opens the same sectioned F1 help overlay, landing directly on the Hotkeys section instead of a single-page document that had drifted out of sync with it. Once open, it is fully navigable exactly like F1-opened help (digit keys, Left/Right, section-scoped search); `?` and Esc both continue to close it from any section.

## [1.14.24]

- Rename the F1 full-help overlay's 5 tab-bar section titles from Russian to English (Обзор → Overview, Источники оценки → Score Sources, Горячие клавиши → Hotkeys, Фильтры → Filters, Карточка модели → Model Detail), matching the English body prose every section already had.

## [1.14.23]

- Restructure the F1 full-help overlay from one long scrollable page into 5 navigable sections — Обзор, Источники оценки, Горячие клавиши, Фильтры, Карточка модели — jumped to with digit keys `1`-`5` or stepped with Left/Right (clamped, no wraparound), shown via a tab bar highlighting the active section with the same style the main table uses for its selected row.
- Expand the Обзор and Источники оценки sections with real explanatory content: the identity-gate/`model-map.tsv` matching philosophy, and the distinction between vals.ai (independent single-harness SWE-bench Verified), swebench.com (self-submitted, median-of-scaffolds SWE-bench Verified), and LMArena Elo (a separate, non-comparable scale) — grounded in `model-map.tsv`'s own header comment and the `internal/sources` package docs.
- Scope help search (`/`) to the active section instead of the whole document; scroll offset resets to 0 on section switch. The `?` shortcut-help overlay is unchanged — still a single page.

## [1.14.22]

- Fix the Score-source "switch" rows in both help screens rendering as raw, unformatted text (a stray extra leading tab was defeating the help table's column parser).
- Fix several help rows silently duplicating the bound key across both the Key and Action columns instead of showing the real key next to its action label (`open settings.`, `open shortcut help.`, `open full help.`, and — nondeterministically — `open model details.`).
- Fix a doubled tab in the full-help "edit a structured filter" row and a capitalization mismatch in the full-help "open settings." row that silently broke its keymap substitution.

## [1.14.21]

- The F1 full-help overlay now opens with a short plain-language description of what the tool does, before the hotkey listing.
- The `?` shortcut-help overlay now points to F1 at the top ("Press F1 for the full help, including what this tool does."), not only at the very end.

## [1.14.20]

- Add `model-map.tsv` mappings for `z-ai/glm-5.2:free` (vals.ai, arena.ai) and `nvidia/nemotron-3.5-lightning:free` (arena.ai).
- Correct the free GLM-5.2 display name to disclose its smaller context window versus the paid tier: `GLM-5.2 (free, 128K context)` (paid tier is 1M).
- Add bare tracking rows for two free models with no available benchmark coverage: `dots-studio/dots-3-note-preview:free`, `liquid/lfm-2.5-2.6b:free`.
- Re-verify and update a stale `notes.yaml` caveat on `nvidia/nemotron-nano-12b-v2-vl:free`.

## [1.14.19]

- Let Left/Right cycle the Filter overlay's Availability field too (previously only Space worked there), adding `tuiPreviousAvailability` alongside the existing `tuiNextAvailability` — closing the same gap the Settings overlay's Availability row already had fixed in v1.14.18, in the separate Filter-overlay code path.

## [1.14.18]

- Let Left/Right cycle the Settings overlay's Availability row (Right = forward any -> free -> paid, Left = backward), matching the same directional idiom the Layout row already uses one case below it.

## [1.14.17]

- Add Z.ai, MiniMax, Moonshot, and Tencent manufacturer icons for clearer provider identity.
- Add Xiaomi and NVIDIA manufacturer icons for clearer provider identity.
- Add contract coverage for manufacturer icon rendering and alignment.
- Make `x` close child TUI windows and exit the application from the main window.
- Add regression coverage for `x` handling across child TUI windows and the main window.
- Fix Settings overlay Availability row opening the Columns editor instead of cycling availability.

## [1.14.16]

- Stop Left arrow from quitting the TUI's bare main view; it shared the close-overlay-or-quit keymap entry with Esc/h.

## [1.14.15]

- Trust `model-map.tsv`'s vals.ai/swebench.com mapping for identity classification instead of requiring an exact slug match, so cross-namespace mappings (Kimi, GLM, Grok, and similar) are no longer silently excluded from ranking.
- Prefer the independent vals.ai measurement over the self-submitted swebench.com leaderboard, which now takes the median across scaffold submissions instead of the max.
- Fall through to the next-priority source when the top-priority source's row fails identity classification.

## [1.14.14]

- Make `x` exit the main TUI and close child or modal windows.
- Add regression coverage for `x` handling, including the Cyrillic `ч` key.

## [1.14.11]

- Use terminal-compatible display widths for variation-selector manufacturer icons while preserving a two-column icon slot.
- Add CLI and TUI alignment contract coverage across terminal widths and configured icon variants.

## [1.14.10]

- Align manufacturer icons and model table columns consistently in the CLI and TUI.
- Add contract coverage for display-width alignment across table and configuration paths.

## [1.14.4]

- Normalize manufacturer icon spacing and display width for consistent table alignment.

## [1.14.3]

- Add a Meta manufacturer icon for clearer provider identity.
- Add configurable `table.name_width` support for CLI and TUI model tables.

## [1.14.0]

- Сделать заголовки и значения таблицы моделей однозначнее для score, Q/P, контекста и цен.
- Сохранить поисковый запрос TUI между обновлениями, показывать число совпадений и корректно очищать поиск.
- Исправить limit и разделитель платных и бесплатных моделей в режиме `top-paid-free`.
- Перестроить экран деталей модели: сначала task fit, цены и оценки, затем provenance, описание и заметки.

## [1.13.41]

- Add selected emoji manufacturer icons for clearer provider identity in the CLI, TUI, and model detail views.
- Use width-safe rendering so manufacturer icons remain correctly aligned across terminal widths.

## [1.13.40]

- Add individual ASCII manufacturer icons for clearer provider identity.
- Verify the Arena identity fallback when manufacturer metadata is incomplete.
- Display manufacturer icons safely in the CLI, TUI, and model detail views.

## [1.13.39]

- Correct the Arena manufacturer/provider fallback so provider identity remains accurate when manufacturer metadata is incomplete.
- Add ASCII producer badges to the CLI, TUI, and model detail views.
- Show pricing history in the model detail view.
- Move Task fit to the beginning of the model detail view.

## [1.13.38]

- Redesign the model detail view with grouped sections, aligned metadata cells, and a calmer color palette for clearer scanning.
- Preserve the existing narrow-terminal behavior so the detail view remains usable in constrained terminal widths.

## [1.13.37]

- Add less-style help search with `n` for next and `N` for previous, match counts and zero-state feedback, current-match highlighting, and a discoverable `v` shortcut.

## [1.13.36]

- Synchronize with the remote local-only release transition.
- Preserve release packaging without the macOS-only LaunchAgent prerequisite for Linux archives.

## [1.13.35]

- Keep the macOS-only LaunchAgent check from blocking the Linux release archive build.
- Preserve the separate LaunchAgent check.

## [1.13.34]

- Fully remove the GitHub Actions workflows.
- Keep release packaging and publication local-only.

## [1.13.33]

- Remove GitHub Actions workflows from the release process.
- Switch releases to a local-only workflow.

## [1.13.29]

- Preserve benchmark provenance and source-family-aware product identity, keeping mismatched, missing, legacy, and observation-only scores out of quality, Q/P, filters, and ranking.
- Treat Arena model keys as a separate namespace and reject ambiguous or cross-namespace identity matches while retaining raw Elo metadata.

## [1.13.31]

- Migrate empty and legacy persisted TUI filters to the configured default while preserving custom filters and explicit CLI overrides.
- Show the effective default filter in the TUI editor and remove persisted overrides when the filter is cleared.

## [1.13.30]

- Complete Q/P and availability metadata for valid Arena rows, including English `n/a` labels and paid/free defaults.
- Add tri-state paid/free filtering with the `p` hotkey, top paid/free layout with the `v` hotkey, and configurable `top_n` defaults.
- Unify filter, sort, search, and limit processing and persist configuration changes with runtime reload support.

## [1.13.28]

- Preserve model detail metadata and provenance in refreshed snapshots and expose it in the model detail view.
- Canonicalize ranking utility and displayed Q/P as `full_utility / mixed_price`, with price-independent base quality and provenance-aware metadata.

## [1.13.27]

- Treat persisted zero context and input/output predicates as `(any)` in the TUI filter editor and omit them when saving the filter.
- Add regression coverage for zero context and input/output predicates in TUI filter parsing and serialization.

## [1.13.26]

- Make `Space` the default main-screen TUI score-source switch and share the source-switch path with Settings.
- Include the runtime/build version in compact and full interactive help.

## [1.13.25]

- Add configurable TUI key bindings through `tui_keymap`, including validation and help rendering.
- Show pending and error states while switching the TUI score source from the local snapshot.

## [1.13.24]

- Treat persisted zero context and input/output predicates as `(any)` in the TUI filter editor and omit them when saving the filter.
- Add regression coverage for zero context and input/output predicates in TUI filter parsing and serialization.

## [1.13.22]

- Correct TUI help so `Enter / Right / l` opens model details and `Space` switches the score source in Settings.

## [1.13.21]

- Group TUI help hotkeys by navigation, data/view, filters/settings, task-fit codes, and general help.
- Keep one action per help row and make help columns responsive on narrow terminals.
- Document and test switching the score source between SWE-bench and Arena.
- Add regression coverage for grouped hotkeys, single-action rows, and responsive help columns.

## [1.13.20]

- Add a macOS LaunchAgent refresh workflow and deterministic local release packaging for the LaunchAgent and cron runtime scripts.

## [1.13.19]

- Add a macOS LaunchAgent refresh workflow with a 15-minute `StartInterval=900` schedule and idempotent Make targets.
- Package the LaunchAgent and cron runtime scripts in deterministic local release archives while keeping the LaunchAgent test script out of runtime packages.
- Document LaunchAgent installation, status, manual start, uninstall, and local release packaging.

## [1.13.18]

- Treat persisted zero context and input/output predicates as `(any)` in the TUI filter editor and omit them when saving the filter.
- Add regression coverage for zero context and input/output predicates in TUI filter parsing and serialization.

## [1.13.17]

- Treat persisted zero context and input/output predicates as `(any)` in the TUI filter editor and omit them when saving the filter.

## [1.13.16]

- Open the TUI filter form with `(any)` for unset numeric fields, while preserving explicit YAML, saved, and CLI filters.
- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Change default TUI Input/Output price steps to $0.05 while keeping YAML overrides.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.15]

- Open the TUI filter form with `(any)` for unset numeric fields, while preserving explicit YAML, saved, and CLI filters.
- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Change default TUI Input/Output price steps to $0.05 while keeping YAML overrides.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.14]

- Open the TUI filter form with `(any)` for unset numeric fields, while preserving explicit YAML, saved, and CLI filters.
- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Change default TUI Input/Output price steps to $0.05 while keeping YAML overrides.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.13]

- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Change default TUI Input/Output price steps to $0.05 while keeping YAML overrides.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.12]

- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Change default TUI Input/Output price steps to $0.05 while keeping YAML overrides.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.11]

- Canonicalize TUI numeric filter values: integer quality/context fields and
  two-decimal input/output prices, including configured absolute step sizes.
- Preserve legacy percentage-based `tui_steps` configs while rejecting mixed
  legacy and new schemas, and document the new numeric configuration contract.

## [1.13.10]

- Add configurable `tui_steps` integer display and numeric step values for the TUI filter form.
- Preserve up/down field navigation, left/right tier navigation and left/right numeric steppers in the TUI filter form.
- Add config support for `cache.dir`, `cache.ttl`, `cache.request_timeout`, table/TUI preferences and limits, and mixed-utility ranking settings.
- Resolve relative cache paths against `data_dir`, preserve CLI-over-config precedence, and handle zero durations as disabled settings.
- Improve `init` and cache path handling, and document the new configuration options and limits.
- Add regression and integration coverage for configuration, cache, refresh, table and TUI behavior.

## [1.13.9]

- Add up/down cursor navigation, left/right tier selection, and left/right numeric steppers to the TUI filter form.

## [1.13.8]

- Maintenance release with no functional changes since 1.13.7.

## [1.13.5]

- Add left and right arrow navigation to the TUI tier picker.

## [1.13.7]

- Add left and right arrow navigation to the TUI tier picker.

## [1.13.2]

- Harden local release archive checksums and manifests across supported platforms.
## [1.13.1]

- Add a tier picker to the TUI filter form using the canonical tier whitelist.

## [1.12.1]

- Maintenance release with no functional changes since 1.12.0.

## [1.12.0]

- Accept quality filter values in both `0..1` and `0..100` ranges.
- Support comma-separated values in CLI filters.
- Expand filter help and status information.
- Add a structured TUI filter form with persistence.

## [1.11.1]

- Maintenance release with no functional changes since 1.11.0.

## [1.11.0]

- Add a dedicated Filter form to the TUI for editing structured filters with checkbox and numeric fields, including apply, cancel and clear actions.
- Persist the active TUI filter in the configuration and restore it when reopening the filter form.
- Rework the TUI help layout into aligned shortcut columns and document filter-form navigation, help search and keyboard shortcuts.

## [1.10.1]

- Maintenance release with no functional changes since 1.10.0.

## [1.10.0]

- Add a Settings overlay to the TUI for switching ranking and score source, editing the structured filter and reviewing selected columns; score-source changes use the local snapshot without a network request.
- Add the `o` Settings shortcut, including its Russian keyboard-layout alias, and keep the active filter, ranking and score-source state visible in the overlay.
- Update the generated model comparison with current catalogue prices, contexts, model coverage and quality/price calculations.

## [1.9.0]

- Highlight all visible help-search matches in the TUI while preserving display case, layout, header styling and the unstyled input/footer lines.

## [1.7.0]

- Show the help search query while it is being typed: `/` inside the TUI help now draws a live `/ <query>_` input line above the help footer, exactly like the model list's own search line, and the scrollable help text gives up exactly one row while it is shown.

## [1.6.0]

- Make every single-character TUI hotkey work under the Russian keyboard layout: the pressed key is resolved through an explicit, code-defined character alias table (no `locale`/`LANG` lookup, no OS keyboard query, no extended keyboard protocol), while the built-in help stays English-only.
- Show the model's OpenRouter page link on the TUI detail screen, built from the catalogue's `canonical_slug`, plus a HuggingFace repository link for the models that have one.
- Colour-code the TUI detail screen: screen title, block headings, field labels, links and the `н/д` placeholder. Styling is applied strictly after layout, so scrolling, wrapping and truncation are unchanged.
- Carry the OpenRouter catalogue's `canonical_slug` and `hugging_face_id` through the pipeline and the run snapshot, including the degraded path where the catalogue fetch fails, with no additional HTTP request and no new cache entry.

## [1.5.0]

- Add cosign-based release signing and provenance verification (static key pair, no Fulcio/Rekor/transparency log): `make sign`, `make attest`, `make verify-provenance`.
- Add a `permissions:` block and pin GitHub Actions to immutable commit SHAs in CI.
- Fix README documenting the wrong TUI refresh hotkey (`r` instead of the actual `R`).
- Install `govulncheck`/`osv-scanner` in the release CI job and surface `dependency-check` blockers instead of a bare exit code.

## [1.4.0]

- Add a full-screen model detail view to `openrouter tui` (`Enter`/`→`/`l` to open, `Esc`/`←`/`h` to close): release date, owner, tier, context, full pricing including the long-context tier, both score sources in separate labelled blocks, task fit, note and the vendor description, word-wrapped instead of truncated.
- Carry the OpenRouter catalogue's `created` and `description` fields through the pipeline and the run snapshot, with no additional HTTP request and no new cache entry.

## [1.3.0]

- Extend `model-map.tsv` with 23 more manual model declarations (SWE-bench Verified sources), cutting the local table's `н/д` Status count from 34/56 to 24/56.
- Add an LMSYS Arena score source with the `--score-source=swebench|arena` flag on `table` and `tui`, an independent Elo-based view (`arena.ai/leaderboard/text`) that never blends with SWE-bench Verified numbers; the generated `docs/openrouter-model-comparison.md` stays SWE-bench-only.
- Fix stale "no score exists" notes in `notes.yaml` for models now covered by live SWE-bench sources, and drop the resulting dead `no_score_reason` field.

## [1.2.3]

- Add a configurable ranking formula for tuning the balance between model quality and price.
- Add local read-only release verification for the stable formula, installed package, release tag, and CLI checks without reinstalling or publishing.

## [1.2.1]

- Make `verify-release` a read-only check of the local stable Homebrew formula and installed package.
- Verify the exact release tag, immutable commit, formula metadata, installed version, CLI versions, and `brew test` without reinstalling or publishing.
- Keep external GitHub publication, signing, and provenance verification explicitly outside the repository.

## [1.2.0]

- Make mixed-utility ranking the default for table and TUI output, with tier-aware ordering.
- Add configurable `ranking.mixed_utility.price_weight` for balancing quality and price.
- Preserve legacy Q/P and tier-priority ranking modes through explicit `--ranking` values.
