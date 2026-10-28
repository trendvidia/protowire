// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// inferCSV is the shared test helper — load CSV into a loadedDoc, run
// the inference engine, render the proto to a string.
func inferCSV(t *testing.T, input string, sampleRows int, fullScan bool, msgName string) (string, error) {
	t.Helper()
	doc, err := loadCSV([]byte(input))
	if err != nil {
		t.Fatalf("loadCSV: %v", err)
	}
	ds, err := pickDataset(doc, "csv")
	if err != nil {
		return "", err
	}
	rep, err := inferDataset(ds, sampleRows, fullScan)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := emitProtoFile(&buf, msgName, rep); err != nil {
		t.Fatalf("emitProtoFile: %v", err)
	}
	return buf.String(), nil
}

func TestInfer_BasicTypes(t *testing.T) {
	input := "name,age,price,active\n" +
		"Alice,30,1.5,true\n" +
		"Bob,25,2.0,false\n"
	got, err := inferCSV(t, input, 1000, false, "x.v1.Person")
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"package x.v1;",
		"message Person {",
		"string name = 1;",
		"int64 age = 2;",
		"double price = 3;",
		"bool active = 4;",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in output:\n%s", w, got)
		}
	}
}

func TestInfer_NullableViaEmptyCell(t *testing.T) {
	// Empty cell in CSV is "absent" → column flips to nullable.
	input := "symbol,price\n" +
		"AAPL,188.42\n" +
		",415.10\n"
	got, err := inferCSV(t, input, 1000, false, "x.v1.Trade")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "optional string symbol = 1;") {
		t.Errorf("expected nullable symbol field, got:\n%s", got)
	}
	if !strings.Contains(got, "double price = 2;") || strings.Contains(got, "optional double price") {
		t.Errorf("expected non-nullable price field, got:\n%s", got)
	}
}

func TestInfer_WidensWithinSampleWindow(t *testing.T) {
	// First row int, second row float — within sample window, the
	// candidate widens to float without erroring.
	input := "x\n1\n2.5\n"
	got, err := inferCSV(t, input, 1000, false, "x.v1.X")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "double x = 1;") {
		t.Errorf("expected widened double type, got:\n%s", got)
	}
}

func TestInfer_FailFastPastSampleWindow(t *testing.T) {
	// sample-rows=2 fixes the candidate after row 2; row 3 must conform.
	// A float would have widened the column to double inside the window.
	input := "age\n30\n25\n2.5\n"
	_, err := inferCSV(t, input, 2, false, "x.v1.Y")
	if err == nil {
		t.Fatal("expected fail-fast contradiction")
	}
	want := []string{
		`row 3, column "age"`,
		"does not match inferred type int64",
		"re-run with --sample-rows=3",
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q missing %q", err.Error(), w)
		}
	}
}

func TestInfer_FullScanAccumulates(t *testing.T) {
	// Two columns, two rows past the sample window — each row has a
	// mismatch in each column. Full-scan reports all four in one
	// error instead of aborting after the first.
	input := "a,b\n1,1\n2,2\n1.5,2.5\n3.5,4.5\n"
	_, err := inferCSV(t, input, 2, true, "x.v1.M")
	if err == nil {
		t.Fatal("expected full-scan errors")
	}
	if !strings.Contains(err.Error(), "4 contradictions") {
		t.Errorf("expected 4 contradictions, got: %v", err)
	}
	for _, want := range []string{`row 3`, `row 4`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error: %v", want, err)
		}
	}
}

func TestInfer_AllNullColumnDefaultsToString(t *testing.T) {
	// All-null cells in a column → default to string (safest).
	input := "a,b\n1,\n2,\n"
	got, err := inferCSV(t, input, 1000, false, "x.v1.X")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "optional string b = 2;") {
		t.Errorf("expected nullable string for all-empty column, got:\n%s", got)
	}
}

func TestInfer_BareMessageNameNoPackage(t *testing.T) {
	got, err := inferCSV(t, "x\n1\n", 1000, false, "Bare")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "package ") {
		t.Errorf("expected no package declaration for bare name, got:\n%s", got)
	}
	if !strings.Contains(got, "message Bare {") {
		t.Errorf("expected message Bare:\n%s", got)
	}
}

