#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port format check.
#
# Each port's `check-decode --format pxf-fmt --input <doc>` parses a PXF
# document and writes what the port's document formatter makes of it on
# stdout. This script compares that text byte for byte with the golden under
# testdata/fmt/documents/golden/, which protowire-go wrote
# (testdata/fmt/documents/generate.sh).
#
# Formatting a document is not the path the text gate checks. The text gate
# compares what a marshaller writes from a message. A formatter works on the
# parsed document: it keeps comments, directives and dataset rows, and reads
# no schema. With no gate on it, every port but TypeScript came to drop
# @dataset rows (#558), and nothing noticed (#584).
#
# The inputs are testdata/fmt/documents/cases/*.pxf: every document under
# testdata/ outside adversarial/ that protowire-go parses, copied in and named
# by its path, plus hand-written edge-*.pxf cases. See that directory's README.
#
# === Environment ===
#
#   Same SKIP_PORTS and REQUIRE_PORTS as cross_security_check.sh. A port whose
#   harness has no pxf-fmt mode (it exits 2 on an empty document) is not run
#   and the script says so, so an absent port can't pass for a clean run.
#   Python has no document formatter, and is listed as not run.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"
FMT_DIR="$REPO_DIR/testdata/fmt/documents"
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
NO_FORMATTER=(python)

# Cases one port is not held to yet, keyed "<port> <case>", each with the
# issue that tracks it. A comment no entry follows is lost or moved in C#,
# Dart, Swift and TypeScript; Java and Dart quote DEL as Go's marshaller
# does, not as its formatter does.
declare -A PORT_EXCLUDED=(
  ["csharp dataset.cell-framing"]="trendvidia/protowire-csharp#108 and #114"
  ["csharp edge-comments-between-directives"]="trendvidia/protowire-csharp#108 and #114"
  ["dart dataset.cell-framing"]="trendvidia/protowire-dart#99"
  ["dart edge-comments-between-directives"]="trendvidia/protowire-dart#99"
  ["dart text.cases.Scalars.string-control.expected"]="trendvidia/protowire-dart#98"
  ["java text.cases.Scalars.string-control.expected"]="trendvidia/protowire-java#255"
  ["java-lite text.cases.Scalars.string-control.expected"]="trendvidia/protowire-java#255"
  ["swift dataset.cell-framing"]="trendvidia/protowire-swift#95"
  ["ts dataset.cell-framing"]="trendvidia/protowire-typescript#117"
  ["swift edge-comments-between-directives"]="trendvidia/protowire-swift#95"
  ["ts edge-comments-between-directives"]="trendvidia/protowire-typescript#117"
)

skip() {
  case ",$SKIP_PORTS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

declare -A PORT_BINS=()
TEMP_BINS=()

if ! skip go && [[ -d "$GO_DIR/scripts/check_decode" ]]; then
  echo "→ Go build" >&2
  PORT_BINS[go]="$(mktemp -t check-decode-go.XXXXXX)"
  TEMP_BINS+=("${PORT_BINS[go]}")
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
  TEMP_BINS+=("${PORT_BINS[dart]}")
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
  TEMP_BINS+=("${PORT_BINS[ts]}")
  printf '#!/usr/bin/env bash\ncd %q && exec npx --yes tsx scripts/check-decode.ts "$@"\n' \
    "$TS_DIR" > "${PORT_BINS[ts]}"
  chmod +x "${PORT_BINS[ts]}"
fi

out="$(mktemp -t cross-fmt.XXXXXX)"
err="$(mktemp -t cross-fmt-err.XXXXXX)"
empty="$(mktemp -t cross-fmt-empty.XXXXXX)"
trap 'rm -f "$out" "$err" "$empty" "${TEMP_BINS[@]}"' EXIT

# A harness that predates this gate exits 2 on an unknown format. Such a port
# is reported as not run rather than failing every case, and REQUIRE_PORTS is
# what holds a port to the mode once it has it.
for p in "${!PORT_BINS[@]}"; do
  rc=0
  "${PORT_BINS[$p]}" --format pxf-fmt --input "$empty" > "$out" 2> "$err" || rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "· $p: harness has no pxf-fmt mode (exit $rc: $(head -c 160 "$err")), not run" >&2
    unset "PORT_BINS[$p]"
  fi
done

for p in go cpp java java-lite csharp rust dart ts swift; do
  [[ -n "${PORT_BINS[$p]:-}" ]] || echo "· $p: not run" >&2
done
for p in "${NO_FORMATTER[@]}"; do
  echo "· $p: no document formatter, not run" >&2
done

for p in ${REQUIRE_PORTS//,/ }; do
  if [[ -z "${PORT_BINS[$p]:-}" ]]; then
    echo "required port did not run: $p (REQUIRE_PORTS=$REQUIRE_PORTS)" >&2
    exit 2
  fi
done
if (( ${#PORT_BINS[@]} == 0 )); then
  echo "no port has a pxf-fmt harness" >&2
  exit 2
fi

pairs=0
fails=0
excluded=0
cases=0

for input in "$FMT_DIR"/cases/*.pxf; do
  name="$(basename "$input" .pxf)"
  golden="$FMT_DIR/golden/$name.pxf"
  if [[ ! -f "$golden" ]]; then
    echo "missing golden: $golden (run testdata/fmt/documents/generate.sh)" >&2
    exit 2
  fi
  cases=$((cases + 1))
  for port in "${!PORT_BINS[@]}"; do
    if [[ -n "${PORT_EXCLUDED[$port $name]:-}" ]]; then
      excluded=$((excluded + 1))
      echo "· $port $name: excluded, waits on ${PORT_EXCLUDED[$port $name]}" >&2
      continue
    fi
    pairs=$((pairs + 1))
    rc=0
    "${PORT_BINS[$port]}" --format pxf-fmt --input "$input" > "$out" 2> "$err" || rc=$?
    if [[ $rc -ne 0 ]] || ! cmp -s "$out" "$golden"; then
      fails=$((fails + 1))
      echo "✗ $port $name (exit $rc)" >&2
      if [[ $rc -ne 0 ]]; then
        sed 's/^/    /' "$err" | head -3 >&2
      else
        diff "$golden" "$out" | head -12 | sed 's/^/    /' >&2 || true
      fi
    fi
  done
done

echo
echo "Ran $pairs (port, case) pairs over $cases cases across ${#PORT_BINS[@]} ports ($(printf '%s\n' "${!PORT_BINS[@]}" | sort | tr '\n' ' ')); $excluded (port, case) pairs excluded."
if [[ $fails -ne 0 ]]; then
  echo "✗ $fails pairs differ from protowire-go." >&2
  exit 1
fi
echo "✓ Every port that ran formats as protowire-go does."
