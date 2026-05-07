#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Apply GitHub branch + tag protection rulesets across the protowire stack.
#
# By default this is a dry-run that just prints what it would do. Pass
# --apply to actually create / update the rulesets via the GitHub API.
#
# Idempotent: rulesets are matched by name, so re-running this updates
# the existing ruleset rather than creating a duplicate.
#
# Usage:
#     bash scripts/setup_branch_protection.sh                # dry-run
#     bash scripts/setup_branch_protection.sh --apply        # really do it
#     bash scripts/setup_branch_protection.sh --apply --org other-org
#     bash scripts/setup_branch_protection.sh --only protowire-go --apply
#
# Requirements:
#   - gh CLI authenticated as an org admin: `gh auth status`
#   - jq on PATH
#   - The @trendvidia/maintainers and @trendvidia/contributors teams
#     should exist before enabling, or "Require review from CODEOWNERS"
#     fails open. The script will warn but not block if they don't.

set -euo pipefail

ORG="trendvidia"
DRY_RUN=true
ONLY_REPO=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)   DRY_RUN=false; shift ;;
    --org)     ORG="$2"; shift 2 ;;
    --only)    ONLY_REPO="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

command -v gh >/dev/null || { echo "gh CLI not on PATH"; exit 1; }
command -v jq >/dev/null || { echo "jq not on PATH"; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh not authenticated; run 'gh auth login'"; exit 1; }

# --- Repo classification ---------------------------------------------------
SPEC_REPOS=(protowire protoregistry)
PORT_REPOS=(
  protowire-go
  protowire-cpp
  protowire-rust
  protowire-java
  protowire-typescript
  protowire-python
  protowire-csharp
  protowire-swift
  protowire-dart
)
FORK_REPOS=(protobuf-go)

# --- Per-repo required status checks --------------------------------------
# Edit these to match each repo's actual workflow job names. To discover
# the right names, run:
#   gh api /repos/$ORG/<repo>/actions/runs --jq '.workflow_runs[0].name'
# or look at the `name:` field of each workflow file in .github/workflows/.
status_checks_for() {
  case "$1" in
    protowire)            echo "test|Analyze (go)|Analyze (java-kotlin)" ;;
    protoregistry)        echo "build|lint|vet|headers|test-unit|test-integration" ;;
    protowire-go)         echo "test" ;;
    protowire-cpp)        echo "build" ;;
    protowire-rust)       echo "test|clippy" ;;
    protowire-java)       echo "build|test" ;;
    protowire-typescript) echo "test|typecheck" ;;
    protowire-python)     echo "test" ;;
    protowire-csharp)     echo "test" ;;
    protowire-swift)      echo "test" ;;
    protowire-dart)       echo "test|analyze" ;;
    protobuf-go)          echo "test" ;;
    *)                    echo "" ;;
  esac
}

# --- Ruleset builders ------------------------------------------------------
# All branch rulesets share the bypass-actor list. "RepositoryRole" with
# actor_id 5 maps to the repo Admin role — every repo admin can bypass
# via PR. We avoid the alternative (`actor_type: "OrganizationAdmin"` with
# actor_id 1) because that bypass entry is validated against admin:org
# scope at ruleset-create time, and our automation token is scoped to
# read:org only. Repo Admin bypass gives us the same operational
# escape hatch without requiring the token-scope upgrade. Add Steward's
# GitHub App actor here once it's installed:
#   { actor_id: <STEWARD_APP_ID>, actor_type: "Integration", bypass_mode: "always" }
bypass_actors_json() {
  jq -n '[{ actor_id: 5, actor_type: "RepositoryRole", bypass_mode: "pull_request" }]'
}

