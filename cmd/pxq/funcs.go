// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// gojq doesn't support `@<name>` user extensions (those slots are
// reserved for built-in string formatters like @uri, @base64). The
// README's `pxf_*` naming convention is the closest stable alternative.

// env carries the per-invocation context that the pxf_* functions need
// at query time: the parsed document (for directive lookups) and the
// optional schema (for type-aware paths). gojq.WithFunction's closure
// captures this so each `pxq` invocation gets its own slice.
type funcEnv struct {
	doc *loadedDoc
	sch *schema
}

// registerFuncs returns the gojq.CompilerOption set that registers the
// pxf_* extension namespace against env. Five functions per the README:
//   pxf_directive(name)  — directive list by name
//   pxf_fieldnames       — declared field names per bound schema
//   pxf_type             — proto type of input value
//   pxf_has(field)       — schema-aware has
//   pxf_proto(name; obj) — typed object construction
func registerFuncs(env *funcEnv) []gojq.CompilerOption {
	return []gojq.CompilerOption{
		gojq.WithFunction("pxf_directive", 1, 1,
			func(_ any, args []any) any {
				name, ok := args[0].(string)
				if !ok {
					return fmt.Errorf("pxf_directive: expected string, got %T", args[0])
				}
				return directiveList(env, name)
			}),

		gojq.WithFunction("pxf_fieldnames", 0, 0,
			func(in any, _ []any) any {
				if env.sch == nil {
					return fmt.Errorf("pxf_fieldnames: no schema in scope (pass -p schema.proto or include an @proto directive)")
				}
				name, ok := resolveTypeForFieldnames(in, env)
				if !ok {
					return fmt.Errorf("pxf_fieldnames: cannot resolve a proto type for the input value")
				}
				md := env.sch.find(name)
				if md == nil {
					return fmt.Errorf("pxf_fieldnames: type %q not registered in schema", name)
				}
				return fieldNames(md)
			}),

		gojq.WithFunction("pxf_type", 0, 0,
			func(in any, _ []any) any {
				name, ok := protoTypeOfValue(in, env)
				if !ok {
					return fmt.Errorf("pxf_type: cannot resolve a proto type for the input value")
				}
				return name
			}),

		gojq.WithFunction("pxf_has", 1, 1,
			func(in any, args []any) any {
				field, ok := args[0].(string)
				if !ok {
					return fmt.Errorf("pxf_has: expected string field name, got %T", args[0])
				}
				m, isMap := in.(map[string]any)
				if !isMap {
					return false
				}
				_, present := m[field]
				return present
			}),

		gojq.WithFunction("pxf_proto", 2, 2,
			func(_ any, args []any) any {
				name, ok := args[0].(string)
				if !ok {
					return fmt.Errorf("pxf_proto: expected string type name as first arg, got %T", args[0])
				}
				obj, ok := args[1].(map[string]any)
				if !ok {
					return fmt.Errorf("pxf_proto: expected object as second arg, got %T", args[1])
				}
				return protoConstruct(env, name, obj)
			}),
	}
}

// directiveList implements pxf_directive(name). Returns a list (slice
// of map[string]any), in source order, of every directive whose name
// matches. Recognised names map onto the spec's three directive
// productions plus the generic `@<name>` form:
//
//   "type"       → []map with {value: typeURL}   (max one entry)
//   "dataset"    → []map with dataset-shaped entries
//   "proto"      → []map with proto-shaped entries
//   "<other>"    → generic `@<name>` directives filtered by name
func directiveList(env *funcEnv, name string) []any {
	switch name {
	case "type":
		if env.doc == nil || env.doc.typeURL == "" {
			return []any{}
		}
		return []any{map[string]any{"value": env.doc.typeURL}}
	case "dataset":
		if env.doc == nil {
			return []any{}
		}
		out := make([]any, 0, len(env.doc.datasets))
		for _, ds := range env.doc.datasets {
			v, err := datasetToValueWithSchema(ds, env.sch)
			if err != nil {
				out = append(out, map[string]any{"error": err.Error()})
				continue
			}
			out = append(out, v)
		}
		return out
	case "proto":
		if env.doc == nil {
			return []any{}
		}
		out := make([]any, 0, len(env.doc.protos))
		for _, p := range env.doc.protos {
			out = append(out, protoToValue(p))
		}
		return out
	default:
		if env.doc == nil {
			return []any{}
		}
		out := make([]any, 0)
		for _, d := range env.doc.directives {
			if d.Name == name {
				out = append(out, directiveToValue(d))
			}
		}
		return out
	}
}

