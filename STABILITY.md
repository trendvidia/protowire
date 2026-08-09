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
- **Wire format unchanged.** PXF, `pb`, SBE, and envelope outputs are byte-identical between v1.1 and v1.2 for any schema that does not use the new constructs. Bracket-written `(pxf.required) = 50000` and `(pxf.default) = 50001` options remain authoritative, unchanged, and lower identically to v1.1. A v1.2 port reading a v1.1 schema produces identical outputs to a v1.1 port reading the same schema.
- **No legacy dual-emission.** The `@required` / `@default(...)` annotation forms lower **only** to the `50400` schema-extension carrier — they do not additionally emit `(pxf.required)` / `(pxf.default)` (RFC-001 §8.5, decided in issue #92). Consumers that read only the legacy extension numbers (e.g. a v1.1 PXF runtime enforcing `(pxf.required)` at decode time) are **not supported against schemas that use the annotation forms**: they observe bracket-written options and nothing else. Migration order therefore matters — upgrade consumers to carrier-aware (v1.2) versions before rewriting schemas from brackets to annotations. Both forms MAY coexist on a field during migration; compilers MAY warn on conflicting values but never reconcile the two surfaces.
- **One foreign extension is emitted: `google.api.http`.** From v1.10.0, `@http` lowers its routing skeleton to the standard `google.api.http` option on `MethodOptions` (field `72295728`) as well as to the `50400` carrier (RFC-001 §5.2, issue [#213](https://github.com/trendvidia/protowire/issues/213)). This is **not** a reversal of the no-legacy-dual-emission rule above: that rule refuses two competing surfaces for semantics this project owns, whereas `72295728` is a number protowire does not own, carrying a strictly narrower fact (verb and path) into a vocabulary every REST binder already reads. The number, its `google.api.HttpRule` shape, and its meaning are governed by googleapis, not by this document; protowire promises only *what it writes there* — the §5.2 mapping table — and that it writes nothing there when the schema author wrote the option by hand. The descriptor output of a schema using `@http` therefore changes at v1.10.0; PXF, `pb`, SBE, envelope, report and doc-pack bytes do not. `pxf build --google-api-http=false` restores the pre-v1.10.0 descriptor bytes.
- **Backward compatibility with stock tooling.** The carrier extensions at `50400`–`50404` are well-formed proto3. Stock `protoc`, `protobuf-go`, and every existing protowire port treat them as opaque options when `proto/schema/v1/descriptor.proto` is not imported, preserving them byte-identically across decode/re-encode. Tools that opt into the extensions decode them as typed values.

A v1.1 port reading a v1.2 schema rejects the new keywords at parse time. A v1.2 port reading a v1.1 schema accepts it unchanged. Per-port adoption of v1.2 grammar is independent — schemas pin to the highest minor version they use, and consumers must run a v1.2+ port to read v1.2 schemas.

### v1.2 — keyed repeated fields

v1.2 also carries keyed repeated fields (issue [#116](https://github.com/trendvidia/protowire/issues/116); IETF draft `-01` §3.13) — a **text-grammar** addition of the kind promise 1 explicitly permits, where the parser accepts strictly more input than before:

- **Grammar.** `field_entry` gains a string alternative for its key (`(identifier | string), (assignment_tail | block_tail)`), so `"us-east-1" { ... }` parses. Both quoted forms were parse errors before v1.2, so no existing document changes meaning. The schema layer confines quoted entry names to keyed repeated fields.
- **Extension number claimed.** `(pxf.key) = 50002` on `FieldOptions` in [`proto/pxf/annotations.proto`](proto/pxf/annotations.proto). The renumbering prohibition of promise 3 applies.
- **Wire format unchanged.** A keyed repeated field is a plain repeated field in `pb` and SBE; the key is an ordinary field of the element message. PXF, `pb`, SBE, and envelope outputs are byte-identical between v1.1 and v1.2 for any document/schema that does not use the keyed form.
- **Version gating.** A document that uses quoted entry names requires a v1.2+ parser (earlier ports reject at parse time). A document in unquoted keyed form parses on any port, but binding it needs a v1.2+ schema layer — an earlier port sees unknown field names inside the block and rejects at bind time. The anonymous list form remains valid on every port, so schemas can adopt `(pxf.key)` before all their consumers upgrade: pre-v1.2 consumers treat the option as opaque and keep reading anonymous-form documents unchanged.

### v1.3 — map-key violation flag

v1.3 is a strictly additive minor bump touching only the runtime validation-report shape ([`proto/schema/v1/report.proto`](proto/schema/v1/report.proto)):

- **New report field.** `EnrichedViolation.for_key` (field 8, `bool`) flags violations whose rule was evaluated against a map entry's *key* rather than the value the path's final subscript addresses (RFC-001 §7, issue [#125](https://github.com/trendvidia/protowire/issues/125)). Engines MUST set it on key violations; a v1.2 report consumer reading a v1.3 report ignores the unknown field, and a report without map-key violations is byte-identical to its v1.2 form.
- **No grammar or descriptor change.** The schema language, extension-number allocations, and descriptor lowering are untouched — schemas and documents need no version gating, and every v1.2 port parses v1.3 schemas unchanged. The version bump exists because the report shape is part of the cross-port conformance surface (all 10 ports emit equivalent reports): engines claiming v1.3 report conformance must implement the `for_key` semantics.

### v1.4 — report-semantics pins

v1.4 is a semantics-only minor: no grammar, descriptor, wire-format, or report-shape changes — [`proto/schema/v1/report.proto`](proto/schema/v1/report.proto) is byte-compatible with v1.3 (comment-only edits), and schemas and documents need no version gating. The bump exists because the cross-port conformance surface tightened (all 10 ports emit equivalent reports):

- **`RULE_KIND_DEFAULT` semantics.** Engines claiming v1.4 report conformance MUST emit `rule_kind: RULE_KIND_DEFAULT` — superseding `VALIDATE`/`TYPE_REFINEMENT` — for violations from rules evaluated against a `@default`-substituted value, with `actual_value` set to the substituted default (RFC-001 §6.4, issue [#133](https://github.com/trendvidia/protowire/issues/133)).
- **Params provenance.** Engines MUST leave `Violation.params` empty on violations produced by inline expression rules; params are authored only by function implementations or spec-defined synthetic violations (RFC-001 §7, issue [#134](https://github.com/trendvidia/protowire/issues/134)). `params` participates in cross-port report equality with no carve-out.
- **Executable conformance target.** The fixture-07 report golden is executable end-to-end: `testdata/schema-extensions/07_report_golden/` pins the §5.3 worked-example schema, the invalid instance, and the conformance stubs (issue [#135](https://github.com/trendvidia/protowire/issues/135)). Reports emitted for v1.3-conformant instances that use neither `@default` substitution nor inline-expression params are unchanged.

### v1.5 — doc packs + OpenAPI boundary

v1.5 is an additive minor: no wire-format changes (PXF, `pb`, SBE, and envelope outputs are byte-identical to v1.4 for every schema and document) and `report.proto` is byte-compatible (comment-only edits). Schemas and documents need no version gating. What changes for conformance claims:

- **`@http` operation surface (additive).** `annotations.proto`'s `@http` gains four defaulted parameters (`summary`, `operation_id`, `tags`, `security`; RFC-001 §5.2). The v1.2 two-argument form keeps its meaning, no existing schema changes shape, and the parameters carry no validation semantics — conformance is carrying them through the §8.1 carrier, nothing more. Responses remain derived; there is no `responses` parameter (GH #177).
- **`@sensitive(class)` (additive + one rejection).** The classification parameter is org-defined and defaulted; compilers claiming v1.5 MUST reject `protowire.`-prefixed class values (§6.7 rule 1, fixture `invalid/reserved_sensitive_class.proto`).
- **Parser surface tightenings.** Message literals as list elements at annotation arguments MUST parse and lower to `LiteralValue.literal` (§5.1/§8.1, fixture 10 / golden 11); untyped `{...}` elements under an `any`-typed parameter and unreferenced broken/cyclic type aliases MUST be rejected at the declaration site (`invalid/untyped_list_element.proto`, `invalid/cyclic_alias.proto`).
- **Report semantics.** §6.4 map-key validation is explicit (key violations set `for_key`), and the four reserved `protowire.*` violation codes carry spec-pinned `fallback_message` strings and templates (§7).
- **New typed artifact families.** `protowire.docs.v1` (doc packs, `pxf docs build`) and `protowire.openapi.v1` (generator config, `pxf openapi`) are new packages allocating no extension numbers; existing consumers are unaffected. Doc packs are byte-stable, timestamp-free artifacts with the anchor-stability and revisor-gate contracts documented in `docs/DOC-PACK.md`; the OpenAPI document is a boundary rendering, not a conformance surface — ports owe nothing for it.

### v1.6 — docpack library + catalog v9 mirror

v1.6 is an additive minor with **no conformance-claim changes for ports**: no grammar, wire-format, envelope, or report change, and doc packs built from identical inputs are byte-identical to v1.5's (`PackProvenance.format_version` is unchanged). What changes:

- **`protowire.docs.v1.WidgetCatalog` gains additive fields** mirroring appviewer catalog schema v9 (`composition_props`, `transitions`/`TransitionSpec`, `WidgetSpec.icon`/`category`/`variadic_children`, `PropSpec.required`/`default_value`; GH [#186](https://github.com/trendvidia/protowire/issues/186)). The mirror is a compiler input model, not a port obligation; existing catalogs parse unchanged. The compiler's catalog version gate is a documented floor — only exports newer than the mirrored version warn.
- **`docpack` is a public Go package** (GH [#185](https://github.com/trendvidia/protowire/issues/185), [#187](https://github.com/trendvidia/protowire/issues/187)). Its API follows the reference-implementation rules, not the spec freeze: additive within a minor. Diagnostic output on stderr now carries `file:line:col:` prefixes and omits the usage block on runtime errors (GH [#188](https://github.com/trendvidia/protowire/issues/188)) — allowed movement under the CLI-surface rules below; exit codes are unchanged.

### v1.7 — locale catalog source format

v1.7 is an additive minor: no grammar, wire-format, envelope, or report change — every output is byte-identical to v1.6 for every schema and document. It closes the RFC-001 §7 i18n gap (GH [#194](https://github.com/trendvidia/protowire/issues/194)): `EngineConfig.catalog_libraries` finally points at a defined artifact. What changes for conformance claims:

- **New typed artifact family: `protowire.schema.catalog.v1`** ([`proto/schema/catalog/v1/catalog.proto`](proto/schema/catalog/v1/catalog.proto), no extension numbers). A catalog library file is a text-format `Catalog` message — one locale per file (BCP 47 `locale` is the `RegisterCatalog` key), `entries` keyed by violation code, `{param}` templates. A port claiming §7 i18n support MUST load this format with the pinned semantics: per-locale merge across files, duplicate code across files for one locale a load error, unmatched placeholders left verbatim, catalog miss falling back to `fallback_message`. Fixture: `testdata/schema-extensions/19_catalog_miss/catalog_de.textproto` (the pinned catalog that was previously prose in the schema header). Ports not claiming i18n owe nothing — catalogs are engine-init data, never compiled into images or descriptors.
- **`catalog_libraries` values are filesystem paths**, resolved relative to the declaring config file — not proto import paths. The `08_engine_config` conformance fixture's values changed from `.proto`-shaped to `.textproto` paths accordingly; config loaders treat the field as opaque strings, so no loader behavior changes.

### v1.8 — doc-platform contracts

v1.8 is an additive minor: no grammar, wire-format, envelope, or report change — every output is byte-identical to v1.7 for every schema and document, and doc packs from corpora using no new anchor kind are byte-identical too (`PackProvenance.format_version` unchanged). What changes:

- **Resolved-id spellings are format contract** (GH [#198](https://github.com/trendvidia/protowire/issues/198)). The canonical `ResolvedAnchor.resolved_id` grammar in `docs/DOC-PACK.md` § Resolved anchor IDs is normative and covered by the pack's byte-stability promise; a spelling change is a format break requiring a `format_version` bump. Consumers implementing the grammar independently (appviewer's ADR-0012 Store mirror, non-Go ports) validate against the `testdata/docs/grammar/` corpus and its `resolved_ids.golden`.
- **`TransitionAnchor` is an additive oneof arm** in `protowire.docs.v1.Anchor` (GH [#199](https://github.com/trendvidia/protowire/issues/199); composition props resolve as ordinary prop anchors on every widget). Consumers joining on `resolved_id` strings are unaffected; typed mirrors of the anchor model should add the arm to see transition anchors as typed targets. No extension numbers; existing topics and packs parse unchanged.
- **Review identity convention** (GH [#197](https://github.com/trendvidia/protowire/issues/197)). Producers of review metadata MUST write the git author email, lowercased, into `author`/`reviewers`/`revisor`/`translator` and compare that form. Compiler enforcement is a warning only — never escalated by `--release` — so no existing corpus starts failing.
- **Doc-coverage policy is opt-in compiler policy** (GH [#200](https://github.com/trendvidia/protowire/issues/200)). `--coverage`/`--coverage-approved` are new-flag additions under the CLI-surface rules below; with the flags absent, `pxf docs build` behavior is unchanged. Ports owe nothing — coverage is a build-time policy of the reference compiler, not a conformance surface.

### v1.9 — docpack element-kind lookup

v1.9 is an additive minor with **no conformance-claim changes for ports**: no grammar, wire-format, envelope, report, or doc-pack change — every output is byte-identical to v1.8 for every schema, document, and topic corpus. What changes:

- **`docpack` gains `ElementKind` and `Image.Kind`** (GH [#206](https://github.com/trendvidia/protowire/issues/206)). Additive Go API under the v1.6 docpack library rules (reference-implementation surface, additive within a minor). The kind spellings (`message`, `field`, `oneof`, `enum`, `enum value`, `service`, `method`, `type alias`) are exported constants consumers may rely on as stable — they are display strings a hover renders verbatim. The lookup is a query surface of the reference compiler's image index, not a conformance surface; ports owe nothing.

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
