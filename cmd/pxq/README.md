# pxq — query tool for PXF, CSV, JSON, and YAML

A `jq`-style command-line query tool whose core operates on PXF documents. CSV, JSON, and YAML inputs are transparently adapted to PXF before the query runs; output is always PXF.

```bash
pxq '.endpoints[0].path' config.pxf      # native
pxq '.endpoints[0].path' config.json     # JSON adapted to PXF
pxq '.endpoints[0].path' config.yaml     # YAML adapted to PXF
pxq '.endpoints[0].path' config.csv      # CSV adapted to PXF
```

All four invocations emit PXF on stdout. The query language is the same across formats — pipelines that consume PXF stay consistent regardless of where the data started.

## Why

Working data lives in CSV, JSON, and YAML. Adopting PXF means either rewriting every producer at once or accepting a parallel-tools era where each format keeps its own query stack. `pxq` collapses that surface: one expression language, one output format, four input formats. The tool is also a soft on-ramp — every quirk of the surrounding formats (CSV's typeless columns, JSON's int-vs-float ambiguity, YAML's `no`→`false`) becomes a small reminder that the PXF version of the same document doesn't have that problem.

## Two modes

`pxq` behaves differently depending on whether type information is available. The default is implicit — schema present → strict, no schema → loose — with `--strict` and `--loose` flags available to force the non-default.

### Strict — schema provided

For data engineers wiring a new pipeline or integration feed. The schema is asserted on every input file, and the query is type-checked against the schema at compile time: unknown field references and incompatible comparisons fail before a single row is scanned. Runtime type mismatches abort with a row/column locator (**errors-as-stop** — consistent with `protowire validate`).

```bash
pxq -p trades.proto -m trades.v1.Trade '.symbol' march.csv
```

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
| `--strict` | Force strict; errors if no schema is available, pointing at `pxq infer-schema` |
| `--loose` | Force loose even when `-p` is set; the schema still resolves `@proto` constructors and field-name completions, but type mismatches degrade to `null` instead of aborting |

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

When the input document carries `@proto` directives (draft §3.4.5), the embedded schema is honoured as if it had been passed via `-p`: anonymous `@proto { ... }` blocks bind to the next typed directive, named `@proto Name { ... }` blocks register `Name` for `@dataset` headers and `@<name>` consumers, source `@proto """ ... """` blocks compile the full `.proto` file, and descriptor `@proto b"..."` blocks consume a serialised `FileDescriptorSet` directly. A self-describing PXF document needs no `-p` flag.

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

The query expression is gojq-compatible — `pxq` embeds [`itchyny/gojq`](https://github.com/itchyny/gojq) as its core engine, so anything that runs in jq runs here, with the same operators, the same standard library, and the same runtime semantics. PXF-specific capability lives behind a reserved `@pxf.*` extension namespace, and typed object output goes through a `@proto` constructor.

### `@pxf.*` extensions

Reserved namespace for query-time operations that have no jq equivalent.

| Function | Purpose |
|---|---|
| `@pxf.directive(name)` | Returns directives matching `name` from the input document — e.g. `@pxf.directive("dataset")` to address rows of an embedded `@dataset` |
| `@pxf.fieldnames` | Declared field names per the bound schema (not just present ones); errors in loose mode |
| `@pxf.type(.x)` | Returns the proto type of a value as a string |
| `@pxf.has(.x; "field")` | Schema-aware `has` — distinguishes absent from zero-value, unlike jq's plain `has` |

Anything jq already does (paths, pipes, `select`, `map`, `reduce`, `to_entries`, string interpolation, etc.) works without prefix.

### `@proto` — typed object construction

```bash
pxq '@proto("trades.v1.Trade") { symbol: .ticker, price: .last, qty: .size }' raw.json
```

`@proto` binds the resulting object to the named descriptor, validates field names, types, oneof exclusivity, and `(pxf.required)`/`(pxf.default)` annotations, and emits a typed PXF document (prefixed with `@type trades.v1.Trade`). Schema resolution sources, in order:

1. **Bundled canonical schemas** (`pxf/*`, `sbe/*`, `envelope/v1/*`) — available by default with no flags
2. **`-p schema.proto`** — user-supplied descriptors, same flag as the main `protowire` CLI
3. **`-s server -n namespace --schema name`** — protoregistry-resident schemas, same flag set as the main CLI

Without `@proto`, object-construction expressions like `{ foo: .x, bar: .y }` produce a free-form PXF map (no schema, no validation).

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

## See also

- [PXF spec](../../docs/draft-trendvidia-protowire-00.txt) — wire format, including `@dataset` (§3.4.4), `@proto` (§3.4.5), reserved directive names (§3.4.6), and field presence semantics
- [Main protowire CLI](../protowire/) — `encode`, `decode`, `validate`, `fmt`, `lint`
- [Schema constraints](../../README.md#schema-constraints) — names that schemas bound for PXF must avoid
- [`itchyny/gojq`](https://github.com/itchyny/gojq) — embedded query engine; jq language reference applies