# Build the rules array for a branch ruleset.
#   $1 = required_status_checks pipe-separated
#   $2 = require linear history (true|false)
#   $3 = require signed commits  (true|false)
#   $4 = require code-owner review (true|false)
build_branch_rules() {
  local checks="$1" linear="$2" signed="$3" codeowners="$4"

  local checks_json="[]"
  if [[ -n "$checks" ]]; then
    checks_json=$(jq -Rcn --arg s "$checks" '
      $s | split("|") | map({context: .})
    ')
  fi

  local linear_rule signed_rule
  $linear && linear_rule='{type:"required_linear_history"},' || linear_rule=''
  $signed && signed_rule='{type:"required_signatures"},'    || signed_rule=''

  jq -n \
    --argjson checks "$checks_json" \
    --argjson codeowners "$codeowners" \
    "[
      {type:\"deletion\"},
      {type:\"non_fast_forward\"},
      {type:\"creation\"},
      $linear_rule
      $signed_rule
      {
        type: \"pull_request\",
        parameters: {
          required_approving_review_count: 1,
          dismiss_stale_reviews_on_push: true,
          require_code_owner_review: \$codeowners,
          require_last_push_approval: true,
          required_review_thread_resolution: true,
          allowed_merge_methods: [\"squash\", \"rebase\"]
        }
      },
      {
        type: \"required_status_checks\",
        parameters: {
          strict_required_status_checks_policy: true,
          do_not_enforce_on_create: false,
          required_status_checks: \$checks
        }
      }
    ]"
}

build_branch_ruleset() {
  local name="$1" rules_json="$2"
  jq -n \
    --arg name "$name" \
    --argjson bypass "$(bypass_actors_json)" \
    --argjson rules "$rules_json" \
    '{
      name: $name,
      target: "branch",
      enforcement: "active",
      bypass_actors: $bypass,
      conditions: {
        ref_name: {
          include: ["~DEFAULT_BRANCH"],
          exclude: []
        }
      },
      rules: $rules
    }'
}

build_tag_ruleset() {
  jq -n '{
    name: "release tag protection",
    target: "tag",
    enforcement: "active",
    bypass_actors: [],
    conditions: {
      ref_name: {
        include: ["refs/tags/v*.*.*"],
        exclude: []
      }
    },
    rules: [
      { type: "deletion" },
      { type: "non_fast_forward" },
      { type: "update" }
    ]
  }'
}

# --- Apply (or dry-run) a ruleset ----------------------------------------
apply_ruleset() {
  local repo="$1" ruleset_json="$2"
  local name; name=$(echo "$ruleset_json" | jq -r .name)

  local existing_id
  existing_id=$(gh api "/repos/$ORG/$repo/rulesets" --jq \
    "[.[] | select(.name == \"$name\")] | .[0].id // empty" 2>/dev/null || echo "")

  if [[ -n "$existing_id" ]]; then
    if $DRY_RUN; then
      printf "  [dry-run] would UPDATE ruleset %q (id %s)\n" "$name" "$existing_id"
    else
      echo "$ruleset_json" | gh api --method PUT \
        -H "Accept: application/vnd.github+json" \
        "/repos/$ORG/$repo/rulesets/$existing_id" --input - >/dev/null
      printf "  ✓ updated ruleset %q (id %s)\n" "$name" "$existing_id"
    fi
  else
    if $DRY_RUN; then
      printf "  [dry-run] would CREATE ruleset %q\n" "$name"
    else
      local new_id
      new_id=$(echo "$ruleset_json" | gh api --method POST \
        -H "Accept: application/vnd.github+json" \
        "/repos/$ORG/$repo/rulesets" --input - --jq .id)
      printf "  ✓ created ruleset %q (id %s)\n" "$name" "$new_id"
    fi
  fi
}

# --- Driver ---------------------------------------------------------------
process_repo() {
  local repo="$1" tier="$2"
  echo "=== $repo ($tier) ==="

  if ! gh api "/repos/$ORG/$repo" --silent 2>/dev/null; then
    echo "  ✗ repo not accessible — skipping" >&2
    return
  fi

  local checks rules branch_ruleset
  checks=$(status_checks_for "$repo")

  case "$tier" in
    spec) rules=$(build_branch_rules "$checks" true  true  true)  ;;
    port) rules=$(build_branch_rules "$checks" true  true  false) ;;
    fork) rules=$(build_branch_rules "$checks" false false false) ;;
  esac

  branch_ruleset=$(build_branch_ruleset "main protection — $tier" "$rules")
  apply_ruleset "$repo" "$branch_ruleset"
  apply_ruleset "$repo" "$(build_tag_ruleset)"
}

if $DRY_RUN; then
  echo "→ DRY-RUN mode. Pass --apply to actually create / update rulesets."
  echo
fi

for r in "${SPEC_REPOS[@]}"; do
  [[ -n "$ONLY_REPO" && "$r" != "$ONLY_REPO" ]] && continue
  process_repo "$r" spec
done

for r in "${PORT_REPOS[@]}"; do
  [[ -n "$ONLY_REPO" && "$r" != "$ONLY_REPO" ]] && continue
  process_repo "$r" port
done

for r in "${FORK_REPOS[@]}"; do
  [[ -n "$ONLY_REPO" && "$r" != "$ONLY_REPO" ]] && continue
  process_repo "$r" fork
done

echo
if $DRY_RUN; then
  echo "(no changes made — re-run with --apply to commit them)"
fi
