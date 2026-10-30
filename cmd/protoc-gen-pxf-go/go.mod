// Nested so a project adopting this plugin takes on protobuf and nothing
// else. In protowire's main module, `go get -tool` raises the project's grpc,
// cobra and x/ versions to protowire's (#458). Keep this require list at one.
module github.com/trendvidia/protowire/cmd/protoc-gen-pxf-go

go 1.23

require google.golang.org/protobuf v1.36.12
