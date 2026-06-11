# Stability

This document defines the stability surface of the `protowire-*` family. It tells you which interfaces will not change incompatibly without a major-version bump, which interfaces are subject to evolution, and what kinds of changes count as breaking.

It is one of the load-bearing documents of the project — alongside [`docs/grammar.ebnf`](docs/grammar.ebnf) (the formal PXF grammar), the canonical `.proto` files under [`proto/`](proto/), and [`docs/HARDENING.md`](docs/HARDENING.md) (the decoder-safety contract for untrusted input). Where this document and one of those disagree, the `.proto` / EBNF file is authoritative for syntax and field numbers; this document governs the broader compatibility contract.

## Promises

Effective at **0.73.0**, the project commits to the following compatibility properties.

### Wire format — stable

A consumer pinned to any `0.73.0+` release of any port can read text and binary written by any `1.0.0+` release of any port, and vice versa, until a major-version bump is announced.

Concretely:

1. **PXF text grammar.** The grammar in [`docs/grammar.ebnf`](docs/grammar.ebnf) defines the surface a `0.73.0`-era port accepts. Productions may be added in a backwards-compatible way (new value forms, new escape sequences) — accepting strictly more input than before is not a break. Removing a production, narrowing accepted input, or renumbering grammar rule IDs is.
2. **PB and SBE wire formats.** The byte-level layouts that Go's reference implementation produces and accepts at `0.73.0` are the contract. Other ports must round-trip those bytes unchanged; this is enforced by [`scripts/cross_envelope_check.sh`](scripts/cross_envelope_check.sh) on every PR (see [M1 in `ROADMAP.md`](ROADMAP.md#m1--ci-gating-target-0710)).
3. **Annotation extension field numbers.** The integers in [`proto/pxf/annotations.proto`](proto/pxf/annotations.proto) and [`proto/sbe/annotations.proto`](proto/sbe/annotations.proto) — `(pxf.required) = 50000`, `(pxf.default) = 50001`, `(sbe.template_id) = 50200`, etc. — are baked into every emitted descriptor and every cross-port codec. Renumbering them breaks every previously-generated `<Message>PxfMeta` and every wire-encoded `FileOptions`/`MessageOptions`/`FieldOptions` blob in the wild. **Do not change them.** New extensions may be added with new numbers in the reserved 50000–59999 range.
4. **Envelope schema.** [`proto/envelope/v1/envelope.proto`](proto/envelope/v1/envelope.proto) is versioned in the package path. A `v1` envelope produced by any `0.73.0+` port is byte-equivalent to a `v1` envelope produced by any `1.0.0+` port for the same logical value. Incompatible changes bump the package to `v2`; `v1` and `v2` may coexist indefinitely.
5. **Well-known type kind constants** (`PxfMeta.WKT_TIMESTAMP = 1`, …, `PxfMeta.WKT_BIG_FLOAT = 14`). The integers are baked into every emitted `WELL_KNOWN_KINDS` table at codegen time. Adding new entries is fine; renumbering is a wire break.

### v1.0 — spec freeze line

v1.0 is the major bump that closes the pre-1.0 spec evolution period. It includes three one-time text-grammar changes that the wire-stability promise above would otherwise forbid; these are permissible at v1.0 because v1.0 is itself the major bump:

- **`@table` → `@dataset` rename** ([draft §3.4.4](docs/draft-trendvidia-protowire-00.txt)). The row-oriented directive is renamed; semantics are unchanged. v1.0 ports do not accept `@table` and no alias period is provided. Migration is textual substitution.
- **`@proto` directive added** ([draft §3.4.5](docs/draft-trendvidia-protowire-00.txt)). New embedded-schema directive with four body shapes (anonymous, named, source, descriptor). Strictly additive — pre-v1.0 documents that don't use `@table` remain valid v1.0 documents without change.
- **Reserved directive names expanded** ([draft §3.4.6](docs/draft-trendvidia-protowire-00.txt)). The reserved set grows from 5 names to 13. Applications that used `entry`, `table`, `datasource`, `view`, `procedure`, `function`, or `permissions` as a named-directive name must rename.

