module github.com/trendvidia/protowire

go 1.26.2

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/itchyny/gojq v0.12.19
	github.com/spf13/cobra v1.10.2
	github.com/trendvidia/protocompile v0.24.1
	github.com/trendvidia/protoregistry v0.72.0
	github.com/trendvidia/protowire-go v1.4.1
	google.golang.org/genproto/googleapis/api v0.0.0-20260807164820-c8921c73eeea
	google.golang.org/grpc v1.83.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	buf.build/gen/go/bufbuild/protodescriptor/protocolbuffers/go v1.36.11-20250109164928-1da0de137947.1 // indirect
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20240920164238-5a7b106cbb87.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/petermattis/goid v0.0.0-20260113132338-7c7de50cc741 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/btree v1.8.1 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260729162451-8efbd57d26e0 // indirect
)

replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.14
