// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// datasetDir is the cross-port @dataset corpus (draft -01 §3.4.4): each
// file's leading comment states its verdict. Driven here through
// `pxf validate`, which parses the document and binds every row to its
// type, against the reference implementation.
const datasetDir = "../../testdata/dataset"

// datasetVerdicts lists every fixture: "" for MUST accept, otherwise a
// substring the rejection must contain. A fixture missing from this
// table fails the test, so a new one cannot go unchecked.
var datasetVerdicts = map[string]string{
	"empty-rows.pxf":              "",
	"rows-end-at-directive.pxf":   "",
	"list-cell.pxf":               "",
	"block-cell.pxf":              "",
	"nested-cells.pxf":            "",
	"cell-framing.pxf":            "",
	"arity-long.pxf":              "3 cells, expected 2",
	"arity-short.pxf":             "2 cells, expected 3",
	"dotted-column.pxf":           "dotted column",
	"junk-between-rows.pxf":       "top-level field entries",
	"truncated-row-eof.pxf":       "expected ',' or ')'",
	"truncated-row-then-rows.pxf": "expected ',' or ')'",
	"with-body.pxf":               "top-level field entries",
	"with-type.pxf":               "@type",
	"block-on-scalar.pxf":         `@dataset row: expected integer for field "int32_field"`,
}

// datasetKnownGaps maps fixtures the pinned protowire-go does not yet meet
// to the change that closes the gap. Each entry is asserted to still
// fail with the old rejection, so the go.mod bump that closes it flips
// the test and forces the entry's removal.
var datasetKnownGaps = map[string]string{
	// Value cells (#341): protowire-go v1.8.1 refuses list and block cells
	// at parse time, with the error below. protowire-go#152 accepts them.
	"list-cell.pxf":       "protowire-go#152",
	"block-cell.pxf":      "protowire-go#152",
	"nested-cells.pxf":    "protowire-go#152",
	"cell-framing.pxf":    "protowire-go#152",
	"block-on-scalar.pxf": "protowire-go#152",
}

const datasetGapRejection = "@dataset cells cannot contain"

func TestDataset_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(datasetDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(datasetVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(datasetVerdicts))
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			want, listed := datasetVerdicts[name]
			if !listed {
				t.Fatalf("%s has no verdict in datasetVerdicts", name)
			}
			_, err := runCLI(t, "validate", "-p", testProto(), "-m", "test.v1.AllTypes", path)
			if gap, known := datasetKnownGaps[name]; known {
				if err == nil || !strings.Contains(err.Error(), datasetGapRejection) {
					t.Fatalf("the pinned protowire-go no longer refuses this fixture as before (got %v); gap %s is closed, remove it from datasetKnownGaps", err, gap)
				}
				t.Skipf("known gap, closed by %s: %v", gap, err)
			}
			switch {
			case want == "" && err != nil:
				t.Fatalf("MUST accept, validate rejected: %v", err)
			case want != "" && err == nil:
				t.Fatalf("MUST reject (%q), validate accepted", want)
			case want != "" && !strings.Contains(err.Error(), want):
				t.Fatalf("rejected for the wrong reason: want %q in %v", want, err)
			}
		})
	}
}

func testProto() string { return filepath.Join("../../testdata", "test.proto") }

// A row is bound to its type, not only parsed: a string in an int32
// column is rejected, naming the row's position in the document.
func TestDataset_ValidateBindsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-row.pxf")
	doc := "@dataset test.v1.AllTypes (string_field, int32_field)\n(\"ok\", 1)\n(\"bad\", \"not-an-int\")\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runCLI(t, "validate", "-p", testProto(), "-m", "test.v1.AllTypes", path)
	if err == nil {
		t.Fatal("validate accepted a string in an int32 column")
	}
	if want := `3:1: @dataset row: expected integer for field "int32_field"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q in %v", want, err)
	}
}
