// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// --- in-doc @proto: named shape ---

func TestInDocProto_Named_RegistersType(t *testing.T) {
	input := `@proto trades.v1.Trade {
  string symbol = 1;
  double price = 2;
  int64 qty = 3;
}
sym = "AAPL"
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch == nil {
		t.Fatal("expected non-nil schema from in-doc @proto")
	}
	md := sch.find("trades.v1.Trade")
	if md == nil {
		t.Fatal("trades.v1.Trade not registered")
	}
	if md.Fields().Len() != 3 {
		t.Errorf("expected 3 fields, got %d", md.Fields().Len())
	}
}

func TestInDocProto_Named_NoPackage(t *testing.T) {
	// Bare name (no dots) → file has no `package` declaration; the
	// message lands at the top level.
	input := `@proto Trade {
  string symbol = 1;
}
x = 1
`
	doc, _ := loadPXF([]byte(input))
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("Trade") == nil {
		t.Fatal("Trade not registered at top level")
	}
}

func TestInDocProto_Named_SchemaResolutionEndToEnd(t *testing.T) {
	// The whole point of in-doc @proto: pxf_proto works without -p.
	input := `@proto trades.v1.Trade {
  string symbol = 1;
}
x = "AAPL"
`
	got := runE2EFullPipeline(t, input,
		`pxf_proto("trades.v1.Trade"; {symbol: .x})`)
	if !strings.Contains(got, `@type trades.v1.Trade`) {
		t.Errorf("expected @type directive: %s", got)
	}
	if !strings.Contains(got, `symbol = "AAPL"`) {
		t.Errorf("expected symbol: %s", got)
	}
}

// --- in-doc @proto: source shape ---

func TestInDocProto_Source_RegistersAllMessages(t *testing.T) {
	// Triple-quoted source contains multiple messages.
	input := "@proto \"\"\"\nsyntax = \"proto3\";\n" +
		"package trades.v1;\n" +
		"message Trade { string symbol = 1; }\n" +
		"message Quote { string symbol = 1; double bid = 2; }\n\"\"\"\nx = 1\n"
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("trades.v1.Trade") == nil {
		t.Error("Trade not registered")
	}
	if sch.find("trades.v1.Quote") == nil {
		t.Error("Quote not registered")
	}
}

// --- in-doc @proto: descriptor shape ---

func TestInDocProto_Descriptor_RegistersType(t *testing.T) {
	// Hand-build a FileDescriptorSet with a single message and serve
	// it as a base64 @proto descriptor body.
	syntax := "proto3"
	pkg := "test.v1"
	name := "test.proto"
	messageName := "M"
	fieldName := "x"
	fieldNum := int32(1)
	fieldType := descriptorpb.FieldDescriptorProto_TYPE_INT32
	fieldLabel := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    &name,
			Package: &pkg,
			Syntax:  &syntax,
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: &messageName,
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:   &fieldName,
					Number: &fieldNum,
					Type:   &fieldType,
					Label:  &fieldLabel,
				}},
			}},
		}},
	}
	blob, err := proto.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(blob)

	input := "@proto b\"" + b64 + "\"\nx = 1\n"
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("test.v1.M") == nil {
		t.Error("test.v1.M not registered from descriptor blob")
	}
}

// --- in-doc @proto: anonymous shape (currently rejected) ---

func TestInDocProto_Anonymous_Rejected(t *testing.T) {
	input := `@proto {
  string symbol = 1;
}
x = 1
`
	doc, _ := loadPXF([]byte(input))
	_, err := loadSchema(nil, doc.protos)
	if err == nil {
		t.Fatal("expected error for anonymous @proto, got nil")
	}
	if !strings.Contains(err.Error(), "anonymous") {
		t.Errorf("expected anonymous-related error, got: %v", err)
	}
}

// --- combined: -p + in-doc cooperate ---

func TestInDocProto_CombinedWithDashP(t *testing.T) {
	p := writeProto(t, `syntax = "proto3";
package outside.v1;
message Outside { string s = 1; }
`)
	input := `@proto inside.v1.Inside {
  string s = 1;
}
x = 1
`
	doc, _ := loadPXF([]byte(input))
	sch, err := loadSchema([]string{p}, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("outside.v1.Outside") == nil {
		t.Error("Outside (from -p) not registered")
	}
	if sch.find("inside.v1.Inside") == nil {
		t.Error("Inside (from in-doc) not registered")
	}
}

// --- end-to-end pipeline helper ---

// runE2EFullPipeline mirrors what cmd/pxq's main() does: parse the
// input, load the schema from in-doc @proto directives, run the
// query, emit. Used to test the schema-resolution chain end-to-end
// without a -p flag.
func runE2EFullPipeline(t *testing.T, input, query string) string {
	t.Helper()
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	results, err := runQuery(query, doc, sch)
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	var sb strings.Builder
	for _, r := range results {
		if err := emitPXF(&sb, r); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}
	return sb.String()
}
