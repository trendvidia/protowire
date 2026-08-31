// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// Canonical annotation FQNs (proto/schema/v1/annotations.proto) as they
// appear in Annotation.name after lowering.
const (
	annValidate    = "protowire.schema.v1.validate"
	annRequired    = "protowire.schema.v1.required"
	annDefault     = "protowire.schema.v1.default"
	annDescription = "protowire.schema.v1.description"
	annExample     = "protowire.schema.v1.example"
	annErrorCode   = "protowire.schema.v1.error_code"
	annDeprecated  = "protowire.schema.v1.deprecated"
	annSensitive   = "protowire.schema.v1.sensitive"
	annHTTP        = "protowire.schema.v1.http"
)

// carrierTypes resolves the §8.1 extension carriers (1327–1331)
// dynamically from the bundled schema/v1/descriptor.proto, following the
// docpack pattern: the repository ships no generated Go for its own
// schemas.
type carrierTypes struct {
	types *protoregistry.Types
	// byName indexes every carrier extension by its field name
	// ("field_annotations", "type_decls", ...).
	byName map[string]protoreflect.ExtensionType
}

var (
	carrierOnce sync.Once
	carrierVal  *carrierTypes
	carrierErr  error
)

func carriers() (*carrierTypes, error) {
	carrierOnce.Do(func() {
		files, err := schemaresolve.CompileBundledFiles("schema/v1/descriptor.proto")
		if err != nil {
			carrierErr = err
			return
		}
		ct := &carrierTypes{
			types:  new(protoregistry.Types),
			byName: make(map[string]protoreflect.ExtensionType),
		}
		for _, fd := range files {
			xs := fd.Extensions()
			for i := 0; i < xs.Len(); i++ {
				xt := dynamicpb.NewExtensionType(xs.Get(i))
				if err := ct.types.RegisterExtension(xt); err != nil {
					carrierErr = err
					return
				}
				full := string(xs.Get(i).FullName())
				ct.byName[full[strings.LastIndexByte(full, '.')+1:]] = xt
			}
		}
		for _, want := range []string{
			"file_annotations", "functions", "type_decls",
			"message_annotations", "field_annotations", "enum_annotations",
			"service_annotations", "method_annotations",
		} {
			if ct.byName[want] == nil {
				carrierErr = fmt.Errorf("bundled schema/v1/descriptor.proto is missing carrier %q", want)
				return
			}
		}
		carrierVal = ct
	})
	return carrierVal, carrierErr
}

// annList decodes one options message and returns the entries of the
// AnnotationList carried under the named extension. The options arrive
// from a plain descriptorpb unmarshal, so the carriers sit in unknown
// fields; round-tripping the bytes through a resolver-aware unmarshal
// turns them back into messages (the docpack decodeFileOptions trick,
// generalized over every options kind).
func (ct *carrierTypes) annList(opts proto.Message, carrier string) ([]dmsg, error) {
	m, err := ct.reparse(opts, carrier)
	if err != nil || !m.valid() {
		return nil, err
	}
	return m.msgs("entries"), nil
}

// reparse decodes one options message and returns the carrier message
// under the named extension, or the zero dmsg when absent.
func (ct *carrierTypes) reparse(opts proto.Message, carrier string) (dmsg, error) {
	if opts == nil {
		return dmsg{}, nil
	}
	xt, ok := ct.byName[carrier]
	if !ok {
		panic(fmt.Sprintf("openapi: unknown carrier %q", carrier))
	}
	raw, err := proto.Marshal(opts)
	if err != nil {
		return dmsg{}, err
	}
	reparsed := opts.ProtoReflect().New().Interface()
	if err := (proto.UnmarshalOptions{Resolver: ct.types}).Unmarshal(raw, reparsed); err != nil {
		return dmsg{}, err
	}
	if !proto.HasExtension(reparsed, xt) {
		return dmsg{}, nil
	}
	m, ok := proto.GetExtension(reparsed, xt).(protoreflect.ProtoMessage)
	if !ok {
		return dmsg{}, nil
	}
	return wrap(m.ProtoReflect()), nil
}

