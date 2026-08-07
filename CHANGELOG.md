# What's New

## [1.2.1]

- Make `verify-release` a read-only check of the local stable Homebrew formula and installed package.
- Verify the exact release tag, immutable commit, formula metadata, installed version, CLI versions, and `brew test` without reinstalling or publishing.
- Keep external GitHub publication, signing, and provenance verification explicitly outside the repository.

## [1.2.0]

- Make mixed-utility ranking the default for table and TUI output, with tier-aware ordering.
- Add configurable `ranking.mixed_utility.price_weight` for balancing quality and price.
- Preserve legacy Q/P and tier-priority ranking modes through explicit `--ranking` values.

## [Unreleased]

- Continue documenting changes here before the next release.
