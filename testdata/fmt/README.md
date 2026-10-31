# fmt

Fixtures for `pxf fmt` on `.proto` sources (#425).

- `annotated.proto`: v1.2 syntax, with annotation uses on a message, field, oneof, oneof member, enum, enum value, service and rpc, plus comments. Not laid out. The tests check that formatting keeps every annotation use and comment, that the output parses, and that formatting it again changes nothing.
- `stock.proto`: no v1.2 syntax, deliberately not laid out.
- `stock.buf-format.proto`: what `buf format` (1.74.0) makes of `stock.proto`. `pxf fmt` must produce the same bytes. Regenerate with `buf format stock.proto > stock.buf-format.proto`.
