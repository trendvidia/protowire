# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

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
