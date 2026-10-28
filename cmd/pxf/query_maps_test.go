// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/proto"
)

const queryMapsProto = `syntax = "proto3";
package qm.v1;
message Inner {
  map<string, int32> counts = 1;
  string name = 2;
}
message M {
  map<string, string> s = 1;
  map<int64, string> i = 2;
  map<bool, string> b = 3;
  map<string, Inner> inner = 4;
  Inner one = 5;
  repeated Inner many = 6;
}
`

// `pxf query .` writes a map field as a map, so its output is the input
// again (#373). Before, the loose graph lowered a map like a block and
// the emitter wrote `k = "v"`, which a decoder rejects in a map field.
func TestQuery_MapsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "qm.proto")
	if err := os.WriteFile(protoPath, []byte(queryMapsProto), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"string keys":      "s = {\n  \"k\": \"v\"\n  \"a b\": \"spaced\"\n  \"true\": \"keyword\"\n  \"x.y\": \"dotted\"\n}",
		"int keys":         "i = {\n  1: \"one\"\n  -2: \"minus two\"\n}",
		"bool keys":        "b = {\n  true: \"yes\"\n  false: \"no\"\n}",
		"map of messages":  `inner = { "k": { counts = { "c": 1 } name = "n" } }`,
		"map in a message": `one { counts = { "c": 2 } }`,
		"map in a list":    `many = [{ counts = { "c": 3 } }, { name = "x" }]`,
	}
	md := func() *schema {
		sch, err := loadSchema([]string{protoPath}, nil, registryRef{})
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}().find("qm.v1.M")
	for name, body := range docs {
		t.Run(name, func(t *testing.T) {
			in := "@type qm.v1.M\n" + body + "\n"
			inPath := filepath.Join(t.TempDir(), "in.pxf")
			if err := os.WriteFile(inPath, []byte(in), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := runCLI(t, "query", "-p", protoPath, ".", inPath)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			outPath := filepath.Join(t.TempDir(), "out.pxf")
			if err := os.WriteFile(outPath, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := runCLI(t, "validate", "-p", protoPath, "-m", "qm.v1.M", outPath); err != nil {
				t.Fatalf("validate rejects query's output: %v\n%s", err, out)
			}
			want, err := pxf.UnmarshalDescriptor([]byte(in), md)
			if err != nil {
				t.Fatal(err)
			}
			got, err := pxf.UnmarshalDescriptor([]byte(out), md)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(want, got) {
				t.Errorf("round trip changed the message:\nin:\n%s\nout:\n%s", in, out)
			}
		})
	}
}
