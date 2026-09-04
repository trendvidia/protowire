#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
#
# Cross-port envelope wire-compatibility check.
#
# Constructs the same canonical Envelope in Go, C++, TypeScript, Java,
# (when WITH_RUST=1) Rust, (when WITH_SWIFT=1) Swift, and (when WITH_DART=1)
# Dart, marshals it via each port's pb codec, and asserts the resulting
# bytes are identical. Divergence indicates a wire-format regression in
# one of the ports.
#
# Then, for the descriptor-driven ports (Go, C++, TypeScript, Java, Rust),
# feeds each port's `dump-envelope --pb|--sbe` the same descriptor set and
# PXF document from testdata/ and compares the bytes with a golden. That is
# the leg that proves every port reads the annotation extension numbers of
# STABILITY.md promise 3 (issue #244) — see the block near the end.
#
# The canonical value uses a single metadata entry to avoid map-iteration
# order ambiguity (proto3 doesn't mandate map entry order on the wire).
#
# Set WITH_RUST=0 to skip the Rust port (protowire-rust).
# Set WITH_SWIFT=0 to skip the Swift port (protowire-swift).
# Set WITH_DART=0 to skip the Dart port (protowire-dart).
# Set WITH_CSHARP=0 to skip the C# port (protowire-csharp).
# Set WITH_JAVA_LITE=0 to skip the Java/Android (protobuf-javalite) port,
# which builds the dump-envelope-android target out of protowire-java/. The
# expected wire bytes match the JVM Java port exactly (any divergence is a
# CI-blocking regression). Defaults to 1 — the *-android Gradle modules are
# stable as of 0.74.0; opt out only when protowire-java is unavailable.
# Set WITH_JAVA_PXF_LITE=0 to skip the PXF-driven java-lite path:
# dump-envelope-pxf-android constructs the canonical envelope from PXF text
# rather than via the typed builder API, exercising the full Parser →
# LiteWireWriter pipeline. The hex must equal the JVM Java port's; divergence
# catches an encoder-side wire bug in :pxf-android that the protobuf-
# javalite-direct path (WITH_JAVA_LITE) can't.

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
GO_DIR="${SIBLING_DIR}/protowire-go"
CPP_DIR="${SIBLING_DIR}/protowire-cpp"
TS_DIR="${SIBLING_DIR}/protowire-typescript"
JAVA_DIR="${SIBLING_DIR}/protowire-java"
JAVA_LITE_DIR="${JAVA_LITE_DIR:-$JAVA_DIR}"
RUST_DIR="${SIBLING_DIR}/protowire-rust"
SWIFT_DIR="${SIBLING_DIR}/protowire-swift"
DART_DIR="${SIBLING_DIR}/protowire-dart"
CSHARP_DIR="${SIBLING_DIR}/protowire-csharp"

WITH_RUST="${WITH_RUST:-1}"
WITH_SWIFT="${WITH_SWIFT:-1}"
WITH_DART="${WITH_DART:-1}"
WITH_CSHARP="${WITH_CSHARP:-1}"
WITH_JAVA_LITE="${WITH_JAVA_LITE:-1}"
WITH_JAVA_PXF_LITE="${WITH_JAVA_PXF_LITE:-1}"

required=("$GO_DIR" "$CPP_DIR" "$TS_DIR" "$JAVA_DIR")
if [[ "$WITH_RUST" == "1" ]]; then
  required+=("$RUST_DIR")
fi
if [[ "$WITH_SWIFT" == "1" ]]; then
  required+=("$SWIFT_DIR")
fi
if [[ "$WITH_DART" == "1" ]]; then
  required+=("$DART_DIR")
fi
if [[ "$WITH_CSHARP" == "1" ]]; then
  required+=("$CSHARP_DIR")
fi
if [[ "$WITH_JAVA_LITE" == "1" ]]; then
  required+=("$JAVA_LITE_DIR")
fi
if [[ "$WITH_JAVA_PXF_LITE" == "1" ]]; then
  required+=("$JAVA_LITE_DIR")
fi
for d in "${required[@]}"; do
  if [[ ! -d "$d" ]]; then
    echo "expected sibling directory: $d" >&2
    exit 1
  fi
done

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# build LABEL CMD... — runs a port's build, showing its output only when it
# fails. These builds used to go to /dev/null, so a compiler error left a
# bare `set -e` exit with nothing on the terminal — a broken C# dumper
# looked like a bug in this script.
build() {
  local label="$1"; shift
  if ! "$@" > "$TMP_DIR/build.log" 2>&1; then
    echo "✗ $label build failed:" >&2
    cat "$TMP_DIR/build.log" >&2
    exit 1
  fi
}

