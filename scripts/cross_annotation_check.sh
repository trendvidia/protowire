#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-repo annotation extension-surface check.
#
# The canonical .proto files under proto/ are hand-copied into every port
# and into several downstream repos. Nothing else verifies the copies, and
# they have drifted before: (pxf.key) = 50002 was missing from eight of ten
# copies when this check was written (issue #243).
#
# For each canonical file this compares the SEMANTIC extension surface —
# the set of (extendee, type, name, number) tuples — against every vendored
# copy. Comments, license headers, ordering and whitespace are ignored by
# construction: every existing copy already differs in those, and none of
# those differences can change what a compiler does.
#
# A missing copy is reported and skipped, not failed: not every repo tracks
# every canonical file (protolsp carries pxf but not sbe). Set the matching
# WITH_* to 0 to skip a repo whose checkout is absent or stale.
#
# Exit status is 1 if any present copy diverges. Divergence means a port
# cannot compile a schema the canonical surface permits — or worse, assigns
# a different number to the same name.
#
#
# SOURCE=local (default) reads sibling checkouts next to this repo.
# SOURCE=remote fetches each copy from GitHub at HEAD, so the check runs in
# CI without checking out twelve repos. Private repos (protolsp, chameleon)
# are SKIPped in remote mode unless GH_TOKEN grants access — CI therefore
# gates the public ports, and a local run before a release covers the rest. Both modes compare against this working tree's
# canonical files, which is the point: a PR that changes proto/ sees, in
# that PR, every copy it is about to invalidate.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"

SOURCE="${SOURCE:-local}"

# Known divergences, waived individually so CI is red on anything NEW.
#
# EMPTY, deliberately. The list previously held ten copies missing
# (pxf.key) = 50002; that gap is subsumed by the 1314-1363 renumber
# (#244), since every copy gains `key` when it is renumbered.
#
# During the renumber the whole surface diverges by design, and that is
# handled by continue-on-error on the CI job, not by waivers -- one
# mechanism, not two. Emptying this list is half the completion criterion:
# the migration is done when the gate passes STRICT, green and unwaived
# with continue-on-error removed.
#
# Add an entry only for a divergence that is deliberate and permanent,
# with a comment saying why. Set STRICT=1 to ignore waivers entirely.
KNOWN_DIVERGENCES=""
STRICT="${STRICT:-0}"

# is_waived REPO TUPLE
is_waived() {
  [[ "$STRICT" == "1" ]] && return 1
  grep -qxF "$(printf '%-20s %s' "$1" "$2")" <<<"$KNOWN_DIVERGENCES"
}


# Repos that may carry a vendored copy. Only the REPO is listed -- the paths
# inside it are DISCOVERED, because a hand-maintained path list silently
# misses copies, which is the exact failure this check exists to catch.
#
# It missed one: protocheck/testdata/pxf/annotations.proto was absent from
# the original manifest, so the 1314-1363 renumber broke protocheck's keyed
# tests with the gate green (trendvidia/protowire#244). Forgetting a whole
# repo is at least visible in this list; forgetting a path inside one was not.
REPOS=(
  protowire-go protowire-cpp protowire-csharp protowire-dart
  protowire-rust protowire-swift protowire-typescript protowire-java
  protocheck protocompile protolsp chameleon
)
PRIVATE_REPOS=" protolsp chameleon "

# Two repos were listed here and are deliberately NOT:
#
#   steward        carries no extension numbers of its own. Its only copy is
#                  node_modules/@trendvidia/protowire/proto/..., an npm install
#                  artifact -- gitignored, zero files tracked. Checking it
#                  reports whatever a developer last installed, so it could go
#                  red or green for reasons unrelated to any repo's content.
#                  steward picks the numbers up by bumping its dependency on
#                  the published @trendvidia/protowire package, which is built
#                  from protowire-typescript -- already in the list above.
#
#   org-protowire  is an archive preserving pre-open-source history. It is
#                  SUPPOSED to hold the retired numbers; flagging it as drift
#                  flags it for being correct.

