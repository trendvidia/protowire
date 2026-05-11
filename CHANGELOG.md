# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

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
