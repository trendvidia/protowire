// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// inferFlags holds the subcommand-only state. Kept in a struct so the
// var declarations don't pollute package-globals.
type inferFlags struct {
	messageName string
	sampleRows  int
	fullScan    bool
}

var infer = &inferFlags{}

func inferSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infer-schema <file>",
		Short: "Produce a .proto schema by inferring per-column types from a sample file",
		Long: "Walks a CSV or PXF @dataset and emits a .proto declaration whose\n" +
			"fields carry the per-column types the values support. Default is\n" +
			"fail-fast: if any row past the sample window introduces a value\n" +
			"that contradicts the candidate type for its column, infer-schema\n" +
			"aborts with a recovery hint. --full-scan walks the rest of the\n" +
			"file collecting every contradiction before reporting.\n\n" +
			"Output goes to stdout — pipe to a file or another tool.",
		Args:          cobra.ExactArgs(1),
		RunE:          runInferSchema,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := cmd.Flags()
	f.StringVarP(&infer.messageName, "message", "m", "",
		"fully-qualified message name for the inferred schema (e.g. trades.v1.Trade); "+
			"required so the generated .proto can be imported elsewhere")
	f.IntVar(&infer.sampleRows, "sample-rows", 1000,
		"number of rows the candidate-fix phase examines; values past this "+
			"row count must conform to the candidate types")
	f.BoolVar(&infer.fullScan, "full-scan", false,
		"on contradiction, walk the entire file collecting every mismatch "+
			"instead of aborting fast")
	return cmd
}

func runInferSchema(_ *cobra.Command, args []string) error {
	if infer.messageName == "" {
		return fmt.Errorf("--message is required (fully-qualified name, e.g. trades.v1.Trade)")
	}
	path := args[0]
	data, err := readInput(path)
	if err != nil {
		return err
	}
	format, err := detectFormat(path, formatFlag)
	if err != nil {
		return err
	}
	doc, err := loadByFormat(format, data)
	if err != nil {
		return fmt.Errorf("parse %s as %s: %w", path, format, err)
	}

	ds, err := pickDataset(doc, format)
	if err != nil {
		return err
	}

	report, err := inferDataset(ds, infer.sampleRows, infer.fullScan)
	if err != nil {
		return err
	}
	return emitProtoFile(os.Stdout, infer.messageName, report)
}

// pickDataset returns the dataset to infer from. For CSV the input
// produces exactly one synthetic dataset; for PXF the file must carry
// at least one @dataset directive. Other formats are rejected — they
// don't have a natural "row" notion that infer-schema can operate on.
func pickDataset(doc *loadedDoc, format string) (pxf.DatasetDirective, error) {
	if len(doc.datasets) == 0 {
		return pxf.DatasetDirective{}, fmt.Errorf("infer-schema requires a tabular input "+
			"(CSV or PXF with @dataset); %s has no dataset", format)
	}
	if len(doc.datasets) > 1 {
		return pxf.DatasetDirective{}, fmt.Errorf("infer-schema needs exactly one @dataset "+
			"in the input; got %d", len(doc.datasets))
	}
	return doc.datasets[0], nil
}

// inferredType is the lattice the candidate-fix phase walks per
// column. Types widen along: bool → int → float → string. Anything
// that doesn't fit in float64 ergonomics (date-like literals, free
// strings) terminates at string. nullable is orthogonal — sticky once
// any null cell appears.
type inferredType int

const (
	typeBool inferredType = iota
	typeInt
	typeFloat
	typeString
)

func (t inferredType) protoType() string {
	switch t {
	case typeBool:
		return "bool"
	case typeInt:
		return "int64"
	case typeFloat:
		return "double"
	case typeString:
		return "string"
	}
	return "string"
}

func (t inferredType) String() string { return t.protoType() }

// classifyCellValue picks an inferredType for one cell. Mirrors the
// CSV adapter's per-cell classifier but expressed in the inference
// lattice. nil cells (absent in PXF / empty in CSV) don't constrain
// the type.
func classifyCellValue(c pxf.Value) (inferredType, bool) {
	if c == nil {
		return 0, false
	}
	switch v := c.(type) {
	case *pxf.BoolVal:
		_ = v
		return typeBool, true
	case *pxf.IntVal:
		return typeInt, true
	case *pxf.FloatVal:
		return typeFloat, true
	case *pxf.StringVal:
		return typeString, true
	case *pxf.IdentVal:
		// Identifiers in a CSV/PXF cell don't have a clean proto
		// type without an enum context; treat as string for the
		// purpose of schema inference.
		return typeString, true
	default:
		return typeString, true
	}
}

