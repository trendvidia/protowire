// Copyright 2026 TrendVidia LLC
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func textMessage(t *testing.T, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md, err := loadMessage("../../testdata/text/text.binpb", name)
	if err != nil {
		t.Fatal(err)
	}
	return md
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A oneof written after a higher-numbered field, as protobuf-go writes it,
// sorts to field-number order; one already in order is unchanged.
func TestFieldOrder_OneofLast(t *testing.T) {
	md := textMessage(t, "text.v1.Ordered")
	// after = 3 (field 3), then a = 1 (field 1): protobuf-go's order.
	got, err := FieldOrder(mustHex(t, "18030801"), md)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, "08011803"); !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
	again, err := FieldOrder(got, md)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, got) {
		t.Fatalf("already sorted bytes changed: %x", again)
	}
}

// Fields inside a submessage sort too, and the outer length stays right.
func TestFieldOrder_Nested(t *testing.T) {
	md := textMessage(t, "text.v1.Shapes")
	// one (field 1) = Item{ weight = 2 (field 2), name = "n" (field 1) }.
	got, err := FieldOrder(mustHex(t, "0a051002"+"0a016e"), md)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, "0a05"+"0a016e"+"1002"); !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

// Elements of one repeated field keep their order, and an unpacked encoding
// stays unpacked: only the order of records changes.
func TestFieldOrder_RepeatedKeepsOrderAndEncoding(t *testing.T) {
	md := textMessage(t, "text.v1.Shapes")
	// ints (field 4) unpacked 2, then opt_int (3) = 1, then ints 1.
	got, err := FieldOrder(mustHex(t, "2002"+"1801"+"2001"), md)
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, "1801"+"2002"+"2001"); !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}