# A discovered path is classified by the family it belongs to.
#   */pxf/annotations.proto        -> PXF
#   */sbe/annotations.proto        -> SBE
#   */schema/v1/descriptor.proto   -> CARRIERS
classify() {
  case "$1" in
    */pxf/annotations.proto)      echo PXF ;;
    */sbe/annotations.proto)      echo SBE ;;
    */schema/v1/descriptor.proto) echo CARRIERS ;;
    *)                            echo "" ;;
  esac
}

# discover_local REPO -> paths relative to SIBLING_DIR
discover_local() {
  local repo="$1" root="${SIBLING_DIR}/$1"
  [[ -d "$root" ]] || return 0
  find "$root" \( -name annotations.proto -o -name descriptor.proto \) \
       -not -path '*/.git/*' -not -path '*/build/*' -not -path '*/.build/*' \
       -not -path '*/.tmp/*' 2>/dev/null \
    | while read -r f; do
        [[ -n "$(classify "$f")" ]] && echo "${f#$SIBLING_DIR/}"
      done
}

# discover_remote REPO -> paths relative to the repo root
discover_remote() {
  local repo="$1"
  if [[ " $PRIVATE_REPOS " == *" $repo "* && -z "${GH_TOKEN:-}${GITHUB_TOKEN:-}" ]]; then
    return 0
  fi
  local branch
  branch="$(gh api "repos/trendvidia/${repo}" --jq .default_branch 2>/dev/null)" || return 0
  gh api "repos/trendvidia/${repo}/git/trees/${branch}?recursive=1" \
      --jq '.tree[] | select(.type=="blob") | .path' 2>/dev/null \
    | while read -r path; do
        case "$path" in */build/*|*/.build/*|*/.tmp/*) continue ;; esac
        [[ -n "$(classify "$path")" ]] && echo "$path"
      done
}

FETCH_TMP=""
# Must end in a success: an EXIT trap's last status replaces the
# script's own, which silently turned exit 0 into exit 1.
cleanup() { [[ -n "$FETCH_TMP" ]] && rm -rf "$FETCH_TMP"; return 0; }
trap cleanup EXIT

# resolve_copy REL_PATH -> prints a readable local path, or nothing if the
# copy is unavailable (missing locally, or private/absent remotely).
resolve_copy() {
  local rel="$1" repo="${1%%/*}" sub="${1#*/}"
  if [[ "$SOURCE" == "local" ]]; then
    local p="${SIBLING_DIR}/${rel}"
    [[ -f "$p" ]] && echo "$p"
    return 0
  fi
  if [[ " $PRIVATE_REPOS " == *" $repo "* && -z "${GH_TOKEN:-}" ]]; then
    return 0
  fi
  [[ -z "$FETCH_TMP" ]] && FETCH_TMP="$(mktemp -d)"
  local dest="${FETCH_TMP}/${rel//\//_}"
  if gh api "repos/trendvidia/${repo}/contents/${sub}" \
       -H "Accept: application/vnd.github.raw" > "$dest" 2>/dev/null; then
    echo "$dest"
  fi
  return 0
}

# extract_surface FILE
# Prints one "extendee|type|name|number" line per extension field, sorted.
# Strips // and /* */ comments before parsing so a number mentioned in prose
# is never mistaken for a declaration.
extract_surface() {
  python3 - "$1" <<'PY'
import re, sys

src = open(sys.argv[1], encoding="utf-8", errors="replace").read()
src = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
src = re.sub(r"//[^\n]*", "", src)

out = []
# extend <FQN> { ... } — brace-matched so nested message literals in
# option values can't terminate the block early.
for m in re.finditer(r"\bextend\s+([\w.]+)\s*\{", src):
    extendee = m.group(1)
    depth, i = 1, m.end()
    while i < len(src) and depth:
        if src[i] == "{": depth += 1
        elif src[i] == "}": depth -= 1
        i += 1
    body = src[m.end():i-1]
    # [optional|repeated|required] <type> <name> = <number>;
    for f in re.finditer(
            r"(?:(optional|repeated|required)\s+)?([\w.]+)\s+(\w+)\s*=\s*(\d+)\s*(?:\[[^\]]*\])?\s*;",
            body):
        out.append(f"{extendee}|{f.group(2)}|{f.group(3)}|{f.group(4)}")

print("\n".join(sorted(out)))
PY
}

