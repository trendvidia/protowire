# `@dataset` adversarial fixtures

Reference fixtures for the `@dataset` directive (draft §3.4.4). Each
file's leading comment states the violation and the expected
parser behavior (accept or reject).

The PXF parser itself lives in the downstream port repos
(`protowire-go`, `protowire-rust`, …); these fixtures are the
cross-port contract every port's `@dataset` implementation must
satisfy. The conformance harness wiring is deferred — see the
CHANGELOG entry for `@dataset` and `scripts/cross_security_check.sh`
for the existing decode-corpus shape these will eventually plug
into.

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
