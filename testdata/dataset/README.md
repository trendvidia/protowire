# `@dataset` adversarial fixtures

Reference fixtures for the `@dataset` directive (draft §3.4.4). Each
file's leading comment states the violation and the expected
behavior (accept or reject) of a decoder that parses the document and
binds each row to its type. Most verdicts are decided by the parse;
`block-on-scalar` is decided by binding, so a harness that only parses
cannot meet it.

The PXF parser itself lives in the downstream port repos
(`protowire-go`, `protowire-rust`, …); these fixtures are the
cross-port contract every port's `@dataset` implementation must
satisfy. `cmd/pxf`'s `dataset_test.go` holds the reference
implementation to every fixture three ways: through `pxf validate`,
which parses the document and binds each row to its type; through the
streaming `DatasetReader`, whose rows must match the materializing
parse; and, for accept fixtures, through `pxf query`. The test's verdict
table must list every file here. The other ports are held to the
fixtures through their own issues; no cross-port harness runs them
yet.

All fixtures bind against `test.v1.AllTypes` in `testdata/test.proto`
unless otherwise noted.

The `truncated-row-*`, `junk-between-rows` and `rows-end-at-directive`
fixtures pin the "Appending" rule (issue
[#336](https://github.com/trendvidia/protowire/issues/336)). The row
sequence ends only at end of input or at the next directive. A
streaming reader must reject the first three, as the materializing
parser does, rather than end the sequence early. protowire-go v1.8.1's
`DatasetReader` does not yet
([protowire-go#141](https://github.com/trendvidia/protowire-go/issues/141)),
and the same bug lets it stream past `with-body.pxf`.

`list-cell`, `block-cell`, `nested-cells`, `cell-framing` (accept) and
`block-on-scalar` (reject) pin value cells (issue
[#341](https://github.com/trendvidia/protowire/issues/341), v1.13). A
cell holds any value, and a non-empty cell in column `c` binds as
`c = <value>` would in a body. `list-cell` and `block-cell` were reject
fixtures before v1.13. Until `cmd/pxf` pins a protowire-go release
carrying [protowire-go#152](https://github.com/trendvidia/protowire-go/pull/152),
its test lists these five as known gaps. The streaming leg also lists
the four fixtures `DatasetReader` streams past (protowire-go#141).
