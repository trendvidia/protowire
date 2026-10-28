// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package schemaresolve

import (
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const nullMaskSchema = `syntax = "proto3";
package nm.v1;
import "google/protobuf/wrappers.proto";
message M {
  google.protobuf.Int32Value w = 1;
  optional int32 o = 2;
  int32 n = 3;
}
`

func resolveNullMaskSchema(t *testing.T) *Registry {
	t.Helper()
	reg, err := Resolve(CompileOptions{
		VirtualFiles: []VirtualFile{{Name: "nm.proto", Body: []byte(nullMaskSchema)}},
		BundledFiles: CompileBundledAll,
	}, RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return reg
}

// roundTrip decodes doc against md, goes through protobuf binary, and
// encodes the result back to PXF. It decodes the way `pxf encode` does:
// only the full decode records nulls in the mask.
func roundTrip(t *testing.T, md protoreflect.MessageDescriptor, doc string) string {
	t.Helper()
	msg, _, err := pxf.UnmarshalFullDescriptor([]byte(doc), md)
	if err != nil {
		t.Fatalf("decode %q: %v", doc, err)
	}
	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	back := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(wire, back); err != nil {
		t.Fatal(err)
	}
	out, err := pxf.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A schema compiled from source gets the generated mask, and a null on a
// wrapper survives PXF → binary → PXF through it (#377).
func TestNullMask_RoundTrip(t *testing.T) {
	md := resolveNullMaskSchema(t).Find("nm.v1.M")
	if md == nil {
		t.Fatal("nm.v1.M not found")
	}
	fd := md.Fields().ByName("_null")
	if fd == nil {
		t.Fatal("nm.v1.M has no generated _null field")
	}
	if fd.Number() != nullMaskNumber || fd.Message() == nil || fd.Message().FullName() != "google.protobuf.FieldMask" {
		t.Fatalf("_null is number %d, type %v; want %d, google.protobuf.FieldMask", fd.Number(), fd.Message(), nullMaskNumber)
	}

	if out := roundTrip(t, md, "w = null\nn = 5\n"); !strings.Contains(out, "w = null") {
		t.Errorf("wrapper null lost in the round trip; got:\n%s", out)
	}

	// An optional scalar holds a null too (#339, option 3), since
	// protowire-go v1.11.0 (protowire-go#182).
	if out := roundTrip(t, md, "o = null\nn = 5\n"); !strings.Contains(out, "o = null") {
		t.Errorf("optional-scalar null lost in the round trip; got:\n%s", out)
	}
	// Both in one document: the mask records each.
	if out := roundTrip(t, md, "w = null\no = null\n"); !strings.Contains(out, "w = null") || !strings.Contains(out, "o = null") {
		t.Errorf("a null lost in the round trip of both; got:\n%s", out)
	}
}

// The canonical schemas this repository bundles are shared types every
// port generates code for, so they never get the mask (#377).
func TestNullMask_CanonicalSchemasExcluded(t *testing.T) {
	reg := resolveNullMaskSchema(t)
	for _, name := range []string{"pxf.Secret", "pxf.BigInt", "envelope.v1.Envelope"} {
		md := reg.Find(name)
		if md == nil {
			t.Errorf("%s not found", name)
			continue
		}
		if fd := md.Fields().ByNumber(nullMaskNumber); fd != nil {
			t.Errorf("%s got a generated %s field", name, fd.Name())
		}
	}
	for _, p := range []string{"pxf/secret.proto", "protowire/schema/v1/annotations.proto", "envelope/v1/envelope.proto"} {
		if NullMask(p) {
			t.Errorf("NullMask(%q) = true, want false for a canonical schema", p)
		}
	}
	if !NullMask("nm.proto") {
		t.Error(`NullMask("nm.proto") = false, want true for a user schema`)
	}
}