Past v1.0, the wire-stability promise applies as written: additive grammar changes are permitted at minor versions, removals or narrowings require another major bump.

### v1.2 — schema language additions

v1.2 is a strictly additive minor bump introducing the Protowire Schema Extensions described in [`docs/RFC-001-schema-extensions.md`](docs/RFC-001-schema-extensions.md) and IETF draft `-01`. Three new top-level declarations enter the schema language — `type`, `function`, `annotation` — together with a `@annotation(...)` use-site syntax. The additions satisfy the post-v1.0 additive-only contract:

- **Contextual keywords added.** `type`, `function`, and `annotation` become contextual keywords at v1.2 — recognized as keywords only at the start of a top-level declaration (file scope); in every other position they continue to be accepted as identifiers. Existing schemas that use these words as message names, oneof names (`oneof type { ... }`, common in Google APIs), field names, or enum-value names remain valid v1.2 schemas without modification. The `@` sigil is reserved as the annotation-use-site marker. `expression` is a parameter-type designator inside annotation declarations only; `this` is bound only inside engine-language bodies. **There is no source-level incompatibility introduced in v1.2.**
- **Extension number sub-range claimed.** Numbers `50400`–`50499` are reserved for schema-extension carriers in [`proto/schema/v1/descriptor.proto`](proto/schema/v1/descriptor.proto). Allocated in v1.2.0: `50400` (kind-specific `*_annotations` field on every Options message — `file_annotations` on FileOptions, `message_annotations` on MessageOptions, etc.), `50401` (`functions` on FileOptions), `50402` (`annotation_decls` on FileOptions), `50403` (`type_decls` on FileOptions), `50404` (`source_map` on FileOptions). Numbers `50405`–`50499` are reserved for future schema-extension carriers and follow the same renumbering prohibition as the existing PXF and SBE allocations. (The pre-merge draft of this RFC used `50100`–`50104`; that range was retired during M0 implementation because it collides with SBE's `schema_id` (50100) and `version` (50101) on FileOptions.)
- **Wire format unchanged.** PXF, `pb`, SBE, and envelope outputs are byte-identical between v1.1 and v1.2 for any schema that does not use the new constructs. The new `@validate(...)`, `@required`, and `@default(...)` annotation forms lower to the same descriptor shape as the existing bracket forms; existing `(pxf.required) = 50000` and `(pxf.default) = 50001` numbers remain authoritative and unchanged. A v1.2 port reading a v1.1 schema produces identical outputs to a v1.1 port reading the same schema.
- **Backward compatibility with stock tooling.** The carrier extensions at `50400`–`50404` are well-formed proto3. Stock `protoc`, `protobuf-go`, and every existing protowire port treat them as opaque options when `proto/schema/v1/descriptor.proto` is not imported, preserving them byte-identically across decode/re-encode. Tools that opt into the extensions decode them as typed values.

A v1.1 port reading a v1.2 schema rejects the new keywords at parse time. A v1.2 port reading a v1.1 schema accepts it unchanged. Per-port adoption of v1.2 grammar is independent — schemas pin to the highest minor version they use, and consumers must run a v1.2+ port to read v1.2 schemas.

### CLI surface — evolves

The shared CLI in [`cmd/pxf`](cmd/pxf) follows looser rules. New subcommands and flags can be added at any minor version. Existing flags are deprecated with one minor-version notice before removal at the next major. CLI exit codes are stable (`0` success, `1` user error, `2` internal error), and the JSON output schema produced by `bench-pxf` / `bench-sbe` is stable per [point 6](#promises) below.

### Bench JSON output — stable

The shape of one JSON object per operation that each port's `bench-pxf` / `bench-sbe` emits is stable wire-of-the-bench-aggregator: [`scripts/cross_pxf_bench.sh`](scripts/cross_pxf_bench.sh) parses these. Field names (`port`, `op`, `ns_per_op`, `mib_per_sec`, `iterations`, `bytes`) and types are pinned. New fields may be added; existing ones may not be renamed or retyped.

## What this does *not* commit to

- **Library-level API stability** for any port's library code (`protowire-go/encoding/pxf/...`, `protowire-java/pxf/...`, etc.). Library APIs may evolve at minor versions per each port's own conventions; the wire stability commitment is what crosses repo boundaries. Most ports follow SemVer for their library API independent of this document, but the cross-port commitment lives here.
- **Performance characteristics.** A port may make any change that preserves wire-equivalence and CLI/bench JSON shape, even if it regresses runtime. The [M5 perf-regression CI gate](ROADMAP.md#m5--performance-regression-ci-target-0740) catches >20% degradations in PR; smaller drift is accepted.
- **Internal codec class names** in the per-port libraries (e.g. Java's `LiteWireWriter`, `LiteWireReader`, `PxfMeta`). Cross-port wire equivalence does not require any specific Java/Go/Rust class layout.

## Runtime-tier exclusions

Targets that strip protobuf descriptor reflection at runtime — the lite tier — drop a documented set of capabilities relative to the full tier. This applies most prominently to the Java port's `*-android` modules built on `protobuf-javalite`, but the same exclusions apply to any future `*-lite` target in any port.

| Capability | Full tier | Lite tier |
|---|---|---|
| `unmarshal(text, descriptor)` schema-agnostic decode (`DynamicMessage`-style) | ✓ | ✗ |
| `TextFormat` (Google's text format, not PXF) | ✓ | ✗ |
| `JsonFormat` round-trip | ✓ | ✗ |
| Runtime descriptor compilation (`protocompile`) | ✓ | ✗ |
| `Any.unpack()` against arbitrary types | ✓ | ✗ — caller pre-registers expected types |
| PXF / PXF-binary / SBE / Envelope | ✓ | ✓ |
| Codegen-driven typed unmarshal/marshal (`<Message>PxfCodec.unmarshal`) | ✓ | ✓ |
| Well-known types (Timestamp, Duration, `*Value` wrappers, `pxf.{BigInt,Decimal,BigFloat}`) | ✓ | ✓ |
| Wire equivalence with the full tier | n/a | ✓ — CI-enforced |

**Lite-mode emitted code is wire-equivalent to full-mode for the same `.proto` input.** This is a CI-enforced invariant via [`scripts/cross_envelope_check.sh`](scripts/cross_envelope_check.sh)'s `java-lite` / `java-pxf-lite` rows, not a documentation promise: divergence between the JVM `java` row and any lite row fails the PR.

## Deprecation policy

When something stable must be removed:

1. **Announce in `CHANGELOG.md`** at the minor version where deprecation begins, with a clear migration path.
2. **Emit a deprecation marker** in code where applicable (Go `//Deprecated:`, Java `@Deprecated`, Rust `#[deprecated]`, etc.). Existing call sites continue to work.
3. **Remove at the next major.** Minimum gap from announcement to removal is one minor version, two is preferred.

Wire-format breaking changes — bumping the envelope from `v1` to `v2`, renumbering an annotation extension, narrowing the PXF grammar — require a major bump on the project as a whole, not just on the affected port.

## Reporting a break

If you observe a port whose wire output diverges from another port for the same input, that's a **wire-equivalence regression** and should be filed as a bug against [`trendvidia/protowire`](https://github.com/trendvidia/protowire) — not against the individual port. Cross-port issues are triaged here.

If you observe a CLI or bench JSON change that breaks downstream tooling, file against the repo whose CLI / bench output changed.

## Versioning of this document

`STABILITY.md` itself is versioned with the project. Edits that strengthen guarantees (add a stable surface, narrow a "may evolve") are welcome at any minor version. Edits that weaken guarantees require a major bump.
