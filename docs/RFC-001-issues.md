# RFC-001 — Tracking Issues

Paste-ready issue bodies for the work tracked under [RFC-001 — Protowire Schema Extensions](RFC-001-schema-extensions.md). Each entry below is a single issue.

**Conventions used throughout:**

- **Repo** indicates where the issue should be filed.
- **Milestone** matches the phasing table in RFC-001 §12.
- **Labels** are suggested; adjust to your tracker's taxonomy.
- **Depends on** lists hard prerequisites (cannot start work until those are complete).
- Issues with no `Depends on` line can start at M0.

Issue numbers (`#001` …) are local to this scaffold — replace with tracker-assigned numbers when filed.

---

## #001 — [META] Protowire Schema Extensions (v1.2.0) — Umbrella tracking

**Repo:** `protowire`
**Milestone:** M0 → M9+
**Labels:** `meta`, `schema-extensions`, `tracking`

This is the umbrella tracking issue for [RFC-001 — Protowire Schema Extensions](docs/RFC-001-schema-extensions.md), targeting protowire v1.2.0.

### Spec (M0)
- [ ] #002 — Ratify RFC-001
- [ ] #003 — Draft IETF `draft-trendvidia-protowire-01`
- [ ] #004 — Add `protowire/proto/schema/v1/annotations.proto`
- [ ] #005 — Add `protowire/proto/schema/v1/descriptor.proto`
- [ ] #006 — Update `STABILITY.md` for v1.2 additive surface
- [ ] #007 — Add v1.2.0 `CHANGELOG.md` entry

### Open questions (resolved or deferred during M0)
- [ ] #010 — Container-shaped type aliases (deferred to v1.3+)
- [ ] #011 — Engine-config file format
- [ ] #012 — Well-known types semantics (Timestamp, Duration, Any)
- [ ] #013 — Recursive message validation depth limits
- [ ] #014 — Streaming RPC validation contract
- [ ] #015 — `Literal` shape in `AnnotationArg`
- [ ] #016 — Validation report wire shape
- [ ] #017 — protovalidate migration story
- [ ] #018 — Performance budget + benchmark suite
- [ ] #019 — Conformance test fixtures
- [ ] #020 — Upstream `buf/protocompile` compatibility

### Implementation (M1–M8)
- [ ] #030 — `protocompile`: extended grammar parser
- [ ] #031 — `protocompile`: IR for type/function/annotation
- [ ] #032 — `protocompile`: linker symbol resolution
- [ ] #033 — `protocompile`: option-interpretation hook for `@annot` → carrier
- [ ] #034 — `protocompile`: descriptor lowering pass
- [ ] #035 — `protocompile`: source-map emission
- [ ] #040 — `protocheck`: engine SPI (Go interface)
- [ ] #041 — `protocheck`: function registration + runtime-init verification
- [ ] #042 — `protocheck`: validation execution (collect-all / fail-fast)
- [ ] #043 — `protocheck`: catalog support + i18n
- [ ] #050 — `protolsp`: extended grammar parsing
- [ ] #051 — `protolsp`: source-map consumption + go-to-definition
- [ ] #052 — `protolsp`: annotation-aware diagnostics
- [ ] #060 — `protobuf-go`: function-stub codegen plugin (Go)
- [ ] #061 — `protobuf-go`: annotation-aware codegen
- [ ] #070 — `protowire-go`: M5 runtime wiring through `protocheck`
- [ ] #080 — OpenAPI generator (M8) — separate tool consuming descriptors

### Per-port adoption (M9+)
- [ ] `protowire-java`
- [ ] `protowire-typescript`
- [ ] `protowire-python`
- [ ] `protowire-cpp`
- [ ] `protowire-rust`
- [ ] `protowire-csharp`
- [ ] `protowire-kotlin`
- [ ] `protowire-swift`
- [ ] `protowire-dart`

---

## #002 — Ratify RFC-001

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `rfc`, `schema-extensions`

