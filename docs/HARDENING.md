# Decoder Hardening

This document is the normative specification of how every `protowire-*` port must behave when given **untrusted input**. It complements [`STABILITY.md`](../STABILITY.md): where `STABILITY.md` defines what bytes a conforming decoder must *accept*, this document defines what bytes a conforming decoder must *safely reject*, and how it must reject them.

It is one of the load-bearing documents of the project alongside [`docs/grammar.ebnf`](grammar.ebnf), the canonical `.proto` files under [`proto/`](../proto/), and `STABILITY.md`. Effective at the next minor release, conformance with this document is a release-gating CI requirement (see [§ Conformance corpus](#conformance-corpus)).

## Threat model

A `protowire-*` decoder operates on bytes (PB binary, SBE binary, Envelope binary) or text (PXF) supplied by an **attacker**. The attacker:

- chooses every byte of the input,
- chooses the byte length of the input up to the configured maximum,
- may submit many inputs in series, in parallel, or concurrently from many sources,
- cannot influence the schema (`.proto`, SBE template) supplied by the calling application — schemas are part of the trusted compute base,
- cannot influence configured limits (see below) — those are set by the calling application.

A conforming decoder, given any such input, must terminate in time and memory bounded by **the input length and the configured limits only**. It must not crash, abort, or unwind the host process; it must not allocate beyond the configured memory budget; it must not return malformed `string` fields that violate proto3 invariants.

Schemas (`FileDescriptorSet`, SBE XML) read at runtime are **not** in scope of this document by default. A port that exposes `unmarshal(bytes, descriptor)` against an attacker-controlled descriptor must apply the same protections to the descriptor parser; see [§ Schema input](#schema-input).

## Mandatory limits

Every decoder must enforce the following hard limits. All limits except `MaxVarintBytes` are configurable per call; the values below are the **default** that ports must ship with and are the values used by [§ Conformance corpus](#conformance-corpus).

| Limit | Default | Applies to | Rationale |
|---|---|---|---|
| `MaxNestingDepth` | **100** | PXF `{` / `[` nesting; PB submessage / group / map-entry nesting; Envelope nested fields | Bounds native call-stack growth. Matches the default in `google.golang.org/protobuf` and `prost`. |
| `MaxMessageSize` | **64 MiB** | Total input length to a single decode call | Bounds peak memory. Matches Google.Protobuf's `CodedInputStream` default. |
| `MaxNumericLiteralDigits` | **4096** | Digit count of any single PXF integer / decimal / float literal before parsing into `BigInt` / `BigFloat` / `Decimal` | Bounds quadratic big-number parsers. 4096 decimal digits is ~13.6 Kbits, far above any legitimate use. |
| `MaxBytesLiteralLength` | **`MaxMessageSize`** | The decoded byte length of any single PXF `b"…"` or base64 literal | Already bounded by `MaxMessageSize` transitively, but stated explicitly so per-token streaming decoders can short-circuit. |
| `MaxVarintBytes` | **10** | Every varint read | The maximum length of a 64-bit varint. Non-configurable. |
| `MaxRepeatedCount` | **`MaxMessageSize`** | Number of elements in any repeated field, map, SBE group | Bounded transitively because each element costs at least one byte on the wire, but ports must reject *before* allocating writer state for `count` elements (see [§ SBE](#sbe-validation)). |

Limits compose multiplicatively: a decoder may not allocate `MaxMessageSize × MaxNestingDepth × MaxRepeatedCount` worst-case. Each individual limit is a hard cap.

A decoder presented with input that requires exceeding any limit **must** return an error before allocating memory proportional to the violating quantity. It must not abort, panic, `fatalError`, `precondition`, throw an uncatchable exception, or unwind into a state from which the caller cannot recover.

## API contract

The following are **non-negotiable** on the decoder hot path:

1. **No process-fatal failures on attacker input.** No `panic` (Rust/Go-of-the-runtime kind), no `fatalError` / `precondition` / `assertionFailure` (Swift), no `unreachable!` (Rust), no `StackOverflowError` propagation (Java), no `RangeError` from V8 stack overflow (TypeScript), no `RecursionError` (Python), no `abort()` / SIGSEGV (C / C++). Every reachable error path returns an error value to the caller.
2. **No force-unwraps on attacker-derived optionals.** Swift `!`, Rust `unwrap()` / `expect()`, C# `!.` against attacker data is a bug.
3. **No trapping integer conversions on attacker-supplied lengths.** Swift `Int(x)`, Rust `as usize`, C `static_cast<int>(x)`, Java implicit `int` narrowing, Go `int(x)` of a `uint64` — when `x` is a wire-supplied length, the conversion must be fallible. Use `checked_add` / `Int(exactly:)` / `Math.toIntExact` / explicit bounds tests.
4. **Length math is checked.** Every `offset + length`, `count × element_size`, `pos + header + count × block_length` is computed in 64-bit unsigned arithmetic and checked against the buffer length **before** narrowing to a native `int` / `usize` and **before** allocating.
5. **Decoder errors do not leak heterogeneous exception types.** A decoder that documents `ValueError` / `Err(DecodeError)` / `IOException` may not let `IndexError` / `panic` / `NullPointerException` escape on a malformed input.

## Recursion

PXF parsers are recursive descent over `{ … }` blocks and `[ … ]` lists. PB decoders recurse on length-delimited submessages, groups, and map entries. Both must:

- track a depth counter, incremented on every recursive descent,
- reject with an error when the counter exceeds `MaxNestingDepth`,
- thread the counter through inner decoders constructed mid-stream — in particular, when a nested protobuf submessage is decoded by handing its bytes to a fresh `CodedInputStream` / `Reader`, the depth counter must be passed in, not reset to zero.

Iterative skip routines (e.g. `skipBraced` in the C++ and Java fast decoders) already use an explicit counter; the live decode path must use the same model.

## UTF-8

A proto3 `string` field is a sequence of valid UTF-8 bytes. A conforming decoder:

- **MUST** validate UTF-8 strictly when populating any `string`-typed field, regardless of the source encoding (PB length-delimited bytes, SBE `char[]` array, SBE varData, PXF `"…"` literal).
- **MUST NOT** use lossy decoders that substitute U+FFFD silently. This rules out `Encoding.UTF8.GetString` (C# default), `TextDecoder("utf-8")` without `{fatal: true}` (TS), `String(decoding: bytes, as: UTF8.self)` (Swift), `String.fromCharCodes(bytes)` (Dart), `new String(bytes, UTF_8)` (Java default), `utf8.decode(bytes, allowMalformed: true)` (Dart), or any equivalent.
- **MUST** reject PXF `\xHH` byte escapes and octal `\NNN` escapes that produce invalid UTF-8 when the surrounding literal is being decoded into a `string` field. Invalid byte sequences inside PXF `b"…"` (`bytes` field) are permitted; invalid byte sequences anywhere else are not.
- **MUST** reject `\u` / `\U` / `\xHH` escapes that encode Unicode surrogates (U+D800–U+DFFF) or values above U+10FFFF.

`bytes` fields and SBE non-string varData impose no UTF-8 constraint.

## SBE validation

The SBE wire format is fixed-layout. Every conforming decoder must, at view construction (or before reading any field):

1. Validate `data.length ≥ HEADER_SIZE + wire_block_length`.
2. Validate `wire_block_length ≥ template_block_length`. A wire block strictly smaller than the schema's block length means at least one field's `offset + size` exceeds the wire block; reject. A wire block larger than the schema's block length is **valid** (forward compatibility — newer schema with extra trailing fields).
3. For each repeating group, validate **before iterating**:
   - `pos + GROUP_HEADER_SIZE ≤ data.length`,
   - `count × wire_block_length` does not overflow 64-bit,
   - `pos + GROUP_HEADER_SIZE + count × wire_block_length ≤ data.length`.
4. Reject `wire_block_length == 0` when `count > 0`. (Otherwise an attacker can ask the decoder to allocate `count` writer entries for arbitrary `count` while consuming zero further bytes.)
5. For composite (sub-block) field reads, validate `composite_offset + composite_size ≤ enclosing_block.length`.

Group walk in `view()` (skipping past unrequested groups) is subject to the same arithmetic — `pos += GROUP_HEADER_SIZE + count × block_length` must be 64-bit-checked against `data.length` before the next iteration.

## Schema input

A port that accepts a `FileDescriptorSet` or SBE XML schema *at runtime* (i.e. not embedded at codegen) and exposes that to attacker bytes must apply the limits above to the schema parser as well. Specifically:

- XML schema parsers **MUST** disable DTDs and external entities. Concretely:
  - Java: `factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)` and disable both `external-general-entities` and `external-parameter-entities`.
  - .NET: `XmlReaderSettings.DtdProcessing = DtdProcessing.Prohibit`.
  - Python: use `defusedxml`.
  - Go: `xml.Decoder` with `Strict = true` and no `Entity` map.
  - C++ / TypeScript: use parsers without entity expansion or apply length caps.
- `FileDescriptorSet` parsers must enforce `MaxMessageSize` and `MaxNestingDepth`.

If a port restricts schema input to *application-trusted* bytes (build-time embedding only), this section is informational; document the restriction prominently.

## Map keys and dynamic property assignment

Languages where dynamic property assignment (`obj[key] = value`) walks a prototype chain (notably JavaScript / TypeScript) **MUST NOT** use plain object literals (`{}`) as the target for attacker-keyed maps. Use `Object.create(null)` or `Map`, or explicitly reject keys equal to `__proto__`, `constructor`, `prototype`. The same applies to any other language with prototype-mutation semantics for reserved string keys.

## Conformance corpus

The repository ships an adversarial test corpus under `testdata/adversarial/`, seeded as part of [ROADMAP M8](../ROADMAP.md#m8--hardening-conformance-corpus-target-0750). Every port's `check-decode` binary must produce the manifest-declared verdict (accept / reject) for each corpus input within the wall-clock budget defined in [`scripts/cross_security_check.sh`](../scripts/cross_security_check.sh). The corpus covers, at minimum:

| Category | What it tests |
|---|---|
| `pxf/deep-nesting/{N}.pxf` for N ∈ {100, 200, 1000} | Nesting depth limit |
| `pxf/long-numeric.pxf` | Numeric literal digit cap |
| `pxf/invalid-utf8-string.pxf` | UTF-8 enforcement on `string` |
| `pxf/lone-surrogate.pxf` | Surrogate rejection in `\u` |
| `pxf/giant-base64.pxf` | Bytes literal length cap |
| `pb/deep-submessage.binpb` | PB submessage depth limit |
| `pb/length-prefix-overflow.binpb` | Length-prefix integer overflow |
| `pb/recursion-via-fresh-stream.binpb` | Depth counter survives nested-stream construction |
| `sbe/group-count-overflow.sbe` | `count × block_length` overflow |
| `sbe/group-zero-blocklength-nonzero-count.sbe` | Pre-iteration allocation |
| `sbe/short-block-length.sbe` | Wire block length < template |
| `envelope/skip-unknown-runaway.binpb` | Skip-field length validation |

CI (`scripts/cross_security_check.sh`) runs each port's binary against the corpus with a 5-second wall-clock budget; any process that crashes, hangs past the budget, or returns success on a corpus input fails the build. A real RSS cap is left to the host runner — `ulimit -v` is virtual-address-space, not RSS, and trips Go-style runtimes that mmap multi-GiB heap arenas at startup; cgroup-based RSS gating is a planned follow-up. In practice the wall-clock budget catches memory blow-ups: anything that allocates enough to matter on adversarial input also runs long enough to trip `timeout`.

## Reporting a hardening regression

A decoder that crashes, hangs, or produces invalid `string` output on an adversarial input is a **hardening regression** and should be filed as a security issue against [`trendvidia/protowire`](https://github.com/trendvidia/protowire), not against the individual port. Cross-port issues are triaged here. Coordinated disclosure: a 30-day embargo applies to issues that affect more than one port.

## Versioning of this document

`HARDENING.md` is versioned with the project. Edits that *strengthen* requirements (lower a limit, add a mandatory check) are accepted at any minor version, with a one-minor-version notice in `CHANGELOG.md` so port maintainers can adjust. Edits that *weaken* requirements (raise a limit beyond the values above, remove a mandatory check) require a major bump.
