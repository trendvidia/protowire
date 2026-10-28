# Null-binding fixtures

Cross-port conformance fixtures for where a `null` literal may bind
([issue #339](https://github.com/trendvidia/protowire/issues/339); IETF
draft `-01` § Booleans, Null, and Identifier Values). A `null`:

- on a singular message field MUST clear it;
- on a singular scalar field MUST be rejected unless the field is a
  wrapper type (`google.protobuf.Int32Value` and the like);
- on a repeated field, or as a list element, MUST be rejected.

Explicit presence does not make a scalar nullable: `optional` scalars and
scalar oneof members reject `null` like any other scalar. The owner
settled this on #339 (2026-10-23): the draft's rule stands, and the
README's `_null` convention was reworked to fit it.

Every fixture binds against [`nulls.proto`](nulls.proto) (package
`nulls.v1`, message `Holder`). Each file's leading comment states the
expected behavior, and `cmd/pxf/nulls_test.go` runs them through the
reference implementation: `Parse` (no schema, which accepts them all), a
decoder bound to `nulls.v1.Holder`, and `pxf validate`. The same rule in a
`@dataset` cell is pinned by
[`../dataset/null-on-scalar-cell.pxf`](../dataset/null-on-scalar-cell.pxf).

| File | Verdict | Asserts |
|---|---|---|
| `err-scalar.pxf` | reject | `n = null` on an `int32`. |
| `err-optional-scalar.pxf` | reject | `o = null` on an `optional int32`. |
| `err-oneof-scalar.pxf` | reject | `ks = null` on a `string` oneof member. |
| `err-repeated.pxf` | reject | `l = null` on a `repeated int32`. |
| `err-list-element.pxf` | reject | `l = [1, null]`: a list element is not nullable. |
| `message.pxf` | accept | `msg = null` leaves `msg` unset. |
| `wrapper.pxf` | accept | `w = null` leaves `w` unset; a full decode reports it present-but-null. |
| `oneof-message.pxf` | accept | `km = null` leaves the oneof `k` unset. |

An error names the field. The reference implementation's wording is
`null is not allowed on scalar field "n"`, `null is not allowed on
repeated field "l"` and `null is not allowed in repeated field "l"`; ports
need not match it.
