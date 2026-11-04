#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port write check.
#
# Each port's `check-decode --format pxf-pb` reads a PXF document and writes
# its PB bytes on stdout. This script compares those bytes with the golden
# under testdata/write/golden/, which protowire-go wrote
# (testdata/write/generate.sh). Where Go rejects the document, the golden is
# a .reject file, and the port must reject it too (exit 1).
#
# The text gate checks pb to text, the security gate compares accept or reject
# verdicts, and the envelope gate compares one canonical message. So before
# this one, a port could write different bytes from text and pass all three
# (#509).
#
# The inputs are every testdata/text/cases/*.expected.pxf, plus the edge cases
# in testdata/write/cases/. Map entries are compared in key order, as Go's
# deterministic writer puts them: a port's harness writes them that way, even
# where the port's own writer does not.
#
# === Environment ===
#
#   Same SKIP_PORTS and REQUIRE_PORTS as cross_security_check.sh. A port with
#   no pxf-pb harness is not run and the script says so, so an absent port
#   can't pass for a clean run. Today go, cpp, java, java-lite, csharp, rust,
#   dart, ts and swift have one.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"
TEXT_DIR="$REPO_DIR/testdata/text"
WRITE_DIR="$REPO_DIR/testdata/write"
GO_DIR="${GO_DIR:-${SIBLING_DIR}/protowire-go}"
CPP_DIR="${CPP_DIR:-${SIBLING_DIR}/protowire-cpp}"
JAVA_DIR="${JAVA_DIR:-${SIBLING_DIR}/protowire-java}"
CSHARP_DIR="${CSHARP_DIR:-${SIBLING_DIR}/protowire-csharp}"
RUST_DIR="${RUST_DIR:-${SIBLING_DIR}/protowire-rust}"
DART_DIR="${DART_DIR:-${SIBLING_DIR}/protowire-dart}"
TS_DIR="${TS_DIR:-${SIBLING_DIR}/protowire-typescript}"
SWIFT_DIR="${SWIFT_DIR:-${SIBLING_DIR}/protowire-swift}"
SKIP_PORTS="${SKIP_PORTS:-}"
REQUIRE_PORTS="${REQUIRE_PORTS:-}"
NOT_YET=(python)

# Cases no port is held to yet, each with the question it waits on.
declare -A EXCLUDED=()

# Cases one port is not held to yet, keyed "<port> <case>", each with the
# issue that tracks it. C# writes a generated message through Google.Protobuf,
# which drops -0.0 and a map entry's zero key or value
# (trendvidia/protowire-csharp#91). Rust writes a DynamicMessage through
# prost-reflect, which drops -0.0 (trendvidia/protowire-rust#98). A generated
# Dart enum cannot hold a number it does not declare, and the draft says
# nothing about unknown enum numbers (trendvidia/protowire-dart#82).
declare -A PORT_EXCLUDED=(
  ["csharp Scalars.float-negzero"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-bool-keys"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-int-keys"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-string-keys"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-bool-keys-unsorted"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-int-keys-unsorted"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-string-keys-unsorted"]="trendvidia/protowire-csharp#91"
  ["csharp Shapes.map-zero-key-and-value"]="trendvidia/protowire-csharp#91"
  ["rust Scalars.float-negzero"]="trendvidia/protowire-rust#98"
  ["dart Scalars.enum-unknown"]="trendvidia/protowire-dart#82"
)

