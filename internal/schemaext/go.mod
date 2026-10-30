// Nested test-only module: the RFC-001 §9.3 end-to-end round trip (#144).
// protocompile and protocheck are public, but their dependency trees are
// heavyweight and test-only here, so they stay off the main
// github.com/trendvidia/protowire module graph. CI runs this module as a
// separate job; every dependency is public, no credentials involved.
module github.com/trendvidia/protowire/internal/schemaext

go 1.26.2

require (
	github.com/trendvidia/protocheck/v2 v2.8.0
	github.com/trendvidia/protocompile v0.36.1
	github.com/trendvidia/protos v0.1.2
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/petermattis/goid v0.0.0-20260113132338-7c7de50cc741 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tidwall/btree v1.8.2 // indirect
	github.com/trendvidia/protowire-go v1.12.1 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/sync v0.24.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260807164820-c8921c73eeea // indirect
)

// The fork carries the RFC-001 §9.3 function-stub codegen in
// cmd/protoc-gen-go (trendvidia/protobuf-go#4); the harness builds the
// plugin binary out of this module's dependency graph.
replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.16-0.20261030182108-ce1fb6c870fd
