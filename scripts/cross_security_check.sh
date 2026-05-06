#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port HARDENING.md conformance check.
#
# Drives the adversarial corpus under testdata/adversarial/ across every
# port's `check-decode` binary and asserts each port produces the expected
# verdict (accept / reject) within a wall-clock budget. The contract
# every `check-decode` binary must satisfy is documented in docs/HARDENING.md
# and summarized below.
#
# === Per-port `check-decode` contract ===
#
#   check-decode --format <pxf|pb|sbe|envelope> \
#                --schema <fully.qualified.MessageType> \
#                --proto  <path-to-adversarial.proto> \
#                --input  <path>
#
#   Exit 0  → input was accepted (decode succeeded)
#   Exit 1  → input was rejected (decode returned a clean error)
#   Other   → bug in the decoder (panic / abort / OOM / hang / SIGSEGV / ...)
#
#   The binary MUST NOT crash, abort, or hang on any input. Memory blow-ups
#   are caught transitively by the wall-clock budget — anything that
#   allocates enough to matter on adversarial input also runs long enough
#   to trip `timeout`. A port that crashes or hangs is failing the
#   HARDENING.md contract.
#
#   The schema names referenced by the corpus manifest (see below) are the
#   message types defined in testdata/adversarial/adversarial.proto. Ports
#   that can compile a .proto at startup (Go via protocompile, etc.) read
#   --proto at runtime; ports that need pre-generated bindings ignore --proto
#   and hard-code message-name → type dispatch from --schema.
#
# === Corpus layout ===
#
#   testdata/adversarial/MANIFEST.jsonl    one JSON object per corpus entry:
#     { "file":   "pxf/deep-nesting-1000.pxf",
#       "format": "pxf",
#       "schema": "adversarial.v1.Tree",
#       "expect": "reject" | "accept",
#       "reason": "MaxNestingDepth exceeded",
#       "skip":   ["python", "swift"]  // optional: ports that legitimately
#                                      //   can't reach this code path
#     }
#
#   testdata/adversarial/<format>/<name>.<ext>   the input file
#
# === Resource limits ===
#
#   Wall-clock: 5s per (port, corpus) pair, enforced via `timeout`.
#   RSS:        not enforced per-process. The `ulimit -v` knob this script
#               formerly applied is virtual-address-space, not RSS, and
#               trips Go-style runtimes that mmap multi-GiB heap arenas at
#               startup before any work is done. The HARDENING.md memory
#               requirement now relies on the host's per-runner ceiling
#               (~16 GiB on github-hosted ubuntu-latest) plus the wall-clock
#               budget — anything that allocates >256 MiB of usable memory
#               on adversarial input would also blow the wall-clock. See
#               protowire#42 for the discussion of real RSS gating
#               (cgroups MemoryMax, prlimit, etc.) as follow-up.
#
# === Environment ===
#
#   Same WITH_<PORT>=0 / SKIP_PORTS opt-outs as the bench/envelope scripts.
#   A port whose `check-decode` binary doesn't exist on disk auto-skips
#   silently — same pattern as cross_pxf_bench.sh for dart / java-lite.

set -euo pipefail

# macOS only: pick up tools registered via /etc/paths.d/* (e.g. .NET pkg
# installer) that the parent shell may have missed. No-op on Linux.
if [[ -x /usr/libexec/path_helper ]]; then
  _path_orig="$PATH"
  eval "$(/usr/libexec/path_helper -s)"
  PATH="$_path_orig:$PATH"
  unset _path_orig
fi

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIBLING_DIR="$(dirname "$REPO_DIR")"
CORPUS_DIR="$REPO_DIR/testdata/adversarial"
MANIFEST="$CORPUS_DIR/MANIFEST.jsonl"
CORPUS_PROTO="$CORPUS_DIR/adversarial.proto"

GO_DIR="${SIBLING_DIR}/protowire-go"
CPP_DIR="${SIBLING_DIR}/protowire-cpp"
TS_DIR="${SIBLING_DIR}/protowire-typescript"
JAVA_DIR="${SIBLING_DIR}/protowire-java"
RUST_DIR="${SIBLING_DIR}/protowire-rust"
SWIFT_DIR="${SIBLING_DIR}/protowire-swift"
DART_DIR="${SIBLING_DIR}/protowire-dart"
CSHARP_DIR="${SIBLING_DIR}/protowire-csharp"
PYTHON_DIR="${SIBLING_DIR}/protowire-python"

WALLCLOCK_SECONDS="${WALLCLOCK_SECONDS:-5}"
SKIP_PORTS="${SKIP_PORTS:-}"

