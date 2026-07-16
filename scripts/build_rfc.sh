#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# build_rfc.sh — regenerate the protowire IETF draft from Markdown source.
#
# Usage:     build_rfc.sh [revision]     (default: 01)
#
# Revision 01 (current):
#   Source:  docs/draft-trendvidia-protowire-01.md    (kramdown-rfc)
#   Outputs: docs/draft-trendvidia-protowire-01.xml   (xml2rfc v3 XML)
#            docs/draft-trendvidia-protowire-01.txt   (canonical .txt)
#
# Revision 00 (historical):
#   Source:  docs/draft-trendvidia-protowire.md
#   Outputs: docs/draft-trendvidia-protowire.xml
#            docs/draft-trendvidia-protowire-00.txt
#   NB: the committed -00.txt was hand-paginated after generation (see
#   scripts/repaginate_draft.py); regenerating overwrites that chrome.
#
# Dependencies:
#   - kramdown-rfc  (Ruby gem; `gem install kramdown-rfc`)
#   - xml2rfc       (Python package; `pip install xml2rfc`)
#
# Both are available via standard package managers; see CONTRIBUTING.md.

set -euo pipefail

cd "$(dirname "$0")/.."

REV="${1:-01}"

case "$REV" in
  00)
    SRC=docs/draft-trendvidia-protowire.md
    XML=docs/draft-trendvidia-protowire.xml
    TXT=docs/draft-trendvidia-protowire-00.txt
    ;;
  *)
    SRC="docs/draft-trendvidia-protowire-${REV}.md"
    XML="docs/draft-trendvidia-protowire-${REV}.xml"
    TXT="docs/draft-trendvidia-protowire-${REV}.txt"
    ;;
esac

if [[ ! -f "$SRC" ]]; then
  echo "error: source file $SRC not found" >&2
  exit 1
fi

if ! command -v kramdown-rfc >/dev/null 2>&1; then
  echo "error: kramdown-rfc not found in PATH" >&2
  echo "       install with: gem install kramdown-rfc" >&2
  exit 1
fi

if ! command -v xml2rfc >/dev/null 2>&1; then
  echo "error: xml2rfc not found in PATH" >&2
  echo "       install with: pip install xml2rfc" >&2
  exit 1
fi

echo "kramdown-rfc: $SRC -> $XML"
kramdown-rfc "$SRC" > "$XML"

echo "xml2rfc:      $XML -> $TXT"
xml2rfc --text "$XML" -o "$TXT"

echo "done. Outputs:"
ls -la "$XML" "$TXT"
