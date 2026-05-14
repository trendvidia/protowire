# Protowire

**PXF** (Proto eXpressive Format) is a human-friendly text serialization format backed by protobuf schemas, plus two complementary binary encodings — `pb` (compact protobuf wire) and `sbe` (FIX Simple Binary Encoding) — and a shared response envelope. One schema (`.proto`) drives every format.

This repository is the **canonical, language-neutral specification**. It contains the proto schemas, the EBNF grammar, the railroad diagram, editor plugins, and the cross-port test fixtures. Implementations live in sibling repositories — see [Implementations](#implementations).

## Resources

- [protowire.org](https://protowire.org) — documentation website and specification overview
- [`docs/grammar.ebnf`](docs/grammar.ebnf) — PXF concrete syntax (source of truth)
- [`docs/grammar.svg`](docs/grammar.svg) — railroad diagram, generated from the EBNF
- [`docs/draft-trendvidia-protowire-00.txt`](docs/draft-trendvidia-protowire-00.txt) — IETF draft of the wire format

```
@type infra.v1.ServerConfig

hostname = "web-01.prod.example.com"
port     = 8443
enabled  = true
status   = STATUS_SERVING

# Well-known type literals
created_at = 2024-01-15T10:30:00Z
timeout    = 30s

# Nested messages use block syntax
tls {
  cert_file = "/etc/ssl/cert.pem"
  key_file  = "/etc/ssl/key.pem"
  verify    = true
}

# Repeated fields use list syntax
tags = ["production", "us-east", "frontend"]

# Maps use : for key-value pairs
labels = {
  env: "production"
  team: "platform"
  "hello world": "quoted keys supported"
}

# Repeated messages
endpoints = [
  {
    path = "/api/v1/users"
    method = "GET"
  }
  {
    path = "/health"
    method = "GET"
  }
]

# Wrapper type sugar
nullable_name = "present"
```

## Why PXF?

| Format | Problem |
|--------|---------|
| JSON | Loosely typed, no comments, verbose, ambiguous without schema |
| YAML | Indentation-fragile, type coercion surprises (`no` -> `false`), complex spec |
| Protobuf textproto | No list/map literals, repeated fields are ugly, `:` separators feel archaic |
| HCL | Own type system, designed for config not serialization, expression evaluation adds complexity |
| CSV | No types, no escaping rules, no nesting, ambiguous nulls (the [`@dataset`](#dataset--bulk-rows-the-csv-replacement) directive covers this case) |

PXF uses your existing `.proto` files as the schema. No new schema language. No ambiguity — the parser always knows every field's type.

## Implementations

PXF, `pb`, `sbe`, and the envelope are defined in this repo and implemented across ten languages. Every port is wire-compatible against the canonical `testdata/` fixtures.

| Language | Repository |
|----------|------------|
| Go | [trendvidia/protowire-go](https://github.com/trendvidia/protowire-go) |
| C++ | [trendvidia/protowire-cpp](https://github.com/trendvidia/protowire-cpp) |
| Rust | [trendvidia/protowire-rust](https://github.com/trendvidia/protowire-rust) |
| Java | [trendvidia/protowire-java](https://github.com/trendvidia/protowire-java) |
| Kotlin | [trendvidia/protowire-kotlin](https://github.com/trendvidia/protowire-kotlin) |
| TypeScript | [trendvidia/protowire-typescript](https://github.com/trendvidia/protowire-typescript) |
| Python | [trendvidia/protowire-python](https://github.com/trendvidia/protowire-python) |
| C# | [trendvidia/protowire-csharp](https://github.com/trendvidia/protowire-csharp) |
| Swift | [trendvidia/protowire-swift](https://github.com/trendvidia/protowire-swift) |
| Dart | [trendvidia/protowire-dart](https://github.com/trendvidia/protowire-dart) |

Eight ports implement the lexer/parser/encoder/decoder from scratch. Two build on a sibling rather than duplicating the codecs: `protowire-python` wraps `protowire-cpp` over a nanobind FFI, and `protowire-kotlin` is a Kotlin-extensions companion (suspending wrappers, DSL builders, `Flow` adapters) that calls `protowire-java`'s codecs natively.

Each port provides the in-language library only. Command-line operations are handled by the shared CLI below.

## CLI

The `protowire` command-line tool lives in this repo at [`cmd/pxf/`](cmd/pxf/) and is shared across every port. It's written in Go and depends on `protowire-go` internally; users of any other language install it the same way:

```bash
go install github.com/trendvidia/protowire/cmd/pxf@latest
```

Subcommands:

```bash
pxf encode    -p schema.proto -m pkg.Type input.pxf > output.pb
pxf decode    -p schema.proto -m pkg.Type input.pb  > output.pxf
pxf validate  -p schema.proto -m pkg.Type input.pxf
pxf fmt       -p schema.proto -m pkg.Type input.pxf
pxf lint      -p schema.proto                       # schema reserved-name check

protowire sbe2proto schema.xml > schema.proto    # SBE XML → .proto
protowire proto2sbe -p schema.proto > schema.xml # .proto → SBE XML
```

`pxf lint` walks every message field, oneof, and enum value in the supplied schema(s) and reports any name colliding with a PXF value keyword (`null` / `true` / `false`) — see [Schema constraints](#schema-constraints).

Registry mode (fetch schemas from a [protoregistry](https://github.com/trendvidia/protoregistry) server) is available on every subcommand via `-s <server> -n <namespace> --schema <name>`:

```bash
pxf encode   -s localhost:50051 -n myns --schema billing -m billing.v1.Invoice input.pxf
pxf validate -s localhost:50051 -n myns --schema billing -m billing.v1.Invoice input.pxf
```

PXF subcommands are also available directly inside the protoregistry CLI:

```bash
protoregistry pxf encode   [namespace] file.pxf --schema billing -m billing.v1.Invoice
protoregistry pxf decode   [namespace] file.pb  --schema billing -m billing.v1.Invoice
protoregistry pxf validate [namespace] file.pxf --schema billing -m billing.v1.Invoice
protoregistry pxf fmt      [namespace] file.pxf --schema billing -m billing.v1.Invoice
```

## Query and schema inference (`pxf query`, `pxf infer-schema`)

The `pxf` binary also includes a `jq`-style query subcommand with transparent input adapters for CSV, JSON, and YAML. Output is always PXF, so downstream pipeline stages stay format-agnostic regardless of where the data started. A schema-inference subcommand produces a `.proto` from a sample file for strict downstream binding; without a schema, the query subcommand runs in a loose mode that mirrors jq's ergonomics for quick attribute fetches. See [`cmd/pxf/QUERY.md`](cmd/pxf/QUERY.md) for full documentation.

```bash
pxf query '.endpoints[0].path' config.pxf      # native
pxf query '.endpoints[0].path' config.yaml     # YAML adapted to PXF on the way in
pxf infer-schema -m trades.v1.Trade march.csv > trades.proto
```

## Schema registry

[`trendvidia/protoregistry`](https://github.com/trendvidia/protoregistry) is the companion `.proto` catalog/registry: a multi-namespace schema store with versioning, two-phase staging, backward-compatibility enforcement, and lock-free hot-swap. It compiles `.proto` sources at runtime, deduplicates them by content hash in PostgreSQL, and serves compiled descriptors over gRPC for dynamic message creation and validation.

Every protowire CLI subcommand can pull schemas from a running registry via `-s <server> -n <namespace> --schema <name>` (see the example above), and the protoregistry CLI ships PXF subcommands directly so registry-resident schemas can encode/decode/validate/format PXF documents without re-exporting the descriptors. See the [protoregistry README](https://github.com/trendvidia/protoregistry#readme) for installation, namespace bootstrapping, and the Go client SDK.

## Directives

A PXF document admits one or more `@<name>` directives at the document root, ahead of the message body. Four directive shapes are spec-defined:

| Directive | Purpose | Spec section |
|---|---|---|
| `@type <message-type>` | Names the body's message type | [§3.4.1](docs/draft-trendvidia-protowire-00.txt) |
| `@<name> *(<prefix-id>) [{ ... }]` | Side-channel metadata interpreted by the consumer, not the body's schema layer | [§3.4.2](docs/draft-trendvidia-protowire-00.txt) |
| `@entry [<label>] [<type>] { ... }` | Bundle of heterogeneous typed sub-messages — manifest-style documents | [§3.4.3](docs/draft-trendvidia-protowire-00.txt) |
| `@dataset <type> ( cols ) ( vals )...` | Many rows of one message type — the protowire-native CSV replacement | [§3.4.4](docs/draft-trendvidia-protowire-00.txt) |

### `@type` — body's message type

```
@type infra.v1.ServerConfig
hostname = "web-01.prod.example.com"
port     = 8443
```

Names the type of the document body. At most one `@type` per document. Decoders may require it when the caller hasn't pre-bound a target type (e.g. when accepting `application/pxf` over HTTP without a `Content-Schema` parameter).

### `@<name>` — side-channel directives

```
@header chameleon.v1.LayerHeader {
  id = "base"
  encrypted = false
  generated_at = 2026-05-12T10:00:00Z
}

string_field = "body content here"
```

Open-ended namespace for application-defined metadata. The block's inner content is parsed for syntactic well-formedness but **not** decoded against the body's schema — the consumer reads it back as raw bytes via `Result.directives()` (Go) / `Result.directives()` (Java) and binds it to a message type of its own choosing.

**Use case in the wild:** [chameleon](https://github.com/trendvidia/chameleon)'s layer files carry an `@header` preamble with per-file sanity-check fields (file ID, encryption flag, generation timestamp). The resolver reads the header, validates it against its chain spec, and then decodes the body against the layer's schema. Before `@<name>` existed, chameleon had to peel the header off the byte stream with a duplicate tokenizer; now it iterates `result.directives()` and hands each `Directive.body()` back to `unmarshalFull` against `LayerHeader`.

`@<name>` accepts zero-or-more prefix identifiers before the optional `{ ... }` block. The convention shipped in v0.72 used a single identifier as the dotted type name (`@header chameleon.v1.LayerHeader { ... }`); v0.73 generalized to zero-or-more so other registrations (notably `@entry`) can carry their own positional metadata. Specific directive names are registered by other specifications or by applications.

### `@entry` — bundle / manifest documents

```
@type bundle.v1.Manifest

@entry alice users.v1.User {
  name  = "Alice"
  email = "alice@example.com"
}

@entry bob users.v1.User {
  name  = "Bob"
  email = "bob@example.com"
}

@entry orders.v1.Order {
  id    = "o-99"
  total = 42.50
}

@entry {
  note = "manifest produced by ops-export 2026-05-11"
}
```

Four permitted shapes, distinguished by the prefix identifiers:

| Shape | Example | Meaning |
|---|---|---|
| anonymous, typeless | `@entry { ... }` | Free-form sub-document; consumer decides the schema |
| labeled, typeless | `@entry name { ... }` | Caller-provided label only |
| typed only (dotted ident) | `@entry some.pkg.Type { ... }` | Dotted prefix is the message type |
| labeled and typed | `@entry name some.pkg.Type { ... }` | Both — manifest entry with a stable name |

The single-prefix form is disambiguated by the presence of a `.` — dotted ⇒ type, bare ⇒ label.

**Use cases:** export bundles where each entry is a different proto type, manifest files describing several typed artifacts, anything that would otherwise be modeled as `repeated google.protobuf.Any` plus a name string.

### `@dataset` — bulk rows (the CSV replacement)

```
@dataset trades.v1.Trade (symbol, price, qty, side, ts)
("AAPL", 192.34, 100, BUY,  2026-05-11T10:00:00Z)
("MSFT", 410.10, 50,  SELL, 2026-05-11T10:00:01Z)
("GOOG", 142.00, 25,  BUY,  2026-05-11T10:00:02Z)
```

A header `@dataset <type> ( cols )` names the row message type and a list of top-level field names; each subsequent `( … )` is a row whose values bind positionally to those columns. The protowire-native CSV replacement: same per-row compactness CSV gives you, but with real types, escape rules, comments, and nullability that CSV famously doesn't.

**Three-state cells** map onto the existing `(pxf.required)` / `(pxf.default)` annotations with zero new spec surface:

```
@dataset users.v1.User (name, email, role)
("alice", "alice@example.com", ADMIN)   # all three set
("bob",   null,                 GUEST)   # email explicitly null — clears the wrapper
("eve",   ,                     GUEST)   # email cell empty — field absent, (pxf.default) applies
```

| Cell | Meaning |
|---|---|
| any value literal | field set to that value |
| `null` | field present-but-null — clears wrappers, oneofs, and `optional` fields per §3.9 |
| empty (between two commas, or at row start/end) | field absent — `(pxf.default)` is applied if declared; `(pxf.required)` errors |

**v1 restrictions** (the spec keeps these tight so row tokenization is trivially comma-splittable):

- Cells are scalar-shaped — no `[...]` list literals or `{...}` block values inside a cell.
- Column entries are unqualified top-level field names — no dotted paths like `addr.city`.
- Row arity MUST equal column count — no trailing-empty shorthand.
- A document containing `@dataset` MUST NOT also carry `@type` or top-level field entries. The `@dataset` header is itself the document's type declaration.

A document MAY contain multiple `@dataset` directives (same or different types). Order is preserved.

**Use cases:** event logs, market-data ticks, audit trails, batch exports — anywhere CSV would otherwise show up. The first-port implementation in [`protowire-go`](https://github.com/trendvidia/protowire-go) ships both a materializing path (`Result.Datasets()`, returns the full row list) and a streaming path (`pxf.DatasetReader` over an `io.Reader`, working-set memory bounded by the largest single row regardless of total row count). The streaming contract is in the spec at [§3.4.4 "Streaming consumption"](docs/draft-trendvidia-protowire-00.txt).

**Per-row binding into proto messages** is exposed via `DatasetReader.Scan(msg)` / `pxf.BindRow(msg, cols, row)` in Go and the analogous `DatasetReader.scan(builder)` / `BindRow.bindRow(...)` in Java. The cell-state semantics above are honored automatically — empty cells leave fields absent, `null` cells clear wrappers, value cells set fields.

### Schema constraints

A protobuf schema bound for PXF use MUST NOT declare a message field, oneof, or enum value whose name is case-sensitively equal to `null`, `true`, or `false` (draft §3.13). Those names lex as PXF value keywords, so the declared element would be unreachable from PXF surface syntax — `field = null` always resolves to the null-literal branch, and an enum value named `null` could never be selected by name. The check is case-sensitive: `NULL`, `True`, `FALSE` lex as ordinary identifiers and are accepted.

Tools enforce this at descriptor-bind time:

- Run [`pxf lint`](#cli) against a `.proto` file or registry-resident schema to surface violations before binding.
- The Go / Java decoders run the check by default at the top of every `Unmarshal`-style call and reject non-conformant schemas with a clear error. Opt-out is available via `UnmarshalOptions.SkipValidate` for callers who've already pre-validated.

Schemas that violate the constraint were never round-trippable through PXF; rejecting them at bind time surfaces a pre-existing latent bug rather than introducing one.

## Syntax

The full concrete syntax is in [`docs/grammar.ebnf`](docs/grammar.ebnf), with a railroad diagram at [`docs/grammar.svg`](docs/grammar.svg).

### Operators

| Context | Operator | Meaning |
|---------|----------|---------|
| `key = value` | `=` | Field assignment (message context) |
| `name { }` | (none) | Nested message block |
| `key: value` | `:` | Map entry (map context) |

### Comments

```
# hash comment
// double-slash comment
/* block comment */
```

### Scalars

```
name    = "string"           # always quoted
port    = 8080               # integer
weight  = 0.85               # float
enabled = true               # bool (true or false)
status  = STATUS_SERVING     # enum (by name)
raw     = b"SGVsbG8="        # bytes (base64)
```

### String escapes

Inside double-quoted strings:

| Escape | Meaning |
|---|---|
| `\"` `\\` `\'` `\?` | Literal char |
| `\a` `\b` `\f` `\n` `\r` `\t` `\v` | Control bytes |
| `\xHH` | One byte (2 hex digits) |
| `\nnn` | One byte (3 octal digits, value ≤ `0xFF`) |
| `\uHHHH` | Unicode codepoint (4 hex digits, BMP) |
| `\UHHHHHHHH` | Unicode codepoint (8 hex digits, full range; surrogate halves rejected) |

Multi-byte UTF-8 may also appear literally between quotes — `"café 日本 😀"` is valid.

### Well-known type literals

```
created_at = 2024-01-15T10:30:00Z   # google.protobuf.Timestamp (RFC 3339)
timeout    = 1h30m45s                # google.protobuf.Duration (Go-style)
```

### Null

Any field can be explicitly set to null:

```
email = null       # explicitly null — different from absent
```

Null is not allowed inside repeated fields or map values.

### Wrapper type sugar

```
# Instead of: nullable_name { value = "hello" }
nullable_name = "hello"              # google.protobuf.StringValue
nullable_port = 8080                 # google.protobuf.Int32Value
```

### google.protobuf.Any

```
payload {
  @type = "mypackage.v1.ErrorDetail"
  code = 42
  reason = "not found"
}
```

Requires a type resolver at decode/encode time to map type URLs to message descriptors.

### Multi-line strings

Triple-quoted strings preserve raw content verbatim — escapes are NOT processed inside `"""..."""`:

```
description = """
  This is a multi-line string.
  Leading indent (based on closing """) is stripped.
  """
```

### Repeated fields

```
# Scalars — commas optional, newlines accepted as separators
tags = ["production", "us-east", "frontend"]
tags = [
  "production"
  "us-east"
  "frontend"
]

# Messages — list of blocks
endpoints = [
  {
    path = "/api"
    method = "GET"
  }
  {
    path = "/health"
    method = "GET"
  }
]
```

### Maps

```
# string -> string
labels = {
  env: "production"
  "content-type": "application/json"
}

# string -> message
servers = {
  primary: {
    hostname = "primary.example.com"
    port = 8080
  }
}

# int -> string
error_codes = {
  404: "Not Found"
  500: "Internal Error"
}
```

### Oneof

Oneof fields use regular block syntax — no special keyword needed. The proto schema enforces exclusivity. Setting two fields from the same oneof group is a decode error.

```proto
message Event {
  string event_id = 1;
  oneof payload {
    UserEvent user = 2;
    SystemEvent system = 3;
  }
}

message UserEvent {
  string user_id = 1;
  oneof action {
    LoginAction login = 2;
    LogoutAction logout = 3;
  }
}

message LoginAction {
  string ip = 1;
  bool mfa = 2;
}
```

```
event_id = "evt-456"

# Just set the oneof field you want — nesting works at any depth
user {
  user_id = "u-123"
  login {
    ip = "192.168.1.1"
    mfa = true
  }
}
```

Each `name { }` block enters a message scope. Oneof constraints are checked independently at each level — `user` vs `system` at the Event level, `login` vs `logout` at the UserEvent level.

## Field presence: set, null, absent

PXF distinguishes three field states that are commonly conflated in other serialization formats:

| State | PXF syntax | Meaning |
|-------|-----------|---------|
| **Set** | `name = "Alice"` | Field has a concrete value |
| **Null** | `name = null` | Field is explicitly null |
| **Absent** | *(field not mentioned)* | Field was not included in the document |

### Why it matters

Consider a PATCH-style update. You need to distinguish between:

- "Set `email` to `alice@example.com`" → `email = "alice@example.com"`
- "Clear `email`" → `email = null`
- "Don't touch `email`" → *(don't mention it)*

With proto3's default semantics, absent and null are indistinguishable. PXF surfaces all three states at the text layer; each implementation exposes them through a "full" decode mode that returns presence metadata alongside the message.

### Required fields and default values

PXF defines two custom proto annotations for field validation:

```proto
import "pxf/annotations.proto";

message Config {
  string name     = 1 [(pxf.required) = true];    // must appear (null counts as present)
  string role     = 2 [(pxf.default) = "viewer"];  // applied when absent, not when null
  int32 priority  = 3 [(pxf.default) = "5"];
  bool enabled    = 4 [(pxf.default) = "true"];
}
```

Validation rules:

| State | Required field | Field with default | Plain field |
|-------|---------------|-------------------|-------------|
| **Set** | OK | Use provided value | OK |
| **Null** | OK (counts as present) | Do NOT apply default | OK |
| **Absent** | Error | Apply default | OK (zero value) |

Annotation field numbers are reserved in [`proto/pxf/annotations.proto`](proto/pxf/annotations.proto).

### Null survival across protobuf binary

Protobuf binary only has two states per field: present or not present. Both "null" and "absent" map to "not present" in binary. To preserve nulls across a protobuf binary round-trip, add a field named `_null` of type `google.protobuf.FieldMask` to your message:

```proto
import "google/protobuf/field_mask.proto";

message Config {
  string name  = 1;
  string email = 2;
  string role  = 3;

  google.protobuf.FieldMask _null = 15;
}
```

PXF implementations recognize `_null` by both name and type — it must be named `_null` AND be a `google.protobuf.FieldMask`. Regular FieldMask fields (e.g., `update_mask`) are not affected. When a field is decoded as null, its name is added to the `_null` mask; on re-encode, those fields are emitted as `field = null`.

The FieldMask is optional. Without it, full-decode results still track nulls in memory, but the distinction is lost when serializing to protobuf binary.

### Proto3 zero-value round-trips

For plain decode (without the full presence-tracking variant), PXF follows standard proto3 semantics. Non-optional scalar fields set to their zero value (`0`, `false`, `""`) are indistinguishable from unset fields, so they are omitted on re-marshal. Use `optional`, wrapper types, or the `_null` FieldMask convention above when you need explicit presence.

## SBE binary encoding

[FIX SBE](https://www.fixtrading.org/standards/sbe/) (Simple Binary Encoding) for latency-sensitive workloads. The same `.proto` schema drives both protobuf and SBE wire formats — add SBE annotations and choose the encoder at runtime.

### Schema annotations

```proto
import "sbe/annotations.proto";

option (sbe.schema_id) = 1;
option (sbe.version)   = 0;

message NewOrderSingle {
  option (sbe.template_id) = 1;

  uint64 order_id = 1;
  string symbol   = 2 [(sbe.length) = 8];     // fixed-size char[8]
  int64  price    = 3;
  uint32 quantity = 4;
  uint32 side     = 5 [(sbe.encoding) = "uint8"]; // narrow to 1 byte

  message Fill {
    int64  fill_price = 1;
    uint32 fill_qty   = 2;
    uint64 fill_id    = 3;
  }
  repeated Fill fills = 6;  // SBE repeating group
}
```

| Proto concept | SBE mapping |
|---|---|
| Scalar fields | Fixed-width at computed offsets |
| `string` / `bytes` with `(sbe.length)` | Fixed-size char array (truncated if longer) |
| `(sbe.encoding)` override | Narrowed type (e.g. uint32 → uint8) |
| Nested message (non-repeated) | SBE composite (inlined at fixed offset) |
| `repeated` message | SBE repeating group |

Annotation field numbers are reserved in [`proto/sbe/annotations.proto`](proto/sbe/annotations.proto). Implementations are wire-compatible with any other SBE codec using the same schema (e.g. Real Logic SBE in C++ / Java).

### SBE XML schema interoperability

Implementations are expected to provide tools that round-trip between FIX SBE XML and `.proto` files with SBE annotations:

| SBE XML | Proto |
|---------|-------|
| `<messageSchema id="1">` | `option (sbe.schema_id) = 1;` |
| `<message name="Order" id="1">` | `option (sbe.template_id) = 1;` |
| `<type primitiveType="char" length="8"/>` | `string [(sbe.length) = 8]` |
| `<field type="uint8"/>` | `uint32 [(sbe.encoding) = "uint8"]` |
| `<composite name="Inner">` | `message Inner { }` (no template_id) |
| `<group name="fills">` | `repeated Fill fills = N;` |
| `<enum name="Side">` | `enum Side { SIDE_BUY = 0; }` |

## Cross-port benchmarks

Every implementation runs the same canonical fixtures (`testdata/bench-test.{proto,pxf}` for PXF, `testdata/sbe-bench.proto` for SBE) and decodes into a descriptor-driven dynamic message — no codegen — so the comparison reflects codec dispatch, not generated-message ergonomics.

|            | PXF unmarshal             | PXF marshal | SBE unmarshal              | SBE marshal |
|------------|---------------------------|-------------|----------------------------|-------------|
| C++        | **3.83 µs** (**162.4 MiB/s**) | **3.16 µs** | 390 ns (229.5 MiB/s)       | **236 ns**  |
| Go         | 5.83 µs (106.6 MiB/s)     | 3.47 µs     | 1.06 µs (84.6 MiB/s)       | 375 ns      |
| Rust       | 6.06 µs (102.7 MiB/s)     | 5.25 µs     | 584 ns (153.4 MiB/s)       | 438 ns      |
| Java       | 9.48 µs (65.6 MiB/s)      | **3.25 µs** | 894 ns (100.2 MiB/s)       | 265 ns      |
| TypeScript | 11.90 µs (52.3 MiB/s)     | 4.84 µs     | 1.59 µs (56.5 MiB/s)       | 939 ns      |
| C#         | 16.36 µs (38.0 MiB/s)     | 4.40 µs     | **342 ns** (**261.9 MiB/s**) | 279 ns    |
| Python     | —                         | —           | 2.44 µs (36.8 MiB/s)       | 1.36 µs     |
| Swift¹     | 277.90 µs (2.2 MiB/s)     | 39.18 µs    | —                          | —           |

Apple M1, 3-second measurement window per op. PXF uses a 624-byte `bench.v1.Config` (mixed scalars, repeated lists, maps, Timestamp, Duration); SBE uses a 94-byte `bench.v1.Order` (10 scalars + a 2-entry repeating group). C++ leads PXF in both directions; C# leads SBE unmarshal/throughput while C++ holds the SBE marshal lead. The Kotlin companion delegates to `protowire-java` and inherits Java's numbers (with one extra dispatch hop when called from a coroutine on `Dispatchers.IO`); Dart and the Java/Android (protobuf-javalite) tier do not yet ship bench harnesses.

¹ Swift PXF lands roughly an order of magnitude behind the other ports on this fixture (release build, descriptor-driven path — same harness as every other row).

To reproduce, clone the language ports next to this repo and run:

```bash
bash scripts/cross_pxf_bench.sh        # all ports, PXF
bash scripts/cross_sbe_bench.sh        # all ports, SBE
bash scripts/cross_envelope_check.sh   # cross-port byte-equality of the response envelope
```

Set `MEASURE_SECONDS=N` to control the per-op window. Set `SKIP_PORTS=cpp,java,…` to omit any port whose toolchain is missing locally.

## Editor support

Both extensions ship the same TextMate grammar plus inline parse-error
squiggles powered by the language's own parser (the JetBrains plugin
embeds `protowire-java`'s parser, the VS Code extension embeds
`protowire-typescript`'s). Neither is published to a marketplace yet, so
install locally:

- **VS Code** — implementation in [`editors/vscode/`](editors/vscode/).
  Install the pre-built package directly:

  ```bash
  code --install-extension editors/vscode/dist/pxf-1.0.0.vsix
  ```

  Or use the **Extensions → Install from VSIX…** menu. To rebuild from
  source or set up a development symlink, see
  [`editors/vscode/README.md`](editors/vscode/README.md).
- **JetBrains** (IntelliJ, GoLand, PyCharm, …) — implementation in
  [`editors/jetbrains/`](editors/jetbrains/). Install the prebuilt
  plugin via **Settings → Plugins → ⚙ → Install Plugin from Disk…** and
  pick `editors/jetbrains/plugin/dist/pxf-jetbrains-1.0.0.zip`. The
  plugin auto-registers the bundled TextMate grammar (no manual "Add
  Bundle" step), adds a **New → PXF File** entry, and surfaces parse
  errors inline. The raw [`pxf.tmbundle/`](editors/jetbrains/pxf.tmbundle/)
  directory is also still available for TextMate / Sublime Text users —
  see [`editors/jetbrains/README.md`](editors/jetbrains/README.md).

Schema-aware validation (field/type checking against a descriptor set) is
intentionally not in either extension yet — it's planned for a follow-up
once descriptor-set discovery is designed.

## Repository layout

```
protowire/
├── LICENSE                                    # MIT
├── README.md                                  # this file
├── CHANGELOG.md                               # spec-level changes (every port mirrors)
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md                            # workflow + Steward rollout note
├── GOVERNANCE.md                              # human-readable preamble for governance.pxf
├── governance.pxf                             # machine-readable constitution
├── ROADMAP.md                                 # milestones M0..M9
├── STABILITY.md                               # SemVer policy, wire-equiv guarantees
├── SECURITY.md                                # disclosure policy + 30-day cross-port embargo
├── go.mod / go.sum                            # canonical CLI module
│
├── cmd/
│   ├── pxf/                                   # canonical CLI (Go; encode/decode/validate/fmt/lint/query/infer-schema)
│   └── protoc-gen-pxf-java-meta/              # codegen plugin for protowire-java's SBE codec
│
├── proto/
│   ├── pxf/annotations.proto                  # (pxf.required), (pxf.default) field options
│   ├── pxf/bignum.proto                       # arbitrary-precision number wrapper types
│   ├── sbe/annotations.proto                  # (sbe.schema_id), (sbe.template_id), (sbe.length), (sbe.encoding)
│   └── envelope/v1/envelope.proto             # canonical response envelope
│
├── docs/
│   ├── grammar.ebnf                           # PXF concrete syntax (source of truth)
│   ├── grammar.svg                            # railroad diagram, generated from grammar.ebnf
│   ├── HARDENING.md                           # adversarial-input invariants every port must honour
│   └── draft-trendvidia-protowire-00.txt      # IETF draft of the wire format
│
├── editors/
│   ├── vscode/                                # VS Code extension; prebuilt .vsix in dist/
│   └── jetbrains/
│       ├── pxf.tmbundle/                      # raw TextMate bundle (also TextMate / Sublime)
│       └── plugin/                            # IntelliJ Platform plugin; prebuilt .zip in dist/
│
├── scripts/
│   ├── cross_pxf_bench.sh                     # cross-port PXF benchmark orchestrator
│   ├── cross_sbe_bench.sh                     # cross-port SBE benchmark orchestrator
│   ├── cross_envelope_check.sh                # cross-port envelope byte-equality check
│   ├── cross_security_check.sh                # adversarial-corpus runner (HARDENING gate)
│   ├── gen_railroad.py                        # regenerates docs/grammar.svg from docs/grammar.ebnf
│   ├── sync_jetbrains_grammar.py              # mirrors PXF grammar into the JetBrains tmbundle
│   ├── refresh_jetbrains_parser_jar.sh        # vendors protowire-java :pxf into the JetBrains plugin
│   └── refresh_vscode_parser_pkg.sh           # vendors protowire-typescript into the VS Code extension
│
├── testdata/                                  # canonical fixtures shared by every port
│   ├── *.proto, *.pxf, *.binpb                # encode/decode round-trip fixtures
│   └── adversarial/                           # hardening conformance corpus (HARDENING.md)
│       ├── adversarial.proto                  # schema referenced by every corpus entry
│       ├── MANIFEST.jsonl                     # one line per fixture: format, schema, expect, reason
│       ├── pxf/, pb/, sbe/                    # adversarial inputs
│       └── generate.py                        # reproducibility helper for parameterised fixtures
│
└── .github/
    ├── workflows/                             # CI: go vet/build/test + CodeQL SAST
    ├── ISSUE_TEMPLATE/                        # bug / feature / config
    └── PULL_REQUEST_TEMPLATE.md
```

## Versioning

- Annotation field numbers in `proto/pxf/annotations.proto` and `proto/sbe/annotations.proto` are part of the wire contract. Adding new options is fine; renumbering or removing existing ones breaks every port.
- The envelope schema in `proto/envelope/v1/envelope.proto` is similarly load-bearing. Bump the version path (`v1` → `v2`) for incompatible changes.
- The PXF grammar in `docs/grammar.ebnf` is the source of truth for what any new port must accept.
