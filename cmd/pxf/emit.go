// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// emitPXF serialises a single query result back as PXF text.
//
// Stage A is loose-mode-only: there's no schema in scope, so the
// emitter walks the untyped graph and picks the natural PXF token for
// each Go type. Strict-mode output (schema-bound, with @dataset
// emission for tabular results) lands in Stage C.
//
// Top-level emission rules:
//   - map[string]any → field_entry list (`key = value` per line)
//   - any other value → wrapped as `value = <v>` so the result is still
//     a valid PXF document (the document grammar requires field_entry
//     at top level; a bare scalar isn't a legal PXF document).
func emitPXF(w io.Writer, v any) error {
	switch x := v.(type) {
	case map[string]any:
		return emitTopMap(w, x)
	default:
		return emitTopScalar(w, v)
	}
}

func emitTopMap(w io.Writer, m map[string]any) error {
	// `@type` is a directive (draft §3.4.1), not a field assignment.
	// Emit it first so consumers can parse the type before scanning
	// the body, matching what pxf.Format produces for typed messages.
	if v, ok := m["@type"]; ok {
		s, isStr := v.(string)
		if !isStr {
			return fmt.Errorf("@type must be a string, got %T", v)
		}
		if _, err := fmt.Fprintf(w, "@type %s\n", s); err != nil {
			return err
		}
	}
	for _, k := range sortedKeys(m) {
		if k == "@type" {
			continue // already emitted as directive
		}
		if _, err := fmt.Fprintf(w, "%s = ", k); err != nil {
			return err
		}
		if err := writeValue(w, m[k], 0); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func emitTopScalar(w io.Writer, v any) error {
	if _, err := io.WriteString(w, "value = "); err != nil {
		return err
	}
	if err := writeValue(w, v, 0); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func writeValue(w io.Writer, v any, indent int) error {
	switch x := v.(type) {
	case nil:
		_, err := io.WriteString(w, "null")
		return err
	case bool:
		if x {
			_, err := io.WriteString(w, "true")
			return err
		}
		_, err := io.WriteString(w, "false")
		return err
	case string:
		// Round-trip base64 bytes (set by loadPXF for BytesVal) back as
		// `b"..."` so the wire format stays equivalent.
		if rest, ok := strings.CutPrefix(x, "b"); ok && isBase64(rest) {
			_, err := fmt.Fprintf(w, "b%q", rest)
			return err
		}
		_, err := fmt.Fprintf(w, "%q", x)
		return err
	case int:
		_, err := fmt.Fprintf(w, "%d", x)
		return err
	case int64:
		_, err := fmt.Fprintf(w, "%d", x)
		return err
	case float64:
		// Non-finite values emit as the spec's identifiers (§3.8) —
		// FormatFloat's `NaN`/`+Inf`/`-Inf` spellings are not PXF
		// literals and would not re-parse.
		var s string
		switch {
		case math.IsInf(x, 1):
			s = "inf"
		case math.IsInf(x, -1):
			s = "-inf"
		case math.IsNaN(x):
			s = "nan"
		default:
			s = strconv.FormatFloat(x, 'g', -1, 64)
		}
		_, err := io.WriteString(w, s)
		return err
	case *big.Int:
		_, err := io.WriteString(w, x.String())
		return err
	case []any:
		return writeList(w, x, indent)
	case map[string]any:
		return writeBlock(w, x, indent)
	default:
		return fmt.Errorf("emitPXF: unsupported type %T", v)
	}
}

func writeList(w io.Writer, xs []any, indent int) error {
	if len(xs) == 0 {
		_, err := io.WriteString(w, "[]")
		return err
	}
	if _, err := io.WriteString(w, "["); err != nil {
		return err
	}
	for i, x := range xs {
		if i > 0 {
			if _, err := io.WriteString(w, ", "); err != nil {
				return err
			}
		}
		if err := writeValue(w, x, indent+1); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "]")
	return err
}

func writeBlock(w io.Writer, m map[string]any, indent int) error {
	if len(m) == 0 {
		_, err := io.WriteString(w, "{}")
		return err
	}
	if _, err := io.WriteString(w, "{\n"); err != nil {
		return err
	}
	pad := strings.Repeat("  ", indent+1)
	for _, k := range sortedKeys(m) {
		if _, err := fmt.Fprintf(w, "%s%s = ", pad, k); err != nil {
			return err
		}
		if err := writeValue(w, m[k], indent+1); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%s}", strings.Repeat("  ", indent))
	return err
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isBase64(s string) bool {
	if s == "" {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}
