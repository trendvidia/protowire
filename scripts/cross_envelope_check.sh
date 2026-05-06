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

echo "→ Go dumper"
go_hex=$(cd "$GO_DIR" && go run ./scripts/dump_envelope)

echo "→ C++ dumper (build + run)"
if [[ ! -d "$CPP_DIR/build" ]]; then
  cmake -S "$CPP_DIR" -B "$CPP_DIR/build" > /dev/null
fi
cmake --build "$CPP_DIR/build" --target dump_envelope -j > /dev/null
cpp_hex=$("$CPP_DIR/build/bin/dump_envelope")

echo "→ TS dumper"
ts_hex=$(cd "$TS_DIR" && npx --yes tsx scripts/dump-envelope.ts)

echo "→ Java dumper (build + run)"
(cd "$JAVA_DIR" && ./gradlew --quiet :dump-envelope:installDist > /dev/null)
java_hex=$("$JAVA_DIR/dump-envelope/build/install/dump-envelope/bin/dump-envelope")

if [[ "$WITH_RUST" == "1" ]]; then
  echo "→ Rust dumper (build + run)"
  rust_hex=$(cd "$RUST_DIR" && cargo run --quiet --release -p dump-envelope)
fi

if [[ "$WITH_SWIFT" == "1" ]]; then
  echo "→ Swift dumper (build + run)"
  (cd "$SWIFT_DIR" && swift build -c release --product dump-envelope > /dev/null)
  swift_hex=$("$SWIFT_DIR/.build/release/dump-envelope")
fi

if [[ "$WITH_DART" == "1" ]]; then
  echo "→ Dart dumper (run)"
  dart_hex=$(cd "$DART_DIR" && dart run bin/dump_envelope.dart)
fi

if [[ "$WITH_CSHARP" == "1" ]]; then
  echo "→ C# dumper (build + run)"
  (cd "$CSHARP_DIR" && dotnet build -c Release --nologo -v quiet \
     cmd/Protowire.DumpEnvelope > /dev/null)
  csharp_hex=$("$CSHARP_DIR/cmd/Protowire.DumpEnvelope/bin/Release/net10.0/dump-envelope")
fi

if [[ "$WITH_JAVA_LITE" == "1" ]]; then
  echo "→ Java/Android (lite) dumper (build + run)"
  (cd "$JAVA_LITE_DIR" && ./gradlew --quiet :dump-envelope-android:installDist > /dev/null)
  java_lite_hex=$("$JAVA_LITE_DIR/dump-envelope-android/build/install/dump-envelope-android/bin/dump-envelope-android")
fi

if [[ "$WITH_JAVA_PXF_LITE" == "1" ]]; then
  echo "→ Java/Android (PXF→lite) dumper (build + run)"
  (cd "$JAVA_LITE_DIR" && ./gradlew --quiet :dump-envelope-pxf-android:installDist > /dev/null)
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
  echo "✓ All $count ports produce identical bytes."
  exit 0
else
  echo "✗ Wire-format divergence detected." >&2
  exit 1
fi
