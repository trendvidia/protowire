// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
)

// loadCSV parses CSV input into gojq's untyped graph. Loose mode
// (Stage B): per-cell type classification with the README's rules.
//
//   - cell matches ^-?\d+$            → int64 (or *big.Int on overflow)
//   - cell matches ^-?\d+\.\d+$       → float64
//   - cell is "true" or "false"       → bool
//   - everything else                 → string
//   - empty cell                      → absent (key omitted from row)
//
// A header row is assumed by default (matches the README; --no-header
// flag will land alongside the strict-mode schema layer in Stage C).
// The output graph mirrors PXF's `@dataset` shape so that
// `.__pxf_datasets[0].rows | map(...)` queries work identically for
// CSV and for an equivalent `@dataset`-headed PXF document.
func loadCSV(data []byte) (any, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate ragged rows; rows shorter than
	// the header surface as absent fields, which matches the @dataset
	// empty-cell semantic.

	header, err := r.Read()
	if err == io.EOF {
		return wrapAsDataset(nil, nil), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	rows := make([]any, 0, 16)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row %d: %w", len(rows)+1, err)
		}
		row := map[string]any{}
		for i, col := range header {
			if i >= len(rec) {
				break // ragged row: trailing columns absent
			}
			cell := rec[i]
			if cell == "" {
				continue // empty → absent
			}
			row[col] = classifyCell(cell)
		}
		rows = append(rows, row)
	}
	return wrapAsDataset(header, rows), nil
}

var (
	intRe   = regexp.MustCompile(`^-?\d+$`)
	floatRe = regexp.MustCompile(`^-?\d+\.\d+$`)
)

func classifyCell(s string) any {
	switch {
	case s == "true":
		return true
	case s == "false":
		return false
	case intRe.MatchString(s):
		return numberFromLexical(s)
	case floatRe.MatchString(s):
		return numberFromLexical(s)
	default:
		return s
	}
}

func wrapAsDataset(header []string, rows []any) any {
	cols := make([]any, len(header))
	for i, h := range header {
		cols[i] = h
	}
	if rows == nil {
		rows = []any{}
	}
	ds := map[string]any{
		"type":    "",
		"columns": cols,
		"rows":    rows,
	}
	return map[string]any{
		"__pxf_datasets": []any{ds},
	}
}