echo "→ Go dumper"
go_hex=$(cd "$GO_DIR" && go run ./scripts/dump_envelope)

echo "→ C++ dumper (build + run)"
if [[ ! -d "$CPP_DIR/build" ]]; then
  build "C++ configure" cmake -S "$CPP_DIR" -B "$CPP_DIR/build"
fi
build "C++ dumper" cmake --build "$CPP_DIR/build" --target dump_envelope -j
cpp_hex=$("$CPP_DIR/build/bin/dump_envelope")

echo "→ TS dumper"
ts_hex=$(cd "$TS_DIR" && npx --yes tsx scripts/dump-envelope.ts)

echo "→ Java dumper (build + run)"
build "Java dumper" bash -c "cd '$JAVA_DIR' && ./gradlew --quiet :dump-envelope:installDist"
java_hex=$("$JAVA_DIR/dump-envelope/build/install/dump-envelope/bin/dump-envelope")

if [[ "$WITH_RUST" == "1" ]]; then
  echo "→ Rust dumper (build + run)"
  rust_hex=$(cd "$RUST_DIR" && cargo run --quiet --release -p dump-envelope)
fi

if [[ "$WITH_SWIFT" == "1" ]]; then
  echo "→ Swift dumper (build + run)"
  build "Swift dumper" bash -c "cd '$SWIFT_DIR' && swift build -c release --product dump-envelope"
  swift_hex=$("$SWIFT_DIR/.build/release/dump-envelope")
fi

if [[ "$WITH_DART" == "1" && ! -f "$DART_DIR/bin/dump_envelope.dart" ]]; then
  # protowire-dart has never carried the dumper this step runs
  # (trendvidia/protowire-dart#13). Under set -e the missing file aborted
  # every full local run at this point, which is how STABILITY.md promise 2
  # came to name a Dart check that could not run. Skip, and say so.
  echo "→ Dart dumper: SKIP — bin/dump_envelope.dart absent (trendvidia/protowire-dart#13)"
  WITH_DART=0
fi

if [[ "$WITH_DART" == "1" ]]; then
  echo "→ Dart dumper (run)"
  dart_hex=$(cd "$DART_DIR" && dart run bin/dump_envelope.dart)
fi

if [[ "$WITH_CSHARP" == "1" ]]; then
  echo "→ C# dumper (build + run)"
  build "C# dumper" bash -c "cd '$CSHARP_DIR' && dotnet build -c Release --nologo -v quiet cmd/Protowire.DumpEnvelope"
  csharp_hex=$("$CSHARP_DIR/cmd/Protowire.DumpEnvelope/bin/Release/net10.0/dump-envelope")
fi

if [[ "$WITH_JAVA_LITE" == "1" && ! -d "$JAVA_LITE_DIR/dump-envelope-android" ]]; then
  # The lite modules are not in every protowire-java checkout (see
  # cross_sbe_bench.sh, which auto-skips on the same test). Missing, they
  # used to fail the Gradle selection and abort the run here.
  echo "→ Java/Android (lite) dumper: SKIP — dump-envelope-android module absent from $JAVA_LITE_DIR"
  WITH_JAVA_LITE=0
fi
if [[ "$WITH_JAVA_PXF_LITE" == "1" && ! -d "$JAVA_LITE_DIR/dump-envelope-pxf-android" ]]; then
  echo "→ Java/Android (PXF→lite) dumper: SKIP — dump-envelope-pxf-android module absent from $JAVA_LITE_DIR"
  WITH_JAVA_PXF_LITE=0
fi

if [[ "$WITH_JAVA_LITE" == "1" ]]; then
  echo "→ Java/Android (lite) dumper (build + run)"
  build "Java/Android (lite) dumper" bash -c "cd '$JAVA_LITE_DIR' && ./gradlew --quiet :dump-envelope-android:installDist"
  java_lite_hex=$("$JAVA_LITE_DIR/dump-envelope-android/build/install/dump-envelope-android/bin/dump-envelope-android")
fi

if [[ "$WITH_JAVA_PXF_LITE" == "1" ]]; then
  echo "→ Java/Android (PXF→lite) dumper (build + run)"
  build "Java/Android (PXF→lite) dumper" bash -c "cd '$JAVA_LITE_DIR' && ./gradlew --quiet :dump-envelope-pxf-android:installDist"
  java_pxf_lite_hex=$("$JAVA_LITE_DIR/dump-envelope-pxf-android/build/install/dump-envelope-pxf-android/bin/dump-envelope-pxf-android")
