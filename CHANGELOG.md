# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

### Spec changes — Protowire Schema Extensions (v1.2.0 target)

First minor on the v1.0 freeze line. Strictly additive — every valid v1.1 schema remains a valid v1.2 schema. Bump driven by [RFC-001 — Protowire Schema Extensions](docs/RFC-001-schema-extensions.md); formal text lands in IETF draft `-01` (in preparation). Issues tracked at [`docs/RFC-001-issues.md`](docs/RFC-001-issues.md).

- **Three new top-level declarations.** `type`, `function`, and `annotation` extend the schema grammar with refinement aliases, validation-function signatures, and user-declarable annotation kinds. See RFC-001 §5 for grammar deltas and worked examples.
- **`@annotation(...)` use-site syntax.** Unified annotation framework subsuming validation rules, descriptions, examples, deprecation, OpenAPI hints, and future metadata. Hybrid placement: leading on block declarations (`message`, `service`, `rpc`, `enum`, `oneof`); trailing on single-line declarations (`type`, `field`, `function`). Existing `[(option) = value]` brackets coexist permanently — annotations are first-class sugar, brackets remain the raw escape hatch.
- **PXF presence semantics promoted into the validation layer.** `(pxf.required)` and `(pxf.default)` gain canonical annotation forms `@required` and `@default(value)`. Bracket forms retain their extension numbers (`50000`, `50001`) and behavior unchanged.
- **New contextual keywords.** `type`, `function`, `annotation` — recognized as keywords only at the start of a top-level declaration; accepted as identifiers everywhere else. Existing schemas that use these words as message/oneof/field/enum-value names (e.g., `oneof type { ... }`, common in Google APIs) remain valid v1.2 schemas without modification. The `@` sigil is reserved as the annotation-use-site marker. No source-level incompatibility is introduced.
- **New extension number sub-range.** `50400`–`50499` reserved in [`proto/schema/v1/descriptor.proto`](proto/schema/v1/descriptor.proto) for schema-extension carriers. Allocated in this release: `50400` (annotation carrier on every Options message, named per kind — `file_annotations`, `message_annotations`, …), `50401` (functions), `50402` (annotation declarations), `50403` (type declarations), `50404` (embedded source map). The `50100`–`50101` numbers are avoided because SBE already uses them on `FileOptions`. See [`STABILITY.md`](STABILITY.md) for the renumbering prohibition.
- **Structured error model.** Functions return `(bool, *Violation)` with stable codes, structured parameters, and engine-enriched type-chain provenance. Per-locale message catalogs handle i18n at render time. Report wire shapes (`Report`, `EnrichedViolation`, `Violation`, structured `FieldPath`, typed `Value`) are pinned in [`proto/schema/v1/report.proto`](proto/schema/v1/report.proto) so all 10 ports emit equivalent reports (RFC-001 §7, issue [#65](https://github.com/trendvidia/protowire/issues/65)). Params and captured values use the typed `Value` oneof — never `google.protobuf.Value` — preserving int64/uint64 width, bytes, and the set/null/absent presence distinction.
- **Project-level engine configuration.** Engine selection and engine knobs live in `protowire.config.textproto` at the project root — a text-format `protowire.schema.config.v1.EngineConfig` message pinned in [`proto/schema/config/v1/config.proto`](proto/schema/config/v1/config.proto) (RFC-001 §9.4, issue [#60](https://github.com/trendvidia/protowire/issues/60)). Discovery walks upward to the nearest config (no merging); precedence is per-setting CLI flags > `--config` > `PROTOWIRE_CONFIG` (file pointer only) > discovered file > defaults (`cel`, lenient, collect-all). Unknown engine names are startup errors, never fallbacks.
- **Well-known type semantics in refinement rules.** `google.protobuf.Timestamp`/`Duration` bind `this` to the engine-native temporal value (parallel to wrapper unwrap; comparisons mandatory); `google.protobuf.Any` never unwraps — `this.type_url` refinement is the canonical pattern and auto-unpacking is forbidden; all other messages bind structurally; engine `now()` builtins must be run-stable within one `Report` (RFC-001 §6.2, issue [#61](https://github.com/trendvidia/protowire/issues/61)). Spec-text only — no descriptor or lowering change.
- **Normative recursion-depth limit.** Nested-message validation is depth-limited: root at depth 0, each message-typed value +1, default **64**, configurable via `EngineConfig.max_recursion_depth` (RFC-001 §6.4, issue [#62](https://github.com/trendvidia/protowire/issues/62)). At the limit the engine records a fail-closed `protowire.depth_exceeded` violation, sets `Report.truncated`, and continues with siblings — never a silent accept, never a report-destroying hard error. Depth definition, default, and at-limit behavior are normative across ports. Violation codes under the `protowire.` prefix are now reserved for spec-defined violations (`protowire.required`, `protowire.depth_exceeded`).
- **Backward compatibility.** Wire format unchanged for v1.1 schemas. Stock `protoc`, `protobuf-go`, and every existing protowire port round-trip the new carrier extensions byte-identically as opaque options. v1.1 ports parse v1.1 schemas as before; v1.2 grammar requires v1.2+ ports.

### New files

- `docs/RFC-001-schema-extensions.md` — design RFC (this release's driving spec).
- `docs/RFC-001-issues.md` — tracking-issue scaffold for the v1.2.0 work, paste-ready across consuming repos.
- `proto/schema/v1/descriptor.proto` — descriptor lowering targets (stock-proto3, parseable by any v1.x port).
- `proto/schema/v1/annotations.proto` — canonical user-facing annotation declarations (requires v1.2 grammar; lands once parser support exists, see [#004](docs/RFC-001-issues.md)).
- `proto/schema/v1/report.proto` — validation report wire shapes (stock-proto3; runtime artifact emitted by engines, allocates no extension numbers).
- `proto/schema/config/v1/config.proto` — project-level engine configuration (stock-proto3; build-time artifact loaded from `protowire.config.textproto`, never embedded in descriptors).

### Per-port implementation status

Adoption is independent per port. Reference Go implementation lands in `protocompile`/`protocheck`/`protolsp`/`pxfed`/`protowire-go` (M1–M5); other ports adopt on their own schedule (M9+). See RFC-001 Appendix B for tracker.

## [1.1.1] – 2026-07-15

Tooling patch on the v1.0 spec line. No grammar, wire-format, or
envelope changes.

### Spec changes

None.

### Tooling

- **`pxf query` binds and emits non-finite floats as the spec's
  identifiers** (#86). With a schema in scope, a `@dataset` cell
  holding bare `nan`/`inf` bound as the *string* `"nan"`/`"inf"`
  instead of a non-finite float (§3.8: the bare forms lex as
  identifiers, and the schema layer never re-bound them on
  float/double fields; the signed forms already bound correctly).
  On output, non-finite floats emitted via Go's `NaN`/`+Inf`/`-Inf`
  spellings, which are not PXF literals and did not re-parse — they
  now emit as `nan`/`inf`/`-inf`. Also bumps the CLI's protowire-go
  pin to v1.2.2, since v1.0.0's lexer rejected the signed forms
  (`-inf`) in dataset cells outright.

## [1.1.0] – 2026-05-15

Tooling release on the v1.0 spec line. No grammar, wire-format, or
envelope changes — the spec is frozen at 1.0. This tag bundles the
post-freeze CLI consolidation, the new `pxq` query tool, and the
editor extension's rewrite as a thin LSP client.

### Spec changes

None. Existing v1.0 parsers and ports continue to work without
modification.

### Tooling

- **`cmd/protowire` and `cmd/pxq` unified into a single `pxf`
  binary** (#42). The old `protowire` and `pxq` commands are gone;
  every subcommand now lives under `pxf <subcommand>`. **Breaking
  for users with scripts that invoke `protowire` or `pxq`
  directly** — update them to `pxf`. The grammar / wire format /
  envelope are unchanged, so on-disk documents remain compatible.
- **`pxq` query subcommand** (now `pxf q`): jq-style transforms
  over `.pxf` documents with three input adapters (JSON, YAML,
  CSV) and protoregistry-backed schema resolution. Lands in
  stages A (#32), B (#33), C (#34), plus follow-ups for in-document
  `@proto` binding (#35, #36), bundled canonical schemas (#37),
  protoregistry `-s` / `-n` / `--schema` flags (#38), schema
  inference (#39), and a strict-mode AST validator (#40). Design
  doc at `docs/design/pxq.md`.

### Editor extensions

- **VS Code 2.0.0 → 2.1.0** — the extension rewrites as a thin
  `vscode-languageclient` host spawning the
  [protolsp](https://github.com/trendvidia/protolsp) Language
  Server over stdio (#47). 2.1.0 adds a `TextDocumentContentProvider`
  for the `registry:` URI scheme so go-to-definition can open
  `.proto` sources fetched from protoregistry when they aren't on
  disk (#48). The in-extension `@trendvidia/protowire` parser is
  gone — diagnostics, hover, completion, code actions, and
  go-to-definition all come from the LSP now.
- **JetBrains**: gradle-wrapper bump only (#44); plugin version
  unchanged at 1.0.0.

### Internal

- Schema-resolver helpers extracted to `internal/schemaresolve` so
  `pxf check` and `pxf q` (and future tools) share one
  protoregistry-resolution code path (#41).

### Pre-built editor bundles

- `editors/vscode/dist/pxf-2.1.0.vsix` (replaces 2.0.0).
- `editors/jetbrains/plugin/dist/pxf-jetbrains-1.0.0.zip` (unchanged).

## [1.0.0] – 2026-05-13

First major-version cut and spec freeze line. Three one-time spec
changes ship in lockstep across the whole `protowire-*` stack.
**Breaking** — there is no alias period; v1.0 is itself the major bump.
See `STABILITY.md` for the rules-of-engagement after v1.0.

### Spec changes

- **`@table` → `@dataset` rename** (draft §3.4.4). Same semantics; the
  directive represents the dataset (rows), not the storage container.
  v1 reserves `@table` for a future storage-definition meaning in the
  database export/import direction sketched in §3.4.6.
- **`@proto` directive added** (draft §3.4.5). Four body shapes
  lexically distinguished: anonymous (`@proto { ... }`), named
  (`@proto pkg.Type { ... }`), source (`@proto """..."""`), descriptor
  (`@proto b"..."`). Descriptor form is the MUST-support shape; the
  other three are QoI. Anonymous `@proto` is consumed one-shot by the
  next directive that requires a typed binding.
- **Reserved directive names expanded from 5 to 13** (draft §3.4.6).
  Adds `dataset`, `proto`, `entry` (promoted from spec-registered),
  and the future-allocated `table`, `datasource`, `view`, `procedure`,
  `function`, `permissions`.

### Source switch

The IETF draft authoring source moves from raw `.txt` to
kramdown-rfc Markdown:
`docs/draft-trendvidia-protowire.md` is now the source of truth;
the paginated `.txt` is regenerated via `scripts/build_rfc.sh`.

### Port releases at v1.0 freeze

- `protowire-go` v1.0.0
- `protowire-java` v1.0.1 (the v1.0.0 tag exists in git but the
  Maven Central publish failed on two leftover javadoc `Result#tables()`
  references that survived the rename; v1.0.1 ships the fix)
- `protowire-typescript` v1.0.0 (published to npm as
  `@trendvidia/protowire@1.0.0`)

### Editor extensions

Both extensions bump to v1.0.0 in lockstep and pick up the new
parser bundles:

- **VS Code** `editors/vscode/dist/pxf-1.0.0.vsix` — embeds
  `@trendvidia/protowire@1.0.0`. TextMate grammar highlights
  `@dataset` and `@proto`.
- **JetBrains** `editors/jetbrains/plugin/dist/pxf-jetbrains-1.0.0.zip`
  — embeds the `protowire-pxf-1.0.1.jar` parser. Same grammar
  changes as VS Code.

Documents that use `@table` will get red squiggles under v1.0.0
extensions; rename to `@dataset` to clear them.

## [0.74.0] – 2026-05-12

Streaming-consumption release. One informational addition to §3.4.4:
implementations MAY expose a row-by-row streaming API alongside the
materializing one (and for `@table`'s CSV-replacement use case
typically SHOULD), with a pinned contract on row order, per-row
enforcement, and working-set memory. No grammar change, no wire
change. First-port implementation: `protowire-go` v0.74.0
(`pxf.TableReader` over `io.Reader`).

### Changed

- **PXF spec — `@table` streaming consumption.** Section 3.4.4 grows
  a "Streaming consumption" paragraph stating that implementations
  MAY (and for the CSV-replacement use case typically SHOULD) expose
  a row-by-row streaming API alongside the materializing one. The
  contract: rows in source order, per-row arity + cell-grammar
  enforced as each row is consumed (not deferred), working-set
  memory bounded by the largest single row. Streaming and
  materializing APIs that coexist in the same implementation MUST
  produce byte-identical row sequences for the same input.

  No grammar change, no wire change — this is informational, making
  explicit what §3.4.4's existing "consumer-interpreted side-channel"
  framing already permitted. Without it, port maintainers reasonably
  read "rows are exposed through a parser API" as mandating full
  materialization.

  Spec changes:
  - `docs/draft-trendvidia-protowire-00.txt` §3.4.4: new paragraph
    after "Consumer contract."
  - `docs/grammar.ebnf`, `docs/grammar.svg`: unchanged.

  First-port implementation: `protowire-go` v0.74.0 (`pxf.TableReader`
  over `io.Reader`, `NewTableReader`/`Type`/`Columns`/`Directives`/
  `Next`). Other ports add streaming when their CSV-replacement
  consumers ask for it; no conformance obligation to expose it.

## [0.73.0] – 2026-05-11

Schema-constraint + directive-expansion release. Three additions to
the PXF text format, all strictly additive on the wire: a new
schema-level reserved-name rule (Section 3.13), the `@entry` bundle
directive (Section 3.4.3) with a generalized zero-or-more
prefix-identifier list on every named directive, and the `@table`
bulk-rows directive (Section 3.4.4) — the protowire-native
replacement for CSV. Wire format unchanged. First-port
implementation: `protowire-go` v0.73.0.

### Added

- **PXF grammar — `@table` directive (CSV replacement).** New top-level
  directive form for representing many instances of a single message
  type in a single PXF document. Syntax:

  ```
  @table <type> ( <col1>, <col2>, ... )
  ( <val1>, <val2>, ... )
  ( <val1>, <val2>, ... )
  ```

  The header names the row message type and the column list (top-level
  field names on `<type>`); each subsequent parenthesized tuple is a
  row whose values bind positionally to the columns. Empty cells
  (between two commas) denote absent fields and engage the existing
  `pxf.default` / `pxf.required` machinery; `null` literals denote
  present-but-null; any other value is present-with-value. Same
  three-state semantics as the keyed form, just spelled positionally.

  v1 restrictions (relaxed in a future revision):
  - Cell values are scalar-shaped (`value − list − block_value`).
    List literals `[...]` and block values `{...}` are NOT permitted
    in cells.
  - Column entries are unqualified field names. Dotted paths
    (`addr.city`) are NOT permitted.
  - Strict row arity: row arity MUST equal column count. No
    trailing-empty shorthand.
  - Standalone: a document containing `@table` MUST NOT also contain
    `@type` or top-level field entries. The `@table` header is the
    document's type declaration.

  `@table` is consumer-interpreted in the same side-channel manner as
  `@header` / `@entry`: rows are exposed through a parser API distinct
  from the body's schema layer. This spec does NOT mandate a canonical
  "decode-as-`repeated <type>`" semantics — applications that want
  that one-liner construct it on top of the rows API.

  Wire format unchanged. Strictly additive — any v0.74.0-valid PXF
  document remains valid (the new productions occupy fresh top-level
  surface).

  Spec changes:
  - `docs/grammar.ebnf`: new `table_directive`, `column_list`, `row`,
    `row_cell`, `row_value` productions; `directive` choice extended;
    `directive_name` excludes `table`.
  - `docs/draft-trendvidia-protowire-00.txt` §3.3 ABNF: matching
    additions. New §3.4.4 "The @table Directive" with normative
    conformance rules.
  - `docs/grammar.svg`: regenerated (47 rules; +5 over the prior
    revision).

  Editor support:
  - `editors/vscode/syntaxes/pxf.tmLanguage.json`: new
    `table-directive` pattern highlights `@table <type>` consistently
    with `@type`; new paren punctuation rules.
  - JetBrains bundle regenerated.

  Testdata:
  - `testdata/example-table.pxf` — happy path over `test.v1.AllTypes`
    with five columns and four rows, exercising the three cell states.
  - `testdata/table/` — adversarial fixtures: short/long row arity,
    `@table` + `@type`, `@table` + body field, list cell, block cell,
    dotted column. Each fixture states its violation in the leading
    comment. Conformance-harness wiring deferred.

  Ports: implementing `@table` is a new lexer entry (`@table`
  keyword), a new parser path (header + row loop), and a thin
  consumer-API surface (`parser.tables()` or equivalent returning
  an ordered list of `TableBlock` records). The schema layer never
  receives table rows as message body entries.

  Carry-forward fix: PR #16's grammar.ebnf comment referenced
  "Section 3.4.4" for `@entry`; the draft places it at §3.4.3.
  Corrected here while updating the same notes block for `@table`.

- **PXF grammar — `@entry` directive + generalized prefix list.** The
  `named_directive` production grows from a single optional prefix
  identifier to zero-or-more: `@<name> *(<prefix-id>) [ { ... } ]`.
  Any v0.72.0-valid named directive remains valid; the change is
  strictly additive. Spec registers `@entry` as the first
  in-spec-defined named directive (Section 3.4.3), with four
  permitted shapes:

  ```
  @entry { ... }                       ; anonymous, typeless
  @entry name { ... }                  ; labeled, typeless
  @entry some.pkg.Type { ... }         ; typed only (dotted ident)
  @entry name some.pkg.Type { ... }    ; labeled and typed
  ```

  `@entry` is consumer-interpreted; this document defines no meaning
  for the label beyond preservation in directive order. The canonical
  use case is manifest documents that bundle heterogeneous, typed
  sub-messages alongside a body.

  Wire format unchanged — text-format-only sugar. Ports MUST relax
  their `named_directive` lexer to accept zero-or-more prefix
  identifiers before claiming the next-version conformance (~1-line
  change in most ports: replace "accept one optional identifier"
  with "loop accepting identifiers until you hit `{` or end-of-
  directive"). Ports MUST enforce the `@entry`-specific cardinality
  (0–2 prefix identifiers) at the consumer layer.

  Spec changes:
  - `docs/grammar.ebnf`: `named_directive` uses `{ identifier }`
    instead of `[ identifier ]`. New notes block describes the
    per-directive cardinality convention.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3 ABNF:
    `named-directive` uses `*( 1*WSP identifier )`. Section 3.4.2
    rewritten to describe per-directive prefix semantics. New
    Section 3.4.3 defines `@entry`.
  - `docs/grammar.svg`: regenerated.

  Editor support:
  - `editors/vscode/syntaxes/pxf.tmLanguage.json`: new
    `named-directive` pattern highlights `@<name>` plus prefix
    identifiers (dotted → type; bare → tag).
  - JetBrains bundle regenerated via
    `scripts/sync_jetbrains_grammar.py`.

  Testdata: `testdata/example-entries.pxf` demonstrates the four
  shapes alongside a `@type` body declaration.

### Changed

- **PXF schema constraint — reserved names.** A protobuf schema bound
  for PXF use MUST NOT declare a message field, oneof, or enum value
  whose name is case-sensitively equal to `null`, `true`, or `false`.
  These names lex as PXF value keywords (Section 3.9 of the draft), so
  a field/oneof/enum value bearing such a name is unreachable from
  PXF surface syntax — the tokenizer always resolves the bare token
  to the keyword branch. The directive-name exclusion is widened
  symmetrically: `@null`, `@true`, and `@false` join `@type` as
  reserved directive names.

  Wire format unchanged — this is a static-time schema check, not a
  wire migration. Schemas that violate the constraint were never
  round-trippable through PXF; rejecting them at descriptor-bind time
  surfaces a pre-existing latent bug rather than introducing one.

  Spec changes:
  - `docs/grammar.ebnf`: `directive_name` production now excludes
    `null`/`true`/`false` in addition to `type`. New "Schema
    Constraints" notes block formalizes the field/oneof/enum-value
    rule.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3 ABNF:
    `directive-name` exclusion list updated. New Section 3.13
    ("Schema Constraints") states the rule with MUST-language and
    bind-time conformance requirements. Cross-references added from
    Sections 3.9 and 6.1. (Section page numbering downstream of
    §3.13 has drifted; re-paginate before submission.)
  - `docs/grammar.svg`: regenerated.

  Tooling:
  - New `internal/pxfschema` Go package with `ValidateReflect` /
    `ValidateProto` entry points covering both descriptor shapes
    used in this repo.
  - `cmd/protoc-gen-pxf-java-meta`: rejects non-conforming
    `FileDescriptorSet`s at the top of `generateFile`.
  - `cmd/pxf`: new `lint` subcommand that runs the same check
    standalone against `--proto` or `--server` schemas.

  Ports MUST add the equivalent descriptor-bind check before
  claiming the next-version conformance. The check is ~30 lines per
  port (walk messages/oneofs/enum-values, case-sensitive set
  match).

## [0.72.0] – 2026-05-11

Named-directive release. Extends the PXF text format with
application-extensible `@<name> [<type>] [{ ... }]` blocks at
document root, alongside the existing `@type` directive. Wire format
unchanged. First-port implementation: `protowire-go` v0.72.0.

### Added

- **PXF grammar — named directives.** The document grammar grows a
  generic `@<name> [<type>] [{ ... }]` form alongside the existing
  `@type` directive. `@type` retains its current meaning (declares
  the body's message type, at most one per document); other names
  carry side-channel metadata that the consumer's runtime
  interprets — never the body's schema layer.

  Decoders that don't recognize a directive name MAY skip the
  directive after parsing its block for syntactic well-formedness,
  or MAY reject. The inner block uses the same entry grammar as a
  message body, so brace-matching, string-literal, and comment
  rules apply uniformly.

  Wire format unchanged — this is text-format-only sugar. Ports
  that already shipped a stricter "only @type recognized" lexer
  MUST relax it to accept the new form before claiming v0.72.0
  conformance.

  Spec changes:
  - `docs/grammar.ebnf`: new `directive`, `named_directive`, and
    `directive_name` productions; `document` rule generalized to
    accept any sequence of directives before the body.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3: ABNF
    grammar updated.  Section 3.4 split into 3.4.1 ("@type
    Directive") and 3.4.2 ("Named Directives") with conformance
    rules for unrecognized names.

  Motivating use case in the wild: chameleon's `@header
  chameleon.v1.LayerHeader { id = "x" }` preamble at the top of
  layer files, carrying per-file sanity-check fields the resolver
  cross-checks against its chain spec.

  First-port implementation: `protowire-go` v0.72.0.

## [0.70.0] – 2026-05-06

First tagged baseline of the spec repo, matching the `v0.70.x` release
line cut by every sibling port. Establishes the cross-port wire-equivalence
reference point and the adversarial corpus all ports must accept (or
reject, where the corpus probes hardening).

### Changed

- **PXF grammar (breaking)** — `docs/grammar.ebnf` now distinguishes
  `field_entry` (identifier key + `=` or `{ … }`) from `map_entry`
  (id/string/integer key + `:`). At the document top level only
  `field_entry` is accepted; `map_entry` is reserved for the inside of
  `{ … }` blocks where it represents map literal entries. Inputs like
  `123 = 234` and top-level `123: 234` are now parse errors.
  - All ports' parsers must mirror this; new adversarial fixtures
    `testdata/adversarial/pxf/{integer-key-assignment,integer-key-in-block,top-level-map-entry}.pxf`
    fail any port that hasn't caught up.

### Added

- **Editor extensions** under [`editors/`](editors/) — VS Code (`.vsix`)
  and JetBrains (`.zip`) plugins shipping prebuilt for offline install.
  Both bundle their port's own parser (`protowire-typescript` and
  `protowire-java` respectively) for inline parse-error squiggles, plus
  a TextMate grammar for syntax highlighting.
- **`docs/HARDENING.md`** adversarial corpus + per-port `check-decode`
  conformance harness, gated by
  [`scripts/cross_security_check.sh`](scripts/cross_security_check.sh).
- **Project security policy** at [`SECURITY.md`](SECURITY.md) with a
  contact and a 30-day coordinated-disclosure embargo for cross-port
  issues.
