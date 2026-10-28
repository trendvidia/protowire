# Null-binding fixtures

Cross-port conformance fixtures for where a `null` literal may bind
([issue #339](https://github.com/trendvidia/protowire/issues/339); IETF
draft `-01` § Booleans, Null, and Identifier Values). A `null`:

- on a singular field with explicit presence (a message field, wrappers
  included, a proto3 `optional` scalar, a oneof member) MUST leave it
  unset, and it is present-but-null;
- on a proto3 scalar declared without `optional` MUST be rejected: the
  field has no unset state, so `null` would read back as its zero value;
- on a repeated field, or as a list element, MUST be rejected.

The owner settled this on #339 (2026-10-27, option 3).

Every fixture binds against [`nulls.proto`](nulls.proto) (package
`nulls.v1`, message `Holder`). Each file's leading comment states the
expected behavior, and `cmd/pxf/nulls_test.go` runs them through the
reference implementation: `Parse` (no schema, which accepts them all), a
decoder bound to `nulls.v1.Holder`, and `pxf validate`. The same rule in a
`@dataset` cell is pinned by
[`../dataset/null-on-scalar-cell.pxf`](../dataset/null-on-scalar-cell.pxf).

| File | Verdict | Asserts |
|---|---|---|
| `err-scalar.pxf` | reject | `n = null` on a plain `int32`. |
| `err-repeated.pxf` | reject | `l = null` on a `repeated int32`. |
| `err-list-element.pxf` | reject | `l = [1, null]`: a list element is not nullable. |
| `message.pxf` | accept | `msg = null` leaves `msg` unset. |
| `wrapper.pxf` | accept | `w = null` leaves `w` unset. |
| `optional-scalar.pxf` | accept | `o = null` on an `optional int32` leaves `o` unset. |
| `oneof-scalar.pxf` | accept | `ks = null` on a `string` oneof member leaves the oneof `k` unset. |
| `oneof-message.pxf` | accept | `km = null` leaves the oneof `k` unset. |

Every accept fixture leaves no field set, and a full decode reports the
named field present-but-null.

An error names the field. The reference implementation's wording is
`null is not allowed on scalar field "n"`, `null is not allowed on
repeated field "l"` and `null is not allowed in repeated field "l"`; ports
need not match it.
