#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port PXF text check.
#
# Each port's `check-decode --format pb-text` reads a message with the stock
# protobuf runtime and writes its PXF on stdout. This script compares that
# text byte for byte with the golden under testdata/text/cases/, which
# protowire-go wrote (testdata/text/generate.sh), so every port is held to the
# reference. Reading with the stock runtime keeps the comparison on the
# marshaller alone.
#
# The envelope gate compares PB and SBE bytes, and the security gate compares
# accept or reject verdicts, so before this one a port could write different
# text and pass both (#503).
#
# === Environment ===
#
#   Same SKIP_PORTS and REQUIRE_PORTS as cross_security_check.sh. A port with
#   no pb-text harness is not run and the script says so, so an absent port
#   can't pass for a clean run. Today go, cpp, java, java-lite, csharp and dart
#   have one.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"
TEXT_DIR="$REPO_DIR/testdata/text"
GO_DIR="${GO_DIR:-${SIBLING_DIR}/protowire-go}"
CPP_DIR="${CPP_DIR:-${SIBLING_DIR}/protowire-cpp}"
JAVA_DIR="${JAVA_DIR:-${SIBLING_DIR}/protowire-java}"
CSHARP_DIR="${CSHARP_DIR:-${SIBLING_DIR}/protowire-csharp}"
DART_DIR="${DART_DIR:-${SIBLING_DIR}/protowire-dart}"
SKIP_PORTS="${SKIP_PORTS:-}"
REQUIRE_PORTS="${REQUIRE_PORTS:-}"
NOT_YET=(ts rust swift python)

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
  (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet cmd/Protowire.CheckDecode >/dev/null)
  PORT_BINS[csharp]="$CSHARP_DIR/cmd/Protowire.CheckDecode/bin/Release/net10.0/check-decode"
fi

if ! skip dart && [[ -f "$DART_DIR/bin/check_decode.dart" ]]; then
  echo "→ Dart build" >&2
  PORT_BINS[dart]="$(mktemp -t check-decode-dart.XXXXXX)"
  (cd "$DART_DIR" && dart pub get >/dev/null &&
    dart compile exe bin/check_decode.dart -o "${PORT_BINS[dart]}" >/dev/null)
fi

for p in go cpp java java-lite csharp dart; do
  [[ -n "${PORT_BINS[$p]:-}" ]] || echo "· $p: not run" >&2
done
for p in "${NOT_YET[@]}"; do
  echo "· $p: no pb-text harness, not run" >&2
done

for p in ${REQUIRE_PORTS//,/ }; do
  if [[ -z "${PORT_BINS[$p]:-}" ]]; then
    echo "required port did not build: $p (REQUIRE_PORTS=$REQUIRE_PORTS)" >&2
    exit 2
  fi
done
if (( ${#PORT_BINS[@]} == 0 )); then
  echo "no port has a pb-text harness" >&2
  exit 2
fi

cd "$TEXT_DIR"
out="$(mktemp -t cross-text.XXXXXX)"
trap 'rm -f "$out"' EXIT
pairs=0
fails=0

for input in cases/*.binpb; do
  name="$(basename "$input" .binpb)"
  golden="cases/$name.expected.pxf"
  schema="text.v1.${name%%.*}"
  if [[ ! -f "$golden" ]]; then
    echo "missing golden: $golden (run testdata/text/generate.sh)" >&2
    exit 2
  fi
  for port in "${!PORT_BINS[@]}"; do
    pairs=$((pairs + 1))
    rc=0
    "${PORT_BINS[$port]}" --format pb-text --schema "$schema" \
      --proto text.proto --input "$input" > "$out" 2>&1 || rc=$?
    if [[ $rc -ne 0 ]] || ! cmp -s "$out" "$golden"; then
      fails=$((fails + 1))
      echo "✗ $port $name (exit $rc)" >&2
      diff "$golden" "$out" | head -12 >&2 || true
    fi
  done
done

echo
echo "Ran $pairs (port, case) pairs across ${#PORT_BINS[@]} ports."
if [[ $fails -ne 0 ]]; then
  echo "✗ $fails pairs differ from the golden." >&2
  exit 1
fi
echo "✓ Every port that ran writes the golden text."