# Resolve the wall-clock enforcer. GNU `timeout` is the canonical tool; macOS
# ships it as `gtimeout` via Homebrew's coreutils. Fall back to no enforcement
# with a one-shot warning so a developer running this on bare macOS still gets
# verdicts (CI runs on Linux where `timeout` is always present).
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD=(timeout --signal=KILL "$WALLCLOCK_SECONDS")
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD=(gtimeout --signal=KILL "$WALLCLOCK_SECONDS")
else
  echo "warning: neither 'timeout' nor 'gtimeout' found — wall-clock budget not enforced" >&2
  echo "  install GNU coreutils ('brew install coreutils' on macOS) for full enforcement" >&2
  TIMEOUT_CMD=()
fi

if [[ ! -f "$MANIFEST" ]]; then
  echo "missing corpus manifest: $MANIFEST" >&2
  echo "see docs/HARDENING.md § Conformance corpus" >&2
  exit 2
fi

skip() {
  case ",$SKIP_PORTS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

# Build each port's check-decode binary on demand. Sets PORT_BIN to the
# resolved path or empty if the binary isn't present. Empty path → port
# auto-skips this run.
build_port() {
  local port="$1"
  PORT_BIN=""
  if skip "$port"; then return 0; fi
  case "$port" in
    go)
      [[ -d "$GO_DIR/scripts/check_decode" ]] || return 0
      echo "→ Go build" >&2
      PORT_BIN="$(mktemp -t check-decode-go.XXXXXX)"
      (cd "$GO_DIR" && go build -o "$PORT_BIN" ./scripts/check_decode)
      ;;
    cpp)
      [[ -f "$CPP_DIR/cmd/check_decode/main.cc" ]] || return 0
      if [[ ! -d "$CPP_DIR/build" ]]; then
        # Skip the test suite — it pulls in libprotoc (Importer/DiskSourceTree)
        # which Ubuntu's libprotobuf-dev doesn't ship, and we only need
        # check_decode for the conformance corpus anyway.
        cmake -S "$CPP_DIR" -B "$CPP_DIR/build" \
              -DPROTOWIRE_BUILD_TESTS=OFF > /dev/null
      fi
      echo "→ C++ build" >&2
      cmake --build "$CPP_DIR/build" --target check_decode -j > /dev/null
      PORT_BIN="$CPP_DIR/build/bin/check_decode"
      ;;
    ts)
      [[ -f "$TS_DIR/scripts/check-decode.ts" ]] || return 0
      PORT_BIN="ts"
      ;;
    java)
      [[ -d "$JAVA_DIR/check-decode" ]] || return 0
      echo "→ Java build" >&2
      (cd "$JAVA_DIR" && ./gradlew --quiet :check-decode:installDist > /dev/null)
      PORT_BIN="$JAVA_DIR/check-decode/build/install/check-decode/bin/check-decode"
      ;;
    rust)
      [[ -d "$RUST_DIR/crates/check-decode" ]] || return 0
      echo "→ Rust build" >&2
      (cd "$RUST_DIR" && cargo build --release --quiet -p check-decode)
      PORT_BIN="$RUST_DIR/target/release/check-decode"
      ;;
    swift)
      grep -q '"check-decode"' "$SWIFT_DIR/Package.swift" 2>/dev/null || return 0
      echo "→ Swift build" >&2
      (cd "$SWIFT_DIR" && swift build -c release --product check-decode > /dev/null)
      PORT_BIN="$SWIFT_DIR/.build/release/check-decode"
      ;;
    dart)
      [[ -f "$DART_DIR/bin/check_decode.dart" ]] || return 0
      PORT_BIN="dart"
      ;;
    csharp)
      [[ -d "$CSHARP_DIR/cmd/Protowire.CheckDecode" ]] || return 0
      echo "→ C# build" >&2
      (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet \
         cmd/Protowire.CheckDecode > /dev/null)
      PORT_BIN="$CSHARP_DIR/cmd/Protowire.CheckDecode/bin/Release/net10.0/check-decode"
      ;;
    python)
      [[ -f "$PYTHON_DIR/scripts/check_decode.py" ]] || return 0
      # Prefer the port's venv interpreter if present — that's where the
      # protowire package and the C++ FFI extension are installed editable.
      # Fall back to bare `python3` (CI is expected to install the package
      # system-wide via `pip install -e .` first).
      if [[ -x "$PYTHON_DIR/.venv/bin/python3" ]]; then
        PORT_BIN="$PYTHON_DIR/.venv/bin/python3"
      else
        PORT_BIN="python3"
      fi
      ;;
  esac
}

