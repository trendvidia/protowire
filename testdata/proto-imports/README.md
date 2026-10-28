# Source-form `@proto` import fixtures

Cross-port conformance fixtures for what a source-form `@proto` may import
([issue #369](https://github.com/trendvidia/protowire/issues/369); IETF draft
`-01` § The "@proto" Directive, "Imports"). The sources are the well-known types
(`google/protobuf/` `any`, `api`, `descriptor`, `duration`, `empty`,
`field_mask`, `source_context`, `struct`, `timestamp`, `type` and
`wrappers`, decided on [#388](https://github.com/trendvidia/protowire/issues/388)), `pxf/annotations.proto`, `pxf/bignum.proto`,
`pxf/secret.proto`, `sbe/annotations.proto`, `envelope/v1/envelope.proto`,
and another `@proto` in the same document. The list is exhaustive: any
other import MUST be rejected.

Each fixture is a body typed by its own `@proto`. Its leading comment states
the expected behavior, and `cmd/pxf/proto_imports_test.go` runs it through
`pxf validate` from this directory.

| File | Verdict | Asserts |
|---|---|---|
| `secret.pxf` | accept | `pxf/secret.proto` is an import source; a `pxf.Secret` field takes a string. |
| `bignum.pxf` | accept | `pxf/bignum.proto` is an import source. |
| `envelope.pxf` | accept | `envelope/v1/envelope.proto` is an import source. |
| `well-known.pxf` | accept | A well-known type is an import source. |
| `err-tool-schema.pxf` | reject | `docs/v1/topic.proto` is distributed with the spec but is not an import source. |
| `descriptor.pxf` | accept | `google/protobuf/descriptor.proto` may be imported directly, for custom options. |
| `err-plugin.pxf` | reject | `google/protobuf/compiler/plugin.proto` ships with protoc but is not a well-known type. |
| `err-features.pxf` | reject | Neither is a language features file such as `go_features.proto`. |
| `err-filesystem.pxf` | reject | A file on disk is not an import source, even when it exists: `sibling.proto` sits next to the fixture. |

`sibling.proto` is not a schema any fixture binds to.
