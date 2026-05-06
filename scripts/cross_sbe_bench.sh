#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port SBE microbench runner.
#
# Builds and runs the per-port `bench-sbe` binaries against the same
# canonical fixture (`testdata/sbe-bench.proto`, a `bench.v1.Order`
# with a 2-entry Fill group, 94 bytes on the wire) and prints a
# comparison table.
#
# Each port's bench prints two JSON lines to stdout:
#   {"port":"<name>","op":"sbe-marshal","ns_per_op":...,"iterations":...,"bytes":...}
#   {"port":"<name>","op":"sbe-unmarshal","ns_per_op":...,"mib_per_sec":...,"iterations":...,"bytes":...}
#
# Set MEASURE_SECONDS to control the per-op timing window (default 3).
# Set SKIP_PORTS=cpp,java,… to omit any port whose toolchain is missing.
# Recognized ports: go, cpp, ts, java, java-lite, rust, python, csharp, swift, dart.
#
# `java-lite` is the protobuf-javalite consumer of the codegen-emitted
# OrderSbeCodec (in :bench-sbe-android). Auto-skipped when the module
# isn't checked out — handy when running against an older protowire-java.
# Two ports auto-skip silently when their bench harness does not yet exist:
#   - swift  — cmd/bench-sbe (descriptor-driven SBE codec, scheduled 0.73.0)
#   - dart   — bin/bench_sbe.dart (cross-port harness, scheduled 0.73.0)

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

GO_DIR="${SIBLING_DIR}/protowire-go"
PYTHON_DIR="${SIBLING_DIR}/protowire-python"
CSHARP_DIR="${SIBLING_DIR}/protowire-csharp"
CPP_DIR="${SIBLING_DIR}/protowire-cpp"
TS_DIR="${SIBLING_DIR}/protowire-typescript"
JAVA_DIR="${SIBLING_DIR}/protowire-java"
RUST_DIR="${SIBLING_DIR}/protowire-rust"
SWIFT_DIR="${SIBLING_DIR}/protowire-swift"
DART_DIR="${SIBLING_DIR}/protowire-dart"

SECONDS_PER_OP="${MEASURE_SECONDS:-3}"
SKIP_PORTS="${SKIP_PORTS:-}"

