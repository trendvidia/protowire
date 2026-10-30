# `pxf` — the protowire toolchain

`pxf` is the canonical CLI for the `protowire` stack. One binary covers every operation you can do against a PXF document or a `.proto` schema: encode and decode against protobuf binary, validate and pretty-print, lint schemas for reserved-name violations, run jq-style queries with input adapters for CSV / JSON / YAML, infer a `.proto` from a tabular sample, convert between SBE XML schemas and `.proto`, compile v1.2 (RFC-001) schema sources to a lowered `FileDescriptorSet` image for stock buf/protoc, and compile documentation topics to a doc pack.

The format is named PXF (Protowire eXpressive Format); the binary is named after the format because that's the artifact users interact with day-to-day. The deeper design rationale for the query subcommand lives in [`QUERY.md`](QUERY.md); this README is the user-facing reference for the binary itself.

```bash
$ pxf --help
pxf is the unified CLI for the protowire stack. Subcommands cover
the encode/decode/validate/fmt/lint surface for the PXF text format,
plus a jq-style `query` subcommand, a `.proto`-emitting
`infer-schema` subcommand for tabular inputs (CSV, PXF @dataset),
a `build` subcommand compiling v1.2 (RFC-001) schema sources
to a lowered FileDescriptorSet image for stock buf/protoc, and a
`docs` subcommand compiling documentation topics to a doc pack.

Usage:
  pxf [command]

Available Commands:
  build        Compile v1.2 schemas to a lowered FileDescriptorSet image
  completion   Generate the autocompletion script for the specified shell
  decode       Decode protobuf binary to PXF (stdout)
  docs         Documentation tooling — the typed doc model and its compiler
  encode       Encode PXF to protobuf binary (stdout)
  fmt          Format a PXF document, canonicalizing keyed repeated fields
  help         Help about any command
  infer-schema Produce a .proto schema by inferring per-column types from a sample file
  lint         Check schema(s) for PXF reserved-name violations (draft §3.14)
  proto2sbe    Convert .proto with SBE annotations to SBE XML (stdout)
  query        Run a jq-style query against PXF, CSV, JSON, or YAML input
  sbe2proto    Convert SBE XML schema to .proto (stdout)
  validate     Validate PXF against schema
```

