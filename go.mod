module github.com/trendvidia/protowire

go 1.26.1

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/itchyny/gojq v0.12.19
	github.com/spf13/cobra v1.10.2
	github.com/trendvidia/protoregistry v0.70.1
	github.com/trendvidia/protowire-go v1.0.0
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

replace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.12