Reach approval on [RFC-001](docs/RFC-001-schema-extensions.md). The RFC's open-questions table (§13) lists items intentionally deferred — they don't block ratification but each becomes its own tracked issue (#010–#020).

**Acceptance criteria:**
- [ ] At least one approving review from spec governance.
- [ ] All open questions either resolved in the RFC or filed as separate issues with the "v1.2 deferred" label.
- [ ] No locked decision blocks an existing port's roadmap.

---

## #003 — Draft IETF `draft-trendvidia-protowire-01`

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `ietf`, `schema-extensions`
**Depends on:** #002

Translate RFC-001's substantive content into formal IETF-draft prose under `docs/draft-trendvidia-protowire-01.{md,xml,txt}`. Follow the conventions established by `-00` (xml2rfc markdown source, generated XML and text).

Scope:
- New keyword reservations (§5.1)
- Grammar additions for `type`, `function`, `annotation`, `@annotation(...)` (§5.1)
- Semantic specification (§6)
- Error model (§7)
- Lowering / extension number reservation (§8)
- Cross-language portability rules (§10)
- Compatibility / version transition (§11)

**Acceptance criteria:**
- [ ] xml2rfc passes
- [ ] Generated `.txt` differs from `-00` only in schema-extension additions
- [ ] PR review by IETF-draft maintainer

---

## #004 — Add `protowire/proto/schema/v1/annotations.proto`

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `proto`, `schema-extensions`
**Depends on:** #002

Add the framework annotation library file as specified in RFC-001 §5.2:

```proto
syntax = "proto3";
package protowire.schema.v1;

annotation validate(rule: expression, code: string = "", message: string = "");
annotation required;
annotation default(value: any);
annotation description(text: string);
annotation example(value: any);
annotation error_code(code: string);
annotation deprecated(reason: string = "");
annotation http(method: string, path: string);
```

Note: this file uses v1.2 grammar (the `annotation` keyword), so it can only be parsed by v1.2+ ports. It's the canonical declaration file that every schema using `@validate`, `@required`, etc. imports.

**Acceptance criteria:**
- [ ] File added at `protowire/proto/schema/v1/annotations.proto`
- [ ] Listed in `README.md` repository-layout section
- [ ] Round-trips through `protocompile` (once parser support lands, see #030)

---

## #005 — Add `protowire/proto/schema/v1/descriptor.proto`

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `proto`, `schema-extensions`
**Depends on:** #002

Add the descriptor lowering schemas as specified in RFC-001 §8 — `AnnotationList`, `Annotation`, `AnnotationArg`, `Expression`, `FileFunctions`, `FileAnnotationDecls`, `FileTypeDecls`, `SourceMap`, plus the `extend google.protobuf.*Options` blocks at numbers `50400`–`50404`.

This file is parseable by stock `protoc` — it does not use v1.2 grammar. It is the lowering target, not a user-facing surface.

**Acceptance criteria:**
- [ ] File added at `protowire/proto/schema/v1/descriptor.proto`
- [ ] All 5 extension numbers (`50400`–`50404`) reserved per spec
- [ ] Parses with stock `protoc`
- [ ] Listed in `README.md` repository-layout section

---

## #006 — Update `STABILITY.md` for v1.2 additive surface

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `docs`, `schema-extensions`
**Depends on:** #002

Update `STABILITY.md` to document the v1.2 additive surface:
- New reserved keywords (`type`, `function`, `annotation`, `expression`, `this`, `@` sigil)
- New extension numbers (`50400`–`50499` reserved for schema-extension carriers; `50400`–`50404` allocated in v1.2.0)
- v1.2 schemas not back-compatible with v1.1 parsers; v1.1 schemas remain valid in v1.2 parsers

**Acceptance criteria:**
- [ ] Stability section added for "Schema language additions" mirroring the existing "Wire format" structure
- [ ] Cross-link to RFC-001 and IETF draft `-01`

---

## #007 — Add v1.2.0 `CHANGELOG.md` entry

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `docs`, `release`
**Depends on:** #002–#006

Add `## [Unreleased]` → `## [1.2.0]` entry to `CHANGELOG.md` summarizing the schema-extension additions, listing affected files and reserved numbers. Follow the existing v1.1.0 entry's structure.

---

## #010 — Container-shaped type aliases (v1.3+)

**Repo:** `protowire`
**Milestone:** Deferred — post-v1.2
**Labels:** `spec`, `deferred`, `schema-extensions`

Allow `type Tags = repeated string @validate(this.size() < 100);` and `type Labels = map<string, string> @validate(...)`. Adds:
- `this` binding semantics for collections (element vs container)
- Composition rules across element + collection refinements
- Lowering: where do per-element annotations live in the descriptor for typed-element collections?

Target: protowire v1.3 minor.

---

## #011 — Engine-config file format

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `schema-extensions`

Define the project-level engine-selection config. Likely a `.proto` (consistent with the "no JSON/YAML" principle):

```proto
package protowire.schema.config.v1;
option (engine) = CEL;
option (function_libraries) = "myco/commons/validator.proto";
option (catalog_libraries) = "myco/i18n/en.proto";
```

Open: file naming convention (e.g., `protowire.config.proto`), discovery rules, override precedence (CLI > env > config).

**Acceptance criteria:**
- [ ] Sub-RFC or RFC-001 amendment specifying the schema
- [ ] Reference Go impl in `protocompile`

---

## #012 — Well-known types semantics

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `schema-extensions`

Pin down what refinement on `google.protobuf.Timestamp`, `Duration`, `Any` means:
- `type Future = Timestamp @validate(this > now());` — does `this` bind to a structured `Timestamp` message or to the engine-native time type?
- `type SmallDuration = Duration @validate(this < 1m);` — same question for `Duration`
- `type ErrorPayload = Any @validate(this.type_url == "...");` — refining `Any` by type URL

Likely answer: WKTs unwrap to engine-native types (parallel to wrapper unwrap) so rules look natural.

---

## #013 — Recursive message validation depth limits

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `engine`, `schema-extensions`

Define a maximum recursion depth for nested-message validation and engine behavior at the limit (error vs. truncate). Likely a `protocheck.Engine` configuration knob with a documented default (e.g., 64).

---

## #014 — Streaming RPC validation contract

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `schema-extensions`

Per-message vs. per-stream validation; behavior when a mid-stream message fails (close stream? skip? continue?); how server/client semantics differ. Likely: validate each message independently as it crosses the wire; failures surface as gRPC stream errors with the structured `Violation` payload.

---

## #015 — `Literal` shape in `AnnotationArg`

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `proto`, `schema-extensions`

Flesh out the `Literal` message used in `AnnotationArg.literal` for enum names, message literals, and list literals. Should accommodate `@example(value: any)` with a message-literal value, `@validate(this in [...])` with a list literal, etc.

**Acceptance criteria:**
- [ ] `Literal` definition added to `protowire/proto/schema/v1/descriptor.proto` (#005)
- [ ] Test fixtures in `testdata/schema-extensions/` covering each variant

---

## #016 — Validation report wire shape

**Repo:** `protowire`
**Milestone:** M0
**Labels:** `spec`, `proto`, `schema-extensions`

Define the wire shape of a complete validation result (`Report`) containing `repeated EnrichedViolation` and any summary metadata (timing, engine info, etc.). Standardize so all 10 ports emit equivalent reports.

---

## #017 — protovalidate migration story

**Repo:** `protowire`
**Milestone:** M0 (design) / M5+ (tooling)
**Labels:** `spec`, `tooling`, `schema-extensions`

Document how a project using `[(buf.validate.field).cel = "..."]` migrates to `@validate(...)`. Options:
- Manual rewrite (acceptable for small projects)
- `pxf migrate-validate` subcommand that transforms in-place
- `--compat` flag in protocompile accepting both forms during transition

---

## #018 — Performance budget + benchmark suite

**Repo:** `protowire`
**Milestone:** M5
**Labels:** `perf`, `schema-extensions`

Establish per-engine validation throughput targets (validations/sec for representative schemas) and a benchmark harness in `protowire/testdata/schema-extensions/bench/` parallel to the existing PXF/SBE benches. Wire into `scripts/cross_*_bench.sh`.

---

## #019 — Conformance test fixtures

**Repo:** `protowire`
**Milestone:** M0 → M5
**Labels:** `tests`, `schema-extensions`

Build a corpus in `testdata/schema-extensions/` covering:
- Every `type`/`function`/`annotation` declaration shape
- All composition patterns (chained refinement, message refinement, wrapper refinement, enum refinement)
- All annotation placements (leading vs. trailing)
- All error states (missing impl, invalid signature, unsatisfiable rule, locale catalog miss)
- Round-trip through `protocompile` → stock `protoc` → re-marshal

Cross-port adoption (M9+) gates on this suite passing in each port.

---

## #020 — Upstream `buf/protocompile` compatibility

**Repo:** `protocompile`
**Milestone:** M0 (decision) / M1+ (implementation)
**Labels:** `spec`, `protocompile`, `schema-extensions`

This fork diverges from upstream `buf/protocompile` once v1.2 grammar lands. Decide:
- Stay forked (own the divergence indefinitely)
- Upstream the changes (likely rejected — adds vendor-specific surface)
- Maintain a clean fork point + cherry-pick upstream non-conflicting commits

Outcome shapes how `protocompile`'s long-term maintenance is organized.

---

## #030 — `protocompile`: extended grammar parser

**Repo:** `protocompile`
**Milestone:** M1
**Labels:** `protocompile`, `parser`, `schema-extensions`
**Depends on:** #002, #006

Implement the grammar additions from RFC-001 §5.1 in the existing `parser/` package. New AST nodes for `TypeDecl`, `FunctionDecl`, `AnnotationDecl`, `Annotation`, `AnnotationArg`, `EngineExpression`.

Engine expressions are parsed with delimiter balancing only (no inner semantic interpretation) — the body is captured verbatim for engine-side parsing.

### Implementation notes — entry points

| Layer | File | Where to hook |
|---|---|---|
| Keyword table | `parser/lexer.go` | `keywords` map (around line 122). Add `type`, `function`, `annotation`, `expression`, `this`. The `@` sigil is **already a token** in `parser/proto.y` (around line 143) — no lexer change needed for it. |
| Token declarations | `parser/proto.y` | `%token` block (around line 136). Add the five new keyword tokens to mirror the lexer additions. |
| Top-level grammar | `parser/proto.y` | `fileElement` production (around line 189). Add alternatives dispatching to `typeDecl`, `functionDecl`, `annotationDecl`. Each new production follows the shape of the existing `messageDecl` rule (around line 1018). |
| Field-trailing annotations | `parser/proto.y` | Extend the `field` production with an optional trailing `annotationList`. Same for `enumValue`. |
| Block-leading annotations | `parser/proto.y` | Extend `messageDecl`, `enumDecl`, `serviceDecl`, `rpcDecl`, `oneofDecl` with an optional leading `annotationList`. |
| AST nodes | new files `ast/type.go`, `ast/function.go`, `ast/annotation.go` | Define `TypeDeclNode`, `FunctionDeclNode`, `AnnotationDeclNode`, `AnnotationNode`. Each embeds `compositeNode` and implements the `FileElement` marker method per `ast/file.go` (var block around line 156). Constructors follow `NewMessageNode` (`ast/message.go`). |
| Field-attached annotations | `ast/field.go` (and `ast/enum.go` for enum values) | Add an optional `Annotations []*AnnotationNode` to `FieldNode` and `EnumValueNode`; include in the `children` slice for position tracking. |

### Acceptance criteria

- [ ] Five new keywords reserved in the lexer and grammar
- [ ] `TypeDeclNode`, `FunctionDeclNode`, `AnnotationDeclNode`, `AnnotationNode` defined with consistent visitor support
- [ ] Trailing `annotationList` accepted on `type`, `function`, `field`, `enum-value`
- [ ] Leading `annotationList` accepted on `message`, `enum`, `service`, `rpc`, `oneof`
- [ ] All existing v1.1 fixtures continue to parse identically (regression-tested against the existing testdata corpus)
- [ ] New fixtures in `internal/testdata/` exercise each new construct (one minimal `.proto` per construct, plus combined examples)
- [ ] Engine-expression bodies captured as opaque source with positions; function-call identifier extraction (FQN + arity) produces source-spans usable by #032 (linker verification)
- [ ] `@` sigil parsing never collides with future use elsewhere in proto grammar

---

## #031 — `protocompile`: IR for type/function/annotation

**Repo:** `protocompile`
**Milestone:** M1
**Labels:** `protocompile`, `ir`, `schema-extensions`
**Depends on:** #030

Extend the `experimental/ir/` package with IR nodes for the new declarations. Each carries its source location, FQN, and structural data (params, base type, expression bodies).

### Implementation notes — entry points

The `experimental/ir/` package is the active new IR (arena-allocated, generated SymbolKind enum).

| Step | File | Pattern |
|---|---|---|
| Symbol kinds | `experimental/ir/symbol_kind.yaml` | Add `TypeKind`, `FunctionKind`, `AnnotationKind` entries; regenerate via the existing `go generate` target. |
| File-level storage | `experimental/ir/ir_file.go` | Extend `File` struct with `types`, `functions`, `annotations` slices (following the existing `messages`/`services` pattern around lines 50–61). |
| Iterators | `experimental/ir/ir_file.go` | Add `File.Types()`, `File.Functions()`, `File.Annotations()` methods. |
| Arena types | `experimental/ir/ir_file.go` arena block | Add `rawType`, `rawFunction`, `rawAnnotation` types and arena allocators. |
| Symbol records | `experimental/ir/ir_symbol.go` | Extend symbol-record shapes to carry the new kinds where applicable. |

### Acceptance criteria

- [ ] `SymbolKind` enum extended with the three new kinds
- [ ] `File` exposes `Types()`, `Functions()`, `Annotations()` iterators
- [ ] AST → IR conversion lossless for new declarations
- [ ] Existing IR consumers (e.g. linker, options) keep building unmodified
- [ ] Source positions preserved end-to-end

---

## #032 — `protocompile`: linker symbol resolution

**Repo:** `protocompile`
**Milestone:** M1
**Labels:** `protocompile`, `linker`, `schema-extensions`
**Depends on:** #031

Extend `linker/` to resolve type aliases, function calls within expressions, and annotation references across imports. Reuse existing FQN resolution machinery.

Function-call sites in engine expressions must verify:
- The referenced FQN resolves to a `function` declaration in scope
- Arity matches
- Parameter types are compatible at call site (primitive/enum/message position checked; deeper semantics deferred to the engine)

### Implementation notes — entry points

| Step | File | Pattern |
|---|---|---|
| Symbol registry | `linker/symbols.go` (around lines 39–80) | Extend `packageSymbols.symbols` to carry the new declaration kinds. Add `Symbols.AddType()`, `AddFunction()`, `AddAnnotation()` following the shape of `AddExtension()` (around line 546). |
| Import-time registration | `linker/symbols.go` `Symbols.Import()` (around line 81) | Extend `importResult()` to scan File's new IR slices and register their FQNs. |
| Resolver interface | `linker/files.go` (around line 180–197) — `Resolver` and `ResolverFromFile` | New kinds are returned via the existing `FindDescriptorByName` path; no new resolver surface needed — descriptor lookups just include the new declarations. |
| Function-call verification | new file under `linker/` (e.g. `linker/expressions.go`) | Walk the `FunctionRef` entries emitted by the parser (#030) for each expression body; verify against `Symbols.Lookup()`. Report mismatches via `reporter.Handler` with source spans. |
| Annotation-arg type checking | new file or `linker/resolve.go` | For each `AnnotationNode`, look up its `AnnotationDecl`, match args (positional then named) against declared params, and report errors. |

### Acceptance criteria

- [ ] Cross-file `type` chains resolve (`import "myco/commons/types.proto"; type X = CompanyEmail @...`)
- [ ] Function-call FQNs in expressions resolve; arity and signature mismatches reported with source spans
- [ ] Annotation-arg type checking against declared parameter types
- [ ] Cyclic `type` chains detected and reported
- [ ] Source locations preserved on every diagnostic

---

## #033 — `protocompile`: option-interpretation hook for `@annot` → carrier

**Repo:** `protocompile`
**Milestone:** M2
**Labels:** `protocompile`, `options`, `schema-extensions`
**Depends on:** #031, #032, #005

Hook into the existing option-interpretation pipeline (`options/options.go`) so that every `@annot(...)` use site lowers into uninterpreted options on the appropriate target's `Options` message, in the carrier extension at `50400`.

The lowering produces standard `UninterpretedOption` entries that the existing interpreter then resolves against the carrier schemas from #005.

### Implementation notes — entry points

| Step | File | Pattern |
|---|---|---|
| Pre-interpretation lowering pass | new function in `options/` (e.g. `options/lower_annotations.go`) | Walk each declaration's IR, gather attached `AnnotationNode`s, and for each one synthesize an `UninterpretedOption` whose `name_part` resolves to the carrier extension on the target's `Options` message and whose `aggregate_value` carries the annotation entry. |
| Existing pipeline entry | `options/options.go` `InterpretOptions()` (around line 104) | Run the new lowering pass **before** the main interpretation loop so the carrier `UninterpretedOption`s feed the existing resolver unchanged. No structural changes to `InterpretOptions` itself. |
| Carrier schema availability | `options/` resolver setup | Ensure `validator/v1/descriptor.proto` (issue #005, `protowire/proto/schema/v1/descriptor.proto`) is registered as an available extension source, mirroring how `descriptor.proto` is made available today via `WithOverrideDescriptorProto`. |
| Bracket coexistence | nothing to do | Existing bracket options continue through the existing `UninterpretedOption` path; annotations add new entries alongside. The two coexist without contention. |

### Acceptance criteria

- [ ] Annotations on every supported declaration kind lower correctly into the carrier extension
- [ ] Stacked annotations preserve source order in `AnnotationList.entries`
- [ ] Brackets and annotations coexist on the same field with no interference
- [ ] Round-trip test: parse → lower → serialize FileDescriptorSet → parse with stock `protobuf-go` → confirm carrier extensions decode to typed values
- [ ] `@required` and `@default(value)` lower correctly to BOTH the carrier annotation entry AND the legacy `(pxf.required)` / `(pxf.default)` bracket options for backward compat

---

## #034 — `protocompile`: descriptor lowering pass

**Repo:** `protocompile`
**Milestone:** M2
**Labels:** `protocompile`, `lowering`, `schema-extensions`
**Depends on:** #033

Emit `FileFunctions` (`50401`), `FileAnnotationDecls` (`50402`), and `FileTypeDecls` (`50403`) extensions on `FileOptions` from the IR. Each `type` declaration is preserved verbatim in the descriptor (not just macro-expanded) so consumers can resolve named types.

Type-alias use sites expand the refinement chain into the field's annotation list at lower time.

### Implementation notes — entry points

| Step | File | Pattern |
|---|---|---|
| File-scope decl lowering | extend `options/lower_annotations.go` from #033 (or new `options/lower_file_decls.go`) | Walk the IR `File.Functions()`, `File.AnnotationDecls()`, `File.TypeDecls()` iterators; emit one `UninterpretedOption` per file-scope decl, targeting the corresponding carrier extension (`50401`/`50402`/`50403`) on `FileOptions`. Same uninterpreted-option-feeds-existing-pipeline pattern as #033. |
| Type-alias expansion at use sites | extend the field-annotation lowering from #033 | For each field whose declared type resolves through one or more `type` aliases, walk the type chain base-to-derived and prepend each alias's `validate`/etc. annotations to the field's annotation list before lowering. |
| Hook point | `options/options.go:104` `InterpretOptions` | Same pre-loop slot as #033 — these passes run in a defined order before main interpretation. |
| Backward-compat shim | extend lowering | For each `@required` and `@default(value)` annotation, also emit the legacy `[(pxf.required) = true]` / `[(pxf.default) = "..."]` brackets to preserve v1.1 reader compatibility. |

### Acceptance criteria

- [ ] `FileFunctions` populated from every `function` decl in the file
- [ ] `FileAnnotationDecls` populated from every `annotation` decl
- [ ] `FileTypeDecls` populated from every `type` decl, with annotations carried through
- [ ] Type-alias chains expanded at field use sites in base-to-derived order
- [ ] Descriptor round-trips through stock `protoc --decode_raw` (well-formed proto)
- [ ] Descriptor round-trips through stock `protobuf-go` Unmarshal/Marshal byte-identically
- [ ] When `validator/v1/descriptor.proto` is imported, `protobuf-go` decodes carrier extensions as typed values
- [ ] `@required`/`@default` lower to both annotation carrier AND `(pxf.required)`/`(pxf.default)` brackets

---

## #035 — `protocompile`: source-map emission

**Repo:** `protocompile`
**Milestone:** M3
**Labels:** `protocompile`, `source-map`, `schema-extensions`
**Depends on:** #034

Populate the embedded `SourceMap` (`50404` on `FileOptions`) during the lowering pass. Each annotation, type-refinement expansion, and function-call gets a `SourceEntry` with `descriptor_path`, `source_location`, and (for refinements) `type_chain`.

### Implementation notes — entry points

| Step | File | Pattern |
|---|---|---|
| Source-map accumulator | extend the lowering pass from #033/#034 | Thread a `*SourceMap` accumulator through the lowering walk. Every time the pass emits a carrier `UninterpretedOption` (annotation, file-scope decl, type expansion, function-call site), it also appends a `SourceEntry` capturing `kind`, `descriptor_path`, `source_location`, and (for `TYPE_REFINEMENT`) `type_chain`. |
| Final emission | end of the lowering pass | After all annotations are lowered, emit one final `UninterpretedOption` carrying the accumulated `SourceMap` as the carrier-extension value on `FileOptions.source_map = 50404`. |
| Descriptor-path scheme | new file `options/descriptor_path.go` | Define the canonical string form (`"User.email[validate#1]"`, `"User#message_validate#0"`, etc.) used in `SourceEntry.descriptor_path`. Stable across versions because `protolsp`/`pxfed` parse it. |
| `protolsp` consumer parity | coordinate with #051 | The descriptor-path string format is the implicit contract with `protolsp`. Both repos need to agree; consider exporting a parser helper. |

### Acceptance criteria

- [ ] Source-map entries cover every annotation lowering
- [ ] Type-refinement chains include every base→derived link with each link's declaration site
- [ ] Descriptor-path strings unambiguously identify each carrier entry
- [ ] Round-trip test: descriptor → re-marshal → source-map preserved byte-identically
- [ ] `protolsp` (issue #051) can resolve `descriptor_path` back to source location

---

## #040 — `protocheck`: engine SPI (Go interface)

**Repo:** `protocheck`
**Milestone:** M4
**Labels:** `protocheck`, `spi`, `schema-extensions`
**Depends on:** #005, #016

Define the `Engine`, `Function`, `Catalog`, and `Report` interfaces per RFC-001 §9. Implementation neutral — usable by CEL-backed, Starlark-backed, or Go-native engines.

### Implementation notes — entry points

Current state: protocheck is a library (no CLI). Single existing entry point `protocheck.New(opts ...Option) → *Validator` + `Validator.Validate(msg)`. Backing evaluator is GoVM (no CEL today); engine choice is hard-coded. Collect-all violations is the existing default. Field paths, repeated/map per-element, oneof active-variant detection: already in place.

| Step | File | Pattern |
|---|---|---|
| Public SPI definitions | new `engine.go` at repo root | Define `Engine`, `Function`, `Catalog`, `Report` as public interfaces. Pattern: thin facades wrapping the existing `Validator`. Keep GoVM evaluation internal; the SPI is for downstream consumers + future alternative engines. |
| `Engine` shape | `engine.go` | `Register(fqn string, impl Function) error`; `RegisterCatalog(locale string, cat Catalog) error`; `Validate(msg proto.Message) (*Report, error)`. |
| `Function` shape | `engine.go` | Wraps the existing `eval.FuncDef` signature with the function's FQN and arity. |
| `Report` shape | `engine.go` | Wraps `[]Violation` with metadata (file path, total count, fail-fast flag). The violation enrichment (source location, type chain, etc.) lives in `Violation` itself — see #042. |
| Wrapper struct | `engine.go` | Internal `goVMEngine` struct implementing `Engine`; constructed from the current `Validator`. |
| Constructor parity | extend existing `protocheck.New()` | Wrap the returned `*Validator` as an `Engine` for SPI consumers; keep the direct `*Validator` API for internal/transitional use. |

### Acceptance criteria

- [ ] `Engine`, `Function`, `Catalog`, `Report` interfaces defined and documented
- [ ] `goVMEngine` implements `Engine` as a thin wrapper around `*Validator`
- [ ] Existing `protocheck.New(...)` API continues to work unchanged
- [ ] `Engine` API can be obtained from a `*Validator` (or vice versa) without breakage
- [ ] Interface shapes match the RFC-001 §9 reference Go signatures

---

## #041 — `protocheck`: function registration + runtime-init verification

**Repo:** `protocheck`
**Milestone:** M4
**Labels:** `protocheck`, `runtime`, `schema-extensions`
**Depends on:** #040

`Engine.Register(fqn, impl)` and the init-time descriptor walk that verifies every referenced FQN has a registered impl (lenient default; `strict_validation=true` opt-in).

### Implementation notes — entry points

Current state: `options.go` already has `WithFunction(name, fn)` and `wrapFunction()` storing impls in `Validator.funcs`. That's the pattern to extend.

| Step | File | Pattern |
|---|---|---|
| Registration entry point | extend `options.go` | Add `WithFunction(fqn, impl)` alias if needed and route through to the existing `Validator.funcs` map keyed by FQN. `Engine.Register(fqn, impl)` from #040 delegates to this. |
| Descriptor-walk verification | new file `validation_init.go` | Function `verifyRegisteredFunctions(desc protoreflect.FileDescriptor, registry map[string]Function, strict bool) error`. Walks the descriptor tree (uses `protoreflect`), extracts FQN references from `Expression.calls` (carrier extension `50400`) and from `FileFunctions` entries (carrier extension `50401`), checks each FQN exists in the registry. In strict mode, returns an error on any miss; in lenient mode, logs and registers a placeholder that returns `Violation{Code: "unimplemented", ...}` on call. |
| Init hook | extend `protocheck.New()` | After option processing, run `verifyRegisteredFunctions` against any descriptors passed at construction time. Lenient is the default; the option `WithStrictValidation(true)` flips to strict. |
| Lenient placeholder | extend `eval.FuncDef` invocation path | If a registry lookup misses in lenient mode, synthesize a placeholder `FuncDef` that returns the unimplemented Violation rather than failing the validator startup. |

### Acceptance criteria

- [ ] `Engine.Register(fqn, impl)` populates the existing `Validator.funcs` map
- [ ] `verifyRegisteredFunctions()` walks descriptors and reports missing impls with FQN + source location (via #035 source map)
- [ ] Strict mode: missing impl fails `New()` / `Engine.Init()`
- [ ] Lenient mode: missing impl substituted with placeholder; first call emits `Violation{Code: "unimplemented"}`
- [ ] Both modes covered by tests against the conformance fixtures (`testdata/schema-extensions/`)

---

## #042 — `protocheck`: validation execution

**Repo:** `protocheck`
**Milestone:** M4
**Labels:** `protocheck`, `runtime`, `schema-extensions`
**Depends on:** #041, #034, #035

Validation walk over a message instance, evaluating constraints per RFC-001 §6.4 (collect-all default, fail-fast opt-in). Produces a `Report` of `EnrichedViolation`s with field paths, type chains, and source locations.

### Implementation notes — entry points

Current state in `protocheck.go:46–143` (`validate()`, `validateField()`) already covers most of the §6.4 surface:

| RFC-001 §6.4 requirement | Current state |
|---|---|
| Collect-all default | ✓ already in place — violations accumulated in a slice |
| Field path tracking | ✓ via `path` param + `joinPath()` |
| Repeated/map per-element | ✓ at `protocheck.go:168–179` (lists) and `:197–211` (maps) |
| Oneof active-variant only | ✓ at `protocheck.go:90–94` |
| Fail-fast opt-in | ✗ to add |
| Source location on Violation | ✗ to add |
| Type-chain provenance on Violation | ✗ to add |

| Step | File | Pattern |
|---|---|---|
| Fail-fast config | `protocheck.go` `validatorConfig` (and corresponding `Option`) | Add `failFast bool` flag (default false). In `validate()` / `validateField()`, after appending a violation, check the flag and short-circuit the walk. |
| Enriched Violation fields | `violations.go` | Add `SourceFile string`, `SourceLine int32`, `SourceColumn int32`, `TypeChain []string`, `RuleKind RuleKind` to the existing `Violation` struct. Populate from the carrier source map (`50404`) during evaluation. |
| Source-map lookup | new helper in `validation_init.go` (from #041) | Build a `descriptorPath → SourceEntry` index from the embedded `SourceMap` at init time; lookup during violation construction. |
| Type-chain population | extend `validate()` field walk | When evaluating a rule that originated from a type alias (kind `TYPE_REFINEMENT` in the source map), populate `Violation.TypeChain` from `SourceEntry.type_chain`. |
| Report wrapper | `Report` struct from #040 | Aggregate violations into the `Report` shape; add `Format(locale string)` to render via the catalog from #043. |
| New schema-extension rules | extend the field-validation loop | For each annotation in the field's `AnnotationList` (carrier extension `50400`), dispatch to the appropriate evaluator: built-in `@required`/`@default` handlers run inline; `@validate(expression)` calls into GoVM as today. |

### Acceptance criteria

- [ ] `failFast` option implemented; default remains collect-all
- [ ] `Violation` carries source file/line/column and type chain
- [ ] `Violation.RuleKind` correctly distinguishes `VALIDATE`/`REQUIRED`/`DEFAULT`/`TYPE_REFINEMENT`
- [ ] `Report.Format(locale)` returns localized output when a catalog is registered
- [ ] All conformance fixtures (`testdata/schema-extensions/01-06`) validate correctly under both modes
- [ ] Existing protocheck tests continue to pass unchanged

---

## #043 — `protocheck`: catalog support + i18n

**Repo:** `protocheck`
**Milestone:** M6
**Labels:** `protocheck`, `i18n`, `schema-extensions`
**Depends on:** #042

`Engine.RegisterCatalog(locale, catalog)` and a renderer that formats `EnrichedViolation`s through the registered catalog by `code` + `params`, falling back to `fallback_message` on miss.

### Implementation notes — entry points

Current state: no locale awareness. `Violation.Message` is a single string; `Violation.String()` (`violations.go:18–23`) renders it directly.

| Step | File | Pattern |
|---|---|---|
| Catalog interface | new `catalog.go` | `type Catalog interface { Lookup(code, locale string, params map[string]any) (string, bool) }`. Concrete impls (file-loaded, in-memory map, etc.) live in user code. |
| Default in-memory catalog | `catalog.go` | Provide `NewMapCatalog(entries map[string]string)` for simple cases — pure-Go entries keyed by code, template strings with `{param}` substitution. |
| Renderer | new `renderer.go` | `func renderViolation(v *Violation, catalogs map[string]Catalog, locale string) string`. Lookup chain: requested locale → fall back to `Violation.FallbackMessage`. Template substitution via the existing `params` map. |
| Registration | extend `options.go` | `WithCatalog(locale string, cat Catalog) Option`. Stores into `Validator.catalogs map[string]Catalog`. Mirrors the existing `WithFunction` pattern. |
| Engine wiring | `Engine.RegisterCatalog()` in `engine.go` (from #040) | Delegates to the underlying `WithCatalog`. |
| Violation rendering | extend `violations.go:18–23` `Violation.String()` and `Report.Format()` | Renderer-aware. `String()` keeps the existing default-locale behavior (or a configured locale via context); `Report.Format(locale)` is the explicit-locale entry. |

### Acceptance criteria

- [ ] `Catalog` interface defined; `NewMapCatalog` reference impl included
- [ ] `Engine.RegisterCatalog(locale, cat)` stores catalog correctly
- [ ] Render path falls back to `fallback_message` on catalog miss (locale missing, code missing)
- [ ] Template substitution interpolates `params` correctly
- [ ] At least two locale catalogs (e.g., `en`, `fr`) exercised in tests against the conformance fixtures

---

## #050 — `protolsp`: extended grammar parsing

**Repo:** `protolsp`
**Milestone:** M3
**Labels:** `protolsp`, `parser`, `schema-extensions`
**Depends on:** #030

Wire `protocompile`'s extended parser into `protolsp` so the editor can syntax-highlight, complete, and diagnose schema-extension constructs.

### Implementation notes — entry points

Current state: protolsp is a Go LSP server (LSP 3.12+ over stdio, `go.lsp.dev/jsonrpc2` + `go.lsp.dev/protocol`). Entry binary at `cmd/protolsp/main.go`; dispatcher is `Server.Handle` in `internal/server/`. **protolsp already embeds `protocompile` as a Go module dependency** — picking up the v1.2 grammar is largely a `go.mod` bump on the existing integration, not new wiring.

| Step | File | Pattern |
|---|---|---|
| Pin to v1.2 protocompile | `go.mod` | Update the `github.com/bufbuild/protocompile` (or fork-replacement directive) version to the v1.2 release. |
| Parse entry point | `internal/server/proto_analyze.go` `analyzeProto()` (line 72) | Already calls `protocompile.Compile()` on `textDocument/didOpen` and `textDocument/didChange`. v1.2 syntax flows through unchanged — the new keywords parse natively in the upgraded library. |
| Diagnostic adapter | `internal/server/proto_analyze.go` `appendDiag` closure (line 86) | Existing protocompile-error → LSP-Diagnostic bridge already covers parse errors. Extend the message-formatting branch to render new-error categories (unknown `@annotation`, wrong arg type, etc.) with helpful text. |
| Semantic tokens | `internal/server/semantic_tokens.go` legend (lines 27–59) | Protolsp does NOT currently highlight `.proto` syntax. Add `.proto`-specific legend entries — `type` keyword, `function` keyword, `annotation` keyword, `@`-annotation use sites — and feed them from the proto AST walk in `analyzeProto()`. |
| AST walk for tokens | new helper in `internal/server/proto_analyze.go` (or new `proto_semantic_tokens.go`) | Walk the AST returned by protocompile; emit `SemanticToken` entries for the new keywords and annotation-use spans. |

### Acceptance criteria

- [ ] `go.mod` pinned to v1.2-compliant `protocompile`; existing analyze pipeline keeps building
- [ ] New keywords parse without spurious errors
- [ ] Parse errors for malformed `type`/`function`/`annotation`/`@annot` use sites render as user-meaningful LSP diagnostics
- [ ] Semantic tokens highlight the new keywords and annotation use sites
- [ ] All existing v1.1 `.proto` fixtures continue to lex and parse identically

---

## #051 — `protolsp`: source-map consumption + go-to-definition

**Repo:** `protolsp`
**Milestone:** M3
**Labels:** `protolsp`, `source-map`, `schema-extensions`
**Depends on:** #035, #050

Consume the embedded `SourceMap` to drive go-to-definition on `type`, `function`, and `annotation` references; resolve runtime validation errors back to source locations for in-editor diagnostics.

### Implementation notes — entry points

Current state: protolsp reads **no FileOptions today**. The schema resolver (`internal/schema/resolver.go`) interfaces with protoregistry but doesn't inspect option extensions. Definition routing is in place but in-file only for proto today; cross-file requires the new source-map.

| Step | File | Pattern |
|---|---|---|
| Compile-time source-map extraction | `internal/schema/protofiles.go` (compile path, around line 100+) | After `protocompile.Compile()` succeeds, walk the resulting `FileDescriptor`'s `FileOptions`, look up extension `50404` (`validator.v1.source_map`), unmarshal it, and pass it through to the index entry. |
| Index entry extension | `internal/protoindex/protoindex.go` `entry` struct (line 82) | Add `SourceMap *validatorv1.SourceMap` field. Populate during `AST()` load (line 96) by reading from the FileOptions. Use the descriptor.proto schema from issue #005. |
| Symbol extraction during indexing | `internal/protoindex/protoindex.go` `entry.AST()` (line 96) | Extend the existing AST walk to collect `type`/`function`/`annotation` declarations into the entry's symbol map. Mirrors how messages and enums are indexed today. |
| Cross-file go-to-definition | `internal/server/proto_definition.go` `resolveProtoImport()` (line 54) | When a definition request targets a `type`/`function`/`annotation` reference, look up the FQN in the importing file's `protoindex` chain, consult the entry's `SourceMap` for the declaration's source range, return as `protocol.Location`. |
| Hover for new symbols | `internal/server/proto_hover.go` `protoHoverFor()` (line 35) | Same source-map lookup; render a Markdown card with the declaration text and its leading doc comment. |
| Position encoding | `internal/positions/` package | Existing helpers convert byte offsets to LSP `Position` honoring the negotiated `s.positionEncoding` (UTF-8 / UTF-16). Use these when converting `SourceMap.SourceLocation` (1-indexed line/column from protocompile) to LSP `Position`. |

### Acceptance criteria

- [ ] `protoindex.entry` carries `SourceMap` when present in the compiled file's `FileOptions`
- [ ] Go-to-definition resolves `type` / `function` / `annotation` references across files
- [ ] Hover renders the declaration site for the same references
- [ ] Position encoding round-trips correctly under both UTF-8 and UTF-16 negotiated modes
- [ ] Missing source map (e.g., `.proto` compiled by a pre-v1.2 toolchain) gracefully degrades to in-file resolution

---

## #052 — `protolsp`: annotation-aware diagnostics

**Repo:** `protolsp`
**Milestone:** M7
**Labels:** `protolsp`, `diagnostics`, `schema-extensions`
**Depends on:** #051

Surface annotation-validation errors (wrong arg types, unknown annotation, missing required arg) as in-editor diagnostics with source positions. Likely shares infrastructure with `protocompile`'s linker error reporting.

### Implementation notes — entry points

Most annotation-validity checks (signature mismatch, unknown FQN, wrong arg type, missing required arg) are already enforced by `protocompile`'s linker (issue #032) and surface through the existing `appendDiag` bridge in `proto_analyze.go`. This issue handles the LSP-side concerns that don't naturally surface through linker errors: hover-time enrichment for diagnostic context, multi-diagnostic grouping, and runtime-error → source mapping.

| Step | File | Pattern |
|---|---|---|
| New document field | `internal/server/document.go` (around `Document` struct, line 139 for the existing `ProtoDiagnostics` field) | Add `AnnotationDiagnostics []protocol.Diagnostic`. Mirrors the existing `ProtoDiagnostics` pattern. |
| Post-parse annotation pass | extend `internal/server/proto_analyze.go` `analyzeProto()` (line 72) | After successful parse, call a new `validateAnnotations(ast, registry)` helper. Walks the AST for `@annot(...)` use sites; for each, looks up the annotation declaration via the protoindex (#051); checks arg counts, arg types, required args; collects diagnostics with source positions from AST nodes. |
| Annotation registry lookup | new helper `internal/server/proto_annotations.go` | `lookupAnnotationDecl(fqn, doc) (*validatorv1.AnnotationDecl, bool)` — consults the importing file's protoindex chain for an `annotation` declaration with the given FQN. |
| Diagnostic emission | extend `internal/server/diagnostics.go` `publishDiagnosticsFor()` | Concatenate `doc.ProtoDiagnostics + doc.AnnotationDiagnostics` when publishing for `.proto` URIs. Both diagnostic sets share `Source: "protolsp/proto"` so the editor groups them. |
| Runtime-error overlay (future) | new file `internal/server/proto_runtime_diagnostics.go` | When a validator (protocheck via integration) returns enriched Violations carrying source locations from the source map, render them as informational diagnostics with `DiagnosticSeverity.Information`. Initial scope: read from a configured file/socket; full integration deferred. |

### Acceptance criteria

- [ ] Unknown `@annotation` references reported as LSP diagnostics with source ranges
- [ ] Arg-count mismatches reported with both expected and actual counts
- [ ] Wrong arg-type errors include the expected type and the provided one
- [ ] Missing required args reported with the parameter name
- [ ] Diagnostics group correctly in the editor's Problems pane
- [ ] No regression on existing `ProtoDiagnostics` flow for files without schema extensions

---

## #060 — `protobuf-go`: function-stub codegen plugin (Go)

**Repo:** `protobuf-go` (custom fork)
**Milestone:** M5
**Labels:** `protobuf-go`, `codegen`, `schema-extensions`
**Depends on:** #034

Add a code generator that emits the `Functions` interface + `UnimplementedFunctions` struct + `RegisterFunctions` helper per RFC-001 §9.3 for each `.proto` containing function declarations. Reference implementation; Java/TS/etc. follow once Go is proven.

*(Original scaffold incorrectly targeted `pxfed`. Re-homed after the pxfed survey confirmed it is an editor, not a code generator.)*

### Implementation notes — entry points

Current state: this fork is **mostly upstream-compatible** with `google.golang.org/protobuf`. `go.mod` still declares the upstream module identity; `cmd/protoc-gen-go/` mirrors upstream layout. Custom-option reading already proven in-tree (e.g., `isTrackedMessage()` in `init.go:140–158`). Low friction for new emitters.

| Step | File | Pattern |
|---|---|---|
| Generation entry | `cmd/protoc-gen-go/main.go:27` (`main()` → `protogen.Options.Run()`) | No change to the entry; existing dispatch already covers per-file processing. |
| Per-file driver | `cmd/protoc-gen-go/internal_gengo/main.go:87` (`gengo.GenerateFile()`) | Drop a new call to `genFunctionStubs(g, f)` into the emission sequence after `genExtensions(g, f)` (around line 147). |
| FileOptions parsing | new helper `cmd/protoc-gen-go/internal_gengo/schema_extensions.go` | Read `FileOptions` extension `50401` (`FileFunctions`) from `f.Desc.Options().ProtoReflect().GetUnknown()` using the existing `protowire.ConsumeTag`/`ConsumeBytes` pattern. Decode into an in-memory `[]functionDecl` view. No dependency on `validator.v1` proto types — wire-format parsing avoids the circular module concern. |
| Stub emitter | new `cmd/protoc-gen-go/internal_gengo/function_stubs.go` | Function `genFunctionStubs(g *protogen.GeneratedFile, f *fileInfo)`. Emits via the existing `g.P(...)` builder pattern (no templates). Produces in order: (a) `type Functions interface { ... }`, (b) `type UnimplementedFunctions struct{}` with one method per declared function, (c) `func RegisterFunctions(eng Engine, impl Functions)` binding by FQN. |
| Engine import | `function_stubs.go` | The generated code imports an `Engine` interface; canonical home is `protocheck` (issue #040). Use `g.QualifiedGoIdent(...)` to resolve the import lazily so the dependency is opt-in for files without functions. |
| Output layout | same `.pb.go` per the existing convention | Function stubs land in the main `.pb.go` alongside messages and enums. No separate `_validator.pb.go` file needed. |

### Acceptance criteria

- [ ] `FileOptions.functions` (50401) parsed without dependency on the `validator.v1` Go module
- [ ] Generated `Functions` interface contains one method per declared function with the correct `(bool, *Violation)` return signature
- [ ] `UnimplementedFunctions` methods return `(false, &Violation{Code: "unimplemented", ...})` and embed naturally into user structs
- [ ] `RegisterFunctions(eng, impl)` binds every method by FQN
- [ ] Files with no `function` declarations generate identically to current output (zero overhead)
- [ ] Generated code passes `go vet` and `staticcheck`
- [ ] Round-trip test against `testdata/schema-extensions/01_basic.proto`: parse → descriptor → generate → compile → invoke RegisterFunctions → invoke each function → expected Violation shapes returned

---

## #061 — `protobuf-go`: annotation-aware codegen

**Repo:** `protobuf-go` (custom fork)
**Milestone:** M7
**Labels:** `protobuf-go`, `codegen`, `schema-extensions`
**Depends on:** #060

Surface schema-extension annotations in generated code where relevant: `@deprecated` → Go `// Deprecated:` comment; `@description` → docstring; `@example` → optional test-fixture emit; etc.

*(Original scaffold incorrectly targeted `pxfed`. Re-homed after the pxfed survey.)*

### Implementation notes — entry points

Integrates inline with the existing per-construct emitters rather than as a separate pass. Reuses the wire-format parsing helper from #060.

| Step | File | Pattern |
|---|---|---|
| Shared annotation reader | extend `cmd/protoc-gen-go/internal_gengo/schema_extensions.go` from #060 | Add `readAnnotations(opts proto.Message) []annotation` — generic helper reading extension `50400` (`AnnotationList`) from any of the 8 Options messages. Same wire-format parsing pattern, factored as a single function. |
| Enum decoration | `cmd/protoc-gen-go/internal_gengo/main.go:289` `genEnum()` | Before emitting the enum type declaration, read annotations from `EnumOptions`. Prepend `// Deprecated: <reason>` for `@deprecated`; append `@description` text into the leading `g.P()` comment block. Same for `EnumValueOptions` on each value. |
| Message decoration | `cmd/protoc-gen-go/internal_gengo/main.go:403` `genMessage()` | Same pattern for `MessageOptions`. Field-level annotations read from `FieldOptions` inside the field-emission loop. |
| Service / RPC decoration | wherever services emit | Same pattern for `ServiceOptions`, `MethodOptions`. |
| Function decoration | `function_stubs.go` from #060 | Read function-level `@deprecated`/`@description` from the per-function annotation list (carried inside `FunctionDecl.options` per `descriptor.proto`); emit as comments on the interface method. |
| Example-as-test emission | new file `cmd/protoc-gen-go/internal_gengo/example_tests.go` (optional, M7+) | For each `@example(value, expect = pass\|fail)`, emit a `Test<TypeName>_example_<n>` function into a sibling `<file>_examples_test.go` file that validates the example against the registered validator. Initial scope: emit only when a build tag is set; promote to default once stable. |
| Comment style | `g.P()` calls | Standard Go doc-comment convention: `// Deprecated: ...` on its own line; description text precedes the symbol's existing comment if any. |

### Acceptance criteria

- [ ] `@deprecated` produces compliant `// Deprecated: <reason>` comments recognized by `go vet` and goimports
- [ ] `@description` surfaces as the leading doc comment on enum / message / field / function / service / RPC declarations
- [ ] Existing leading comments from `.proto` files (`//`-style above the declaration) coexist with `@description` without duplication or loss
- [ ] Example-as-test emission (when enabled) compiles and passes for all `expect = pass` examples and fails for all `expect = fail` examples
- [ ] Files without schema-extension annotations generate identically to v1.1 output

---

## #070 — `protowire-go`: M5 runtime wiring through `protocheck`

**Repo:** `protowire-go`
**Milestone:** M5
**Labels:** `protowire-go`, `runtime`, `schema-extensions`
**Depends on:** #042, #060

Wire `protocheck` into `protowire-go`'s decode path so that PXF / `pb` / SBE decoders can optionally run validation and surface `Report`s. Likely an `UnmarshalOptions.Validate bool` knob.

---

## #080 — OpenAPI generator (separate tool)

**Repo:** TBD (likely `pxfed` or new `protowire-openapi`)
**Milestone:** M8
**Labels:** `openapi`, `tooling`, `schema-extensions`
**Depends on:** #034

Consume descriptors; map common `@validate` shapes to OpenAPI keywords (`matches` → `pattern`, `this.size()` → `minLength`/`maxLength`, `this in [...]` → `enum`, etc.). Emit `x-validation` extension for non-mappable rules. Type aliases become `components/schemas/<Name>`. Gnostic-style `@description` / `@example` / `@http` integrate.

---

## Labels suggested

| Label | Use |
|---|---|
| `meta` | Tracking / umbrella issues |
| `spec` | Spec / RFC / IETF-draft work |
| `proto` | Adds or changes a `.proto` schema file |
| `schema-extensions` | All RFC-001 work (parent label) |
| `protocompile` / `protocheck` / `protolsp` / `protobuf-go` / `pxfed` / `protowire-go` / etc. | Per-repo scope |
| `parser`, `ir`, `linker`, `options`, `lowering`, `runtime`, `codegen`, `source-map`, `i18n` | Component scope |
| `deferred` | v1.2 design decisions deferring to a later release |
| `release` | CHANGELOG / version-cut work |
| `tests` | Conformance / fixture work |
| `perf` | Benchmarks / performance budget |
| `tooling` | CLI / migration tools |

---

## Suggested milestone breakdown

- **M0** (Spec freeze): #002–#007, #010–#020 — all decisions made or deferred; carrier `.proto` files merged; STABILITY/CHANGELOG updated.
- **M1** (Parser + IR): #030–#032.
- **M2** (Lowering): #033–#034.
- **M3** (Source map + LSP): #035, #050–#051.
- **M4** (Engine + runtime): #040–#042.
- **M5** (Codegen + protowire-go wiring): #060, #070, #018.
- **M6** (i18n): #043.
- **M7** (Tooling integration): #052, #061.
- **M8** (OpenAPI): #080.
- **M9+** (Per-port adoption): one issue per port repo following the same shape.
