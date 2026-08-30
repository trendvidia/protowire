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
# Set WITH_CHAMELEON=0 / WITH_ORG=0 / WITH_STEWARD=0 to skip the downstream
# consumers, which are not ports and may legitimately lag a release.
#
# SOURCE=local (default) reads sibling checkouts next to this repo.
# SOURCE=remote fetches each copy from GitHub at HEAD, so the check runs in
# CI without checking out eleven repos. Private repos (protolsp, chameleon,
# steward, org-protowire) are SKIPped in remote mode unless GH_TOKEN grants
# access — CI therefore gates the public ports, and a local run before a
# release covers the rest. Both modes compare against this working tree's
# canonical files, which is the point: a PR that changes proto/ sees, in
# that PR, every copy it is about to invalidate.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"

SOURCE="${SOURCE:-local}"

# Known divergences, waived so CI is green on drift that already existed
# and RED on anything new. Each line is "<repo> <extendee>|<type>|<name>|<number>".
#
# These exist because (pxf.key) shipped in protowire-go and the canonical
# file (v1.2, issue #116) but was never copied outward — the drift this
# script was written to catch, recorded rather than hidden (#243).
#
# Every one of these copies is touched by the 1314-1363 renumber (#244), so
# this list is expected to reach zero there. Deleting an entry is how a fix
# is proven: remove the line, run the check, watch it pass.
# Set STRICT=1 to ignore the waivers and see the true state.
KNOWN_DIVERGENCES="$(cat <<'WAIVED'
protowire-cpp        google.protobuf.FieldOptions|string|key|50002
protowire-csharp     google.protobuf.FieldOptions|string|key|50002
protowire-dart       google.protobuf.FieldOptions|string|key|50002
protowire-rust       google.protobuf.FieldOptions|string|key|50002
protowire-swift      google.protobuf.FieldOptions|string|key|50002
protowire-typescript google.protobuf.FieldOptions|string|key|50002
protowire-java       google.protobuf.FieldOptions|string|key|50002
chameleon            google.protobuf.FieldOptions|string|key|50002
org-protowire        google.protobuf.FieldOptions|string|key|50002
steward              google.protobuf.FieldOptions|string|key|50002
WAIVED
)"
STRICT="${STRICT:-0}"

# is_waived REPO TUPLE
is_waived() {
  [[ "$STRICT" == "1" ]] && return 1
  grep -qxF "$(printf '%-20s %s' "$1" "$2")" <<<"$KNOWN_DIVERGENCES"
}
PRIVATE_REPOS=" protolsp chameleon steward org-protowire "

WITH_CHAMELEON="${WITH_CHAMELEON:-1}"
WITH_ORG="${WITH_ORG:-1}"
WITH_STEWARD="${WITH_STEWARD:-1}"

# canonical-relative-path : copy-path-relative-to-SIBLING_DIR ...
# Kept as a flat table so adding a port is one line, and so the set of
# places a number lives is readable in one screen.
PXF_COPIES=(
  "protolsp/proto/pxf/annotations.proto"
  "protowire-cpp/proto/pxf/annotations.proto"
  "protowire-csharp/proto/pxf/annotations.proto"
  "protowire-dart/proto/pxf/annotations.proto"
  "protowire-rust/proto/pxf/annotations.proto"
  "protowire-swift/proto/pxf/annotations.proto"
  "protowire-typescript/proto/pxf/annotations.proto"
  "protowire-java/proto-annotations/src/main/proto/pxf/annotations.proto"
)
SBE_COPIES=(
  "protowire-cpp/proto/sbe/annotations.proto"
  "protowire-csharp/proto/sbe/annotations.proto"
  "protowire-dart/proto/sbe/annotations.proto"
  "protowire-rust/proto/sbe/annotations.proto"
  "protowire-swift/proto/sbe/annotations.proto"
  "protowire-typescript/proto/sbe/annotations.proto"
  "protowire-java/proto-annotations/src/main/proto/sbe/annotations.proto"
)
CARRIER_COPIES=(
  "protocompile/proto/protowire/schema/v1/descriptor.proto"
)

[[ "$WITH_CHAMELEON" == "1" ]] && PXF_COPIES+=("chameleon/proto/pxf/annotations.proto")
[[ "$WITH_ORG" == "1" ]] && PXF_COPIES+=("org-protowire/proto/pxf/annotations.proto")
[[ "$WITH_ORG" == "1" ]] && SBE_COPIES+=("org-protowire/proto/sbe/annotations.proto")
[[ "$WITH_STEWARD" == "1" ]] && PXF_COPIES+=("steward/node_modules/@trendvidia/protowire/proto/pxf/annotations.proto")
[[ "$WITH_STEWARD" == "1" ]] && SBE_COPIES+=("steward/node_modules/@trendvidia/protowire/proto/sbe/annotations.proto")

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
  # Remote: steward vendors via npm, so its copy has no repo path of its own.
  [[ "$repo" == "steward" ]] && return 0
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

check_family() {
  local label="$1" canonical="$2"; shift 2
  local copies=("$@")
  local canon_surface rc=0

  if [[ ! -f "$canonical" ]]; then
    echo "FATAL: canonical $label file not found: $canonical" >&2
    return 1
  fi
  canon_surface="$(extract_surface "$canonical")"

  echo "=== $label — canonical: ${canonical#$SIBLING_DIR/}  [source: $SOURCE]"
  echo "$canon_surface" | sed 's/^/    /'
  echo

  for rel in "${copies[@]}"; do
    local repo="${rel%%/*}"
    local path
    path="$(resolve_copy "$rel")"
    if [[ -z "$path" ]]; then
      printf "  %-24s SKIP (unavailable in %s mode: %s)\n" "$repo" "$SOURCE" "$rel"
      continue
    fi
    local copy_surface
    copy_surface="$(extract_surface "$path")"
    if [[ "$copy_surface" == "$canon_surface" ]]; then
      printf "  %-24s ok\n" "$repo"
      continue
    fi

    # Only-in-canonical => the copy is missing a declaration.
    # Only-in-copy      => the copy invented or kept a stale one.
    local unwaived=0 line
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      if is_waived "$repo" "$line"; then
        printf "  %-24s waived  missing: %s\n" "$repo" "$line"
      else
        printf "  %-24s DIVERGED missing from copy: %s\n" "$repo" "$line"
        unwaived=1
      fi
    done < <(comm -23 <(echo "$canon_surface") <(echo "$copy_surface"))

    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      printf "  %-24s DIVERGED unexpected in copy: %s\n" "$repo" "$line"
      unwaived=1
    done < <(comm -13 <(echo "$canon_surface") <(echo "$copy_surface"))

    [[ $unwaived -eq 1 ]] && rc=1
  done
  echo
  return $rc
}

status=0
check_family "PXF"      "${REPO_DIR}/proto/pxf/annotations.proto"        "${PXF_COPIES[@]}"     || status=1
check_family "SBE"      "${REPO_DIR}/proto/sbe/annotations.proto"        "${SBE_COPIES[@]}"     || status=1
check_family "CARRIERS" "${REPO_DIR}/proto/schema/v1/descriptor.proto"   "${CARRIER_COPIES[@]}" || status=1

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
