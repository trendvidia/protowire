# Document formatter fixtures

Cross-port fixtures for what a port's document formatter writes (#584). Formatting works on the parsed document, not on a message: it keeps comments, directives and dataset rows, and reads no schema. The text gate (`../../text/`) checks the other path, a marshaller writing a message.

| File | What it is |
|---|---|
| `cases/<name>.pxf` | A document to format |
| `golden/<name>.pxf` | What protowire-go's `FormatDocument` writes for it |

Most cases are copies of every document under `testdata/` outside `adversarial/` that protowire-go parses, as of f22a10b. Each is named by its path, with `/` written as `.`, so `dataset/cell-framing.pxf` is `dataset.cell-framing.pxf`. They are copies so that a change elsewhere in `testdata/` does not change this gate. The `edge-*.pxf` cases are written for this gate:

- `edge-comments-between-directives`: comments of each kind before, between and after directives and rows, and a comment that ends the document.
- `edge-entry-label-type`: `@entry` with a label and a type, with a type only, and with neither, then a body.
- `edge-bare-named-directive`: named directives with no prefix, one prefix, two prefixes, and an empty block.
- `edge-dataset-cells`: dataset cells holding an escaped string, bytes, a timestamp with a fraction and an offset, a duration, a list (empty too), `null`, `-0.0`, and a row of absent cells.
- `edge-proto-then-type`: a triple-quoted `@proto` and an anonymous `@proto` block, then `@type`.
- `edge-named-proto`: a named `@proto` block, then a dataset of that type.

Every case must parse. To add one, add a `.pxf` under `cases/` and run `generate.sh`. A changed golden changes the text every port's formatter must write, so read the diff before committing it.

`scripts/cross_fmt_check.sh` runs each port's `check-decode --format pxf-fmt --input <case>` and compares its stdout with the golden, byte for byte. Cases one port is not held to yet are listed in the script, each with the issue that tracks it.
