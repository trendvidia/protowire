#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Regenerates the format-gate goldens with protowire-go, the reference
# (#584). For each cases/<name>.pxf, protowire-go's
# `check_decode --format pxf-fmt` writes the formatted document to
# golden/<name>.pxf. Every case must parse: formatting a document that does
# not parse is not what the gate compares. GO_DIR points at a protowire-go
# checkout. A changed golden changes the text every port's formatter must
# write, so read the diff before committing it.

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GO_DIR="${GO_DIR:-$HERE/../../../../protowire-go}"
bin="$(mktemp -t check-decode-go.XXXXXX)"
trap 'rm -f "$bin"' EXIT
(cd "$GO_DIR" && go build -o "$bin" ./scripts/check_decode)

rm -rf "$HERE/golden"
mkdir -p "$HERE/golden"
n=0
for input in "$HERE"/cases/*.pxf; do
  name="$(basename "$input" .pxf)"
  rc=0
  "$bin" --format pxf-fmt --input "$input" > "$HERE/golden/$name.pxf" || rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "protowire-go does not format $name (exit $rc); every case must parse" >&2
    exit 2
  fi
  n=$((n + 1))
done
echo "wrote $n goldens"