// columnReport is the per-column accumulator the inference loop
// builds. typ is the current candidate; nullable flips on as soon as
// any null cell appears. firstSeenAt is informational for diagnostics.
type columnReport struct {
	name        string
	typ         inferredType
	typSet      bool
	nullable    bool
	firstSeenAt int // 1-indexed row number where typ was first set
}

// inferReport is the full per-column accumulator + any collected
// contradictions when --full-scan is in effect.
type inferReport struct {
	columns        []columnReport
	contradictions []string
}

// inferDataset walks ds.Rows and builds the per-column type report.
// The candidate type for each column is set the first time a non-null
// cell appears in the first `sampleRows` rows; later rows that
// disagree with the candidate either abort (fail-fast) or accumulate
// into report.contradictions (--full-scan).
func inferDataset(ds pxf.DatasetDirective, sampleRows int, fullScan bool) (*inferReport, error) {
	if len(ds.Columns) == 0 {
		return nil, fmt.Errorf("infer-schema: dataset has no columns")
	}

	rep := &inferReport{columns: make([]columnReport, len(ds.Columns))}
	for i, c := range ds.Columns {
		rep.columns[i].name = c
	}

	for rowIdx, row := range ds.Rows {
		rowNum := rowIdx + 1 // 1-indexed for diagnostics
		for colIdx, cell := range row.Cells {
			cr := &rep.columns[colIdx]
			if cell == nil {
				cr.nullable = true
				continue
			}
			t, ok := classifyCellValue(cell)
			if !ok {
				continue
			}
			if !cr.typSet {
				// First non-null cell in this column — fix the candidate.
				cr.typ = t
				cr.typSet = true
				cr.firstSeenAt = rowNum
				continue
			}
			if t == cr.typ {
				continue
			}
			// Conflict. Widen vs. abort?
			//
			// Within the sample window: widen along the type lattice
			// (the candidate-fix phase is allowed to broaden).
			// Past the sample window: abort or accumulate.
			widened := widen(cr.typ, t)
			if rowNum <= sampleRows {
				cr.typ = widened
				continue
			}
			// Past the sample window, conflict that the candidate
			// can't accommodate. The widened type might still be
			// compatible if it's the existing one or a strict
			// widening of the cell's actual type. The simpler rule:
			// any mismatch past the sample window is a contradiction.
			msg := fmt.Sprintf(
				"row %d, column %q: value type %s does not match inferred type %s\n"+
					"the first %d rows suggested %s; re-run with --sample-rows=%d to widen "+
					"the type during inference, or edit the generated .proto manually to set %q as %s",
				rowNum, cr.name, t, cr.typ, sampleRows, cr.typ, rowNum, cr.name, widened,
			)
			if !fullScan {
				return nil, fmt.Errorf("%s", msg)
			}
			rep.contradictions = append(rep.contradictions, msg)
		}
	}
	if len(rep.contradictions) > 0 {
		return nil, fmt.Errorf("infer-schema --full-scan found %d contradictions:\n  %s",
			len(rep.contradictions), strings.Join(rep.contradictions, "\n  "))
	}

	// Columns whose first non-null cell never arrived default to string —
	// safest fallback (proto3 string is always-valid).
	for i := range rep.columns {
		if !rep.columns[i].typSet {
			rep.columns[i].typ = typeString
		}
	}
	return rep, nil
}

// widen returns the join of two inferred types along the lattice.
// Used during the sample window when the candidate hasn't been pinned
// yet. After the window, conflicts become contradictions.
func widen(a, b inferredType) inferredType {
	if a > b {
		return a
	}
	return b
}

// emitProtoFile writes a minimal .proto matching the inference report.
// fields use snake_case names as-is, sequential numbers from 1, and
// `optional` keyword on nullable columns so the absent-vs-zero
// distinction round-trips through proto3 presence.
func emitProtoFile(w io.Writer, fqName string, rep *inferReport) error {
	pkg, msg := splitDottedName(fqName)
	var b strings.Builder
	b.WriteString("// SPDX-License-Identifier: MIT\n")
	b.WriteString("// Copyright (c) 2026 TrendVidia, LLC.\n")
	b.WriteString("//\n")
	b.WriteString("// Generated by `pxf infer-schema`.\n\n")
	b.WriteString("syntax = \"proto3\";\n")
	if pkg != "" {
		fmt.Fprintf(&b, "package %s;\n", pkg)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "message %s {\n", msg)

	// Header order is the natural one — column 0 first, column 1
	// second, etc. — which keeps output deterministic across runs
	// against the same input.
	for i, col := range rep.columns {
		prefix := ""
		if col.nullable {
			prefix = "optional "
		}
		fmt.Fprintf(&b, "  %s%s %s = %d;\n", prefix, col.typ.protoType(), col.name, i+1)
	}
	b.WriteString("}\n")
	_, err := io.WriteString(w, b.String())
	return err
}