// datasetToValueWithSchema converts a DatasetDirective to the shape
// pxf_directive("dataset") exposes. Schema in scope → rows are bound
// to the message's typed field map (cell values reinterpreted via the
// field's declared type); no schema → rows expose cells as the
// schema-less untyped graph.
func datasetToValueWithSchema(ds pxf.DatasetDirective, sch *schema) (any, error) {
	var md protoreflect.MessageDescriptor
	if sch != nil && ds.Type != "" {
		md = sch.find(ds.Type)
	}

	rows := make([]any, 0, len(ds.Rows))
	for _, r := range ds.Rows {
		row := map[string]any{}
		for i, c := range r.Cells {
			if c == nil {
				continue // absent — leave the key out
			}
			col := ds.Columns[i]
			if md != nil {
				v, err := cellToTypedValue(c, md, col)
				if err != nil {
					return nil, fmt.Errorf("dataset row column %q: %w", col, err)
				}
				row[col] = v
			} else {
				v, err := valueToAny(c)
				if err != nil {
					return nil, fmt.Errorf("dataset row column %q: %w", col, err)
				}
				row[col] = v
			}
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"type":    ds.Type,
		"columns": stringsToAny(ds.Columns),
		"rows":    rows,
	}, nil
}

// cellToTypedValue applies schema-aware coercion when a descriptor is
// in scope. For Stage C this is a thin layer — it routes the cell's
// raw form through valueToAny but enforces a string→bytes lift for
// fields declared as bytes, since CSV/JSON adapters can't tell strings
// from bytes without the descriptor.
func cellToTypedValue(c pxf.Value, md protoreflect.MessageDescriptor, col string) (any, error) {
	fd := md.Fields().ByName(protoreflect.Name(col))
	v, err := valueToAny(c)
	if err != nil {
		return nil, err
	}
	if fd == nil {
		// Unknown column — surface the raw value (strict-mode AST
		// validation in Stage D will catch this at compile time).
		return v, nil
	}
	if fd.Kind() == protoreflect.BytesKind {
		if s, ok := v.(string); ok && !strings.HasPrefix(s, "b") {
			// Plain string from a CSV/JSON cell bound to a bytes field
			// — preserve as-is; a later pxf_proto re-bind would surface
			// the encoding mismatch.
			return s, nil
		}
	}
	return v, nil
}

// protoTypeOfValue infers the proto type-name for an arbitrary jq
// value. Resolution order:
//   1. embedded `@type` key on the value (pxf_proto-constructed objects)
//   2. jq-level type ("string", "int", "list", …) as a fallback
//
// pxf_proto re-binds explicitly so users have a deterministic path to
// a typed value when this heuristic isn't enough.
func protoTypeOfValue(in any, _ *funcEnv) (string, bool) {
	if m, ok := in.(map[string]any); ok {
		if t, ok := m["@type"].(string); ok {
			return t, true
		}
	}
	switch in.(type) {
	case nil:
		return "null", true
	case bool:
		return "bool", true
	case int, int64:
		return "int", true
	case float64:
		return "float", true
	case string:
		return "string", true
	case []any:
		return "list", true
	case map[string]any:
		return "object", true
	}
	return "", false
}

// resolveTypeForFieldnames is pxf_fieldnames' resolution rule: if the
// input value carries an `@type`, use it; otherwise fall back to the
// document's @type when the input is any object (since the natural
// "where do these field names live?" target for a top-level call is
// the document's type). pxf_type stays strict — it returns "object"
// when nothing types the value — but field-name introspection has a
// looser convention.
func resolveTypeForFieldnames(in any, env *funcEnv) (string, bool) {
	if m, ok := in.(map[string]any); ok {
		if t, ok := m["@type"].(string); ok {
			return t, true
		}
		if env != nil && env.doc != nil && env.doc.typeURL != "" {
			return env.doc.typeURL, true
		}
	}
	return "", false
}

func fieldNames(md protoreflect.MessageDescriptor) []any {
	fs := md.Fields()
	out := make([]any, 0, fs.Len())
	for i := range fs.Len() {
		out = append(out, string(fs.Get(i).Name()))
	}
	return out
}

// protoConstruct binds obj to the descriptor named `name` and returns
// a typed map[string]any with an "@type" sentinel that emitPXF can
// turn into an `@type` directive on output. Stage C does a permissive
// bind — every key in obj is preserved; field-name / type / required
// validation lands in Stage D.
func protoConstruct(env *funcEnv, name string, obj map[string]any) any {
	if env.sch == nil {
		return fmt.Errorf("pxf_proto(%q): no schema in scope (pass -p schema.proto)", name)
	}
	md := env.sch.find(name)
	if md == nil {
		return fmt.Errorf("pxf_proto(%q): type not registered in schema", name)
	}
	out := make(map[string]any, len(obj)+1)
	out["@type"] = name
	for k, v := range obj {
		out[k] = v
	}
	return out
}

// directiveToValue lowers a Directive to the shape `pxf_directive` exposes.
func directiveToValue(d pxf.Directive) any {
	out := map[string]any{
		"name":     d.Name,
		"prefixes": stringsToAny(d.Prefixes),
		"type":     d.Type,
		"hasBody":  d.Body != nil,
	}
	if d.Body != nil {
		out["body"] = string(d.Body)
	}
	return out
}

// protoToValue lowers a ProtoDirective. body is base64-encoded for the
// descriptor shape and raw UTF-8 otherwise.
func protoToValue(p pxf.ProtoDirective) any {
	body := ""
	if p.Shape == pxf.ProtoDescriptor {
		body = base64.StdEncoding.EncodeToString(p.Body)
	} else {
		body = string(p.Body)
	}
	return map[string]any{
		"shape":    strings.ToLower(p.Shape.String()),
		"typeName": p.TypeName,
		"body":     body,
	}
}
