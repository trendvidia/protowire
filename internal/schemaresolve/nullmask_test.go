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

// nullMaskNumber is the field number protocompile gives the generated
// `_null` mask (RFC-001 §8.6).
const nullMaskNumber = 536870911

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

	// The optional scalar needs protowire-go#182, which accepts null on a
	// field with explicit presence (#339, option 3). The pinned release
	// still rejects it; this fails once the bump that closes the gap
	// lands, so a round trip of the optional scalar can replace it.
	_, _, err := pxf.UnmarshalFullDescriptor([]byte("o = null\n"), md)
	if err == nil || !strings.Contains(err.Error(), `null is not allowed on scalar field "o"`) {
		t.Errorf("known gap protowire-go#182 changed: decode of o = null gave %v; replace this with the round trip of the optional scalar", err)
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
