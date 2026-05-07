# Changelog

## 0.1.2

- **Parser dependency now resolved from npm** instead of a vendored
  tarball. The extension now declares
  `"@trendvidia/protowire": "^0.70.0"` and pulls the package directly
  from the npm registry — meaning the published .vsix is built against
  the same audited, signed (`npm audit signatures`-verifiable) artifact
  that any other consumer would resolve. The `editors/vscode/libs/`
  directory and the `scripts/refresh_vscode_parser_pkg.sh` helper that
  the previous "tier 2 packaging refactor" flagged as temporary are
  both gone.
- No user-visible behaviour change — the bundled parser is the same
  `0.70.0` build that was previously vendored.

## 0.1.1

- **Parser stricter on key forms** (matching the updated `docs/grammar.ebnf`):
  - `=` and `{ … }` now require an identifier key. Examples like
    `123 = 234` or `child { 123 = 123 }` are flagged as parse errors.
  - The `:` (map-entry) form is rejected at the document top level —
    map literals only make sense inside a `{ … }` block. Use `=` for
    top-level field assignments.
- **Highlighting**: identifiers used as block keys (`nested_field { … }`)
  now pick up the same `variable.other.member` color as keys on the
  left of `=` and `:`.

## 0.1.0

- Initial release.
- **Syntax highlighting** for PXF (Proto eXpressive Format) files: comments,
  `@type` directive, strings (incl. triple-quoted and `b"…"` bytes),
  RFC 3339 timestamps, durations, integers, floats, booleans, `null`,
  enum values, field assignments, and map key/value separators.
- **Inline parse-error squiggles** via the bundled `@trendvidia/protowire`
  parser (syntax errors only — schema-aware validation comes in a
  follow-up release).
- Bracket matching, auto-closing pairs, and brace folding via
  `language-configuration.json`.
