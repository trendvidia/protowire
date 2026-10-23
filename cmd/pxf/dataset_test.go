// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
