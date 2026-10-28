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

// pxf query keeps every binding of a field bound twice: two lists
// concatenate (#337) and two maps merge (#396), as a decoder reads them.
// Before, the loader kept only the last binding, so `.` printed one list
// or map where the document had two.
func TestQuery_BoundTwiceKeepsBoth(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "qm.proto")
	if err := os.WriteFile(protoPath, []byte(queryMapsProto), 0o644); err != nil {
		t.Fatal(err)
	}
	md := func() *schema {
		sch, err := loadSchema([]string{protoPath}, nil, registryRef{})
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}().find("qm.v1.M")
	for name, body := range map[string]string{
		"repeated":             "many = [{ name = \"a\" }]\nmany = [{ name = \"b\" }]",
		"map":                  "s = {\n  a: \"1\"\n}\ns = {\n  b: \"2\"\n}",
		"map in a sub-message": "one { counts = {\n  a: 1\n} }\none { counts = {\n  b: 2\n} }",
	} {
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
			// A message bound twice is rejected since #347, so only the
			// repeated and map cases have a decoded value to compare to.
			got, err := pxf.UnmarshalDescriptor([]byte(out), md)
			if err != nil {
				t.Fatalf("query's output does not decode: %v\n%s", err, out)
			}
			if name == "map in a sub-message" {
				counts := got.Get(md.Fields().ByName("one")).Message()
				if n := counts.Get(counts.Descriptor().Fields().ByName("counts")).Map().Len(); n != 2 {
					t.Errorf("one.counts has %d entries, want 2:\n%s", n, out)
				}
				return
			}
			want, err := pxf.UnmarshalDescriptor([]byte(in), md)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(want, got) {
				t.Errorf("query dropped a binding:\nin:\n%s\nout:\n%s", in, out)
			}
		})
	}
}

const queryAnyProto = `syntax = "proto3";
package qa.v1;
import "google/protobuf/any.proto";
message Leaf { map<string, int32> counts = 1; string name = 2; }
message H {
  google.protobuf.Any one = 1;
  repeated google.protobuf.Any many = 2;
  map<string, google.protobuf.Any> by_key = 3;
}
`

// A map inside the message an Any block names is written as a map, the
// block resolved by its @type through the document's schema (#390).
// Before, the walk stopped at the Any, whose own fields are type_url and
// value, and the map came out as a block.
func TestQuery_MapsInAnyBodies(t *testing.T) {
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "qa.proto")
	if err := os.WriteFile(protoPath, []byte(queryAnyProto), 0o644); err != nil {
		t.Fatal(err)
	}
	sch, err := loadSchema([]string{protoPath}, nil, registryRef{})
	if err != nil {
		t.Fatal(err)
	}
	md := sch.find("qa.v1.H")
	leaf := "{\n    @type = \"type.googleapis.com/qa.v1.Leaf\"\n    counts = {\n      a: 1\n      \"b c\": 2\n    }\n  }"
	for name, body := range map[string]string{
		"singular":  "one " + leaf,
		"list":      "many = [" + leaf + ", " + leaf + "]",
		"map value": "by_key = {\n  k: " + leaf + "\n}",
	} {
		t.Run(name, func(t *testing.T) {
			in := "@type qa.v1.H\n" + body + "\n"
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
			if _, err := runCLI(t, "validate", "-p", protoPath, "-m", "qa.v1.H", outPath); err != nil {
				t.Fatalf("validate rejects query's output: %v\n%s", err, out)
			}
			opts := pxf.UnmarshalOptions{TypeResolver: typeResolver{find: sch.find}}
			want, err := opts.UnmarshalDescriptor([]byte(in), md)
			if err != nil {
				t.Fatal(err)
			}
			got, err := opts.UnmarshalDescriptor([]byte(out), md)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(want, got) {
				t.Errorf("round trip changed the message:\nin:\n%s\nout:\n%s", in, out)
			}
		})
	}
}