canonical_for() {
  case "$1" in
    PXF)      echo "${REPO_DIR}/proto/pxf/annotations.proto" ;;
    SBE)      echo "${REPO_DIR}/proto/sbe/annotations.proto" ;;
    CARRIERS) echo "${REPO_DIR}/proto/schema/v1/descriptor.proto" ;;
  esac
}

declare -A CANON_SURFACE
for fam in PXF SBE CARRIERS; do
  cf="$(canonical_for "$fam")"
  if [[ ! -f "$cf" ]]; then
    echo "FATAL: canonical $fam file not found: $cf" >&2; exit 1
  fi
  CANON_SURFACE[$fam]="$(extract_surface "$cf")"
  echo "=== $fam canonical: ${cf#$SIBLING_DIR/}  [source: $SOURCE]"
  echo "${CANON_SURFACE[$fam]}" | sed 's/^/    /'
  echo
done

status=0
found_any=0

for repo in "${REPOS[@]}"; do
  if [[ "$SOURCE" == "local" ]]; then
    mapfile -t paths < <(discover_local "$repo")
  else
    mapfile -t paths < <(discover_remote "$repo")
  fi

  if [[ ${#paths[@]} -eq 0 ]]; then
    printf "  %-22s SKIP (no copies visible in %s mode)\n" "$repo" "$SOURCE"
    continue
  fi

  for rel in "${paths[@]}"; do
    [[ -z "$rel" ]] && continue
    fam="$(classify "$rel")"
    [[ -z "$fam" ]] && continue
    found_any=1

    if [[ "$SOURCE" == "local" ]]; then
      path="${SIBLING_DIR}/${rel}"
    else
      [[ -z "$FETCH_TMP" ]] && FETCH_TMP="$(mktemp -d)"
      path="${FETCH_TMP}/${repo}_${rel//\//_}"
      gh api "repos/trendvidia/${repo}/contents/${rel}" \
         -H "Accept: application/vnd.github.raw" > "$path" 2>/dev/null || continue
    fi
    [[ -f "$path" ]] || continue

    copy_surface="$(extract_surface "$path")"
    label="${repo} (${fam})"
    if [[ "$copy_surface" == "${CANON_SURFACE[$fam]}" ]]; then
      printf "  %-34s ok\n" "$label"
      continue
    fi

    unwaived=0
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      if is_waived "$repo" "$line"; then
        printf "  %-34s waived  missing: %s\n" "$label" "$line"
      else
        printf "  %-34s DIVERGED missing from copy: %s\n" "$label" "$line"
        unwaived=1
      fi
    done < <(comm -23 <(echo "${CANON_SURFACE[$fam]}") <(echo "$copy_surface"))

    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      printf "  %-34s DIVERGED unexpected in copy: %s\n" "$label" "$line"
      unwaived=1
    done < <(comm -13 <(echo "${CANON_SURFACE[$fam]}") <(echo "$copy_surface"))

    [[ $unwaived -eq 1 ]] && status=1
  done
done
echo

if [[ $found_any -eq 0 ]]; then
  echo "FATAL: discovery found no vendored copies at all -- the check is not" >&2
  echo "actually looking at anything. Treat as a failure, not a pass." >&2
  exit 1
fi

if [[ $status -ne 0 ]]; then
  cat >&2 <<'MSG'
FAIL: at least one vendored copy diverges from its canonical file.

A copy missing a declaration cannot compile a schema that uses it. A copy
with a different number for the same name is a wire split — fix that first
and treat any descriptor emitted from it as suspect.

Update the copy from the canonical file (comments may differ; the tuples
above may not), or record the omission deliberately per issue #243.
MSG
else
  if [[ "$STRICT" != "1" ]] && grep -q '[^[:space:]]' <<<"$KNOWN_DIVERGENCES"; then
    echo "OK: no new drift. $(grep -c '[^[:space:]]' <<<"$KNOWN_DIVERGENCES") waived divergence(s) remain — see #243, cleared by #244."
  else
    echo "OK: every present copy matches its canonical extension surface."
  fi
fi
exit $status
