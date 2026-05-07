# PXF — Proto eXpressive Format (VS Code)

Syntax highlighting **and inline parse-error squiggles** for `.pxf` files
(the text-based PXF encoding defined in
[`docs/grammar.ebnf`](../../docs/grammar.ebnf)).

What you get:

- **TextMate-based syntax highlighting** for `@type`, comments
  (`#`, `//`, `/* … */`), strings (incl. triple-quoted and `b"…"`
  bytes), RFC 3339 timestamps, durations, integers, floats, booleans,
  `null`, enum constants, field assignments (`name = value`), and map
  entries (`key: value`).
- **Live syntax validation**: the bundled `@trendvidia/protowire` parser
  runs on every edit and flags malformed PXF (unclosed strings,
  unbalanced braces, bad numeric literals, …) with a red squiggle at
  the exact line and column reported by the parser.
- Bracket matching, auto-closing pairs, and brace folding.

> **Not yet — coming in a follow-up:** schema-aware validation
> (field-not-found, type mismatch, missing-required). That requires a
> per-workspace descriptor-set setting; tracked alongside the packaging
> refactor below.

The extension is **not** yet published to the VS Code Marketplace. Install
it locally with one of the options below.

## Option 1 — Install the shipped `.vsix` (recommended)

A pre-built package lives at [`dist/pxf-0.1.1.vsix`](dist/pxf-0.1.1.vsix).
No toolchain required to install:

```bash
code --install-extension editors/vscode/dist/pxf-0.1.1.vsix
```

Or, in the VS Code UI: open the **Extensions** panel → click **…** →
**Install from VSIX…** → pick `editors/vscode/dist/pxf-0.1.1.vsix`.

## Option 2 — Rebuild the `.vsix`

Useful after changing the grammar or the parser. Requires Node.js 18+:

```bash
cd editors/vscode
npm install              # one-time
npm run package          # bundles extension.js with esbuild + writes dist/pxf-<version>.vsix
code --install-extension dist/pxf-0.1.1.vsix
```

## Option 3 — Symlink for development

Useful while iterating (changes pick up on `Developer: Reload Window`):

```bash
cd editors/vscode && npm install && npm run build
# macOS / Linux
ln -s "$PWD" "$HOME/.vscode/extensions/trendvidia.pxf-dev"
# Windows (PowerShell, run as admin)
New-Item -ItemType SymbolicLink -Path "$env:USERPROFILE\.vscode\extensions\trendvidia.pxf-dev" `
        -Target "$PWD"
```

## Layout

```
editors/vscode/
├── package.json                    # extension manifest (vsce reads this)
├── tsconfig.json
├── language-configuration.json
├── src/
│   └── extension.ts                # activate() + DiagnosticCollection
├── syntaxes/
│   └── pxf.tmLanguage.json         # canonical grammar (mirrored into JetBrains bundle)
└── dist/
    ├── extension.js                # esbuild bundle (build product, not committed)
    └── pxf-0.1.2.vsix              # prebuilt extension package (committed)
```

## Parser dependency

The extension's parser comes from
[`@trendvidia/protowire`](https://www.npmjs.com/package/@trendvidia/protowire)
on the npm registry, declared in `package.json` as:

```json
"dependencies": {
  "@trendvidia/protowire": "^0.70.0"
}
```

When `npm run package` runs, esbuild tree-shakes the
`@trendvidia/protowire/pxf` subpath into the bundled `dist/extension.js`,
so the published `.vsix` is self-contained — no runtime npm install on
the user's side. To refresh against a newer parser release, just bump
the version range in `package.json` and `npm install && npm run package`.

The published artifact is signed via npm provenance; if you want to
verify what got bundled was built from a known commit of
`trendvidia/protowire-typescript`, run `npm audit signatures` after
`npm install`.

## Keeping the grammar in sync

The grammar file in `syntaxes/` is the single source of truth. After
editing it, regenerate the JetBrains bundle copies with:

```bash
python3 scripts/sync_jetbrains_grammar.py
```

The script is idempotent — re-running with no source change produces
byte-identical output.