# Run one (port, corpus) pair under the wall-clock budget.
# Echoes one of: PASS / FAIL_VERDICT / FAIL_CRASH / FAIL_TIMEOUT
run_one() {
  local port="$1" bin="$2" format="$3" schema="$4" input="$5" expect="$6"
  local args=(--format "$format" --schema "$schema" --proto "$CORPUS_PROTO" --input "$input")
  local cmd
  case "$port" in
    ts)     cmd=(npx --yes tsx "$TS_DIR/scripts/check-decode.ts" "${args[@]}") ;;
    dart)   cmd=(dart run "$DART_DIR/bin/check_decode.dart" "${args[@]}") ;;
    python) cmd=("$bin" "$PYTHON_DIR/scripts/check_decode.py" "${args[@]}") ;;
    *)      cmd=("$bin" "${args[@]}") ;;
  esac

  local rc=0
  "${TIMEOUT_CMD[@]}" "${cmd[@]}" >/dev/null 2>&1 || rc=$?

  case "$rc" in
    0)   [[ "$expect" == "accept" ]] && echo PASS || echo FAIL_VERDICT ;;
    1)   [[ "$expect" == "reject" ]] && echo PASS || echo FAIL_VERDICT ;;
    124|137) echo FAIL_TIMEOUT ;;
    134|139|136|135) echo FAIL_CRASH ;;   # SIGABRT, SIGSEGV, SIGBUS, SIGFPE
    *)   echo FAIL_CRASH ;;
  esac
}

# Resolve binaries for every port up front; empty PORT_BIN means auto-skip.
declare -A PORT_BINS=()
PORT_COUNT=0
for p in go cpp ts java rust swift dart csharp python; do
  build_port "$p"
  if [[ -n "$PORT_BIN" ]]; then
    PORT_BINS["$p"]="$PORT_BIN"
    PORT_COUNT=$((PORT_COUNT + 1))
  fi
done

if [[ "$PORT_COUNT" -eq 0 ]]; then
  echo "no port has a check-decode binary built — see ROADMAP.md M8" >&2
  exit 2
fi

# Walk the manifest, run every (port, corpus) pair.
declare -A FAILS=()
fail_count=0
total_pairs=0

while IFS= read -r line; do
  [[ -z "$line" || "$line" == \#* ]] && continue
  fields=$(printf '%s' "$line" | python3 -c '
import json, sys
d = json.loads(sys.stdin.read())
print(d["file"])
print(d["format"])
print(d["schema"])
print(d["expect"])
print(",".join(d.get("skip", [])))
')
  file=$(  printf '%s\n' "$fields" | sed -n 1p)
  format=$(printf '%s\n' "$fields" | sed -n 2p)
  schema=$(printf '%s\n' "$fields" | sed -n 3p)
  expect=$(printf '%s\n' "$fields" | sed -n 4p)
  per_skip=$(printf '%s\n' "$fields" | sed -n 5p)

  input="$CORPUS_DIR/$file"
  if [[ ! -f "$input" ]]; then
    echo "missing corpus file referenced by manifest: $input" >&2
    exit 2
  fi

  for port in "${!PORT_BINS[@]}"; do
    case ",$per_skip," in *",$port,"*) continue ;; esac
    total_pairs=$((total_pairs + 1))
    verdict=$(run_one "$port" "${PORT_BINS[$port]}" "$format" "$schema" "$input" "$expect")
    if [[ "$verdict" != "PASS" ]]; then
      FAILS["$port|$file"]="$verdict"
      fail_count=$((fail_count + 1))
    fi
  done
done < "$MANIFEST"

echo
echo "Ran $total_pairs (port, corpus) pairs across $PORT_COUNT ports."

if [[ "$fail_count" -eq 0 ]]; then
  echo "✓ All ports conform to docs/HARDENING.md on the adversarial corpus."
  exit 0
fi

echo "✗ HARDENING conformance regressions:" >&2
for k in "${!FAILS[@]}"; do
  port="${k%%|*}"
  file="${k#*|}"
  printf '  %-8s %-50s %s\n' "$port" "$file" "${FAILS[$k]}" >&2
done
echo >&2
echo "Verdict legend:" >&2
echo "  FAIL_VERDICT  decoder accepted attacker input it should reject (or vice versa)" >&2
echo "  FAIL_CRASH    decoder crashed (SIGSEGV / SIGABRT / panic / unhandled exception)" >&2
echo "  FAIL_TIMEOUT  decoder exceeded ${WALLCLOCK_SECONDS}s wall-clock budget" >&2
exit 1
