# stock-image

`plain.proto` uses no v1.2 syntax. `plain.buf.binpb` is the image the buf
CLI builds from it, with source info:

```bash
cd testdata/stock-image && buf build -o plain.buf.binpb --path plain.proto
```

`TestBuildMatchesBuf` (cmd/pxf/build_test.go) checks that
`pxf build --null-mask=false` makes the same descriptor, so code generated
from a `pxf build` image is the code `buf generate` makes from source
(#424). Built with buf 1.74.0. Regenerate it with the command above when
`plain.proto` changes.
