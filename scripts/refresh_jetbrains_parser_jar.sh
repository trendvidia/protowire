#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Rebuild the protowire-java :pxf jar and copy it into the JetBrains plugin's
# bundled-libs directory. Run after any change to protowire-java's parser code:
#
#     bash scripts/refresh_jetbrains_parser_jar.sh
#     cd editors/jetbrains/plugin && ./gradlew buildPlugin
#
# TEMPORARY: this exists only because protowire-java is not yet published to
# Maven Central. Once it is, the JetBrains plugin will switch to a regular
# `implementation("com.trendvidia.protowire:pxf:<version>")` dependency and
# this script + the libs/ directory go away. Tracked as the "phase 2 packaging"
# refactor; see editors/jetbrains/plugin/build.gradle.kts (TODO comment).

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
JAVA_DIR="$(dirname "$REPO_DIR")/protowire-java"
PLUGIN_LIBS_DIR="$REPO_DIR/editors/jetbrains/plugin/libs"
DST="$PLUGIN_LIBS_DIR/protowire-pxf.jar"

if [[ ! -d "$JAVA_DIR" ]]; then
  echo "error: expected sibling protowire-java repo at $JAVA_DIR" >&2
  exit 1
fi

echo "→ building :pxf jar in $JAVA_DIR"
(cd "$JAVA_DIR" && ./gradlew --quiet :pxf:jar)

# protowire-java versions the jar (e.g. pxf-0.70.0.jar). Pick the most
# recent main-classes jar — explicitly exclude -sources.jar / -javadoc.jar
# variants the maven-publish plugin produces alongside it.
SRC="$(ls -t "$JAVA_DIR/pxf/build/libs/"pxf-*.jar 2>/dev/null \
  | grep -Ev -- '-(sources|javadoc)\.jar$' | head -1)"
if [[ -z "$SRC" || ! -f "$SRC" ]]; then
  echo "error: build did not produce a main pxf jar in $JAVA_DIR/pxf/build/libs/" >&2
  exit 1
fi

mkdir -p "$PLUGIN_LIBS_DIR"
cp "$SRC" "$DST"
SIZE="$(wc -c < "$DST" | tr -d ' ')"
echo "→ wrote $DST ($SIZE bytes, from $(basename "$SRC"))"
