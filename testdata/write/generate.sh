#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Regenerates the write-gate goldens with protowire-go, the reference. Each
# input is a PXF document: every testdata/text/cases/*.expected.pxf, and every
# testdata/write/cases/*.pxf. For each, protowire-go's
# `check_decode --format pxf-pb` writes either the PB bytes, to
# golden/<name>.binpb, or its rejection, to golden/<name>.reject. GO_DIR points
# at a protowire-go checkout. A changed golden changes the bytes every port must
# write, so read the diff before committing it.

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GO_DIR="${GO_DIR:-$HERE/../../../protowire-go}"
TEXT="$HERE/../text"
bin="$(mktemp -t check-decode-go.XXXXXX)"
trap 'rm -f "$bin"' EXIT
(cd "$GO_DIR" && go build -o "$bin" ./scripts/check_decode)

rm -rf "$HERE/golden"
mkdir -p "$HERE/golden"

inputs=()
for f in "$TEXT"/cases/*.expected.pxf "$HERE"/cases/*.pxf; do inputs+=("$f"); done

for input in "${inputs[@]}"; do
  base="$(basename "$input")"
  name="${base%.expected.pxf}"
  name="${name%.pxf}"
  schema="text.v1.${name%%.*}"
  if [[ -e "$HERE/golden/$name.binpb" || -e "$HERE/golden/$name.reject" ]]; then
    echo "two inputs are named $name" >&2
    exit 2
  fi
  rc=0
  "$bin" --format pxf-pb --schema "$schema" --proto "$TEXT/text.proto" \
    --input "$input" > "$HERE/golden/$name.binpb" 2> "$HERE/golden/$name.err" || rc=$?
  case $rc in
    0) rm "$HERE/golden/$name.err" ;;
    1) rm "$HERE/golden/$name.binpb"; mv "$HERE/golden/$name.err" "$HERE/golden/$name.reject" ;;
    *) echo "protowire-go crashed on $name (exit $rc):" >&2; cat "$HERE/golden/$name.err" >&2; exit 2 ;;
  esac
done
echo "wrote $(ls "$HERE/golden" | wc -l | tr -d ' ') goldens for ${#inputs[@]} inputs"
