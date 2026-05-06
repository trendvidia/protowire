#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Rebuild the protowire-typescript package and copy the npm pack tarball into
# the VS Code extension's bundled-libs directory. Run after any change to
# protowire-typescript's parser code:
#
#     bash scripts/refresh_vscode_parser_pkg.sh
#     cd editors/vscode && npm install && npm run package
#
# TEMPORARY: this exists only because protowire-typescript is not yet
# published to the npm registry. Once it is, the VS Code extension will
# switch to a regular `"@trendvidia/protowire": "<version>"` dependency
# and this script + the libs/ directory go away. Tracked as the "phase 2
# packaging" refactor; see editors/vscode/package.json (TODO comment).

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TS_DIR="$(dirname "$REPO_DIR")/protowire-typescript"
LIBS_DIR="$REPO_DIR/editors/vscode/libs"
DST="$LIBS_DIR/trendvidia-protowire.tgz"

if [[ ! -d "$TS_DIR" ]]; then
  echo "error: expected sibling protowire-typescript repo at $TS_DIR" >&2
  exit 1
fi

echo "→ installing + building protowire-typescript"
(cd "$TS_DIR" && npm install --silent && npm run build --silent)

mkdir -p "$LIBS_DIR"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "→ npm pack"
(cd "$TS_DIR" && npm pack --pack-destination "$TMP_DIR" --silent)

# Strip the version suffix (trendvidia-protowire-X.Y.Z.tgz → trendvidia-protowire.tgz)
# so the package.json file: reference stays stable across version bumps.
SRC="$(ls "$TMP_DIR"/trendvidia-protowire-*.tgz | head -1)"
mv "$SRC" "$DST"

SIZE="$(wc -c < "$DST" | tr -d ' ')"
echo "→ wrote $DST ($SIZE bytes)"
