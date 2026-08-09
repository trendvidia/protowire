// Nested test-only module: the RFC-001 §9.3 end-to-end round trip (#144).
// protocompile and protocheck are public, but their dependency trees are
// heavyweight and test-only here, so they stay off the main
// github.com/trendvidia/protowire module graph. CI runs this module as a
// separate job; every dependency is public, no credentials involved.
module github.com/trendvidia/protowire/internal/schemaext

go 1.26.2

require (
	github.com/trendvidia/protocheck/v2 v2.3.0
	github.com/trendvidia/protocompile v0.23.1-0.20260809042801-5426adb52c7e
	google.golang.org/protobuf v1.36.11
)

require (
	buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go v1.36.11-20250109164928-1da0de137947.1 // indirect
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20240920164238-5a7b106cbb87.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/petermattis/goid v0.0.0-20260113132338-7c7de50cc741 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tidwall/btree v1.8.1 // indirect
	github.com/trendvidia/protowire-go v1.3.1 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/sync v0.22.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260807164820-c8921c73eeea // indirect
)

// The fork carries the RFC-001 §9.3 function-stub codegen in
// cmd/protoc-gen-go (trendvidia/protobuf-go#4); the harness builds the
// plugin binary out of this module's dependency graph.
replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.14
