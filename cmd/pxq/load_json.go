// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// loadJSON parses JSON input into gojq's untyped graph with the
// disambiguation rules documented in cmd/pxq/README.md. Returns a
// loadedDoc with no directives — JSON inputs are body-only.
//
//   - 1 (no decimal)     → int (or *big.Int on overflow)
//   - 1.0 (decimal)       → float64
//   - ""                  → "" (empty string, NOT null)
//   - null                → nil
//   - [], {}              → empty list / empty map
//
// UseNumber() preserves the lexical form so jsonNormalize can route
// int-vs-float — the stdlib default coerces every number to float64.
func loadJSON(data []byte) (*loadedDoc, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return &loadedDoc{body: jsonNormalize(raw)}, nil
}

func jsonNormalize(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, string:
		return x
	case json.Number:
		return numberFromLexical(string(x))
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = jsonNormalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = jsonNormalize(e)
		}
		return out
	default:
		// json.Decoder doesn't emit other types from UseNumber mode;
		// fall through with a defensive cast.
		return fmt.Sprintf("%v", v)
	}
}

// numberFromLexical inspects the raw token to decide int vs float.
// JSON's grammar treats `1` and `1.0` as the same number; PXF (and
// proto) distinguish them. We preserve the source-level intent.
func numberFromLexical(s string) any {
	if strings.ContainsAny(s, ".eE") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return f
	}
	// gojq's numeric domain is Go `int`. ParseInt with bitSize=0
	// validates the value fits the platform's int width — on a 32-bit
	// build, values > int32-max fail here and fall through to *big.Int,
	// which is the correct routing per the README.
	if n, err := strconv.ParseInt(s, 10, 0); err == nil {
		return int(n)
	}
	if z, ok := new(big.Int).SetString(s, 10); ok {
		return z
	}
	return s
}
