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
// disambiguation rules documented in cmd/pxq/README.md:
//
//   - 1 (no decimal)     → int64 (or *big.Int on overflow)
//   - 1.0 (decimal)       → float64
//   - ""                  → "" (empty string, NOT null)
//   - null                → nil
//   - [], {}              → empty list / empty map
//
// We use UseNumber() so the lexical form of numeric literals survives
// to our type-routing layer — the stdlib's default would coerce every
// number to float64 and lose the int-vs-float distinction.
func loadJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return jsonNormalize(raw), nil
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
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		// gojq operates on `int`; on 64-bit platforms this is int64
		// wide. Values exceeding int64 fall through to the *big.Int
		// branch below.
		return int(n)
	}
	if z, ok := new(big.Int).SetString(s, 10); ok {
		return z
	}
	return s
}
