# protoc-gen-pxf-go

Generates Go code for lowered RFC-001 schemas on stock protobuf. It takes the place of `protoc-gen-go` and accepts the same options.

For each file it writes:

- `<name>.pb.go`, the normal `protoc-gen-go` output, with `@description` and `@deprecated` as doc comments.
- `<name>_functions.pb.go`, the [§9.3](../../docs/RFC-001-schema-extensions.md#93-codegen-contract) function stubs, when the file declares functions. This file imports `github.com/trendvidia/protocheck/v2`.

## Use

```yaml
# buf.gen.yaml
version: v2
plugins:
  - local: protoc-gen-pxf-go
    out: gen
```

```bash
pxf build -o image.binpb ./schemas/...
buf generate image.binpb
```

Install from a `cmd/protoc-gen-pxf-go/vX.Y.Z` tag. It is a module of its own, requiring only protobuf, so adding it as a `go tool` raises no other dependency in your project.

## Limits

- **Getters.** Stock getters do not read a field's comments, so a getter carries neither `@description` nor `@deprecated`. On the hybrid and opaque APIs the text appears on the builder field. A field with the classic `deprecated` option keeps that option's notice on its getter.
- **`internal_gengo`.** The plugin calls `google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo`, which protobuf marks internal. A protobuf upgrade can break the build, so CI builds it against the version pinned here.
- **Cross-file aliases, enums and messages.** Function parameters of these types generate as `any`.
