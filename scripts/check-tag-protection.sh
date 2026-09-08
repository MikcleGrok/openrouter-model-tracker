#!/usr/bin/env bash
# Repository tag-protection evidence gate (guide-tools 06-release.md: "ещё до
# tag `make release-check` MUST получить machine-readable evidence repository
# tag policy -- canonical remote, tag pattern, запрет force-update/delete,
# required permissions, результат проверки и timestamp ... Repository policy
# MUST запрещать force-update и delete release tags"). This is the gate's own
# job per 06-release.md, not a separately-run maintainer step, so it is wired
# into `make release-check` directly (Makefile).
#
# Deliberately NOT part of `make check`: that target MUST stay network-free
# and tool-free (Makefile's own "check is the guide-tools baseline gate"
# comment) and this script needs both the network and `gh`. `make sbom`
# (syft) and `make release-github-check` (jq, gh) already carry the same kind
# of release-path-only external dependency, so this is not a new precedent.
#
# Mechanism: GitHub repository rulesets (the current replacement for the
# deprecated classic tag-protection API), with a fallback to that legacy
# `tags/protection` endpoint when no ruleset exists yet -- 06-release.md
# leaves the exact mechanism to the platform, and rulesets are what GitHub
# itself now recommends. Every field this script cannot resolve is a hard
# BLOCKED, never a skip and never a declared-but-unverified pass -- a README
# declaration with no checkable source does not count as evidence
# (06-release.md).
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
evidence_dir="$root/.release"
evidence_file="$evidence_dir/tag-protection-evidence.json"
tag_pattern="refs/tags/v*"

fail() {
  printf 'BLOCKED: %s\n' "$1" >&2
  exit 1
}

command -v gh >/dev/null 2>&1 || fail "gh is required to verify repository tag protection"
command -v jq >/dev/null 2>&1 || fail "jq is required to verify repository tag protection"
gh auth status >/dev/null 2>&1 || fail "gh is not authenticated (run: gh auth login)"

expected_repo="${GITHUB_REPOSITORY:-MikcleGrok/openrouter-model-tracker}"
remote_url="$(git -C "$root" remote get-url origin 2>/dev/null)" || fail "cannot resolve git remote 'origin'"
canonical_remote="$(printf '%s' "$remote_url" | sed -E 's#^git@github\.com:##; s#^https://github\.com/##; s#\.git$##')"
test -n "$canonical_remote" || fail "could not normalize origin remote URL into an owner/repo: $remote_url"
if [[ "$canonical_remote" != "$expected_repo" ]]; then
  fail "origin remote ($canonical_remote) does not match GITHUB_REPOSITORY ($expected_repo)"
fi

rulesets_json="$(gh api "repos/$canonical_remote/rulesets" 2>/dev/null)" || fail "could not fetch repository rulesets from the GitHub API (network/auth/permissions)"
printf '%s' "$rulesets_json" | jq -e . >/dev/null 2>&1 || fail "GitHub API returned unparseable JSON for repository rulesets"

tag_ruleset_ids="$(printf '%s' "$rulesets_json" | jq -r '.[] | select(.target == "tag") | .id')"

forbids_deletion=false
forbids_force_update=false
required_permissions="[]"
matched_id=""

if [[ -n "$tag_ruleset_ids" ]]; then
  while IFS= read -r id; do
    [[ -n "$id" ]] || continue
    detail="$(gh api "repos/$canonical_remote/rulesets/$id" 2>/dev/null)" || fail "could not fetch detail for ruleset $id"
    printf '%s' "$detail" | jq -e . >/dev/null 2>&1 || fail "GitHub API returned unparseable JSON for ruleset $id"

    enforcement="$(printf '%s' "$detail" | jq -r '.enforcement // empty')"
    test -n "$enforcement" || fail "ruleset $id has no resolvable enforcement field"
    [[ "$enforcement" == "active" ]] || continue

    covers_pattern="$(printf '%s' "$detail" | jq -r --arg pattern "$tag_pattern" \
      '[(.conditions.ref_name.include // [])[] | select(. == $pattern or . == "~ALL" or . == "refs/tags/*")] | length > 0')"
    [[ "$covers_pattern" == "true" ]] || continue

    has_deletion="$(printf '%s' "$detail" | jq -r '[.rules[]? | select(.type == "deletion")] | length > 0')"
    has_non_ff="$(printf '%s' "$detail" | jq -r '[.rules[]? | select(.type == "non_fast_forward")] | length > 0')"

    if [[ "$has_deletion" == "true" && "$has_non_ff" == "true" ]]; then
      matched_id="$id"
      forbids_deletion=true
      forbids_force_update=true
      required_permissions="$(printf '%s' "$detail" | jq -c '.bypass_actors // []')"
      break
    fi
  done <<<"$tag_ruleset_ids"
fi

if [[ -z "$matched_id" ]]; then
  # Fallback: the deprecated classic tag-protection API, for a repository
  # that predates rulesets and has not migrated. Unlike rulesets, this
  # endpoint has no per-rule deletion/non-fast-forward distinction -- its
  # mere existence and enabled state is the only signal it offers, so a
  # match here can only prove deletion protection; force-update protection
  # still requires a ruleset (this endpoint predates non-fast-forward
  # protection entirely) and MUST NOT be assumed.
  legacy_json="$(gh api "repos/$canonical_remote/tags/protection" 2>/dev/null)" || legacy_json=""
  if [[ -n "$legacy_json" ]] && printf '%s' "$legacy_json" | jq -e '. != null and (length > 0)' >/dev/null 2>&1; then
    fail "a legacy tag-protection record exists but this script cannot prove non-fast-forward (force-update) protection from it -- create a repository ruleset on $tag_pattern with 'deletion' and 'non_fast_forward' rules instead of relying on the legacy API"
  fi
  fail "no active repository ruleset protects $tag_pattern with both 'deletion' and 'non_fast_forward' rules, and no usable legacy tag-protection record exists -- create one (e.g. gh api repos/$canonical_remote/rulesets --method POST) before cutting a release tag"
fi

mkdir -p "$evidence_dir"
checked_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq -n \
  --arg canonical_remote "$canonical_remote" \
  --arg tag_pattern "$tag_pattern" \
  --argjson forbids_force_update "$forbids_force_update" \
  --argjson forbids_deletion "$forbids_deletion" \
  --argjson required_permissions "$required_permissions" \
  --arg result "pass" \
  --arg checked_at "$checked_at" \
  '{canonical_remote: $canonical_remote, tag_pattern: $tag_pattern, forbids_force_update: $forbids_force_update, forbids_deletion: $forbids_deletion, required_permissions: $required_permissions, result: $result, checked_at: $checked_at}' \
  >"$evidence_file"

printf 'PASS: repository tag policy for %s forbids force-update and deletion on %s (ruleset id %s); evidence written to %s\n' "$canonical_remote" "$tag_pattern" "$matched_id" "$evidence_file"
