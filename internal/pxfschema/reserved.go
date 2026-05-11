// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package pxfschema enforces the PXF schema-level constraints defined in
// draft-trendvidia-protowire-00 §3.13. Today's only constraint: a protobuf
// schema MUST NOT declare a message field, oneof, or enum value whose name
// is case-sensitively equal to a PXF value keyword (null, true, false).
// Such names are unreachable from PXF surface syntax — the tokenizer would
// always resolve them to the keyword branch — so accepting them silently
// produces bindings in which the offending element can never be selected.
//
// The package exposes two entry points covering the two descriptor shapes
// used in this repo:
//
//   - ValidateReflect, for tools that already hold a protoreflect.FileDescriptor
//     (the CLI's protocompile / protoregistry paths).
//   - ValidateProto, for protoc plugins that consume descriptorpb.FileDescriptorProto
//     directly off the CodeGeneratorRequest wire.
//
// Both return a flat slice of Violation values (empty means conformant) so
// callers can render every offending element rather than failing on the
// first one.
package pxfschema

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// reservedNames is the case-sensitive set of names that PXF reserves as
// value keywords (and that schemas therefore MUST NOT use for fields,
// oneofs, or enum values). Source: draft §3.13.
var reservedNames = map[string]struct{}{
	"null":  {},
	"true":  {},
	"false": {},
}

// Kind identifies which schema element a Violation refers to.
type Kind int

const (
	KindField Kind = iota + 1
	KindOneof
	KindEnumValue
)

func (k Kind) String() string {
	switch k {
	case KindField:
		return "message field"
	case KindOneof:
		return "oneof"
	case KindEnumValue:
		return "enum value"
	default:
		return "unknown"
	}
}

// Violation describes a single reserved-name collision.
type Violation struct {
	// File is the .proto file path (as recorded in the descriptor).
	File string
	// Element is the fully-qualified protobuf name of the offending
	// element (e.g. "trades.v1.Side.null" for an enum value, or
	// "trades.v1.Trade.true" for a field).
	Element string
	// Name is the bare offending identifier ("null" / "true" / "false").
	Name string
	// Kind is what the element is.
	Kind Kind
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s %q uses PXF-reserved name %q (draft §3.13)",
		v.File, v.Kind, v.Element, v.Name)
}

// ValidateReflect walks fd and returns every reserved-name collision. The
// returned slice is sorted by fully-qualified element name for stable output.
func ValidateReflect(fd protoreflect.FileDescriptor) []Violation {
	var out []Violation
	walkMessagesReflect(fd.Path(), fd.Messages(), &out)
	walkEnumsReflect(fd.Path(), fd.Enums(), &out)
	sortViolations(out)
	return out
}

func walkMessagesReflect(path string, msgs protoreflect.MessageDescriptors, out *[]Violation) {
	for i := range msgs.Len() {
		md := msgs.Get(i)
		fields := md.Fields()
		for j := range fields.Len() {
			f := fields.Get(j)
			name := string(f.Name())
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: string(f.FullName()),
					Name:    name,
					Kind:    KindField,
				})
			}
		}
		oneofs := md.Oneofs()
		for j := range oneofs.Len() {
			o := oneofs.Get(j)
			if o.IsSynthetic() {
				continue
			}
			name := string(o.Name())
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: string(o.FullName()),
					Name:    name,
					Kind:    KindOneof,
				})
			}
		}
		walkMessagesReflect(path, md.Messages(), out)
		walkEnumsReflect(path, md.Enums(), out)
	}
}

func walkEnumsReflect(path string, enums protoreflect.EnumDescriptors, out *[]Violation) {
	for i := range enums.Len() {
		e := enums.Get(i)
		vs := e.Values()
		for j := range vs.Len() {
			v := vs.Get(j)
			name := string(v.Name())
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: string(v.FullName()),
					Name:    name,
					Kind:    KindEnumValue,
				})
			}
		}
	}
}

// ValidateProto walks the raw descriptor proto and returns every reserved-
// name collision. Used by protoc plugins, which receive descriptorpb
// shapes directly off the CodeGeneratorRequest.
func ValidateProto(file *descriptorpb.FileDescriptorProto) []Violation {
	var out []Violation
	pkg := file.GetPackage()
	walkMessagesProto(file.GetName(), pkg, file.GetMessageType(), &out)
	walkEnumsProto(file.GetName(), pkg, file.GetEnumType(), &out)
	sortViolations(out)
	return out
}

func walkMessagesProto(path, parent string, msgs []*descriptorpb.DescriptorProto, out *[]Violation) {
	for _, m := range msgs {
		fullMsg := joinName(parent, m.GetName())
		for _, f := range m.GetField() {
			name := f.GetName()
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: joinName(fullMsg, name),
					Name:    name,
					Kind:    KindField,
				})
			}
		}
		for _, o := range m.GetOneofDecl() {
			// Skip synthetic oneofs introduced for proto3 optional fields.
			// A synthetic oneof contains exactly one field with
			// proto3_optional=true; its name shadows the field name and a
			// reserved-name field would already be reported above.
			if isSyntheticOneofProto(m, o) {
				continue
			}
			name := o.GetName()
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: joinName(fullMsg, name),
					Name:    name,
					Kind:    KindOneof,
				})
			}
		}
		walkMessagesProto(path, fullMsg, m.GetNestedType(), out)
		walkEnumsProto(path, fullMsg, m.GetEnumType(), out)
	}
}

func walkEnumsProto(path, parent string, enums []*descriptorpb.EnumDescriptorProto, out *[]Violation) {
	for _, e := range enums {
		fullEnum := joinName(parent, e.GetName())
		for _, v := range e.GetValue() {
			name := v.GetName()
			if _, hit := reservedNames[name]; hit {
				*out = append(*out, Violation{
					File:    path,
					Element: joinName(fullEnum, name),
					Name:    name,
					Kind:    KindEnumValue,
				})
			}
		}
	}
}

func isSyntheticOneofProto(m *descriptorpb.DescriptorProto, o *descriptorpb.OneofDescriptorProto) bool {
	// Find the index of o in m.OneofDecl, then check whether any field
	// references it with proto3_optional=true.
	idx := -1
	for i, decl := range m.GetOneofDecl() {
		if decl == o {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	for _, f := range m.GetField() {
		if f.OneofIndex != nil && int(f.GetOneofIndex()) == idx && f.GetProto3Optional() {
			return true
		}
	}
	return false
}

func joinName(parent, leaf string) string {
	if parent == "" {
		return leaf
	}
	return parent + "." + leaf
}

func sortViolations(vs []Violation) {
	sort.Slice(vs, func(i, j int) bool {
		return vs[i].Element < vs[j].Element
	})
}

// AsError joins a slice of violations into a single error suitable for
// returning from binaries that prefer one error per call. Returns nil when
// the slice is empty.
func AsError(vs []Violation) error {
	if len(vs) == 0 {
		return nil
	}
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return fmt.Errorf("PXF schema reserved-name violations:\n  %s", strings.Join(parts, "\n  "))
}
