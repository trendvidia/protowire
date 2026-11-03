# Struct-binding table

`values.tsv` lists values written through a port's struct API (Go `encoding/pb` structs, Java's `Pb`, and so on), with the bytes protowire-go writes for each (#509). The byte gate for what a port writes from PXF text is `scripts/cross_write_check.sh` (#511). No gate can drive a struct API across ports, because each language spells the value its own way. So the table is the shared half, and each port's golden test is the other.

Each line is `<case>\t<hex>\t<value>`. The case name is the struct's name. The value is written in words, so a port can build its twin. An empty hex column is an empty message.

A port keeps a twin of every struct (field numbers and types are in `values_table_test.go`), marshals the value, and compares the bytes with this file. It reads either this file, or a byte copy that its CI checks against this file.

`go test -run TestValuesTable` fails when the table is not what the pinned protowire-go writes. `go test -run TestValuesTable -update` rewrites it. A changed row changes what every port must write.
