// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// loadedDoc is what every input adapter produces: a gojq-ready body
// graph (the document's field entries) plus separately-addressed
// directive lists. Directives live off the body so query expressions
// don't accidentally trip over synthetic keys; pxf_directive(name)
// reaches them at runtime via the loadedDoc carried in the gojq env.
type loadedDoc struct {
	body       any                  // map[string]any for the typical document; nil if empty
	typeURL    string               // from @type, empty if absent
	directives []pxf.Directive      // generic @<name> directives (excludes @type/@dataset/@proto)
	datasets   []pxf.DatasetDirective
	protos     []pxf.ProtoDirective
}

// loadPXF parses a PXF document and lowers it to a loadedDoc.
//
// Loose-mode rules per cmd/pxf/QUERY.md: types are inferred from PXF
// source-level tokens (INT→int, FLOAT→float64, BYTES→base64 string with
// "b" prefix, BOOL/NULL/STRING/IDENT→native Go types, TIMESTAMP→RFC3339
// string, DURATION→Go-duration-string). Values larger than int64-max
// fall through to *big.Int.
//
// Document-level @type / @dataset / @proto / @<name> directives are
// separated from the body so the query layer can expose them via the
// pxf_directive(name) extension rather than synthetic top-level keys.
func loadPXF(data []byte) (*loadedDoc, error) {
	doc, err := pxf.Parse(data)
	if err != nil {
		return nil, err
	}
	body, err := entriesToValue(doc.Entries)
	if err != nil {
		return nil, err
	}
	return &loadedDoc{
		body:       body,
		typeURL:    doc.TypeURL,
		directives: doc.Directives,
		datasets:   doc.Datasets,
		protos:     doc.Protos,
	}, nil
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
		// gojq's numeric domain is Go `int`. ParseInt with bitSize=0
		// validates the value fits the platform's int width — on a
		// 32-bit build, values > int32-max fail here and fall through
		// to *big.Int, which is the correct routing per the README.
		if x, err := strconv.ParseInt(n.Raw, 10, 0); err == nil {
			return int(x), nil
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

// directiveToValue / protoToValue live in funcs.go alongside the
// pxf_directive function that exposes them.

func stringsToAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
