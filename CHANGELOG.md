# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

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
  - `cmd/protowire`: new `lint` subcommand that runs the same check
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
