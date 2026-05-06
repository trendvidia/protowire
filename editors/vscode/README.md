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
├── libs/
│   └── trendvidia-protowire.tgz    # vendored parser package (see below)
└── dist/
    ├── extension.js                # esbuild bundle (build product, not committed)
    └── pxf-0.1.1.vsix              # prebuilt extension package (committed)
```

## Refreshing the parser package (temporary workflow)

The extension's parser comes from **protowire-typescript**'s
`@trendvidia/protowire/pxf` export. Until that package is published to
the npm registry, the build is **vendored** as a tarball at
[`libs/trendvidia-protowire.tgz`](libs/trendvidia-protowire.tgz) and
refreshed by a script:

```bash
bash scripts/refresh_vscode_parser_pkg.sh
cd editors/vscode && npm install && npm run package
```

The script expects the sibling `protowire-typescript/` checkout next to
this repo, runs `npm run build` over there, and `npm pack`s the result
into `editors/vscode/libs/`.

> **TODO (packaging refactor)**: switch to a regular registry dependency
> `"@trendvidia/protowire": "^X.Y.Z"` once protowire-typescript is
> published to npm. At that point `libs/`, the `file:` reference in
> `package.json`, and this entire section all go away. Marked in
> `package.json` next to the dependency.

## Keeping the grammar in sync

The grammar file in `syntaxes/` is the single source of truth. After
editing it, regenerate the JetBrains bundle copies with:

```bash
python3 scripts/sync_jetbrains_grammar.py
```

The script is idempotent — re-running with no source change produces
byte-identical output.
