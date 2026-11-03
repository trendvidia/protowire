# Write fixtures

Cross-port fixtures for the PB bytes a port writes from PXF text (#509). Each case is a PXF document and what protowire-go does with it: the bytes it writes, or the error it rejects with.

| File | What it is |
|---|---|
| `cases/<Message>.<name>.pxf` | An edge-case document. The file name's first word is the message in `text.v1`, from [`../text/text.proto`](../text/text.proto). |
| `golden/<name>.binpb` | The bytes protowire-go writes for it |
| `golden/<name>.reject` | Present instead when protowire-go rejects it, holding Go's error |

The gate also reads every `../text/cases/*.expected.pxf`, so each text case has a golden here too.

To add a case, add a `.pxf` and run `generate.sh`. A changed golden changes the bytes every port must write.

`scripts/cross_write_check.sh` runs each port's `check-decode --format pxf-pb` over the cases. A port must write the golden bytes, or reject where Go rejects. Map entries are compared in key order (false before true, integers by value, strings by UTF-8 bytes), as Go's deterministic writer puts them, so a port's harness sorts them where its own writer does not.

Excluded cases are listed in the script, each with the question it waits on.
