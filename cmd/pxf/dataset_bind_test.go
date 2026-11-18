// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDoc writes doc to a file and returns its path.
func writeDoc(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pxf")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const rowsQuery = `pxf_directive("dataset")[0].rows`

// selfDescribing is a @dataset whose only type is the anonymous @proto
// before it (draft §3.4.5): nothing names the row type, so no -m can.
const selfDescribing = "@proto {\n  string sym = 1;\n  double px  = 2;\n}\n@dataset (sym, px)\n"

// query binds a typed @dataset's rows as validate does, so a cell that
// does not fit its field fails with validate's row and column error.
// Before #618, query printed the string "x" in the double column px and
// exited 0, while validate rejected the same document.
func TestDataset_QueryRejectsCellThatDoesNotBind(t *testing.T) {
	bad := "@dataset test.v1.AllTypes (string_field, int32_field)\n(\"ok\", 1)\n(\"bad\", \"not-an-int\")\n"
	path := writeDoc(t, bad)
	const want = `3:1: column int32_field: expected integer for field "int32_field"`

	_, err := runCLI(t, "validate", "-p", testProto(), "-m", "test.v1.AllTypes", path)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("validate: want %q, got %v", want, err)
	}
	out, err := runCLI(t, "query", "-p", testProto(), "-m", "test.v1.AllTypes", rowsQuery, path)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("query: want %q, got err %v and output %q", want, err, out)
	}

	good := writeDoc(t, "@dataset test.v1.AllTypes (string_field, int32_field)\n(\"ok\", 1)\n")
	out, err = runCLI(t, "query", "-p", testProto(), "-m", "test.v1.AllTypes", rowsQuery, good)
	if err != nil || !strings.Contains(out, "int32_field = 1") {
		t.Fatalf("query on rows that bind: err %v, output %q", err, out)
	}
}

// A document that carries its own @proto runs strict with no flags
// (QUERY.md, "Two modes"), so a row that does not bind fails the run.
// Before #618 such a document ran loose, since a @dataset document has
// no @type, and the row read as null with exit 0.
func TestDataset_QuerySelfDescribingRunsStrict(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"anonymous @proto", selfDescribing + "(\"A\", \"x\")\n", `6:1: column px: expected number for field "px"`},
		{"named @proto", "@proto t.v1.T { string sym = 1; double px = 2; }\n@dataset t.v1.T (sym, px)\n(\"A\", \"x\")\n", `3:1: column px: expected number for field "px"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, "query", rowsQuery, writeDoc(t, tc.doc))
			if err == nil || !strings.Contains(err.Error(), "strict-mode: runtime error: "+tc.want) {
				t.Fatalf("want a strict-mode error %q, got err %v and output %q", tc.want, err, out)
			}
		})
	}

	// --loose still turns the error into null, as loose mode does for
	// any runtime error.
	out, err := runCLI(t, "query", "--loose", rowsQuery, writeDoc(t, selfDescribing+"(\"A\", \"x\")\n"))
	if err != nil || !strings.Contains(out, "null") {
		t.Fatalf("--loose: want null, got err %v and output %q", err, out)
	}
}

// validate reads a self-describing @dataset with no flags: the rows
// carry their type, and a @dataset document has no body for -m to type.
// Before #618 it stopped at "--message is required".
func TestDataset_ValidateSelfDescribingNeedsNoMessage(t *testing.T) {
	if err := validateDoc(t, selfDescribing+"(\"A\", 1.5)\n"); err != nil {
		t.Fatalf("a well-typed self-describing dataset: %v", err)
	}
	err := validateDoc(t, selfDescribing+"(\"A\", 1.5)\n(\"B\", \"x\")\n")
	if want := `7:1: column px: expected number for field "px"`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q, got %v", want, err)
	}
	// A typed header with its schema from -p needs no -m either.
	if err := validateDoc(t, "@dataset test.v1.AllTypes (string_field)\n(\"a\")\n", "-p", testProto()); err != nil {
		t.Fatalf("typed header, -p, no -m: %v", err)
	}
	// An anonymous @proto with no @dataset after it is still unbound.
	err = validateDoc(t, "@proto { string sym = 1; }\n@proto { string a = 1; }\n@dataset (a)\n(\"x\")\n")
	if err == nil || !strings.Contains(err.Error(), "anonymous @proto") {
		t.Fatalf("want the unbound anonymous @proto rejected, got %v", err)
	}
}