skip() {
  case ",$SKIP_PORTS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

declare -A PORT_BINS=()

if ! skip go && [[ -d "$GO_DIR/scripts/check_decode" ]]; then
  echo "→ Go build" >&2
  PORT_BINS[go]="$(mktemp -t check-decode-go.XXXXXX)"
  (cd "$GO_DIR" && go build -o "${PORT_BINS[go]}" ./scripts/check_decode)
fi
if ! skip cpp && [[ -f "$CPP_DIR/cmd/check_decode/main.cc" ]]; then
  echo "→ C++ build" >&2
  # check_decode only: the test suite needs libprotoc pieces Ubuntu's
  # libprotobuf-dev lacks, as in cross_security_check.sh.
  if [[ ! -d "$CPP_DIR/build" ]]; then
    cmake -S "$CPP_DIR" -B "$CPP_DIR/build" -DPROTOWIRE_BUILD_TESTS=OFF >&2
  fi
  cmake --build "$CPP_DIR/build" --target check_decode -j >&2
  PORT_BINS[cpp]="$CPP_DIR/build/bin/check_decode"
fi
if ! skip java && [[ -d "$JAVA_DIR/check-decode" ]]; then
  echo "→ Java build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :check-decode:installDist)
  PORT_BINS[java]="$JAVA_DIR/check-decode/build/install/check-decode/bin/check-decode"
fi
if ! skip java-lite && [[ -d "$JAVA_DIR/check-decode-android" ]]; then
  echo "→ Java lite build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :check-decode-android:installDist)
  PORT_BINS[java-lite]="$JAVA_DIR/check-decode-android/build/install/check-decode-android/bin/check-decode-android"
fi
if ! skip csharp && [[ -d "$CSHARP_DIR/cmd/Protowire.CheckDecode" ]]; then
  echo "→ C# build" >&2
  (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet cmd/Protowire.CheckDecode >&2)
  PORT_BINS[csharp]="$CSHARP_DIR/cmd/Protowire.CheckDecode/bin/Release/net10.0/check-decode"
fi
if ! skip rust && [[ -d "$RUST_DIR/crates/check-decode" ]]; then
  echo "→ Rust build" >&2
  (cd "$RUST_DIR" && cargo build --release --quiet -p check-decode)
  PORT_BINS[rust]="$RUST_DIR/target/release/check-decode"
fi
if ! skip dart && [[ -f "$DART_DIR/bin/check_decode.dart" ]]; then
  echo "→ Dart build" >&2
  PORT_BINS[dart]="$(mktemp -t check-decode-dart.XXXXXX)"
  (cd "$DART_DIR" && dart pub get >/dev/null &&
    dart compile exe bin/check_decode.dart -o "${PORT_BINS[dart]}" >/dev/null)
fi
if ! skip swift && [[ -f "$SWIFT_DIR/cmd/check-decode/main.swift" ]]; then
  echo "→ Swift build" >&2
  (cd "$SWIFT_DIR" && swift build -c release --product check-decode >&2)
  PORT_BINS[swift]="$SWIFT_DIR/.build/release/check-decode"
fi

if ! skip ts && [[ -f "$TS_DIR/scripts/check-decode.ts" ]]; then
  # A script, not a build: it runs the harness from the port's directory, so
  # npx runs the tsx the port's lockfile pins without asking the registry.
  # The port's dependencies must be installed (npm ci).
  echo "→ TypeScript harness" >&2
  PORT_BINS[ts]="$(mktemp -t check-decode-ts.XXXXXX)"
  printf '#!/usr/bin/env bash\ncd %q && exec npx --yes tsx scripts/check-decode.ts "$@"\n' \
    "$TS_DIR" > "${PORT_BINS[ts]}"
  chmod +x "${PORT_BINS[ts]}"
fi

for p in go cpp java java-lite csharp rust dart ts swift; do
  [[ -n "${PORT_BINS[$p]:-}" ]] || echo "· $p: not run" >&2
done
for p in "${NOT_YET[@]}"; do
  echo "· $p: no pxf-pb harness, not run" >&2
done

for p in ${REQUIRE_PORTS//,/ }; do
  if [[ -z "${PORT_BINS[$p]:-}" ]]; then
    echo "required port did not build: $p (REQUIRE_PORTS=$REQUIRE_PORTS)" >&2
    exit 2
  fi
done
if (( ${#PORT_BINS[@]} == 0 )); then
  echo "no port has a pxf-pb harness" >&2
  exit 2
fi

out="$(mktemp -t cross-write.XXXXXX)"
err="$(mktemp -t cross-write-err.XXXXXX)"
trap 'rm -f "$out" "$err"' EXIT
pairs=0
fails=0
excluded=0

for input in "$TEXT_DIR"/cases/*.expected.pxf "$WRITE_DIR"/cases/*.pxf; do
  base="$(basename "$input")"
  name="${base%.expected.pxf}"
  name="${name%.pxf}"
  schema="text.v1.${name%%.*}"
  if [[ -n "${EXCLUDED[$name]:-}" ]]; then
    excluded=$((excluded + 1))
    echo "· $name: excluded, waits on ${EXCLUDED[$name]}" >&2
    continue
  fi
  golden="$WRITE_DIR/golden/$name.binpb"
  reject="$WRITE_DIR/golden/$name.reject"
  if [[ ! -f "$golden" && ! -f "$reject" ]]; then
    echo "missing golden for $name (run testdata/write/generate.sh)" >&2
    exit 2
  fi
  for port in "${!PORT_BINS[@]}"; do
    if [[ -n "${PORT_EXCLUDED[$port $name]:-}" ]]; then
      excluded=$((excluded + 1))
      echo "· $port $name: excluded, waits on ${PORT_EXCLUDED[$port $name]}" >&2
      continue
    fi
    pairs=$((pairs + 1))
    rc=0
    "${PORT_BINS[$port]}" --format pxf-pb --schema "$schema" \
      --proto "$TEXT_DIR/text.proto" --input "$input" > "$out" 2> "$err" || rc=$?
    if [[ -f "$reject" ]]; then
      if [[ $rc -ne 1 ]]; then
        fails=$((fails + 1))
        echo "✗ $port $name: Go rejects ($(cat "$reject")), port exit $rc" >&2
        [[ $rc -eq 0 ]] && echo "    port wrote $(xxd -p "$out" | tr -d '\n' | cut -c1-120)" >&2
      fi
    elif [[ $rc -ne 0 ]] || ! cmp -s "$out" "$golden"; then
      fails=$((fails + 1))
      echo "✗ $port $name (exit $rc)" >&2
      echo "    go:   $(xxd -p "$golden" | tr -d '\n' | cut -c1-120)" >&2
      echo "    port: $(xxd -p "$out" | tr -d '\n' | cut -c1-120)" >&2
      [[ $rc -ne 0 ]] && sed 's/^/    /' "$err" | head -3 >&2
    fi
  done
done

echo
echo "Ran $pairs (port, case) pairs across ${#PORT_BINS[@]} ports; $excluded (port, case) pairs excluded."
if [[ $fails -ne 0 ]]; then
  echo "✗ $fails pairs differ from protowire-go." >&2
  exit 1
fi
echo "✓ Every port that ran writes protowire-go's bytes."
