# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

### Added

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
