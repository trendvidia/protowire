// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"strings"
	"testing"
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
	input := "age\n30\n25\ntwenty\n"
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
	input := "a,b\n1,foo\n2,bar\nx,3\ny,4\n"
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
