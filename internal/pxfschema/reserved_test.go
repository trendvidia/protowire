// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package pxfschema

import (
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// buildDescriptor assembles a minimal FileDescriptorProto. It avoids the
// protocompile/protoreflect path so the test stays hermetic.
func buildDescriptor() *descriptorpb.FileDescriptorProto {
	syntax := "proto3"
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test.v1"),
		Syntax:  &syntax,
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Trade"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("symbol"), Number: proto.Int32(1), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_STRING)},
					{Name: proto.String("true"), Number: proto.Int32(2), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_BOOL)},   // VIOLATION
					{Name: proto.String("null"), Number: proto.Int32(3), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_STRING)}, // VIOLATION
					{Name: proto.String("TRUE"), Number: proto.Int32(4), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_STRING)}, // OK (case-sensitive)
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("false")},          // VIOLATION
					{Name: proto.String("choice_payload")}, // OK
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Inner"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: proto.String("false"), Number: proto.Int32(1), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_BOOL)}, // VIOLATION nested
						},
					},
				},
				EnumType: []*descriptorpb.EnumDescriptorProto{
					{
						Name: proto.String("Side"),
						Value: []*descriptorpb.EnumValueDescriptorProto{
							{Name: proto.String("SIDE_UNSPECIFIED"), Number: proto.Int32(0)},
							{Name: proto.String("null"), Number: proto.Int32(1)},   // VIOLATION (enum value)
							{Name: proto.String("BUY"), Number: proto.Int32(2)},
						},
					},
				},
			},
		},
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{
				Name: proto.String("Status"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
					{Name: proto.String("true"), Number: proto.Int32(1)}, // VIOLATION (top-level enum)
				},
			},
		},
	}
}

func typeP(t descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &t
}

func TestValidateProto_findsAllViolations(t *testing.T) {
	got := ValidateProto(buildDescriptor())

	wantElems := []string{
		"test.v1.Status.true",
		"test.v1.Trade.Inner.false",
		"test.v1.Trade.Side.null",
		"test.v1.Trade.false",
		"test.v1.Trade.null",
		"test.v1.Trade.true",
	}
	gotElems := make([]string, len(got))
	for i, v := range got {
		gotElems[i] = v.Element
	}
	sort.Strings(wantElems)
	if !equal(gotElems, wantElems) {
		t.Fatalf("violations mismatch\nwant: %v\ngot:  %v", wantElems, gotElems)
	}

	// Spot-check Kind assignment via element name suffix.
	for _, v := range got {
		switch {
		case strings.HasSuffix(v.Element, "Trade.false") && v.Kind != KindOneof:
			t.Errorf("Trade.false: want KindOneof, got %v", v.Kind)
		case strings.HasSuffix(v.Element, "Side.null") && v.Kind != KindEnumValue:
			t.Errorf("Side.null: want KindEnumValue, got %v", v.Kind)
		case strings.HasSuffix(v.Element, "Status.true") && v.Kind != KindEnumValue:
			t.Errorf("Status.true: want KindEnumValue, got %v", v.Kind)
		case strings.HasSuffix(v.Element, "Trade.true") && v.Kind != KindField:
			t.Errorf("Trade.true: want KindField, got %v", v.Kind)
		case strings.HasSuffix(v.Element, "Trade.null") && v.Kind != KindField:
			t.Errorf("Trade.null: want KindField, got %v", v.Kind)
		case strings.HasSuffix(v.Element, "Trade.Inner.false") && v.Kind != KindField:
			t.Errorf("Trade.Inner.false: want KindField, got %v", v.Kind)
		}
	}
}

func TestValidateProto_skipsSyntheticOneof(t *testing.T) {
	// A proto3 `optional bool null = 1;` produces a synthetic oneof named
	// `_null` (protoc's convention) wrapping a field with proto3_optional=true.
	// The field name violation is reported; the synthetic oneof must not be.
	syntax := "proto3"
	trueB := true
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("opt.proto"),
		Package: proto.String("opt.v1"),
		Syntax:  &syntax,
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("M"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:           proto.String("null"),
						Number:         proto.Int32(1),
						Type:           typeP(descriptorpb.FieldDescriptorProto_TYPE_BOOL),
						OneofIndex:     proto.Int32(0),
						Proto3Optional: &trueB,
					},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("_null")},
				},
			},
		},
	}
	got := ValidateProto(fd)
	if len(got) != 1 {
		t.Fatalf("want 1 violation (the field), got %d: %v", len(got), got)
	}
	if got[0].Kind != KindField || got[0].Name != "null" {
		t.Errorf("want KindField/null, got %v/%q", got[0].Kind, got[0].Name)
	}
}

func TestValidateProto_cleanSchemaProducesNoViolations(t *testing.T) {
	syntax := "proto3"
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("clean.proto"),
		Package: proto.String("clean.v1"),
		Syntax:  &syntax,
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("M"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("x"), Number: proto.Int32(1), Type: typeP(descriptorpb.FieldDescriptorProto_TYPE_INT32)},
				},
			},
		},
	}
	if got := ValidateProto(fd); len(got) != 0 {
		t.Errorf("want no violations, got %v", got)
	}
}

func TestAsError(t *testing.T) {
	if err := AsError(nil); err != nil {
		t.Errorf("nil input: want nil, got %v", err)
	}
	err := AsError([]Violation{
		{File: "t.proto", Element: "p.M.null", Name: "null", Kind: KindField},
	})
	if err == nil || !strings.Contains(err.Error(), "p.M.null") {
		t.Errorf("want error mentioning p.M.null, got %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
