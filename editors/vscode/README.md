# PXF — Proto eXpressive Format (VS Code)

Schema-aware authoring for `.pxf` files (the text-based PXF encoding
defined in [`docs/grammar.ebnf`](../../docs/grammar.ebnf)) via the
[protolsp](https://github.com/trendvidia/protolsp) Language Server.

What you get:

- **TextMate-based syntax highlighting** for `@type`, comments
  (`#`, `//`, `/* … */`), strings (incl. triple-quoted and `b"…"`
  bytes), RFC 3339 timestamps, durations, integers, floats, booleans,
  `null`, enum constants, field assignments (`name = value`), and map
  entries (`key: value`).
- **Parse diagnostics** as you type: malformed PXF (unclosed strings,
  unbalanced braces, bad numeric literals, …) gets a red squiggle at
  the line and column protolsp reports.
- **Schema-aware diagnostics** when a `protoregistry` is configured:
  unknown fields, type mismatches, and unknown `@type` targets are
  flagged against the live schema. Diagnostics walk the registry's
  namespace chain so org-shared types resolve transparently.
- **Hover** on a field shows its protobuf type, the defining `.proto`
  file path, and the leading doc comment (when the registry's stored
  descriptors carry source info).
- Bracket matching, auto-closing pairs, and brace folding.

Behind the scenes the extension is a thin
[`vscode-languageclient`](https://www.npmjs.com/package/vscode-languageclient)
host that spawns the `protolsp` binary over stdio. Almost all of the
behavior above lives server-side — the extension itself is ~80 lines.

## Prerequisites

You need the `protolsp` binary on disk. Install with Go:

```bash
go install github.com/trendvidia/protolsp/cmd/protolsp@latest
```

The default search order at extension activation is:
1. `protolsp.path` setting (workspace or user)
2. `PROTOLSP_PATH` environment variable
3. `protolsp` on `$PATH` (which `go install` adds to via `~/go/bin`)

## Configuration

Open VS Code settings (UI or `settings.json`) and search for
**"PXF Language Server"**:

| Setting | Default | Purpose |
|---|---|---|
| `protolsp.path` | `""` | Override the binary location. Useful for development builds (`/path/to/protolsp/cmd/protolsp/protolsp`). |
| `protolsp.logLevel` | `""` | Pass-through to the server (`debug`/`info`/`warn`/`error`). Empty means the server picks its own default. |
| `protolsp.registry.address` | `""` | gRPC endpoint of the protoregistry server. Empty disables schema validation — protolsp falls back to parse-only diagnostics. |
| `protolsp.registry.namespace` | `""` | Primary namespace for this workspace. protolsp consults `GetNamespaceChain` to walk ancestors automatically; don't list them here. |
| `protolsp.registry.token` | `""` | Bearer token for protoregistry auth. Production deployments should pull this from a secret manager — the literal-string setting is a development convenience. |

A minimal `.vscode/settings.json` for a hierarchy-aware workspace:

```json
{
  "protolsp.registry.address": "registry.example.com:443",
  "protolsp.registry.namespace": "acme-billing"
}
```

## Install

The extension is **not** yet published to the VS Code Marketplace.
Install locally:

### Option 1 — Install the shipped `.vsix`

A pre-built package lives at [`dist/pxf-2.0.0.vsix`](dist/pxf-2.0.0.vsix).
No toolchain required to install:

```bash
code --install-extension editors/vscode/dist/pxf-2.0.0.vsix
```

Or in the VS Code UI: open the **Extensions** panel → click **…** →
**Install from VSIX…** → pick `editors/vscode/dist/pxf-2.0.0.vsix`.

### Option 2 — Rebuild the `.vsix`

Useful after changing the extension code or the grammar. Requires
Node.js 18+:

```bash
cd editors/vscode
npm install              # one-time
npm run package          # bundles extension.js with esbuild + writes dist/pxf-<version>.vsix
code --install-extension dist/pxf-2.0.0.vsix
```

### Option 3 — Symlink for development

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
├── package.json                    # extension manifest + configuration schema
├── tsconfig.json
├── language-configuration.json
├── src/
│   └── extension.ts                # vscode-languageclient host
├── syntaxes/
│   └── pxf.tmLanguage.json         # canonical grammar (mirrored into JetBrains bundle)
└── dist/
    ├── extension.js                # esbuild bundle (build product, not committed)
    └── pxf-2.0.0.vsix              # prebuilt extension package (committed)
```

## Architecture

Until v1.0 the extension ran the PXF parser in-process via
`@trendvidia/protowire` and produced parse-only diagnostics from the
extension host. v2.0 replaces that with the LSP-client model:

- The extension spawns `protolsp` over stdio.
- Document open/edit events flow to the server unmodified.
- Diagnostics, hover, and (future) completion come back from the
  server via standard LSP notifications.

This means **every editor that speaks LSP** (Neovim, Helix, Zed, the
hosted Monaco IDE over WebSocket) can use the same schema-aware
behavior without duplicating the parser in their host language.

## Keeping the grammar in sync

The grammar file in `syntaxes/` is the single source of truth. After
editing it, regenerate the JetBrains bundle copies with:

```bash
python3 scripts/sync_jetbrains_grammar.py
```

The script is idempotent — re-running with no source change produces
byte-identical output.
