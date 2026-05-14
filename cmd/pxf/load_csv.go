// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// loadCSV parses CSV input into a loadedDoc whose single dataset
// directive carries the rows. Loose-mode rules per cmd/pxf/QUERY.md:
//
//   - cell matches ^-?\d+$            → int (or *big.Int on overflow)
//   - cell matches ^-?\d+\.\d+$       → float64
//   - cell is "true" or "false"       → bool
//   - everything else                 → string
//   - empty cell                      → absent (omitted from the row map)
//
// A header row is assumed (matches the README; --no-header flag lands
// alongside the strict-mode schema layer). The synthetic
// DatasetDirective makes `pxf_directive("dataset")` work identically
// for CSV and for an equivalent `@dataset`-headed PXF document.
func loadCSV(data []byte) (*loadedDoc, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // tolerate ragged rows; rows shorter than
	// the header surface as absent fields — matches the @dataset
	// empty-cell semantic.

	header, err := r.Read()
	if err == io.EOF {
		return &loadedDoc{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	ds := pxf.DatasetDirective{Columns: header}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row %d: %w", len(ds.Rows)+1, err)
		}
		row := pxf.DatasetRow{Cells: make([]pxf.Value, len(header))}
		for i := range header {
			if i >= len(rec) {
				break // ragged row: trailing columns absent (nil cell)
			}
			cell := rec[i]
			if cell == "" {
				continue // empty → absent (nil cell)
			}
			row.Cells[i] = csvCellToPXFValue(cell)
		}
		ds.Rows = append(ds.Rows, row)
	}
	return &loadedDoc{datasets: []pxf.DatasetDirective{ds}}, nil
}

// csvCellToPXFValue applies the per-cell classifier and wraps the
// result in a pxf.Value so the dataset round-trips through the
// pxf_directive("dataset") extension uniformly with native PXF
// datasets.
func csvCellToPXFValue(s string) pxf.Value {
	switch {
	case s == "true":
		return &pxf.BoolVal{Value: true}
	case s == "false":
		return &pxf.BoolVal{Value: false}
	case intRe.MatchString(s):
		return &pxf.IntVal{Raw: s}
	case floatRe.MatchString(s):
		return &pxf.FloatVal{Raw: s}
	default:
		return &pxf.StringVal{Value: s}
	}
}

var (
	intRe   = regexp.MustCompile(`^-?\d+$`)
	floatRe = regexp.MustCompile(`^-?\d+\.\d+$`)
)
