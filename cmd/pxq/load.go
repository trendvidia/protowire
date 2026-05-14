// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// loadPXF parses a PXF document and lowers it to gojq's untyped graph
// (map[string]any / []any / string / float64 / int / nil).
//
// Loose-mode rules: types are inferred from PXF source-level tokens.
//   - INT  → int64 if it fits, else *big.Int
//   - FLOAT → float64
//   - BOOL/NULL/STRING/IDENT → corresponding Go types
//   - TIMESTAMP/DURATION → RFC3339Nano / Go-duration-string
//   - BYTES → base64-encoded string (round-trips through emitPXF as b"...")
//
// Document-level directives are flattened into the result:
//   - Body entries land at top level
//   - `@type`, `@dataset`, `@proto`, `@<name>` directives are reachable via
//     the `@pxf.directive(name)` extension (Stage C); for Stage A we
//     attach them on synthetic keys prefixed with "__pxf_" so a raw jq
//     query can still see them.
func loadPXF(data []byte) (any, error) {
	doc, err := pxf.Parse(data)
	if err != nil {
		return nil, err
	}

	body, err := entriesToValue(doc.Entries)
	if err != nil {
		return nil, err
	}

	root, ok := body.(map[string]any)
	if !ok {
		// Top-level was empty or unusual; wrap so directives can still attach.
		root = map[string]any{}
		if body != nil {
			root["__pxf_body"] = body
		}
	}

	if doc.TypeURL != "" {
		root["__pxf_type"] = doc.TypeURL
	}
	if len(doc.Directives) > 0 {
		dirs := make([]any, 0, len(doc.Directives))
		for _, d := range doc.Directives {
			dirs = append(dirs, directiveToValue(d))
		}
		root["__pxf_directives"] = dirs
	}
	if len(doc.Datasets) > 0 {
		dss := make([]any, 0, len(doc.Datasets))
		for _, ds := range doc.Datasets {
			v, err := datasetToValue(ds)
			if err != nil {
				return nil, err
			}
			dss = append(dss, v)
		}
		root["__pxf_datasets"] = dss
	}
	if len(doc.Protos) > 0 {
		ps := make([]any, 0, len(doc.Protos))
		for _, p := range doc.Protos {
			ps = append(ps, protoToValue(p))
		}
		root["__pxf_protos"] = ps
	}
	return root, nil
}

func entriesToValue(entries []pxf.Entry) (any, error) {
	if len(entries) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	for _, e := range entries {
		switch n := e.(type) {
		case *pxf.Assignment:
			v, err := valueToAny(n.Value)
			if err != nil {
				return nil, err
			}
			out[n.Key] = v
		case *pxf.MapEntry:
			v, err := valueToAny(n.Value)
			if err != nil {
				return nil, err
			}
			out[n.Key] = v
		case *pxf.Block:
			v, err := entriesToValue(n.Entries)
			if err != nil {
				return nil, err
			}
			out[n.Name] = v
		default:
			return nil, fmt.Errorf("unsupported entry type %T", e)
		}
	}
	return out, nil
}

func valueToAny(v pxf.Value) (any, error) {
	switch n := v.(type) {
	case *pxf.StringVal:
		return n.Value, nil
	case *pxf.IntVal:
		// Try int64 first, fall back to *big.Int for very large ints.
		if x, err := strconv.ParseInt(n.Raw, 10, 64); err == nil {
			return x, nil
		}
		if x, ok := new(big.Int).SetString(n.Raw, 10); ok {
			return x, nil
		}
		return nil, fmt.Errorf("invalid integer %q", n.Raw)
	case *pxf.FloatVal:
		x, err := strconv.ParseFloat(n.Raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float %q: %w", n.Raw, err)
		}
		return x, nil
	case *pxf.BoolVal:
		return n.Value, nil
	case *pxf.BytesVal:
		// Round-trip as base64 string — the emitter recognises this and
		// emits b"..." again. Storing []byte instead would let gojq see
		// it as a typed array, which complicates downstream `length` /
		// `.[0]` semantics. Strings are jq-idiomatic.
		return "b" + base64.StdEncoding.EncodeToString(n.Value), nil
	case *pxf.NullVal:
		return nil, nil
	case *pxf.IdentVal:
		// Bare identifiers (typically enum names) stay as strings; the
		// schema layer (Stage C) re-binds them to declared enum values.
		return n.Name, nil
	case *pxf.TimestampVal:
		return n.Value.UTC().Format(time.RFC3339Nano), nil
	case *pxf.DurationVal:
		return n.Raw, nil
	case *pxf.ListVal:
		out := make([]any, 0, len(n.Elements))
		for _, e := range n.Elements {
			v, err := valueToAny(e)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case *pxf.BlockVal:
		return entriesToValue(n.Entries)
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

// directiveToValue lowers a Directive to the shape `@pxf.directive` will
// expose in Stage C: { name, prefixes, type, body, hasBody }.
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

// datasetToValue lowers a DatasetDirective. In loose mode (no schema in
// scope) each row's cells are exposed as { <colName>: <value> } maps so
// `.rows[].<col>` works the way the README's quick-start example uses.
// Empty cells (absent fields) are surfaced as absent map keys; null
// cells (`null` literal) are surfaced as nil values.
func datasetToValue(ds pxf.DatasetDirective) (any, error) {
	rows := make([]any, 0, len(ds.Rows))
	for _, r := range ds.Rows {
		row := map[string]any{}
		for i, c := range r.Cells {
			col := ds.Columns[i]
			if c == nil {
				continue // absent — leave the key out
			}
			v, err := valueToAny(c)
			if err != nil {
				return nil, fmt.Errorf("dataset row cell %q: %w", col, err)
			}
			row[col] = v
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"type":    ds.Type,
		"columns": stringsToAny(ds.Columns),
		"rows":    rows,
	}, nil
}

// protoToValue lowers a ProtoDirective. body is base64-encoded for the
// descriptor shape and raw UTF-8 otherwise — matches what `@pxf.directive`
// will surface in Stage C.
func protoToValue(p pxf.ProtoDirective) any {
	bodyStr := ""
	if p.Shape == pxf.ProtoDescriptor {
		bodyStr = base64.StdEncoding.EncodeToString(p.Body)
	} else {
		bodyStr = string(p.Body)
	}
	return map[string]any{
		"shape":    strings.ToLower(p.Shape.String()),
		"typeName": p.TypeName,
		"body":     bodyStr,
	}
}

func stringsToAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