func TestInfer_RejectsJSONInput(t *testing.T) {
	// JSON has no `@dataset` notion — infer-schema requires tabular input.
	doc, err := loadJSON([]byte(`{"a": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pickDataset(doc, "json")
	if err == nil {
		t.Fatal("expected error for non-tabular input")
	}
	if !strings.Contains(err.Error(), "tabular input") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInfer_PXFDatasetInput(t *testing.T) {
	// Native PXF @dataset is just as valid an input as CSV.
	input := `@dataset trades.v1.Trade ( symbol, price )
( "AAPL", 188.42 )
( "MSFT", 415.10 )
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	ds, err := pickDataset(doc, "pxf")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := inferDataset(ds, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := emitProtoFile(&buf, "trades.v1.Trade", rep); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "string symbol = 1;") {
		t.Errorf("expected string symbol: %s", got)
	}
	if !strings.Contains(got, "double price = 2;") {
		t.Errorf("expected double price: %s", got)
	}
}

func TestInfer_MultipleDatasetsRejected(t *testing.T) {
	input := `@dataset x.A ( a )
( 1 )
@dataset x.B ( b )
( 2 )
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pickDataset(doc, "pxf")
	if err == nil {
		t.Fatal("expected error for multi-dataset input")
	}
	if !strings.Contains(err.Error(), "exactly one @dataset") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInfer_NoDatasetRejected(t *testing.T) {
	doc, err := loadPXF([]byte("x = 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pickDataset(doc, "pxf")
	if err == nil {
		t.Fatal("expected error for body-only PXF input")
	}
	if !strings.Contains(err.Error(), "has no dataset") {
		t.Errorf("unexpected error: %v", err)
	}
}

// inferPXF runs the inference engine on a PXF @dataset document.
func inferPXF(t *testing.T, input string) error {
	t.Helper()
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	ds, err := pickDataset(doc, "pxf")
	if err != nil {
		t.Fatalf("pickDataset: %v", err)
	}
	_, err = inferDataset(ds, 1000, false)
	return err
}

// infer-schema infers scalar columns only (#348). A dotted column, from a
// PXF header or a CSV one, is refused rather than written as a field name
// protoc rejects; a list or block cell is refused rather than typed as a
// string.
func TestInfer_RefusesNestedColumns(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"dotted", "@dataset ev.Event (id, desc.energy)\n(\"e1\", 0.5)\n", `column "desc.energy" is a path into a sub-message`},
		{"list cell", "@dataset ev.Event (id, tags)\n(\"e1\", 1)\n(\"e2\", [\"a\"])\n", `row 2, column "tags" holds a list or a block`},
		{"block cell", "@dataset ev.Event (id, desc)\n(\"e1\", { energy = 0.5 })\n", `row 1, column "desc" holds a list or a block`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := inferPXF(t, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	if _, err := inferCSV(t, "id,desc.energy\ne1,0.5\n", 1000, false, "ev.v1.Event"); err == nil || !strings.Contains(err.Error(), `column "desc.energy" is a path into a sub-message`) {
		t.Fatalf("CSV: want the dotted column refused, got %v", err)
	}
}

// A column whose cells no single field type holds is refused, naming
// the column and both rows (#360). Typed string, as before, it gave a
// schema that rejected the column's other cells: a string field rejects
// 1 and true.
func TestInfer_RefusesUnbindableMix(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"int then string", "age\n30\ntwenty\n", `column "age" holds an integer (row 1) and a string (row 2)`},
		{"bool then int", "f\ntrue\n1\n", `column "f" holds a bool (row 1) and an integer (row 2)`},
		{"string then float", "s\nabc\n1.5\n", `column "s" holds a string (row 1) and a float (row 2)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := inferCSV(t, c.input, 1000, false, "x.v1.M")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
	// --full-scan collects refusals with the contradictions.
	_, err := inferCSV(t, "a,b\n1,x\ny,2\n", 1000, true, "x.v1.M")
	if err == nil || !strings.Contains(err.Error(), "2 contradictions") {
		t.Fatalf("want both columns reported, got %v", err)
	}
}

// #360's done-when: for every accept fixture in testdata/dataset/ that
// infer-schema accepts, `pxf validate` against the schema it wrote
// accepts the fixture. Before, a null, timestamp, duration or bytes cell
// typed its column string, and the schema rejected its own input.
func TestInfer_DatasetCorpusRoundTrip(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(datasetDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	var checked []string
	for _, f := range files {
		name := filepath.Base(f)
		if datasetVerdicts[name] != "" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := loadPXF(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// A document whose own @proto defines the row type is checked
		// against that type, whatever -p says, so it says nothing about
		// the inferred schema.
		if len(doc.protos) > 0 || len(doc.datasets) != 1 {
			continue
		}
		ds := doc.datasets[0]
		rep, err := inferDataset(ds, 1000, false)
		if err != nil {
			t.Logf("%s: refused: %v", name, err)
			continue
		}
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := emitProtoFile(&buf, ds.Type, rep); err != nil {
				t.Fatal(err)
			}
			proto := filepath.Join(t.TempDir(), "inferred.proto")
			if err := os.WriteFile(proto, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := runCLI(t, "validate", "-p", proto, "-m", ds.Type, f); err != nil {
				t.Errorf("the inferred schema rejects its input: %v\n%s", err, buf.String())
			}
		})
		checked = append(checked, name)
	}
	if len(checked) == 0 {
		t.Fatal("no fixture was round-tripped")
	}
	t.Logf("round-tripped %d fixtures: %s", len(checked), strings.Join(checked, ", "))
}

// #360's reproduction: a null in an int column, and timestamp, duration
// and bytes cells. Each column gets a type that binds its cells, and
// `pxf validate` accepts the document against the schema written for it.
func TestInfer_TypedAndNullCells(t *testing.T) {
	doc := "@dataset x.v1.R (n, ts, d, b, s)\n" +
		"(1, 2026-01-01T00:00:00Z, 5s, b\"aGk=\", \"a\")\n" +
		"(null, , , , null)\n" +
		"(3, null, null, , )\n"
	path := filepath.Join(t.TempDir(), "r.pxf")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	ld, err := loadPXF([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := inferDataset(ld.datasets[0], 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := emitProtoFile(&buf, "x.v1.R", rep); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		`import "google/protobuf/duration.proto";`,
		`import "google/protobuf/timestamp.proto";`,
		`import "google/protobuf/wrappers.proto";`,
		"google.protobuf.Int64Value n = 1;",
		"google.protobuf.Timestamp ts = 2;",
		"google.protobuf.Duration d = 3;",
		"optional bytes b = 4;",
		"google.protobuf.StringValue s = 5;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	proto := filepath.Join(t.TempDir(), "r.proto")
	if err := os.WriteFile(proto, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "validate", "-p", proto, "-m", "x.v1.R", path); err != nil {
		t.Errorf("the inferred schema rejects its input: %v", err)
	}

	// A timestamp column with a string in it has no type that holds both.
	_, err = inferDataset(mustDataset(t, "@dataset x.v1.R (ts)\n(2026-01-01T00:00:00Z)\n(\"later\")\n"), 1000, false)
	if err == nil || !strings.Contains(err.Error(), `column "ts" holds a timestamp (row 1) and a string (row 2)`) {
		t.Errorf("want the mixed column refused, got %v", err)
	}
	// An identifier binds only to an enum.
	_, err = inferDataset(mustDataset(t, "@dataset x.v1.R (c)\n(RED)\n"), 1000, false)
	if err == nil || !strings.Contains(err.Error(), `column "c" holds an identifier`) {
		t.Errorf("want the identifier refused, got %v", err)
	}
}

func mustDataset(t *testing.T, doc string) pxf.DatasetDirective {
	t.Helper()
	ld, err := loadPXF([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return ld.datasets[0]
}
