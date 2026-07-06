module github.com/trendvidia/protowire

go 1.26.1

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/itchyny/gojq v0.12.19
	github.com/spf13/cobra v1.10.2
	github.com/trendvidia/protoregistry v0.72.0
	github.com/trendvidia/protowire-go v1.1.0
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260420184626-e10c466a9529 // indirect
)

replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.12
