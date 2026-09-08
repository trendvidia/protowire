# Envelope wire vectors

Cross-port wire vectors for the schema-free `pb` codecs behind each
port's `dump-envelope` (STABILITY.md promise 2). The canonical envelope
the gate has always compared port to port is built inside each dumper;
the vectors here are checked against a **golden** instead, because ports
that all write the same wrong layout agree with each other — the
argument [`../annotations/`](../annotations/) made for the extension
numbers.

`scripts/cross_envelope_check.sh` runs `dump-envelope --vector NAME` on
every port and compares the hex it prints with `NAME.expected.hex`. A
dumper that has not built a vector exits 3 with `not-implemented: NAME`;
a port whose dumper predates the mode fails the flag with its usage
error. Either counts as ok only while the port is declared in the
script's `VECTOR_NOT_IMPLEMENTED`, with the issue that tracks it.

All vectors are messages of
[`proto/envelope/v1/envelope.proto`](../../proto/envelope/v1/envelope.proto).
Each `NAME.textproto` is the value in protobuf text format; each
`NAME.expected.hex` was produced by two independent oracles that agree
byte for byte:

```
protoc -I proto --encode=envelope.v1.Envelope envelope/v1/envelope.proto \
  < testdata/envelope/NAME.textproto | xxd -p
```

and protobuf-go's `proto.MarshalOptions{Deterministic: true}` on a
`dynamicpb` message parsed from the same text.

## `zero-map-entry`

```
error { metadata { key: "" value: "" } }
```

Expected: `22 06 2a 04 0a 00 12 00` — `Envelope.error` (4) carrying
`AppError.metadata` (5) with one entry whose `key` (1) and `value` (2)
are both present and both empty. That is what protoc, protobuf-go and
C++ protobuf write for a map entry: **both fields, always, zero-valued
or not** (issue #295; decided on trendvidia/protowire-go#105).

The three layouts measured across the family on 2026-09-07 produce
three different outputs, so this one vector tells them apart:

| output | layout | ports that wrote it on 2026-09-07 |
|---|---|---|
| `22 06 2a 04 0a 00 12 00` | both fields written | C#, Swift |
| `22 04 2a 02 12 00` | value written, zero key omitted | TypeScript |
| `22 02 2a 00` | both omitted (proto3 zero-skip inside the entry) | Go (the reference), C++, Java, Rust |

Every reader in the family decodes all three to the same message, so
the divergence was lossless and invisible until measured. The canonical
envelope carries a single populated entry (`request_id → req-123`) and
cannot show it.