fi

echo
echo "Go:    $go_hex"
echo "C++:   $cpp_hex"
echo "TS:    $ts_hex"
echo "Java:  $java_hex"
if [[ "$WITH_RUST" == "1" ]]; then
  echo "Rust:  $rust_hex"
fi
if [[ "$WITH_SWIFT" == "1" ]]; then
  echo "Swift: $swift_hex"
fi
if [[ "$WITH_DART" == "1" ]]; then
  echo "Dart:  $dart_hex"
fi
if [[ "$WITH_CSHARP" == "1" ]]; then
  echo "C#:    $csharp_hex"
fi
if [[ "$WITH_JAVA_LITE" == "1" ]]; then
  echo "Java/Lite: $java_lite_hex"
fi
if [[ "$WITH_JAVA_PXF_LITE" == "1" ]]; then
  echo "Java/PXF-Lite: $java_pxf_lite_hex"
fi
echo

ok=1
if [[ "$go_hex" != "$cpp_hex" || "$cpp_hex" != "$ts_hex" || "$ts_hex" != "$java_hex" ]]; then
  ok=0
fi
if [[ "$WITH_RUST" == "1" && "$java_hex" != "$rust_hex" ]]; then
  ok=0
fi
if [[ "$WITH_SWIFT" == "1" && "$java_hex" != "$swift_hex" ]]; then
  ok=0
fi
if [[ "$WITH_DART" == "1" && "$java_hex" != "$dart_hex" ]]; then
  ok=0
fi
if [[ "$WITH_CSHARP" == "1" && "$java_hex" != "$csharp_hex" ]]; then
  ok=0
fi
if [[ "$WITH_JAVA_LITE" == "1" && "$java_hex" != "$java_lite_hex" ]]; then
  ok=0
fi
if [[ "$WITH_JAVA_PXF_LITE" == "1" && "$java_hex" != "$java_pxf_lite_hex" ]]; then
  ok=0
fi

count=4
[[ "$WITH_RUST" == "1" ]] && count=$((count + 1))
[[ "$WITH_SWIFT" == "1" ]] && count=$((count + 1))
[[ "$WITH_DART" == "1" ]] && count=$((count + 1))
[[ "$WITH_CSHARP" == "1" ]] && count=$((count + 1))
[[ "$WITH_JAVA_LITE" == "1" ]] && count=$((count + 1))
[[ "$WITH_JAVA_PXF_LITE" == "1" ]] && count=$((count + 1))

if [[ "$ok" == "1" ]]; then
  echo "✓ All $count ports produce identical envelope bytes."
else
  echo "✗ Envelope wire-format divergence detected." >&2
fi

# ── Annotation extension numbers (STABILITY.md promise 3, issue #244) ────
#
# The envelope above proves the pb codec and carries no annotations. This
# leg hands every descriptor-driven port ONE descriptor set and ONE PXF
# document from testdata/ through `dump-envelope --pb|--sbe` and compares
# the bytes with a checked-in golden. The golden is the oracle, not
# port-to-port agreement: ports that all read the wrong number would agree
# with each other. A port whose reader still looks at a retired 5xxxx
# number decodes ok.pxf without the (pxf.default) values it missed, writes
# an SBE header with template_id 0 (or refuses the file for lacking
# (sbe.schema_id)), or accepts missing-required.pxf, which it must reject
# with exit 1. A port's own suite cannot catch any of that: it compiles the
# port's own vendored annotations.proto, so reader and copy agree with each
# other whatever number they hold.
#
# Ports whose codecs are generated ahead of time cannot load a descriptor
# set at runtime and are listed as SKIP with the issue tracking their
# harness. Python inherits protowire-cpp's numbers at wheel-build time and
# is covered by the C++ leg.

TESTDATA_DIR="$REPO_DIR/testdata"

# mode  descriptor-set  message  document  expected (hex file, or REJECT)
FIXTURES=(
  "pb  annotations/settings.binpb settings.v1.Settings annotations/ok.pxf               annotations/ok.expected.hex"
  "pb  annotations/settings.binpb settings.v1.Settings annotations/missing-required.pxf REJECT"
  "sbe sbe-bench.binpb            bench.v1.Order       sbe-bench.pxf                    sbe-bench.expected.hex"
)

