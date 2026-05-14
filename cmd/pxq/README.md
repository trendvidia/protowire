# pxq — query tool for PXF, CSV, JSON, and YAML

A `jq`-style command-line query tool whose core operates on PXF documents. CSV, JSON, and YAML inputs are transparently adapted to PXF before the query runs; output is always PXF.

```bash
pxq '.endpoints[0].path' config.pxf      # native
pxq '.endpoints[0].path' config.json     # JSON adapted to PXF
pxq '.endpoints[0].path' config.yaml     # YAML adapted to PXF
pxq '.endpoints[0].path' config.csv      # CSV adapted to PXF
```

All four invocations emit PXF on stdout. The query language is the same across formats — pipelines that consume PXF stay consistent regardless of where the data started.

### A note on `@proto` and `pxf_proto`

PXF v1.0 introduced `@proto` as a top-of-document directive (draft §3.4.5) that embeds a schema in the data file. `pxq` exposes the query-language counterpart as `pxf_proto(<name>; <object>)` — a regular gojq function rather than a built-in `@<name>` formatter, because gojq reserves `@<name>` for string-encoding pipelines (`@uri`, `@base64`, …) and doesn't allow third-party `@<name>` extensions. The two are complementary halves of the same primitive:

| Layer | Form | Direction | Use |
|---|---|---|---|
| Document (parser) | `@proto Name { ... }` directive | schema **flows in** with the data | Self-describing PXF — no `-p` flag needed |
| Query (gojq) | `pxf_proto("Name"; {...})` function | schema **flows out** with the query result | Typed output binding for downstream consumers |

When both appear in the same pipeline, a document-level `@proto` registers the schema and a query-level `pxf_proto(...)` selects it by name. Throughout this doc, "the `@proto` directive" means the document form; "`pxf_proto`" means the query-language function.

## Why