`completion` (shell tab-completion) and `help` are added automatically by [cobra](https://github.com/spf13/cobra) — they're not pxf-specific and aren't covered further here.

## Install

```bash
go install github.com/trendvidia/protowire/cmd/pxf@latest
```

Every published `protowire-*` language port ships the same binary as part of its CI image; users who only need to author or inspect PXF documents install just `pxf` and never touch the language ports.

## Subcommands at a glance

| Subcommand | Reads | Writes (stdout) | Needs `-m`? |
|---|---|---|---|
| `encode <file.pxf>` | PXF text | protobuf binary | yes |
| `decode <file.pb>` | protobuf binary | PXF text | yes |
| `validate <file.pxf>` | PXF text | `valid` on stderr | yes |
| `fmt <file.pxf>` | PXF text | canonical PXF text | yes |
| `lint` | every `.proto` reachable from the schema source | violations on stderr (none ⇒ `ok`) | no |
| `query <q> <file>` | PXF / CSV / JSON / YAML | PXF text | no (auto-strict if `-m` or `@type` present) |
| `infer-schema <file>` | CSV or PXF `@dataset` | `.proto` source | yes |
| `sbe2proto <schema.xml>` | SBE XML | `.proto` source | no |
| `proto2sbe` | `.proto` source via `-p` | SBE XML | no |
| `build <roots-or-files>...` | v1.2 (RFC-001) schema sources | lowered `FileDescriptorSet` binary | no |
| `docs build <roots-or-files>...` | PXF topic sources (+ image, registry export) | doc pack binary | no |
| `docs digest <roots-or-files>...` | PXF topic sources | one content digest per topic | no |
| `openapi <image.binpb>` | lowered image (+ doc pack, generator config) | OpenAPI 3.1 YAML/JSON | no |

## Schema resolution chain

Most subcommands need a schema to do their work. `pxf` resolves schemas through a four-step chain; sources later in the chain shadow earlier ones, so a user-supplied `-p` overrides a stale embedded schema, and a `protoregistry` fetch takes precedence over everything:

1. **Bundled canonical schemas** (`pxf/*`, `sbe/*`, `envelope/v1/*`) — always available; you can `encode -m envelope.v1.AppError` with no flags. Built into the binary via Go's `//go:embed`.
2. **In-document `@proto` directives** (`query` and `infer-schema` only) — when the input PXF carries `@proto Name { ... }`, `@proto """source"""`, or `@proto b"<base64-FDSet>"`, the embedded schema is honoured without a `-p` flag. See [`QUERY.md`](QUERY.md#input-adapters) for the four `@proto` body shapes.
3. **`-p schema.proto`** — one or more `.proto` files compiled at invocation time. Multiple `-p` flags are allowed; the files compile together so cross-imports work. Imports of canonical bundled paths (e.g. `import "envelope/v1/envelope.proto";`) resolve from the embed without an extra `-p`.
4. **`-s <server> -n <namespace> --schema <name>`** — fetch a descriptor bundle from a [protoregistry](https://github.com/trendvidia/protoregistry) gRPC server. All three flags must appear together; partial triples surface as a clear error before any network call. The bundled set is still loaded alongside the registry result.

A missing message surfaces with the actual sources searched:

```
$ pxf encode -m no.such.Type file.pxf
Error: message "no.such.Type" not found in resolved schema (bundled canonical types + -p + protoregistry)
```

## Common flags

These flags are persistent on every subcommand:

| Flag | Env fallback | Purpose |
|---|---|---|
| `-p, --proto <file>` | — | `.proto` source(s) to compile. Repeatable. |
| `-m, --message <FQN>` | — | Fully-qualified message name. Required for the schema-bound subcommands. |
| `-s, --server <addr>` | `PROTOREGISTRY_SERVER` | protoregistry gRPC address. |
| `-n, --namespace <ns>` | `PROTOREGISTRY_NAMESPACE` | protoregistry namespace. |
| `--schema <name>` | — | protoregistry schema name within the namespace. |

The `query` subcommand adds `--format`, `--strict`, and `--loose`; `infer-schema` adds `--sample-rows` and `--full-scan`. See the per-subcommand sections below.

Stdin is supported by `query` and `infer-schema` via the filename `-`. The other subcommands take a real path; they exit with a "no such file" error on `-`.

## Subcommands

### `pxf encode <file.pxf>`

Encodes a PXF document to protobuf binary on stdout. The bound message comes from the schema-resolution chain above; `-m` is required. The decode is the library's full one: `(pxf.default)` values are applied and an absent `(pxf.required)` field is an error, exactly as `validate` reports it — the bytes are what any port's full decode path produces for the same document.

```bash
# With a user schema:
pxf encode -p schema.proto -m pkg.Type input.pxf > output.pb

# Bundled canonical types — no -p needed:
pxf encode -m envelope.v1.AppError error.pxf > error.pb

# Registry-resident schemas:
pxf encode -s localhost:50051 -n billing --schema v1 -m billing.v1.Invoice invoice.pxf > invoice.pb
```

### `pxf decode <file.pb>`

Decodes a protobuf binary file to PXF on stdout. Inverse of `encode`. The output uses canonical PXF formatting (sorted field order is *not* applied — the field order follows the binary's wire order, which matches the schema's field-number order for protobuf-conformant producers).

```bash
pxf decode -p schema.proto -m pkg.Type input.pb > output.pxf
```

### `pxf validate <file.pxf>`

Parses the PXF document against the bound message and reports `valid` on stderr if it round-trips cleanly. Used as a CI step before publishing PXF artifacts. The validation runs through `pxf.UnmarshalDescriptor`, so all the v1.0 grammar (directives, `@dataset`, presence semantics) is honoured.

A body whose `@type` names a type the document's own `@proto` defines is checked against that type, whatever `-m` says: the embedded schema is authoritative (draft § The "@proto" Directive). For any other document `-m` names the type, and the document's `@type` stands in when `-m` is absent.

An `Any` block that opens with a type entry (`@type = "..."`, draft § Any Blocks) is read as the message its URL names, resolved by the name after the URL's last `/` through the same chain: `-p`, the bundled schemas, protoregistry and the document's own `@proto`. `encode` resolves it the same way, and so do `@dataset` cells.

```bash
pxf validate -p schema.proto -m pkg.Type input.pxf
pxf validate self-describing.pxf   # @type + @proto, no -m needed
# stderr: valid
# exit:   0 on success, 1 on any parse / bind / required-field error
```

### `pxf fmt [-w | --check] <file-or-dir>...`

Formats `.pxf` documents and `.proto` schema sources. A directory argument covers every `.pxf` and `.proto` beneath it. Without a flag, the one file's result goes to stdout. `-w` rewrites files in place. `--check` writes nothing, prints a unified diff for each file that is not formatted, and exits non-zero if there is one: the CI replacement for `buf format --diff --exit-code`, which cannot parse `@` ([#425](https://github.com/trendvidia/protowire/issues/425)).

```bash
pxf fmt --check ./schemas    # CI: fails, with a diff, on an unformatted file
pxf fmt -w ./schemas         # rewrite in place
```

**`.proto` sources**, v1.2 syntax included, are laid out as `buf format` lays them out. That is protocompile's `Legacy` printer preset, which protolsp's "format document" uses too. On every stock `.proto` in this repository that `buf format` accepts, the two outputs are byte-identical. A file that does not parse is reported and left alone. So is a file whose formatted output would lose a comment, an annotation use, or any name, string or number: the formatter refuses rather than delete source.

**`.pxf` documents** are re-encoded through the canonical writer, and `fmt` is idempotent on them.

The canonical writer follows the formatting rules in draft §3.10:

- Two-space indent for nested blocks
- One field per line at the top level
- Lists fit on one line when they fit a comfortable width; otherwise one element per line
- String values use the shortest valid quoting (single line, triple-quoted, or `b"..."` for bytes)

### `pxf lint`

Walks every `.proto` reachable from the configured schema source and reports any message field, oneof, or enum value whose name collides with a PXF value keyword (`null` / `true` / `false`) — see [Schema constraints](../../README.md#schema-constraints) and draft §3.13.

```bash
$ pxf lint -p schema.proto
ok

$ pxf lint -p offending.proto
offending.proto: message field "pkg.Side.null" uses PXF-reserved name "null" (draft §3.13)
1 reserved-name violation(s)
# exit 1
```

`pxf lint` doesn't require `-m`; it walks every message in scope. It does require either `-p` or `-s` — bundled-only invocations are intentionally not supported because the bundled schemas are already known clean (every release verifies this in CI).

### `pxf query <query> <file>`

Runs a jq-style query against PXF, CSV, JSON, or YAML input; output is always PXF. Embeds [`itchyny/gojq`](https://github.com/itchyny/gojq) as the engine; PXF-specific capability is exposed under a `pxf_` prefix (`pxf_directive`, `pxf_fieldnames`, `pxf_type`, `pxf_has`, `pxf_proto`). Format is inferred from the file extension; pass `--format pxf|json|yaml|csv` to override or to read from stdin (`-`).

```bash
# Count AAPL trades in a CSV file
pxf query 'pxf_directive("dataset")[0].rows | map(select(.symbol == "AAPL")) | length' march.csv

# Self-describing PXF — schema travels with data:
pxf query '.endpoints | map(.port)' config.pxf

# Construct a typed object from raw JSON:
pxf query '{symbol: .ticker, price: .last} | pxf_proto("trades.v1.Trade")' raw.json
```

Two modes:

- **Strict**: enabled implicitly when a top-level type is bound (`-m`, `@type` in the input, or an `@proto` directive). Unknown field references error at compile time with a did-you-mean hint.
- **Loose**: enabled implicitly when no root type is bound. Runtime errors degrade to `null` per jq.

Override the implicit choice with `--strict` (errors if no root type) or `--loose` (skips validation even when bound). See [`QUERY.md`](QUERY.md) for the full design — engine internals, every `pxf_*` function, input-adapter disambiguation rules, schema resolution detail, and the `@proto` ↔ `pxf_proto` terminology note.

### `pxf infer-schema <file>`

Produces a `.proto` schema by inferring per-column types from a sample file. Accepts CSV or PXF `@dataset` input. The generated schema goes on stdout; redirect to a file to seed a downstream binding workflow.

```bash
pxf infer-schema -m trades.v1.Trade march.csv > trades.proto
```

Each column gets the type its cells bind to: `bool`, `int64`, `double`, `string`, `bytes`, `google.protobuf.Timestamp` or `google.protobuf.Duration`. Only one widening exists: integer and float cells together make a `double` column, since a `double` field binds `1`. Within `--sample-rows` (default 1000), an integer column widens to `double` when a float appears. Past the window, a float in an integer column aborts with a recovery hint:

```
row 1247, column "age": value type double does not match inferred type int64
the first 1000 rows suggested int64; re-run with --sample-rows=1247 to
widen the type during inference, or edit the generated .proto manually to set "age" as double
```

A column that mixes any other kinds (an integer and a string, a bool and an integer, a timestamp and a string) is refused wherever the mix appears, naming the column and both rows: no field type binds both cells, so a `string` field would reject the integers.

It infers scalar columns only. A dotted column (`desc.energy`, a path into a sub-message), a list or block cell, or an identifier cell (which binds only to an enum) is refused, naming the column; write those fields by hand.

`--full-scan` walks the rest of the file collecting every contradiction and refusal instead of aborting fast. Empty cells (CSV) and absent cells (PXF) make the field `optional`, so proto3 presence round-trips. A `null` cell makes a scalar column its wrapper type (`google.protobuf.Int64Value`, `StringValue` and so on), because a `null` binds only to a field with explicit presence (draft `-01` § Booleans, Null, and Identifier Values). A timestamp or duration column holds a `null` as it is.

### `pxf sbe2proto <schema.xml>`

Converts an SBE XML schema to `.proto` on stdout. Used to onboard producers that already publish FIX-format SBE messages onto the protowire pipeline.

```bash
pxf sbe2proto trade-schema.xml > trade.proto
```

The output includes the `(sbe.*)` annotations needed to round-trip back through `proto2sbe`.

### `pxf proto2sbe`

Inverse of `sbe2proto`: takes a `.proto` source via `-p` and emits SBE XML on stdout. The input schema must carry the `(sbe.*)` annotations that `sbe2proto` writes (or be hand-annotated equivalently).

```bash
pxf proto2sbe -p trade.proto > trade-schema.xml
```

### `pxf build <schema-root-or-file>...`

Compiles Protowire v1.2 ([RFC-001](../../docs/RFC-001-schema-extensions.md)) schema sources with the reference compiler — the [trendvidia/protocompile](https://github.com/trendvidia/protocompile) fork — and writes a lowered `FileDescriptorSet` whose schema-extension semantics ride in the 1327–1331 carrier options. This is the **only v1.2-aware step** in the toolchain; the image it produces is stock protobuf, so everything downstream runs on unmodified buf/protoc:

```bash
pxf build -o image.binpb ./schemas/...   # the only step that parses v1.2 source
buf generate image.binpb                 # stock buf + plugin chain (forked protoc-gen-go, protoc-gen-pxf-*)
```

Stock buf/protoc never see v1.2 *source* — only images. There is no buf fork, and none is planned.

- Each **directory** argument is an import root; every `.proto` beneath it is compiled under its root-relative import path (a trailing `/...` is accepted and equivalent). A **file** argument is compiled under its base name, with its parent directory as import root.
- The canonical annotation libraries (`protowire/schema/v1/annotations.proto`, `pxf/annotations.proto`) and the well-known types resolve from the bundled schemas — no `-p` needed.
- Output is deterministic: the same sources produce a byte-identical image across runs, so it caches cleanly as a build artifact.
- The image carries `SourceCodeInfo`, as `buf build`'s does, so generated code keeps the schema's doc comments. `--source-info=false` leaves it out.
- Every message with a field that can hold a `null` gets the generated `_null` FieldMask (RFC-001 §8.6). `--null-mask=false` adds none, for a project that does not carry PXF nulls over binary. With it, a file with no v1.2 syntax builds to the same descriptor as `buf build`, so `buf generate image.binpb` makes the same code as `buf generate` from source ([#424](https://github.com/trendvidia/protowire/issues/424)).
- Each file carries buf's image marker, as `buf build`'s images do: a file the arguments did not name is marked as an import, so `buf generate image.binpb` generates only the named files ([#430](https://github.com/trendvidia/protowire/issues/430)). `--include-imports` generates the rest too.
- `-I <dir>` adds an import root: imports resolve from it, but its files are compiled only when imported, and are marked as imports in the image, so `buf generate` skips them. This is how a `buf.yaml` dependency is supplied ([#436](https://github.com/trendvidia/protowire/issues/436)):

  ```bash
  buf export buf.build/trendvidia/protocheck -o .deps/protocheck
  pxf build -o image.binpb -I .deps/protocheck ./proto
  buf generate image.binpb      # generates ./proto's files; protocheck's Go comes from its own module
  ```

  Pass a dependency with `-I`, not as an argument: a directory argument is compiled whole as the project's own files, and `buf generate` would write a copy of the dependency's code.
- `--check` compiles and reports diagnostics without writing an image — the CI entry point (`ok` on stderr, exit `0` when clean; exit `1` on any error diagnostic).
- `@http` lowers **twice** (RFC-001 §5.2): the annotation carrier keeps the whole operation surface (`summary`, `operation_id`, `tags`, `security`), and the routing skeleton is written again as the standard `google.api.http` option, so connect vanguard, grpc-gateway, Envoy's `grpc_json_transcoder` and buf's OpenAPI plugins bind the routes off a stock image with no protowire awareness. No `google/api/annotations.proto` import is added — the option rides in unknown-field bytes like the carriers. `--google-api-http=false` emits the carrier alone. A `{name}` path segment that binds no field of the request message — or binds a repeated one — is a compile error: it would be a route that 404s while the OpenAPI document promises it. A segment name is a dotted path from the request message's top level (`{order.id}`), optionally constrained by the `HttpRule` sub-path form (`{name=shelves/*}`); `pxf openapi` renders both (issue [#217](https://github.com/trendvidia/protowire/issues/217)). Two edges still refuse at render time although they compile and bind: a segment whose *leaf* is message-typed, and two bindings whose paths differ only in a sub-path constraint (they normalise to one OpenAPI path key) — see RFC-001 §5.2, "What the grammar above still leaves unsettled".

Engine configuration (RFC-001 §9.4) is discovered by upward walk from the first argument's directory, honoring the normative precedence: per-setting flags (`--function-library`) > `--config <path>` > `PROTOWIRE_CONFIG` > discovered `protowire.config.textproto` > defaults. For `build`, the configuration's `function_libraries` are what matter: each listed import path is compiled into the image so downstream consumers (§9.3 function-stub codegen, engine startup) see the declarations. Engine-runtime knobs (`engine`, `default_mode`, …) are validation-time concerns and ignored here.

```bash
pxf build --check ./schemas/...                          # CI gate, no output
pxf build -o image.binpb --config ci/protowire.config.textproto ./schemas/...
protoc --descriptor_set_in=image.binpb --cpp_out=gen $(FILES)   # stock protoc accepts the image
```

### `pxf docs build <topic-root-or-file>...`

Compiles PXF documentation topics (`@type protowire.docs.v1.TopicFile`) into a **doc pack** — the documentation analog of the lowered image: compiled topics, resolved anchors, redirects and an embedded full-text search index in one typed, byte-stable artifact. The model and the compiler contract are documented in [`docs/DOC-PACK.md`](../../docs/DOC-PACK.md).

```bash
pxf docs build -o docs.binpb --image image.binpb --registry registry.json ./topics/...
```

- Topic identity is `(key, locale)`, never the file path; directory arguments contribute every `.pxf` beneath them (a trailing `/...` is accepted and equivalent).
- Anchors resolve against **data inputs only**: `--image` (the lowered schema image) for schema and descriptor-path anchors, `--registry` (the appviewer registry export, `.binpb` / `.pxf` / `.json`) for widget anchors. Either may be omitted when the corpus uses no anchors of that kind; an anchor with no input to resolve against is an error naming the flag it needs.
- A **dangling anchor is a compile error**; a moved target is a redirect entry in the model. Descriptor paths are re-derived on every build and stamped with the image digest — never trusted from the source.
- `--release` applies release policy: topics that are not `REVIEW_STATE_APPROVED`, approvals invalidated by a later edit, and topics that never chose an audience tier all become errors. The revisor gate is compiler policy, so no authoring tool can skip it.
- `--stale-translations-fatal` escalates translation drift from warning to error.
- Output is deterministic, and `--check` reports diagnostics without writing a pack. A build with errors emits no pack at all.

```bash
pxf docs build --check ./topics/...             # CI gate, no output
pxf docs build --check --release ./topics/...   # the revisor gate
pxf decode -p proto/docs/v1/pack.proto -m protowire.docs.v1.DocPack docs.binpb   # inspect a pack
```

### `pxf docs digest <topic-root-or-file>...`

Prints the canonical content digest of each topic — `<digest>\t<key>\t<locale>\t<file>` — covering the title, summary and body a revisor reads. This is the value `review.approved_digest` and `translation.source_digest` record, so approving a topic or filing a translation means writing the digest printed here into the source; the authoring flow never reimplements the canonical encoding.

### `pxf openapi <image.binpb>`

Renders the OpenAPI boundary from a `pxf build` image and an optional `pxf docs build` pack (RFC-001 §#080, issue #173). Third member of the family: `build` (schemas → image), `docs build` (topics → pack), `openapi` (image + pack → boundary formats) — JSON/YAML exist only at this edge.

```bash
pxf openapi image.binpb                          # YAML on stdout
pxf openapi -o api.json image.binpb              # JSON, inferred from the extension
pxf openapi --pack docs.binpb --audience partner image.binpb
pxf openapi --check image.binpb                  # CI gate, no output
```

- **Schema half.** Messages, enums and v1.2 type aliases become `components/schemas` entries keyed by FQN; alias chains compose with `allOf` and fields reference their alias by `$ref`. Common `@validate` shapes map to native keywords — `matches` → `pattern`, `size()` bounds → `minLength`/`maxLength` (`minItems`/`maxItems`, `minProperties`/`maxProperties` by subject), `this in [...]` → `enum`, numeric comparisons → `minimum`/`maximum`/`exclusive*` — and every non-mappable rule is carried through verbatim under `x-validation`, never dropped. `@description`/`@deprecated(reason)`/`@example`/`@default`/`@required` map to their counterparts; a field is nullable exactly where PXF accepts `null` (§6.1): a singular field with explicit presence, so a message field, a wrapper, an `optional` scalar, a oneof member, a proto2 `optional` scalar or an editions EXPLICIT scalar, and never a plain proto3 scalar, a repeated or map field, or a proto2 `required` one ([#380](https://github.com/trendvidia/protowire/issues/380)); `@sensitive` fields honor the §6.7 doc-emit minima (no values or examples, `x-sensitive`/`x-sensitive-class` markers only). Property and parameter names are the proto field names as written — the same names the §5.2 binding rules speak in.
- **Operation half.** Methods carrying `@http` become operations: a `{…}` path segment binds a request field named from the top level as a dotted path (`{order_id}`, `{shelf.id}`) and optionally constrained by a sub-path pattern (`{name=shelves/*}`, dropped from the path key since OpenAPI has no such template form), remaining fields bind to the query string for bodyless methods (GET/HEAD/DELETE/OPTIONS) and to the request body otherwise — a nested binding removes only the leaf it names, so its siblings still bind under dotted names (`shelf.display_name`), a container's own message-typed members with nothing bound below them are refused as ever, and the §6.7 markers of a flattened container travel to whatever the flattening leaves behind, since the `$ref` that carried them is gone (issue [#218](https://github.com/trendvidia/protowire/issues/218)); `operation_id` defaults to `<Service>_<Method>`; `summary` falls back to the first sentence of `@description`. Responses are **derived, never authored** (GH #177): `200` from the return type, `default` from the §7 report model (`protowire.schema.v1.Report`, emitted into components), with the stable violation codes reachable from the request closure listed under `x-error-codes`.
- **Generator config** (`protowire.openapi.textproto`, discovered upward from the image or named with `--config`): `info`, `servers`, security-scheme definitions (an `@http(security = [...])` name with no definition is a generation error), audience tiers as FQN globs, and protoregistry coordinates for `x-since`. All of it is deployment policy the §9.4/#112 line keeps out of descriptors.
- **Audience tiers** (`--audience`, default public): elements at or below the tier are emitted; descriptors are never rewritten. Doc-pack topics anchored to an element contribute their own tier. An element whose schema closure reaches a more restricted element fails generation naming both ends — a dangling `$ref` and a silently inlined restricted definition are both worse than refusing.
- **`x-since`** is stamped from protoregistry history (first registered version containing the element) when the config carries registry coordinates, and omitted otherwise; availability is never authored.
- Output is byte-stable in both formats; `--check` reports diagnostics and writes nothing.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Any user-visible failure (missing flag, parse error, schema lookup miss, validation failure, lint violation, query compile-time rejection, infer-schema contradiction, gRPC transport error) |
| `2` | Reserved by [STABILITY.md](../../STABILITY.md) for "internal error" but not currently emitted; runtime panics manifest as the standard Go crash trace rather than a clean exit |

The exit-code contract is stable per [STABILITY.md](../../STABILITY.md). The 1-vs-2 split is intentional in the contract so future hardening can route truly-internal failures (e.g. caught panics in CI loops) to `2` without breaking scripts that grep on `1`.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PROTOREGISTRY_SERVER` | unset | Default value for `-s` when the flag isn't passed |
| `PROTOREGISTRY_NAMESPACE` | unset | Default value for `-n` when the flag isn't passed |
| `PROTOWIRE_CONFIG` | unset | Path to an engine configuration file for `pxf build`; sits between `--config` and discovery in the §9.4 precedence chain |

These let scripts treat the registry triple as ambient configuration. A combined CI step might set `PROTOREGISTRY_SERVER=registry.internal:50051` and `PROTOREGISTRY_NAMESPACE=billing` once, then every `pxf encode --schema invoice -m billing.v1.Invoice ...` call inherits the connection.

There is no `PROTOREGISTRY_SCHEMA` — schema name is intentionally per-call, since different subcommand invocations typically target different schemas within the same namespace.

## See also

- [`QUERY.md`](QUERY.md) — deep design doc for `pxf query` / `pxf infer-schema` (engine internals, `pxf_*` extension reference, `@proto` ↔ `pxf_proto` terminology)
- [Top-level README](../../README.md) — PXF format overview, directive grammar, presence semantics
- [Draft IETF spec](../../docs/draft-trendvidia-protowire-00.txt) — the wire format normatively
- [HARDENING.md](../../docs/HARDENING.md) — decoder safety contract every port enforces
- [STABILITY.md](../../STABILITY.md) — versioning, deprecation policy, exit-code stability
- [protoregistry](https://github.com/trendvidia/protoregistry) — the companion gRPC schema registry consumed by `-s`/`-n`/`--schema`