skip() {
  case ",$SKIP_PORTS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

RESULTS_FILE="$(mktemp -t cross_sbe_bench.XXXXXX.jsonl)"
trap 'rm -f "$RESULTS_FILE"' EXIT

run_port() {
  local label="$1"; shift
  echo "→ $label" >&2
  "$@" --seconds "$SECONDS_PER_OP" --testdata "$TESTDATA_DIR" >> "$RESULTS_FILE"
}

# --- Go ---
if ! skip go; then
  run_port "Go bench-sbe" \
    bash -c "cd '$GO_DIR' && exec go run ./scripts/bench_sbe \"\$@\"" --
fi

# --- C++ ---
if ! skip cpp; then
  if [[ ! -d "$CPP_DIR/build" ]]; then
    echo "→ C++ configure" >&2
    cmake -S "$CPP_DIR" -B "$CPP_DIR/build" > /dev/null
  fi
  echo "→ C++ build" >&2
  cmake --build "$CPP_DIR/build" --target bench_sbe -j > /dev/null
  run_port "C++ bench-sbe" "$CPP_DIR/build/bin/bench_sbe"
fi

# --- TS ---
if ! skip ts; then
  run_port "TS bench-sbe" \
    bash -c "cd '$TS_DIR' && exec npx --yes tsx scripts/bench-sbe.ts \"\$@\"" --
fi

# --- Java ---
if ! skip java; then
  echo "→ Java build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :bench-sbe:installDist > /dev/null)
  run_port "Java bench-sbe" "$JAVA_DIR/bench-sbe/build/install/bench-sbe/bin/bench-sbe"
fi

# --- Java/Android (protobuf-javalite) ---
# Auto-skipped when :bench-sbe-android isn't checked out. Drives the work
# through codegen-emitted OrderSbeCodec (no descriptor reflection).
if ! skip java-lite && [[ -d "$JAVA_DIR/bench-sbe-android" ]]; then
  echo "→ Java/Android build" >&2
  (cd "$JAVA_DIR" && ./gradlew --quiet :bench-sbe-android:installDist > /dev/null)
  run_port "Java/Android bench-sbe" "$JAVA_DIR/bench-sbe-android/build/install/bench-sbe-android/bin/bench-sbe-android"
fi

# --- Rust ---
if ! skip rust; then
  echo "→ Rust build" >&2
  (cd "$RUST_DIR" && cargo build --release --quiet -p bench-sbe)
  run_port "Rust bench-sbe" "$RUST_DIR/target/release/bench-sbe"
fi

# --- Python ---
if ! skip python; then
  if [[ ! -d "$PYTHON_DIR/.venv" ]]; then
    echo "→ Python venv create" >&2
    python3 -m venv "$PYTHON_DIR/.venv"
  fi
  if [[ ! -f "$PYTHON_DIR/.venv/.installed" ]] || \
     [[ "$PYTHON_DIR/CMakeLists.txt" -nt "$PYTHON_DIR/.venv/.installed" ]] || \
     [[ "$PYTHON_DIR/src/_protowire/module.cc" -nt "$PYTHON_DIR/.venv/.installed" ]]; then
    echo "→ Python build (pip install -e .)" >&2
    "$PYTHON_DIR/.venv/bin/pip" install --quiet -e "$PYTHON_DIR"
    touch "$PYTHON_DIR/.venv/.installed"
  fi
  run_port "Python bench-sbe" \
    "$PYTHON_DIR/.venv/bin/python3" "$PYTHON_DIR/scripts/bench_sbe.py"
fi

# --- C# ---
if ! skip csharp; then
  echo "→ C# build" >&2
  (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet \
     cmd/Protowire.BenchSbe > /dev/null)
  run_port "C# bench-sbe" \
    "$CSHARP_DIR/cmd/Protowire.BenchSbe/bin/Release/net10.0/bench-sbe"
fi

# --- Swift ---
# Auto-skipped when bench-sbe is absent. Scheduled for 0.73.0 (depends
# on the descriptor-driven SBE codec landing first).
if ! skip swift && [[ -d "$SWIFT_DIR/cmd/bench-sbe" || -d "$SWIFT_DIR/Sources/bench-sbe" ]]; then
  echo "→ Swift build" >&2
  (cd "$SWIFT_DIR" && swift build -c release --product bench-sbe > /dev/null)
  run_port "Swift bench-sbe" "$SWIFT_DIR/.build/release/bench-sbe"
fi

# --- Dart ---
# Auto-skipped when bin/bench_sbe.dart is absent. Scheduled for 0.73.0.
if ! skip dart && [[ -f "$DART_DIR/bin/bench_sbe.dart" ]]; then
  run_port "Dart bench-sbe" \
    bash -c "cd '$DART_DIR' && exec dart run bin/bench_sbe.dart \"\$@\"" --
fi

echo
python3 - "$RESULTS_FILE" <<'PY'
import json
import sys

results = {}
with open(sys.argv[1]) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        r = json.loads(line)
        results.setdefault(r["port"], {})[r["op"]] = r

order = ["go", "cpp", "ts", "java", "java-lite", "rust", "python", "csharp", "swift", "dart"]

def fmt_ns(ns):
    if ns < 1000:
        return f"{ns:5d} ns "
    return f"{ns / 1000.0:6.2f} µs"

def fmt_thrpt(mib):
    return f"{mib:7.1f} MiB/s" if mib else " " * 13

print(f"{'Port':<11}{'Marshal':<14}{'Unmarshal':<14}{'Throughput':<16}")
print("-" * 55)
for p in order:
    if p not in results:
        continue
    m = results[p].get("sbe-marshal", {})
    u = results[p].get("sbe-unmarshal", {})
    print(f"{p:<11}"
          f"{fmt_ns(m.get('ns_per_op', 0)):<14}"
          f"{fmt_ns(u.get('ns_per_op', 0)):<14}"
          f"{fmt_thrpt(u.get('mib_per_sec', 0)):<16}")

# Footer with relative-to-Go ratios.
go_m = results.get("go", {}).get("sbe-marshal", {}).get("ns_per_op")
go_u = results.get("go", {}).get("sbe-unmarshal", {}).get("ns_per_op")
if go_m and go_u:
    print()
    print("Relative latency vs Go (Go = 1.0):")
    for p in order:
        if p == "go" or p not in results:
            continue
        m = results[p].get("sbe-marshal", {}).get("ns_per_op")
        u = results[p].get("sbe-unmarshal", {}).get("ns_per_op")
        if m and u:
            print(f"  {p:<11}marshal {m / go_m:>5.2f}x   unmarshal {u / go_u:>5.2f}x")
PY