# dumper PORT ARGS... — runs PORT's dump-envelope, built above, with ARGS.
dumper() {
  local port="$1"; shift
  case "$port" in
    go)   (cd "$GO_DIR" && go run ./scripts/dump_envelope "$@") ;;
    cpp)  "$CPP_DIR/build/bin/dump_envelope" "$@" ;;
    ts)   (cd "$TS_DIR" && npx --yes tsx scripts/dump-envelope.ts "$@") ;;
    java) "$JAVA_DIR/dump-envelope/build/install/dump-envelope/bin/dump-envelope" "$@" ;;
    rust) (cd "$RUST_DIR" && cargo run --quiet --release -p dump-envelope -- "$@") ;;
  esac
}

fixture_ports=(go cpp ts java)
[[ "$WITH_RUST" == "1" ]] && fixture_ports+=(rust)

fixtures_ok=1
err_tmp="$TMP_DIR/dumper.err"

echo
echo "Annotation extension numbers — testdata/ descriptor sets through dump-envelope --pb/--sbe:"
for port in "${fixture_ports[@]}"; do
  for line in "${FIXTURES[@]}"; do
    read -r mode fds msg doc expect <<<"$line"
    label="$(printf '%-5s %-4s %s' "$port" "$mode" "$doc")"
    if out="$(dumper "$port" "--$mode" "$TESTDATA_DIR/$fds" "$msg" "$TESTDATA_DIR/$doc" 2>"$err_tmp")"; then
      rc=0
    else
      rc=$?
    fi
    # `|| true`: with pipefail, grep's "no lines" status would end the run
    # on the first port whose stderr is empty -- i.e. on the first success.
    err="$(grep -v -i 'deprecat' "$err_tmp" | head -1 || true)"
    if [[ "$expect" == "REJECT" ]]; then
      case "$rc" in
        1) printf "  %-48s ok (%s)\n" "$label" "$err" ;;
        0) if [[ "$out" == "$go_hex" ]]; then
             printf "  %-48s NO --%s MODE: dumper printed the envelope instead\n" "$label" "$mode"
           else
             printf "  %-48s ACCEPTED, must reject: %s\n" "$label" "$out"
           fi
           fixtures_ok=0 ;;
        *) printf "  %-48s ERROR rc=%s: %s\n" "$label" "$rc" "$err"; fixtures_ok=0 ;;
      esac
      continue
    fi
    golden="$(tr -d '[:space:]' < "$TESTDATA_DIR/$expect")"
    case "$rc" in
      0)
        if [[ "$out" == "$golden" ]]; then
          printf "  %-48s ok\n" "$label"
        elif [[ "$out" == "$go_hex" ]]; then
          # A dumper without the fixture modes ignores its arguments and
          # prints the envelope: the port's default branch predates them.
          printf "  %-48s NO --%s MODE: dumper printed the envelope instead\n" "$label" "$mode"
          fixtures_ok=0
        else
          printf "  %-48s DIVERGED from %s\n" "$label" "$expect"
          printf "    want %s\n    got  %s\n" "$golden" "$out"
          fixtures_ok=0
        fi ;;
      1) printf "  %-48s REJECTED, must accept: %s\n" "$label" "$err"; fixtures_ok=0 ;;
      *) printf "  %-48s ERROR rc=%s: %s\n" "$label" "$rc" "$err"; fixtures_ok=0 ;;
    esac
  done
done

# Codegen ports: no descriptor set at runtime, so no fixture mode yet.
# Each line cites the issue whose done-when replaces it with a dumper call.
[[ "$WITH_CSHARP" == "1" ]]        && echo "  csharp           SKIP  codegen port; fixture modes tracked in trendvidia/protowire-csharp#26"
[[ "$WITH_SWIFT" == "1" ]]         && echo "  swift            SKIP  codegen port; fixture modes tracked in trendvidia/protowire-swift#10"
[[ "$WITH_DART" == "1" ]]          && echo "  dart             SKIP  codegen port; fixture modes tracked in trendvidia/protowire-dart#13"
[[ "$WITH_JAVA_LITE" == "1" ]]     && echo "  java-lite        SKIP  protobuf-javalite has no runtime descriptors; tracked in trendvidia/protowire-java#58"
[[ "$WITH_JAVA_PXF_LITE" == "1" ]] && echo "  java-pxf-lite    SKIP  protobuf-javalite has no runtime descriptors; tracked in trendvidia/protowire-java#58"

echo
if [[ "$fixtures_ok" == "1" ]]; then
  echo "✓ All ${#fixture_ports[@]} descriptor-driven ports read the registered extension numbers."
else
  echo "✗ Annotation extension-number divergence detected." >&2
fi

if [[ "$ok" == "1" && "$fixtures_ok" == "1" ]]; then
  exit 0
fi
exit 1
