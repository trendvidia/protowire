#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port PXF microbench runner.
#
# Builds and runs the per-port `bench-pxf` binaries against the same
# canonical fixture (`testdata/bench-test.proto` + `testdata/bench-test.pxf`)
# and prints a comparison table.
#
# Each port's bench prints two JSON lines to stdout:
#   {"port":"<name>","op":"unmarshal","ns_per_op":...,"mib_per_sec":...,"iterations":...,"bytes":...}
#   {"port":"<name>","op":"marshal","ns_per_op":...,"iterations":...}
#
# Set MEASURE_SECONDS to control the per-op timing window (default 3).
# Set SKIP_PORTS=cpp,java (etc) to omit any port; useful when a toolchain
# is missing locally. Recognized ports: go, cpp, ts, java, java-lite, rust,
# swift, dart, csharp. Two ports auto-skip silently when their bench harness
# does not yet exist on disk:
#   - dart       — bin/bench_pxf.dart (scheduled for 0.73.0)
#   - java-lite  — bench-pxf-android Gradle module (Android/protobuf-javalite,
#                  scheduled for 0.74.0)

set -euo pipefail

# macOS only: pick up tools registered via /etc/paths.d/* (e.g. .NET pkg
# installer) that the parent shell may have missed. Preserves existing PATH
# entries by prepending; path_helper output ADDS standard system paths +
# /etc/paths.d/* contents. No-op on Linux.
if [[ -x /usr/libexec/path_helper ]]; then
  _path_orig="$PATH"
  eval "$(/usr/libexec/path_helper -s)"
  PATH="$_path_orig:$PATH"
  unset _path_orig
fi

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"
TESTDATA_DIR="$REPO_DIR/testdata"

CPP_DIR="${SIBLING_DIR}/protowire-cpp"
TS_DIR="${SIBLING_DIR}/protowire-typescript"
JAVA_DIR="${SIBLING_DIR}/protowire-java"
RUST_DIR="${SIBLING_DIR}/protowire-rust"
SWIFT_DIR="${SIBLING_DIR}/protowire-swift"
DART_DIR="${SIBLING_DIR}/protowire-dart"
CSHARP_DIR="${SIBLING_DIR}/protowire-csharp"

SECONDS_PER_OP="${MEASURE_SECONDS:-3}"
SKIP_PORTS="${SKIP_PORTS:-}"

skip() {
  case ",$SKIP_PORTS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

# Where each port's JSON lines land before the table is rendered.
RESULTS_FILE="$(mktemp -t cross_pxf_bench.XXXXXX.jsonl)"
trap 'rm -f "$RESULTS_FILE"' EXIT

run_port() {
  local label="$1"; shift
  echo "→ $label" >&2
  "$@" --seconds "$SECONDS_PER_OP" --testdata "$TESTDATA_DIR" >> "$RESULTS_FILE"
}

# --- Go ---
if ! skip go; then
  run_port "Go bench" \
    bash -c "cd '$REPO_DIR' && exec go run ./scripts/bench_pxf \"\$@\"" --
fi

# --- C++ ---
if ! skip cpp; then
  if [[ ! -d "$CPP_DIR/build" ]]; then
    echo "→ C++ configure" >&2
    cmake -S "$CPP_DIR" -B "$CPP_DIR/build" > /dev/null
  fi
  echo "→ C++ build" >&2
  cmake --build "$CPP_DIR/build" --target bench_pxf -j > /dev/null
  run_port "C++ bench" "$CPP_DIR/build/bin/bench_pxf"
fi

# --- TS ---
if ! skip ts; then
  run_port "TS bench" \
    bash -c "cd '$TS_DIR' && exec npx --yes tsx scripts/bench-pxf.ts \"\$@\"" --
fi

# --- Java ---
if ! skip java; then
  echo "→ Java build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :bench-pxf:installDist > /dev/null)
  run_port "Java bench" "$JAVA_DIR/bench-pxf/build/install/bench-pxf/bin/bench-pxf"
fi

# --- Java/Android (protobuf-javalite) ---
# Auto-skipped when the bench-pxf-android Gradle module is absent. The module
# is scheduled for the 0.74.0 milestone; until then this block is a no-op.
if ! skip java-lite && [[ -d "$JAVA_DIR/bench-pxf-android" ]]; then
  echo "→ Java/Android (lite) build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :bench-pxf-android:installDist > /dev/null)
  run_port "Java/Android bench" "$JAVA_DIR/bench-pxf-android/build/install/bench-pxf-android/bin/bench-pxf-android"
fi

# --- Rust ---
if ! skip rust; then
  echo "→ Rust build" >&2
  (cd "$RUST_DIR" && cargo build --release --quiet -p bench-pxf)
  run_port "Rust bench" "$RUST_DIR/target/release/bench-pxf"
fi

# --- Swift ---
if ! skip swift; then
  echo "→ Swift build" >&2
  (cd "$SWIFT_DIR" && swift build -c release --product bench-pxf > /dev/null)
  run_port "Swift bench" "$SWIFT_DIR/.build/release/bench-pxf"
fi

# --- Dart ---
# Auto-skipped when bin/bench_pxf.dart is absent. The harness is scheduled
# for 0.73.0; until then this block is a no-op.
if ! skip dart && [[ -f "$DART_DIR/bin/bench_pxf.dart" ]]; then
  run_port "Dart bench" \
    bash -c "cd '$DART_DIR' && exec dart run bin/bench_pxf.dart \"\$@\"" --
fi

# --- C# ---
if ! skip csharp; then
  echo "→ C# build" >&2
  (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet \
     cmd/Protowire.BenchPxf > /dev/null)
  run_port "C# bench" \
    "$CSHARP_DIR/cmd/Protowire.BenchPxf/bin/Release/net10.0/bench-pxf"
fi

# --- Render the comparison table ---
echo
python3 - "$RESULTS_FILE" <<'PY'
import json
import sys

results = {}  # port -> { "unmarshal": {...}, "marshal": {...} }
with open(sys.argv[1]) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        r = json.loads(line)
        results.setdefault(r["port"], {})[r["op"]] = r

order = ["go", "cpp", "ts", "java", "java-lite", "rust", "swift", "dart", "csharp"]

def fmt_us(ns):
    return f"{ns / 1000.0:7.2f} µs"

def fmt_thrpt(mib):
    return f"{mib:7.1f} MiB/s" if mib else " " * 13

print(f"{'Port':<10}{'Unmarshal':<22}{'Throughput':<16}{'Marshal':<14}")
print("-" * 62)
for p in order:
    if p not in results:
        continue
    u = results[p].get("unmarshal", {})
    m = results[p].get("marshal", {})
    print(f"{p:<10}"
          f"{fmt_us(u.get('ns_per_op', 0)):<22}"
          f"{fmt_thrpt(u.get('mib_per_sec', 0)):<16}"
          f"{fmt_us(m.get('ns_per_op', 0)):<14}")

# Summary footer with relative-to-Go ratios.
go_u = results.get("go", {}).get("unmarshal", {}).get("ns_per_op")
if go_u:
    print()
    print(f"Relative unmarshal latency vs Go (Go = 1.0):")
    for p in order:
        if p == "go" or p not in results:
            continue
        ns = results[p].get("unmarshal", {}).get("ns_per_op")
        if ns:
            print(f"  {p:<10}{ns / go_u:>5.2f}x")
PY
