// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
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
		// Usage-on-parse-errors-only is inherited from the root command's
		// PersistentPreRun (#188); SilenceErrors additionally left runtime
		// failures unprinted.
		Args: cobra.ExactArgs(1),
		RunE: runInferSchema,
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

// inferredType is the field type inference gives a column. Only one
// widening holds: an int cell binds to a double field, so int and float
// join to double. Every other pair has no field type a PXF decoder binds
// both to (a string field rejects 1, an int64 field rejects true,
// measured on #360), so a column that mixes them is refused rather than
// typed string, which would give a schema that rejects its own input.
// Whether a column holds a null is orthogonal: see columnReport.
type inferredType int

const (
	typeBool inferredType = iota
	typeInt
	typeFloat
	typeString
	typeBytes
	typeTimestamp
	typeDuration
)

func (t inferredType) protoType() string {
	switch t {
	case typeBool:
		return "bool"
	case typeInt:
		return "int64"
	case typeFloat:
		return "double"
	case typeBytes:
		return "bytes"
	case typeTimestamp:
		return "google.protobuf.Timestamp"
	case typeDuration:
		return "google.protobuf.Duration"
	}
	return "string"
}

// isMessage reports whether t is a message type, which has presence of
// its own and so holds a null as it is.
func (t inferredType) isMessage() bool {
	return t == typeTimestamp || t == typeDuration
}

// wrapperType is the wrapper a scalar column that holds a null becomes
// (decided by the owner on #360): since #339 a null binds only to a field
// with explicit presence, and a wrapper is the scalar's nullable form.
func (t inferredType) wrapperType() string {
	switch t {
	case typeBool:
		return "google.protobuf.BoolValue"
	case typeInt:
		return "google.protobuf.Int64Value"
	case typeFloat:
		return "google.protobuf.DoubleValue"
	case typeBytes:
		return "google.protobuf.BytesValue"
	}
	return "google.protobuf.StringValue"
}

// noun names t in a diagnostic.
func (t inferredType) noun() string {
	switch t {
	case typeBool:
		return "a bool"
	case typeInt:
		return "an integer"
	case typeFloat:
		return "a float"
	case typeBytes:
		return "a bytes literal"
	case typeTimestamp:
		return "a timestamp"
	case typeDuration:
		return "a duration"
	}
	return "a string"
}

func (t inferredType) String() string { return t.protoType() }

// classifyCellValue picks an inferredType for one non-null cell. ok is
// false for a cell no inferred type holds: an identifier, which binds
// only to an enum field, and infer-schema infers no enums.
func classifyCellValue(c pxf.Value) (t inferredType, ok bool) {
	switch c.(type) {
	case *pxf.BoolVal:
		return typeBool, true
	case *pxf.IntVal:
		return typeInt, true
	case *pxf.FloatVal:
		return typeFloat, true
	case *pxf.StringVal:
		return typeString, true
	case *pxf.BytesVal:
		return typeBytes, true
	case *pxf.TimestampVal:
		return typeTimestamp, true
	case *pxf.DurationVal:
		return typeDuration, true
	}
	return 0, false
}

// columnReport is the per-column accumulator the inference loop
// builds. typ is the current candidate; nullable flips on as soon as
// any null cell appears. firstSeenAt is informational for diagnostics.
type columnReport struct {
	name        string
	typ         inferredType
	typSet      bool
	nullable    bool // an empty cell: the field is optional
	holdsNull   bool // a null cell: the field needs explicit presence that binds null (#360)
	firstSeenAt int  // 1-indexed row number where typ was first set
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
		// A dotted column is a path into a sub-message (draft §3.4.4),
		// and inference builds no nested messages (#348). Written as a
		// field name, it would give a .proto that protoc rejects.
		if strings.Contains(c, ".") {
			return nil, fmt.Errorf("infer-schema: column %q is a path into a sub-message; infer-schema infers scalar columns only, so write the nested message by hand", c)
		}
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
			switch cell.(type) {
			case *pxf.NullVal:
				cr.holdsNull = true
				continue
			case *pxf.ListVal, *pxf.BlockVal:
				// A value cell (draft §3.4.4) is a repeated field or a
				// sub-message, which the scalar lattice cannot type (#348).
				return nil, fmt.Errorf("infer-schema: row %d, column %q holds a list or a block; infer-schema infers scalar columns only, so write that field by hand", rowNum, cr.name)
			}
			t, ok := classifyCellValue(cell)
			if !ok {
				return nil, fmt.Errorf("infer-schema: row %d, column %q holds an identifier, which binds only to an enum field; infer-schema infers no enums, so write that field by hand", rowNum, cr.name)
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
			widened, ok := widen(cr.typ, t)
			if !ok {
				// No field type binds both, inside the sample window or
				// past it, so this is a refusal and not a contradiction a
				// larger window could fix (decided by the owner on #360).
				// --full-scan collects these with the contradictions.
				msg := fmt.Sprintf("column %q holds %s (row %d) and %s (row %d), and no field type holds both; write that field by hand", cr.name, cr.typ.noun(), cr.firstSeenAt, t.noun(), rowNum)
				if !fullScan {
					return nil, fmt.Errorf("infer-schema: %s", msg)
				}
				rep.contradictions = append(rep.contradictions, msg)
				continue
			}
			if widened == cr.typ {
				// The candidate already holds this cell: an int in a
				// double column.
				continue
			}
			// Conflict. Widen vs. abort?
			//
			// Within the sample window: widen along the type lattice
			// (the candidate-fix phase is allowed to broaden).
			// Past the sample window: abort or accumulate.
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

// widen returns the type that holds both a and b, and false when none
// does. The only join of two different types is int and float to double.
func widen(a, b inferredType) (inferredType, bool) {
	switch {
	case a == b:
		return a, true
	case (a == typeInt && b == typeFloat) || (a == typeFloat && b == typeInt):
		return typeFloat, true
	}
	return 0, false
}

// emitProtoFile writes a minimal .proto matching the inference report.
// fields use snake_case names as-is, sequential numbers from 1, and
// `optional` keyword on nullable columns so the absent-vs-zero
// distinction round-trips through proto3 presence. A scalar column that
// holds a null is its wrapper type instead, and a timestamp or duration
// column is its message type, which has presence of its own.
func emitProtoFile(w io.Writer, fqName string, rep *inferReport) error {
	pkg, msg := splitDottedName(fqName)
	fields := make([]string, len(rep.columns))
	imports := map[string]bool{}
	for i, col := range rep.columns {
		typ, prefix := col.typ.protoType(), ""
		switch {
		case col.typ.isMessage():
			imports["google/protobuf/"+strings.ToLower(strings.TrimPrefix(typ, "google.protobuf."))+".proto"] = true
		case col.holdsNull:
			typ = col.typ.wrapperType()
			imports["google/protobuf/wrappers.proto"] = true
		case col.nullable:
			prefix = "optional "
		}
		fields[i] = fmt.Sprintf("  %s%s %s = %d;\n", prefix, typ, col.name, i+1)
	}
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
	if len(imports) > 0 {
		for _, imp := range slices.Sorted(maps.Keys(imports)) {
			fmt.Fprintf(&b, "import %q;\n", imp)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "message %s {\n", msg)

	// Header order is the natural one — column 0 first, column 1
	// second, etc. — which keeps output deterministic across runs
	// against the same input.
	for _, f := range fields {
		b.WriteString(f)
	}
	b.WriteString("}\n")
	_, err := io.WriteString(w, b.String())
	return err
}
