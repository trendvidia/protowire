# Roadmap

This document is the canonical roadmap for the `protowire-*` family of repositories. It tracks **family-wide milestones** (cross-port concerns that ship together) and **per-port milestones** (gaps specific to one language port).

The roadmap is itself governed by [Steward](https://steward-dev.ai) — the [`governance.pxf`](governance.pxf) constitution treats this file as the project's source of truth for what's scheduled, what's deferred, and why.

---

## Versioning policy

All ports share a single major.minor release cadence so cross-port wire-equivalence is verified at known points in time. Patch numbers are per-port and may diverge.

- **0.70.0** — current baseline. Library code works; published packages, CI gating, security review, and several per-port features are still missing. Not recommended for production in the senses described in the README "Limitations & open gaps" section of each port.
- **0.71.0 → 0.79.x** — see milestones below.
- **1.0.0** — declared only when (a) all ports are published on their native registry, (b) cross-port wire-equivalence runs in CI on every PR, (c) the wire format has a formal stability statement, and (d) PXF parsing has been fuzzed. Not before.

The version drift across port manifests at the start of 0.70 (some at `0.0.0`, some at `0.1.0`, one at `1.0.0`) was an artifact of independent slice work and has been reset.

---

## Family-wide milestones

These ship together — each gates a release across all 9 ports.

### M1 — CI gating (target: 0.71.0)

**Problem.** Cross-port wire-equivalence is verified by `scripts/cross_envelope_check.sh` only when someone runs it locally. A wire-format regression in one port can merge and not be caught for days.

**Plan.**
1. ✅ GitHub Actions workflow in this repo that runs on PRs touching anything wire-format-relevant (`proto/**`, `scripts/cross_*`, port repo refs). See `.github/workflows/cross-port-envelope-check.yml`.
2. Same workflow in each port repo — runs the local test suite and (where applicable) `dump_envelope` against the spec testdata.
3. Cross-port equivalence as a *required* check before merge (branch protection rule).
4. Cache toolchains; total wall-clock target ≤ 8 min.

**Acceptance.** A wire-format regression introduced in any port fails CI within a single PR cycle.

### M2 — Publishing pipelines (target: 0.72.0)

**Problem.** Nothing is published. To use any port today you vendor source. That's an absolute bar for any organization with supply-chain review.

**Plan.** Each port publishes to its native registry, with reproducible-build receipts where available:

| Port | Registry | Publishing path |
|---|---|---|
| Go | proxy.golang.org | git tag, [no work needed beyond version pinning] |
| Java | Maven Central | sigstore-signed JAR via `maven-publish` |
| C++ | vcpkg + Conan | recipe upstreamed |
| Rust | crates.io | `cargo publish` workspace |
| TypeScript | npm | scoped `@trendvidia/protowire` |
| Python | PyPI | wheels for linux x86_64/aarch64, macOS, Windows via cibuildwheel |
| C# | NuGet | signed package |
| Dart | pub.dev | `dart pub publish` |
| Swift | Swift Package Index + CocoaPods | git tag + podspec |

**Pre-built CLI binaries.** The shared CLI in `cmd/pxf` ships as platform-native binaries for linux/macOS/windows × x86_64/arm64 via GoReleaser. Currently non-Go users must either install Go or build from source.

**Acceptance.** A fresh project can `<install one of: go install / cargo add / pip install / npm install / dotnet add package / pub get / pod install / swift add / mvn / vcpkg install>` and use the library without a build-time toolchain (other than Python's wheel-or-build fallback).

### M3 — Wire format stability (target: 0.73.0)

**Problem.** PXF's grammar lives in `docs/grammar.ebnf` (~150 lines) — that's the spec. Nothing commits the project to backwards compatibility. Annotation extension numbers in the 50000s are reserved but the syntax could still evolve, and the Envelope schema is `v1` with no formal contract.

**Plan.**
1. ✅ **`STABILITY.md`** in this repo defining: which surfaces are stable (wire format, envelope schema, annotation field numbers), which surfaces evolve (CLI flags, grammar productions added in a backwards-compatible way), and the deprecation policy. Includes an explicit subsection on **runtime tier exclusions**: targets that strip descriptor reflection at runtime (notably the Java/Android `*-android` modules built on `protobuf-javalite`) drop `DynamicMessage`-style schema-agnostic unmarshal, `TextFormat`, `JsonFormat`, runtime descriptor compilation, and schema-agnostic `Any.unpack()`. Lite-mode emitted code is wire-equivalent to full-mode for the same `.proto` input — this is a CI-enforced invariant, not a documentation promise.
2. Tag `proto/envelope/v1/envelope.proto` and `proto/pxf/annotations.proto` field numbers as "load-bearing — do not change" with a comment + a CI lint that rejects renumbering.
3. Add wire-corpus regression tests: hand-curated PXF/PB byte fixtures that every port must continue to round-trip across versions.

**Acceptance.** A consumer pinned to 0.73.0 of any port can read text/binary written by 1.0.0+, and vice versa, until the project explicitly bumps the schema major version.

### M4 — Fuzzing PXF parsing (target: 0.73.0)

**Problem.** `unmarshal()` accepts user text input across every port. Deeply-nested blocks, oversized integers, billion-laughs-style attacks, malformed UTF-8 — none of it has been adversarial-tested.

**Plan.**
1. Go: `go-fuzz` corpus + 24h fuzz job in CI on a schedule (not blocking on PRs).
2. C++: libFuzzer harness reusing the Go corpus.
3. Rust: `cargo-fuzz` harness reusing the corpus.
4. The 5 reflection-based ports (Java/TS/Python/C#/Dart/Swift) inherit the input-side robustness from the C++ or Go core where they wrap one; standalone implementations get a per-port harness.
5. Track findings in `SECURITY.md`; embargo any RCE-class issues per the Steward escrow rules.

**Acceptance.** 24 hours of dedicated fuzzing without any port crashing, hanging, or returning attacker-controlled memory.

### M5 — Performance regression CI (target: 0.74.0)

**Problem.** `scripts/cross_pxf_bench.sh` and `cross_sbe_bench.sh` produce numbers but nothing gates on them. A 10× perf regression could land silently.

**Plan.** GitHub Actions job runs the bench scripts on a fixed runner, compares against the previous main commit's recorded baseline, fails the PR if any port regresses by >20% beyond noise. Numbers are written to a results-archive branch so historical drift is visible.

**Acceptance.** A 10× slowdown in any port's PXF unmarshal hot path fails the offending PR.

### M6 — Security audit + threat model (target: 0.74.0)

**Problem.** No formal security review of the wire codecs. Threats include parser DoS, unsafe deserialization (Java/C# reflection paths), supply-chain integrity (Go's `protobuf-go` fork).

**Plan.**
1. Threat model document in `docs/THREAT_MODEL.md` enumerating the assets, trust boundaries, and known risks per port.
2. Third-party audit of the Go reference implementation (the canonical wire path).
3. Per-port code review of any `unsafe` blocks (Rust, Go SetUnsafe paths) and any reflection-driven setters.
4. Established vuln-disclosure process: `SECURITY.md` with a contact and embargo policy.

### M8 — HARDENING conformance corpus (target: 0.75.0)

**Problem.** [`docs/HARDENING.md`](docs/HARDENING.md) defines the decoder-safety contract every port must enforce against untrusted input — depth caps, size caps, UTF-8 strictness, SBE bounds checks, no-crash-on-attacker-input. Today nothing checks that any port actually obeys that contract, and three of the universal critical bugs (recursion-depth bypass, SBE `count × block_length` overflow, lossy UTF-8 in `string` fields) sit unfixed across most ports because there's no failing CI to drive their fix. M4 (fuzzing) finds *new* attacker inputs; M8 prevents *known* attacker inputs from regressing.

This is distinct from [M6](#m6--security-audit--threat-model) (third-party point-in-time review) and [M4](#m4--fuzzing-pxf-parsing) (generative fuzzing for novel bugs): M8 is a hand-curated, version-controlled corpus that travels with the spec and gates every PR.

**Plan.**
1. **Adversarial corpus** under `testdata/adversarial/`, organised per format (`pxf/`, `pb/`, `sbe/`, `envelope/`). Each input file is named after the attack class (e.g. `pxf/deep-nesting-1000.pxf`, `sbe/group-count-overflow.sbe`); a sibling `MANIFEST.jsonl` declares each file's format, the expected reject reason, and any port-specific opt-out (when a port legitimately can't reach the code path being tested). The corpus categories are enumerated in [`docs/HARDENING.md` § Conformance corpus](docs/HARDENING.md#conformance-corpus).
2. **`cmd/check-decode/` per port** — a tiny harness invoked as `check-decode --format <pxf|pb|sbe|envelope> --input <path>`. Exits 0 on successful decode (the corpus contains some happy-path inputs to verify the decoder isn't broken), non-zero with a clean error on rejection, and is required to neither crash, hang, nor exceed 256 MiB RSS on any corpus input. The binary is ~50 lines per port: it wires the existing decoder up to a single-file CLI.
3. **`scripts/cross_security_check.sh`** drives the corpus across every port's `check-decode` binary under a 5-second wall-clock + 256 MiB RSS budget, asserts the expected reject/accept verdict for each (corpus, port) pair, and prints a table. Auto-skips ports whose `check-decode` binary doesn't yet exist (same pattern as `cross_pxf_bench.sh` for dart / java-lite).
4. **CI gate** — GitHub Actions runs `cross_security_check.sh` on every PR touching `proto/**`, `docs/HARDENING.md`, `testdata/adversarial/**`, or any port's decoder. Required check.
5. **Disclosure** — when a corpus addition exposes a live vulnerability in a published port, the addition is held under a 30-day embargo per the `SECURITY.md` policy from M6; the corpus file lands together with the per-port fix.

**Acceptance.** Every shipping port's `check-decode` binary, run against the full `testdata/adversarial/` corpus, produces the expected verdict for every (corpus, port) pair within the wall-clock and RSS budget. A regression that re-introduces any of the three universal critical bugs from the 2026-Q2 cross-port review fails CI.

### M7 — Pre-built CLI binaries (target: 0.72.0, paired with M2)

**Problem.** The shared CLI is Go-only. Non-Go users either install a Go toolchain or build from source.

**Plan.** GoReleaser config in `cmd/pxf/`:
- linux x86_64/aarch64 (musl + glibc)
- macOS x86_64/arm64 (signed + notarized)
- windows x86_64
- arm64 build for Raspberry Pi class
- Hosted on the GitHub Releases page; `protowire` Homebrew formula tracks releases; SHA256s pinned in port READMEs.

---

## Per-port milestones

Ordered roughly by "biggest deal-breaker first" within each port.

### Go (`protowire-go`)

The reference implementation. Fewest gaps; the focus here is supply-chain hygiene.

#### Already shipped

- ✅ **`pxf.Secret` well-known type recognition.** PXF scalar shorthand (`pw = "x"`) and explicit block form (`pw { value = "x", hint = "h" }`) both decode; encode picks the form based on whether `hint`/`fingerprint` is set so authoring metadata round-trips. Canonical descriptor in `proto/pxf/secret.proto`; codec stays free of any memory-protection dependency (mlock/wipe semantics are the consumer runtime's concern). See `encoding/pxf/secret_test.go` for the conformance set. Sibling ports gain equivalent recognition as consumer demand appears — see "Cross-port follow-up" below.

#### Pending

- **0.71.0** — Pin `trendvidia/protobuf-go` fork to a specific SHA in `go.mod` with a comment explaining (a) why the fork exists, (b) what the diff against upstream is, (c) when we'll attempt upstreaming.
- **0.72.0** — Reproducible-build verification: `go install github.com/trendvidia/protowire/cmd/pxf@<tag>` produces byte-identical binaries given the same Go toolchain version.
- **0.74.0** — File the upstream issue against `google.golang.org/protobuf` for `dynamicpb.SetUnsafe` / `AppendUnsafe` / `MapSetUnsafe`. If accepted, drop the fork at 1.0.0; if rejected, document the fork as permanent.

#### Cross-port follow-up

- **`pxf.Secret` recognition in non-Go ports** is *not* a roadmap commitment — it lands in a port only when (a) a consumer for that language needs it, or (b) a wire-equivalence test fails. The reference implementation is `protowire-go @ encoding/pxf/{wellknown,decode_fast,encode}.go`; the conformance fixtures are `protowire-go @ encoding/pxf/secret_test.go`. The canonical descriptor lives in `proto/pxf/secret.proto` (this repo) alongside `bignum.proto`; ports that pre-generate WKT bindings can pick it up from there.

### Java (`protowire-java`)

#### Already shipped

- ✅ **`protoc-gen-pxf-java-meta` Go plugin** (canonical-side `cmd/protoc-gen-pxf-java-meta/`). Emits `<Message>PxfMeta` / `<Message>PxfCodec` / `<Message>SbeMeta` / `<Message>SbeCodec` companions from `.proto` files including the `(pxf.required)`, `(pxf.default)`, `(sbe.*)` annotation values lifted at codegen time. `lite` plugin parameter triggers the typed codec emit. Consumed via `buf generate` (recommended) or plain `protoc`; Gradle users wire it through the official [`build.buf.gradle`](https://github.com/bufbuild/buf-gradle-plugin) plugin and a small `buf.gen.yaml` next to their `.proto` sources. There is **no custom Gradle plugin in this project** — this avoids per-port Gradle/Bazel/Maven plugin rewrites and lets the same plugin shape generate code for the C++/Rust/Swift ports later (sibling `protoc-gen-pxf-<lang>-meta` binaries). The plugin replaces the runtime `DynamicMessage` cost for projects that opt in.
- ✅ **`pxf-runtime` + `sbe-runtime` extractions.** Descriptor-free pieces of the codecs live in their own modules; `:pxf` and `:sbe` keep the descriptor-driven adapter on top. Behavior-preserving; tests pass unchanged.
- ✅ **`pxf-android` + `envelope-android`.** Lite-tier wire codec on `protobuf-javalite`. `LiteWireWriter` (text → wire) and `LiteWireReader` (wire → text via AST + `Format`) cover all field types: scalars, repeated/packed, nested, maps, oneofs, defaults, required, well-known types (`Timestamp`, `Duration`, `*Value` wrappers, `pxf.BigInt` / `Decimal` / `BigFloat`). WKT dispatch keys off codegen-emitted `WELL_KNOWN_KINDS` constants on both encoder and decoder.
- ✅ **`bench-pxf-android` + `bench-sbe-android` lite-tier perf harnesses.** Cross-port `scripts/cross_pxf_bench.sh` and `scripts/cross_sbe_bench.sh` report `java-lite` rows alongside `java`. On the canonical 94-byte SBE fixture, lite is 2.4× / 4.1× faster than the descriptor-driven path on marshal / unmarshal.
- ✅ **Cross-port wire-equivalence CI.** GHA workflow (`.github/workflows/cross-port-envelope-check.yml`) runs the 10-port envelope check on every relevant PR.
- ✅ **`STABILITY.md`** documenting the wire-format compatibility surface.

#### Pending

- **0.72.0** — Maven Central publishing via the `maven-publish` Gradle plugin + sigstore signing. Artifacts land under the registered `org.protowire` Maven groupId.
- **0.73.0** — `:sbe-android` + `:pb-android` thin library modules under `org.protowire` Maven coordinates. `:sbe-android` is essentially a re-export of `:sbe-runtime`; `:pb-android` re-exports `protobuf-javalite`. Closes the published-artifact story for the lite tier.
- **0.74.0** — Performance work: codegen path hits 1.5× of the Go reference's PXF unmarshal on the canonical fixture. (Lite tier already wins decisively over JVM full tier on SBE; PXF measurement vs Go is the remaining target.)
- **0.74.0** — Document & decide: backport to Java 11 (significant work — sealed classes, records, pattern switches all used) or hold the line at 17+. The decision is a function of demand. Note: the `*-runtime` and `*-android` modules may need to drop sealed/record/pattern-switch syntax independently of the JVM library decision, since Android desugaring of those features is brittle on minSdk 21+.
- **0.75.0** — Android polish: `consumer-rules.pro` shipped inside each `*-android` artifact (R8 keep rules to prevent `Class.forName()` strip-and-crash), AAR packaging if a downstream sample app surfaces the need, and an Android sample project living in a sibling repo (`protowire-java-samples`) that runs an instrumented test on an emulator nightly to catch ART quirks (e.g. javalite's `BoundedByteString` reference semantics). Not PR-blocking; nightly only.
- **0.75.0** — Method-count audit + cold-start benchmark on a real Android device (not desktop JVM). Desktop JVM benchmark numbers are misleading for cold start, dex loading, and battery profiling on Android.

#### Android support — explicit out-of-scope

Lite-tier targets that strip protobuf descriptor reflection at runtime drop `DynamicMessage`-style schema-agnostic unmarshal, `TextFormat`, `JsonFormat`, runtime descriptor compilation, and schema-agnostic `Any.unpack()` (lite users must pre-register expected `Any` types). Wire equivalence with the full tier is a CI-enforced invariant. See [`STABILITY.md`](STABILITY.md) for the authoritative list.

#### Kotlin extensions companion (`protowire-kotlin`)

A separate full Kotlin port would be redundant — Kotlin code calling the Java port works today, the wire codec doesn't differ between Java and Kotlin, and dual implementations are a perpetual drift problem. The right shape is a small Kotlin companion module that adds the idiomatic surface around the Java codecs (the same pattern Retrofit, OkHttp, Coil, and most modern JVM libraries use).

- **0.73.0** — `protowire-kotlin` companion artifact (sibling repo, depends on the Java port). Adds:
  - `suspend` extensions on `Pxf` / `Pb` / `Sbe` codecs so callers don't manually `withContext(Dispatchers.IO)`.
  - DSL builders for `Envelope` / `AppError` / `FieldError`.
  - Sealed-class `PxfResult` mapping `unmarshalFull`'s output (cleaner than the Java `Result` interface from idiomatic Kotlin).
  - `Flow<T>` extensions for any future streaming reads.
- **0.73.0** — Maven Central publish alongside the Java artifact; same release cadence.

**Kotlin Multiplatform (KMP) is explicitly out of scope** for the roadmap below 1.0.0. The protowire-* family already has a Swift port; a KMP-Common implementation would compete with both Java and Swift, and the wire codecs aren't the part of the API where shared code-bases pay off. If iOS/Android code-sharing becomes a goal later, that's a separate strategic conversation.

### C++ (`protowire-cpp`)

- **0.72.0** — vcpkg port + Conan recipe upstreamed; users can `vcpkg install protowire-cpp` or add a single line to `conanfile.py`.
- **0.73.0** — Header-only / single-include amalgamation generated at release time; lets Bazel / single-translation-unit build systems consume without CMake.
- **0.74.0** — `-fno-exceptions` support: convert internal error paths to `std::expected` (C++23) or a custom result type while preserving the public exception-throwing API as opt-in.
- **0.74.0** — Eliminate the macOS Homebrew include-path footgun by detecting the conflict at CMake configure time and failing fast with a clear message rather than silently picking the older install.

### Rust (`protowire-rust`)

- **0.72.0** — `cargo publish` to crates.io; tested on stable + MSRV pinned.
- **0.72.0** — Track `prost-reflect` upstream API changes; pin to a specific version with explicit upgrade notes.
- **0.73.0** — Native `num-bigint::BigInt` / `rust_decimal::Decimal` integration for `pxf.BigInt` / `pxf.Decimal` schemas. Today users get `Vec<u8>` and convert themselves.
- **0.74.0** — Optional `libprotoc` binding behind a feature flag so the SBE XML round-trip works in-process (matches the Go reference's `protocompile` capability).

### TypeScript (`protowire-typescript`)

- **0.72.0** — `npm publish` of `@trendvidia/protowire`; CI verifies the published tarball against the source.
- **0.72.0** — Bundle-size budget: ESM tree-shake test in CI, < 50 KB gzipped for the PXF-only entry.
- **0.73.0** — Browser SBE codec verification: replace `Buffer` with `Uint8Array` everywhere in the SBE path; add a Vitest browser-env suite.
- **0.73.0** — Deno + Bun compatibility verification; the published tarball declares `exports` for both runtimes.
- **0.74.0** — Decouple the PXF annotation reader from `protobuf-es` internals — the current implementation hand-decodes unknown bytes and is fragile.

### Python (`protowire-python`)

- **0.72.0** — `cibuildwheel` matrix for linux x86_64/aarch64 (manylinux + musllinux), macOS x86_64/arm64, Windows x86_64. Wheels published to PyPI on each tag. **Highest-value contribution opportunity** — eliminates the C++ toolchain barrier today's `pip install` requires.
- **0.73.0** — Optional pure-Python fallback (uses `google.protobuf` directly): heavier but lets `pip install protowire --no-binary` work without compilers, and unlocks Lambda / App Engine layers.
- **0.74.0** — Free-threaded Python (3.13t / `--disable-gil`) compatibility; nanobind supports it but the build hasn't been validated.
- **0.74.0** — Zero-copy `MessageView` path so Python callers don't serialize once before each FFI call.

### C# (`protowire-csharp`)

- **0.72.0** — `net8.0` target alongside `net10.0` (multi-target) so existing enterprise .NET shops on LTS aren't excluded. **Highest-value adoption fix** for this port.
- **0.72.0** — NuGet publishing.
- **0.73.0** — Async API surface: `Task<Document> UnmarshalAsync(Stream)` for large documents.
- **0.74.0** — Source generator path (alongside reflection): compile-time field-binding eliminates the per-call `Reflection.IFieldAccessor` cost. Should bring PXF unmarshal under 5 µs on the canonical fixture.

### Dart (`protowire-dart`)

- **0.72.0** — `pub.dev` publishing.
- **0.72.0** — `(pxf.required)` / `(pxf.default)` annotation enforcement. The Dart `protobuf` package exposes `FieldOptions` via `BuilderInfo` (unlike Swift), so this is tractable. Validate required-but-absent fields, apply declared defaults to absent fields. Tests for both.
- **0.73.0** — `bin/bench_pxf.dart` and `bin/bench_sbe.dart` cross-port harnesses. Requires adding `bench-test.proto` + `sbe-bench.proto` to `proto/` and regenerating; harness then mirrors the C# / Swift pattern.
- **0.73.0** — Move `lib/src/encoding/pb/native.dart` (the `dart:mirrors`-using codec) into a separate `protowire_dynamic` package or delete entirely. The current header doc-comment quarantines it but it's still in-tree; Flutter consumers shouldn't be able to accidentally pull it.
- **0.74.0** — Performance: replace `Mirror`-driven encoder paths in PXF and SBE with descriptor-driven equivalents matching the cross-port design.

### Swift (`protowire-swift`)

- **0.72.0** — Swift Package Index registration; CocoaPods podspec.
- **0.72.0** — `(pxf.required)` / `(pxf.default)` annotation enforcement. swift-protobuf doesn't expose runtime descriptor reflection, so this needs one of two approaches:
  - **(a)** Marker protocol — `PXFAnnotated` that user Codable types opt into, declaring `static var requiredFields` and `static var defaults`.
  - **(b)** Build a Swift descriptor reflection layer on top of `Google_Protobuf_FileDescriptorProto` / `FieldOptions`.
  - Decision deferred to design discussion in 0.71.0; reference implementation in 0.72.0.
- **0.73.0** — Descriptor-driven SBE codec. The current SBE codec is dictionary-template-based; users hand-build `SBE.MessageTemplate` instances. Build the template from `(sbe.template_id)` / `(sbe.length)` / `(sbe.encoding)` annotations on a proto file at runtime.
- **0.73.0** — `cmd/bench-sbe` harness, depending on the descriptor-driven SBE codec landing first.
- **0.74.0** — Performance: replace `Mirror`-based encoder heuristics with a SwiftProtobuf-Message-driven path. Target: bring PXF unmarshal under 50 µs on the canonical fixture (currently ~280 µs, ≈50× behind Go).

---

## Out of scope (for now)

- **Multi-byte SBE schema versioning.** The SBE wire format declares a `(sbe.version)` per schema; we treat it as a hint, not a migration mechanism. A formal SBE schema-version negotiation protocol would be its own project.
- **Streaming PXF.** The decoder loads the whole document. Streaming-aware variants are interesting but add complexity that hasn't been demanded.
- **Schema compilation at runtime in non-Go ports.** The Go reference uses `protocompile`; ports that wrap Go (or rebuild on top of `libprotoc`) could expose it later, but it's not on the critical path to 1.0.
- **PXF / SBE editor tooling beyond `editors/` syntax-highlighting.** Language-server protocol support, autocomplete, schema-aware lint — open work, not on the 1.0 path.

---

## How to pick something up

1. Find an item above whose acceptance criteria you can satisfy.
2. Open a draft PR in the relevant port repo. Steward will route it through private mentorship before public review — first contributions get private feedback rather than public friction.
3. New contributors start at zero trust; large PRs go through the escrow pipeline (2–3 sandbox issues to provide proof-of-work) before unlocking the primary PR for community voting. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full lifecycle.
4. The `governance.pxf` constitution in the Steward repo defines the voting math; in short, voting weight is per-directory expertise that decays with inactivity, so contributors who actually ship in `pxf/` decide the next `pxf/` change.

If your interest doesn't fit any item above, file an issue on this repo (`trendvidia/protowire`) describing the gap. Steward auto-triages, deduplicates, and labels.