Working data lives in CSV, JSON, and YAML. Adopting PXF means either rewriting every producer at once or accepting a parallel-tools era where each format keeps its own query stack. `pxq` collapses that surface: one expression language, one output format, four input formats. The tool is also a soft on-ramp — every quirk of the surrounding formats (CSV's typeless columns, JSON's int-vs-float ambiguity, YAML's `no`→`false`) becomes a small reminder that the PXF version of the same document doesn't have that problem.

## Two modes

`pxq` behaves differently depending on whether a top-level type is bound to the document — via `-m <fully-qualified>`, an `@type` directive in the input, or one of the four `@proto` shapes. The default is implicit — type bound → strict, none bound → loose — with `--strict` and `--loose` flags available to force the non-default.

### Strict — type bound

For data engineers wiring a new pipeline or integration feed. The bound message type is asserted at the document root, and direct field-chain accesses (`.foo`, `.foo.bar`, …) are type-checked against the schema at compile time. Unknown field references surface with a did-you-mean hint and the parent message's fully-qualified name; the query never executes if the field set is wrong:

```bash
$ pxq -p trades.proto -m trades.v1.Trade '.symbool' march.csv
pxq: strict-mode: unknown field "symbool" on trades.v1.Trade (did you mean "symbol"?)
```

Dynamic access patterns (array indexing, `pxf_directive(...)`, function calls, object construction) keep gojq's runtime semantics — validation tracks message field chains only.

### Loose — no schema

For one-off scripts and quick attribute fetches against PXF/JSON/YAML/CSV files sitting somewhere. Types are inferred per-value using cheap unambiguous rules; ambiguous cases stay as strings, and runtime type mismatches surface as `null` rather than aborting (**errors-as-null** — jq-compatible).

```bash
pxq '.symbol'                march.csv
pxq '.endpoints | length'    config.yaml
```

Loose mode prints a one-line stderr hint on first use of a file, pointing at `pxq infer-schema` for users who want the strict path.

### Forcing a mode

| Flag | Effect |
|---|---|
| `--strict` | Force strict; errors if no root type is bound, with the prescribed help (`-m`, `@type`, or `pxq infer-schema`) |
| `--loose` | Force loose even when a root type is bound; the validator is skipped and unknown fields degrade to `null` at runtime per jq |
| `-m <FQN>` | Bind the document root to a fully-qualified message name when the input doesn't carry `@type` |

## Schema inference

`pxq infer-schema` produces a `.proto` schema from a sample file. The output is an ordinary protobuf schema — usable by every tool in the protowire ecosystem, not just `pxq`.

```bash
pxq infer-schema -m trades.v1.Trade march.csv > trades.proto
```

The inference pass picks candidate types from the first `--sample-rows=N` rows (default `N=1000`), then validates the remainder of the file against those candidates. The full file is always walked; the sample size only controls when the candidate is fixed.

### Fail-fast on contradiction

If row `K > N` introduces a value that doesn't match the candidate type for its column, inference aborts with the line number, the offending value, the candidate type, and a recovery hint:

```
row 1247, column "age": value "23y" does not match inferred type integer
the first 1000 rows suggested integer; re-run with --sample-rows=1247 to
widen the type during inference, or edit trades.proto manually to set "age"
as string
```

The default is fail-fast. Pass `--full-scan` to walk the entire file even after a mismatch, collecting every contradiction before reporting — useful when fixing a large dataset in a single pass.

### Reusing a schema across files

Once a schema exists, it applies to any file of the same shape. Columns are matched by name; behaviour on shape drift is controlled by flags:

| Flag | Effect |
|---|---|
| (default) | strict — extra or missing columns error |
| `--allow-missing` | missing columns are treated as absent (`(pxf.default)` applies if declared) |
| `--allow-extra` | extra columns are ignored |

## Input adapters

### PXF (native)

No conversion. The document is parsed against `-p schema.proto -m message-type` when provided, or in best-effort schemaless mode otherwise. `@type`, `@dataset`, `@proto`, `@entry`, and `@<name>` directives are all visible to the query.

When the input document carries `@proto` directives (draft §3.4.5), the embedded schema is honoured as if it had been passed via `-p` — a self-describing PXF document needs no flags:

| Directive shape | Body interpretation | Schema effect |
|---|---|---|
| `@proto { ... }` | message-body source, anonymous | binds to the next typed directive without an explicit name |
| `@proto Name { ... }` | message-body source, named | registers `Name` for `@dataset` headers and `@<name>` consumers |
| `@proto """ ... """` | full `.proto` source file (triple-quoted) | compiled at load time; all declared messages registered |
| `@proto b"..."` | base64-encoded `FileDescriptorSet` | descriptors loaded directly; no compile step |

Schema resolution order: bundled canonical schemas → `@proto` directives in the input → `-p` flag → protoregistry (`-s/-n/--schema`). Later sources can shadow earlier ones, so a user-supplied `-p` overrides whatever the document carries — useful when a producer's embedded schema lags the latest revision.

All four `@proto` body shapes are honoured as schema sources — **named** (`@proto Name { body }` — sugar over a single-message `.proto`), **source** (`@proto """..."""` — full `.proto` file), **descriptor** (`@proto b"..."` — base64-encoded `FileDescriptorSet`), and **anonymous** (`@proto { body }` — binds to the next typeless `@dataset` in document order under a synthetic `_pxq_anon_N` name; access via `pxf_directive("dataset")[N].type`). Anonymous and typeless `@dataset` must appear in matched pairs; orphans in either direction surface as a clear error.

### JSON

Standard JSON parsing with explicit disambiguation for each known ambiguity:

| Source | PXF result |
|---|---|
| `1` (no decimal) | int |
| `1.0` (decimal present) | float |
| `""` | empty string — *not* null |
| `null` | null |
| `[]`, `{}` | empty list, empty map |

### YAML

YAML 1.2 parsing. Explicit `!!` tags are authoritative when present — `!!str 1` stays a string, `!!int 01` stays an int. Without tags the JSON rules above apply, and the historic YAML 1.1 implicit coercions (`no`/`yes`/`on`/`off` to bool) are **not** applied: those values remain strings.

### CSV

Without a schema:

- Each cell is classified independently: `^-?\d+$` → int, `^-?\d+\.\d+$` → float, exact `true`/`false` → bool. Everything else is string.
- Empty cells are *absent* — consistent with the `@dataset` empty-cell rule (§3.4.4 of the PXF spec). Use the literal `null` to mark a present-but-null cell when emitting CSV is not your concern.

With a schema, cells are bound positionally to typed fields using the same rules as `@dataset` row binding. Type mismatches abort with the same row/column error as `infer-schema`.

A header row is assumed by default; pass `--no-header` to bind purely positionally against the schema's field order.

## Output

Output is always PXF. There is no `--output=json` flag — back-conversion to the source format is a separate concern, and forcing PXF on the way out is what makes downstream pipeline composition trivial. When a binary protobuf is needed downstream, pipe through `protowire encode`:

```bash
pxq '...' input.csv | protowire encode -p trades.proto -m trades.v1.Trade
```

## Query language

The query expression is gojq-compatible — `pxq` embeds [`itchyny/gojq`](https://github.com/itchyny/gojq) as its core engine, so anything that runs in jq runs here, with the same operators, the same standard library, and the same runtime semantics. PXF-specific capability is registered as ordinary gojq functions under a `pxf_` prefix (gojq's `@<name>` syntax is reserved for string formatters and isn't user-extensible, so a namespacing prefix on plain identifiers is the next-best convention).

### `pxf_*` extensions

Namespaced functions for query-time operations that have no jq equivalent.

| Function | Purpose |
|---|---|
| `pxf_directive(name)` | Returns the list of directives matching `name` from the input document, in source order. For `pxf_directive("dataset")` each entry's `.rows` is an array of schema-bound row objects when a schema is in scope (otherwise an array of cell tuples). For `pxf_directive("proto")` each entry exposes `.shape`, `.typeName`, and the raw `.body` bytes (draft §3.4.5). |
| `pxf_fieldnames` | Declared field names per the bound schema (not just present ones); errors in loose mode |
| `pxf_type` | Returns the proto type of the input value as a string. Use as `.x \| pxf_type`. |
| `pxf_has(field)` | Schema-aware `has` — distinguishes absent from zero-value, unlike jq's plain `has`. Use as `.x \| pxf_has("field")`. |

Anything jq already does (paths, pipes, `select`, `map`, `reduce`, `to_entries`, string interpolation, etc.) works without prefix.

### `pxf_proto(name; obj)` — typed object construction

The query-level counterpart to the document-level `@proto` directive (see [terminology note](#a-note-on-proto-and-pxf_proto)). Binds an object-construction expression to a named descriptor:

```bash
# Two-argument form — concise when the object is short:
pxq 'pxf_proto("trades.v1.Trade"; { symbol: .ticker, price: .last, qty: .size })' raw.json

# Pipe form — easier to read when the object expression is multi-line:
pxq '{ symbol: .ticker, price: .last, qty: .size } | pxf_proto("trades.v1.Trade")' raw.json
```

Both forms are equivalent. `pxf_proto` validates field names, types, oneof exclusivity, and `(pxf.required)`/`(pxf.default)` annotations, then emits a typed PXF document prefixed with `@type trades.v1.Trade`. Same schema-resolution chain as the document parser:

1. **Bundled canonical schemas** (`pxf/*`, `sbe/*`, `envelope/v1/*`) — available by default with no flags
2. **`@proto` directives in the input** — when present, the embedded schema registers names the constructor can resolve
3. **`-p schema.proto`** — user-supplied descriptors, same flag as the main `protowire` CLI
4. **`-s server -n namespace --schema name`** — protoregistry-resident schemas, same flag set as the main CLI

Without `pxf_proto`, object-construction expressions like `{ foo: .x, bar: .y }` produce a free-form PXF map (no schema, no validation).

### Engine internals

gojq operates on an untyped `any` graph by design; strictness is layered *around* the engine, not inside it. Three layers do the work:

1. **Compile-time AST validation** against the bound schema — unknown field references (`.aeg` vs `.age`) and incompatible comparisons (`.age > "30"` where `age` is int32) error before execution.
2. **Typed boundary at input** — proto messages are converted to `any` with proto-correct Go types (int32 → int64, double → float64, enum → string by name, bytes → `[]byte`), so gojq's existing runtime type rules behave correctly.
3. **Typed boundary at output** — `@proto` re-binds the constructed result to the descriptor and validates.

Loose mode skips layer 1 entirely; the engine runs jq-identically.

## Install

```bash
go install github.com/trendvidia/protowire/cmd/pxq@latest
```

`pxq` depends on `protowire-go` for PXF parsing and on the canonical `pxf/annotations.proto` for `(pxf.required)` / `(pxf.default)` semantics during schema-bound CSV decoding.

### Quick start — self-describing PXF

A document that embeds its schema via `@proto` runs strict-mode out of the box, no flags:

```bash
$ cat trades.pxf
@proto trades.v1.Trade {
  string symbol = 1;
  double price  = 2;
  int64  qty    = 3;
}
@dataset trades.v1.Trade ( symbol, price, qty )
( "AAPL", 188.42, 100 )
( "MSFT", 415.10,  50 )
( "AAPL", 188.55,  75 )

$ pxq 'pxf_directive("dataset")[0].rows | map(select(.symbol == "AAPL")) | length' trades.pxf
2
```

(`pxf_directive(name)` returns a list — `[0]` picks the first occurrence; documents with a single `@dataset` are the common case.) Each row in `.rows` is a schema-bound object with field access by name, so `.symbol` works without a separate destructuring step.

Compare with the schema-external form, which needs `-p` and `-m`:

```bash
$ pxq -p trades.proto -m trades.v1.Trade '.symbol' march.csv
```

The two produce byte-identical PXF on stdout when fed equivalent inputs — the difference is purely where the schema travels.

## See also

- [PXF spec](../../docs/draft-trendvidia-protowire-00.txt) — wire format, including `@dataset` (§3.4.4), `@proto` (§3.4.5), reserved directive names (§3.4.6), and field presence semantics
- [Main protowire CLI](../protowire/) — `encode`, `decode`, `validate`, `fmt`, `lint`
- [Schema constraints](../../README.md#schema-constraints) — names that schemas bound for PXF must avoid
- [`itchyny/gojq`](https://github.com/itchyny/gojq) — embedded query engine; jq language reference applies