// fieldAnnotations is a convenience wrapper for the most common lookup.
func (ct *carrierTypes) fieldAnnotations(f *descriptorpb.FieldDescriptorProto) ([]dmsg, error) {
	if f.GetOptions() == nil {
		return nil, nil
	}
	return ct.annList(f.GetOptions(), "field_annotations")
}

// ── Annotation entry accessors ────────────────────────────────────────────

// findAnn returns the first entry with the given FQN, or the zero dmsg.
func findAnn(entries []dmsg, fqn string) dmsg {
	for _, e := range entries {
		if e.str("name") == fqn {
			return e
		}
	}
	return dmsg{}
}

// allAnns returns every entry with the given FQN, in carrier order.
func allAnns(entries []dmsg, fqn string) []dmsg {
	var out []dmsg
	for _, e := range entries {
		if e.str("name") == fqn {
			out = append(out, e)
		}
	}
	return out
}

// arg resolves an annotation argument against the declared parameter
// list: a named arg matches by name; a positional arg (empty name) is
// counted against the parameter's declared position. This mirrors the
// §5.1 use-site rule that positional args precede named ones.
func arg(ann dmsg, param string, pos int) dmsg {
	if !ann.valid() {
		return dmsg{}
	}
	positional := 0
	for _, a := range ann.msgs("args") {
		switch a.str("name") {
		case param:
			return a
		case "":
			if positional == pos {
				return a
			}
			positional++
		}
	}
	return dmsg{}
}

// argStr returns a string-valued argument, or "" when absent.
func argStr(ann dmsg, param string, pos int) string {
	a := arg(ann, param, pos)
	if !a.valid() || a.which("value") != "string_value" {
		return ""
	}
	return a.str("string_value")
}

// argStrings returns a list-literal argument's string elements, or nil.
func argStrings(ann dmsg, param string, pos int) []string {
	a := arg(ann, param, pos)
	if !a.valid() || a.which("value") != "literal" {
		return nil
	}
	lit := a.sub("literal")
	if lit.which("kind") != "list" {
		return nil
	}
	var out []string
	for _, el := range lit.sub("list").msgs("elements") {
		if el.which("kind") == "string_value" {
			out = append(out, el.str("string_value"))
		}
	}
	return out
}

// argScalar converts an argument to an emission value (string, int64,
// float64, bool, []any, enum value name). Message-literal and
// expression arguments — and bytes, which have no canonical JSON form
// here — yield (nil, false): the caller omits the keyword rather than
// emitting a lossy guess.
func argScalar(a dmsg) (any, bool) {
	if !a.valid() {
		return nil, false
	}
	switch a.which("value") {
	case "string_value":
		return a.str("string_value"), true
	case "int_value":
		return a.i64("int_value"), true
	case "double_value":
		return a.f64("double_value"), true
	case "bool_value":
		return a.boolean("bool_value"), true
	case "literal":
		return literalScalar(a.sub("literal"))
	}
	return nil, false
}

func literalScalar(lit dmsg) (any, bool) {
	switch lit.which("kind") {
	case "enum_value":
		return lit.sub("enum_value").str("value_name"), true
	case "list":
		out := []any{}
		for _, el := range lit.sub("list").msgs("elements") {
			v, ok := literalValueScalar(el)
			if !ok {
				return nil, false
			}
			out = append(out, v)
		}
		return out, true
	}
	return nil, false
}

func literalValueScalar(el dmsg) (any, bool) {
	switch el.which("kind") {
	case "string_value":
		return el.str("string_value"), true
	case "int_value":
		return el.i64("int_value"), true
	case "double_value":
		return el.f64("double_value"), true
	case "bool_value":
		return el.boolean("bool_value"), true
	case "literal":
		return literalScalar(el.sub("literal"))
	}
	return nil, false
}
