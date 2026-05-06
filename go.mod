module github.com/trendvidia/protowire

go 1.26.1

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/spf13/cobra v1.10.2
	github.com/trendvidia/protoregistry v0.70.0
	github.com/trendvidia/protowire-go v0.70.2
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260217215200-42d3e9bedb6d // indirect
)

replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.12
