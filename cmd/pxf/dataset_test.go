// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// datasetDir is the cross-port @dataset corpus (draft -01 §3.4.4): each
// file's leading comment states its verdict. Driven here against the
// reference implementation three ways: through `pxf validate`, which
// parses the document and binds every row to its type; through the
// streaming DatasetReader, which must yield the rows the materializing
// parse does; and, for accept fixtures, through `pxf query`.
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
	"junk-between-rows.pxf":       "top-level field entries",
	"truncated-row-eof.pxf":       "expected ',' or ')'",
	"truncated-row-then-rows.pxf": "expected ',' or ')'",
	"with-body.pxf":               "top-level field entries",
	"with-type.pxf":               "@type",
	"block-on-scalar.pxf":         `expected integer for field "int32_field"`,
	// A raw LF in a simple string (#350), here in a cell.
	"string-newline-in-cell.pxf": "newline in string literal",
	// null on a plain scalar (#339), here in a cell.
	"null-on-scalar-cell.pxf": `null is not allowed on scalar field "int32_field"`,
	// Column paths (#342). Row type dataset.v1.Event in paths.proto
	// unless the fixture says otherwise.
	"dotted-column.pxf":               "",
	"dotted-grouping.pxf":             "",
	"dotted-null-leaf.pxf":            "",
	"dotted-oneof.pxf":                "",
	"dotted-required.pxf":             "",
	"dotted-with-value-cells.pxf":     "",
	"dotted-oneof-two-members.pxf":    "conflicts with already-set field",
	"dotted-through-repeated.pxf":     `"picks" is a repeated field`,
	"dotted-through-map.pxf":          `"by_name" is a map field`,
	"dotted-into-timestamp.pxf":       `"ts" is a google.protobuf.Timestamp`,
	"dotted-into-wrapper.pxf":         `"wrap" is a google.protobuf.Int32Value`,
	"dotted-overlap.pxf":              `column "desc.energy" overlaps column "desc"`,
	"duplicate-column.pxf":            `column "string_field" appears twice`,
	"dotted-required-missing.pxf":     `required field "req.n" is absent`,
	"dotted-empty-segment.pxf":        "a column path has an empty segment",
	"dotted-trailing-dot.pxf":         "a column path has an empty segment",
	"dotted-unknown-segment.pxf":      `has no field "nope"`,
	"dotted-through-scalar.pxf":       `"plain" is not a message field`,
	"dotted-into-duration.pxf":        `"dur" is a google.protobuf.Duration`,
	"dotted-into-bignum.pxf":          `"big" is a pxf.BigInt`,
	"dotted-overlap-deep.pxf":         `column "desc.tempo.min_bpm" overlaps column "desc.tempo"`,
	"dotted-bad-header-empty-row.pxf": `"picks" is a repeated field`,
	// The follow-ups of #342 (#349).
	"dotted-into-secret.pxf":        "",
	"dotted-into-any.pxf":           "",
	"dotted-bad-header-no-rows.pxf": "",
}

// datasetKnownGaps maps fixtures the pinned protowire-go does not yet meet
// to the change that closes the gap and what the pinned release does
// with the fixture instead. Each entry is asserted to still behave that
// way, so the go.mod bump that closes it flips the test and forces the
// entry's removal. Empty since protowire-go v1.10.0, which binds dotted
// column paths (protowire-go#159) and rejects a raw newline in a simple
// string (protowire-go#167).
var datasetKnownGaps = map[string]datasetGap{}

// datasetGap is a known gap: by is the change that closes it, and old is
// a substring of the pinned release's rejection, or "" if it accepts. An
// accepting gap is allowed only on a reject fixture: on an accept
// fixture the fixed release accepts too, so the bump would never flip
// the test.
type datasetGap struct{ by, old string }

// datasetStreamGaps are fixtures the pinned DatasetReader streams through
// without error where the verdict is rejection: it ends the row sequence
// at a broken row instead of reporting it. Each is asserted to still be
// accepted, so the bump that fixes the reader forces the entry's removal.
// Empty since protowire-go v1.9.0, which reports broken rows
// (protowire-go#141).
var datasetStreamGaps = map[string]string{}

func TestDataset_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(datasetDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(datasetVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(datasetVerdicts))
	}
	sch, err := loadSchema([]string{testProto(), pathsProto()}, nil, registryRef{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		want, listed := datasetVerdicts[name]
		if !listed {
			t.Errorf("%s has no verdict in datasetVerdicts", name)
			continue
		}
		t.Run("validate/"+name, func(t *testing.T) {
			_, err := runCLI(t, "validate", "-p", testProto(), "-p", pathsProto(), "-m", "test.v1.AllTypes", path)
			skipKnownGap(t, name, err)
			checkVerdict(t, want, err)
		})
		t.Run("stream/"+name, func(t *testing.T) {
			rows, err := streamDataset(path, sch)
			skipKnownGap(t, name, err)
			if gap, known := datasetStreamGaps[name]; known {
				if err != nil {
					t.Fatalf("the pinned DatasetReader now rejects this fixture (%v); gap %s is closed, remove it from datasetStreamGaps", err, gap)
				}
				t.Skipf("known gap, closed by %s: the reader streams past the broken row", gap)
			}
			checkVerdict(t, want, err)
			if want != "" {
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := pxf.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			var parsed []pxf.DatasetRow
			for _, ds := range doc.Datasets {
				parsed = append(parsed, ds.Rows...)
			}
			if got, want := renderRows(rows), renderRows(parsed); got != want {
				t.Fatalf("streamed rows differ from the materializing parse:\nstream:\n%s\nparse:\n%s", got, want)
			}
		})
		if want != "" {
			continue
		}
		t.Run("query/"+name, func(t *testing.T) {
			out, err := runCLI(t, "query", "-p", testProto(), "-p", pathsProto(), `[pxf_directive("dataset")[] | .rows | length] | add`, path)
			skipKnownGap(t, name, err)
			if err != nil {
				t.Fatalf("MUST accept, query rejected: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := pxf.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, ds := range doc.Datasets {
				n += len(ds.Rows)
			}
			// A dataset that query fails to bind is reported as an error in place of
			// the dataset, with no rows, so the count comes up short.
			if want := fmt.Sprintf("value = %d", n); strings.TrimSpace(out) != want {
				t.Fatalf("query counted %q rows, want %q", strings.TrimSpace(out), want)
			}
		})
	}
}

// skipKnownGap skips a fixture listed in datasetKnownGaps after checking
// that the pinned protowire-go still treats it as the entry records:
// refused with the old rejection, or accepted when old is "".
func skipKnownGap(t *testing.T, name string, err error) {
	t.Helper()
	gap, known := datasetKnownGaps[name]
	if !known {
		return
	}
	if gap.old == "" && datasetVerdicts[name] == "" {
		t.Fatalf("known gap %s records an acceptance on an accept fixture, which the fixed release meets too; record the old rejection instead", gap.by)
	}
	asBefore := err == nil
	if gap.old != "" {
		asBefore = err != nil && strings.Contains(err.Error(), gap.old)
	}
	if !asBefore {
		t.Fatalf("the pinned protowire-go no longer treats this fixture as before (got %v); gap %s is closed, remove it from datasetKnownGaps", err, gap.by)
	}
	t.Skipf("known gap, closed by %s: %v", gap.by, err)
}

func checkVerdict(t *testing.T, want string, err error) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("MUST accept, rejected: %v", err)
	case want != "" && err == nil:
		t.Fatalf("MUST reject (%q), accepted", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("rejected for the wrong reason: want %q in %v", want, err)
	}
}

// streamDataset reads every @dataset in path through a DatasetReader,
// binding each row with Scan, and returns the rows in the order read.
func streamDataset(path string, sch *schema) ([]pxf.DatasetRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []pxf.DatasetRow
	var src io.Reader = bytes.NewReader(data)
	for first := true; ; first = false {
		tr, err := pxf.NewDatasetReader(src)
		if errors.Is(err, pxf.ErrNoDataset) && !first {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		md := sch.find(tr.Type())
		if md == nil {
			return nil, fmt.Errorf("@dataset %s: type not found", tr.Type())
		}
		for {
			row, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if err := pxf.BindRow(dynamicpb.NewMessage(md), tr.Columns(), row); err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
		src = tr.Tail()
	}
}

// renderRows prints rows one per line, each cell as its PXF literal and
// an empty cell as nothing, so two row sequences compare as text.
func renderRows(rows []pxf.DatasetRow) string {
	var b strings.Builder
	for _, r := range rows {
		for i, c := range r.Cells {
			if i > 0 {
				b.WriteString(" | ")
			}
			if c != nil {
				b.Write(pxf.FormatValue(c))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func testProto() string { return filepath.Join("../../testdata", "test.proto") }

// pathsProto holds dataset.v1.Event, the row type of the dotted-column
// fixtures.
func pathsProto() string { return filepath.Join(datasetDir, "paths.proto") }

// validateDoc writes doc to a file and runs validate on it.
func validateDoc(t *testing.T, doc string, args ...string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pxf")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runCLI(t, append(append([]string{"validate"}, args...), path)...)
	return err
}

// A row is bound to its type, not only parsed: a string in an int32
// column is rejected, naming the row's position in the document and the
// column.
func TestDataset_ValidateBindsRows(t *testing.T) {
	doc := "@dataset test.v1.AllTypes (string_field, int32_field)\n(\"ok\", 1)\n(\"bad\", \"not-an-int\")\n"
	err := validateDoc(t, doc, "-p", testProto(), "-m", "test.v1.AllTypes")
	if err == nil {
		t.Fatal("validate accepted a string in an int32 column")
	}
	if want := `3:1: column int32_field: expected integer for field "int32_field"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q in %v", want, err)
	}
}

// The error names the column it fell in, which the row's position alone
// does not: here the bad cell is the third of four non-empty ones.
func TestDataset_ValidateNamesTheColumn(t *testing.T) {
	doc := "@dataset test.v1.AllTypes (string_field, int32_field, int64_field, bool_field)\n(\"a\", 1, \"x\", true)\n"
	err := validateDoc(t, doc, "-p", testProto(), "-m", "test.v1.AllTypes")
	if err == nil || !strings.Contains(err.Error(), "2:1: column int64_field: ") {
		t.Fatalf("want the error at row 2:1 naming int64_field, got %v", err)
	}
}

// An empty cell in a (pxf.required) column is an absent required field,
// which draft §3.4.4's three-state rule says MUST be rejected. --message
// is a type with no required field, so only the row can fail.
func TestDataset_ValidateRejectsEmptyRequiredCell(t *testing.T) {
	args := []string{"-p", annotationsProto(), "-p", testProto(), "-m", "test.v1.AllTypes"}
	doc := "@dataset settings.v1.Settings (name, retries)\n(\"a\", 1)\n( , 2)\n"
	err := validateDoc(t, doc, args...)
	if err == nil {
		t.Fatal("validate accepted an empty cell in a (pxf.required) column")
	}
	if want := `3:1: required field "name" is absent`; !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q in %v", want, err)
	}
	// A null cell is present-but-null, which satisfies (pxf.required) on a
	// field that can hold null; protowire-go's TestBindRow_Required pins
	// that on a StringValue. "name" is a plain string, which cannot hold
	// null (draft § Booleans, Null, and Identifier Values; #339), so the
	// row is rejected for the null, not for the missing value.
	doc = "@dataset settings.v1.Settings (name, retries)\n(null, 1)\n"
	err = validateDoc(t, doc, args...)
	if want := `2:1: column name: null is not allowed on scalar field "name"`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want %q, got %v", want, err)
	}
}

// A @dataset document has no body, so the --message type's required
// fields are not absent from it: the rows carry them.
func TestDataset_ValidateIgnoresMessageRequiredOnDatasetDocument(t *testing.T) {
	doc := "@dataset settings.v1.Settings (name)\n(\"a\")\n"
	if err := validateDoc(t, doc, "-p", annotationsProto(), "-m", "settings.v1.Settings"); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// A row type defined by the document's own @proto resolves, as it does
// for `pxf query`.
func TestDataset_ValidateResolvesInDocumentProto(t *testing.T) {
	schema := "@proto \"\"\"\nsyntax = \"proto3\";\npackage t.v1;\nmessage Trade { string sym = 1; double px = 2; }\n\"\"\"\n"
	doc := schema + "@dataset t.v1.Trade (sym, px)\n(\"A\", 1.5)\n"
	if err := validateDoc(t, doc, "-m", "test.v1.AllTypes", "-p", testProto()); err != nil {
		t.Fatalf("validate: %v", err)
	}
	doc = schema + "@dataset t.v1.Trade (sym, px)\n(\"A\", \"x\")\n"
	if err := validateDoc(t, doc, "-m", "test.v1.AllTypes", "-p", testProto()); err == nil || !strings.Contains(err.Error(), "column px") {
		t.Fatalf("want a binding error on column px, got %v", err)
	}
}

// An untyped @dataset binds to the anonymous @proto before it, not to
// --message: here --message has neither column.
func TestDataset_ValidateBindsAnonymousProto(t *testing.T) {
	schema := "@proto { string sym = 1; double px = 2; }\n"
	doc := schema + "@dataset (sym, px)\n(\"A\", 1.5)\n"
	if err := validateDoc(t, doc, "-m", "test.v1.AllTypes", "-p", testProto()); err != nil {
		t.Fatalf("validate: %v", err)
	}
	doc = schema + "@dataset (sym, px)\n(\"A\", \"x\")\n"
	if err := validateDoc(t, doc, "-m", "test.v1.AllTypes", "-p", testProto()); err == nil || !strings.Contains(err.Error(), "column px") {
		t.Fatalf("want a binding error on column px, got %v", err)
	}
}

// query types a dotted column's cell by the leaf field its path names
// (#348). desc.energy is a double, so a bare `inf` cell becomes +Inf; a
// lookup by the whole column name finds no field and leaves the string
// "inf". The row key stays the column name.
func TestDataset_QueryTypesDottedColumnByLeaf(t *testing.T) {
	sch, err := loadSchema([]string{pathsProto()}, nil, registryRef{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := pxf.Parse([]byte("@dataset dataset.v1.Event (id, desc.energy)\n(\"e1\", inf)\n"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := datasetToValueWithSchema(res.Datasets[0], sch)
	if err != nil {
		t.Fatal(err)
	}
	row := v.(map[string]any)["rows"].([]any)[0].(map[string]any)
	if f, ok := row["desc.energy"].(float64); !ok || !math.IsInf(f, 1) {
		t.Fatalf(`row["desc.energy"] = %#v, want +Inf`, row["desc.energy"])
	}
}

// A path into a pxf.Secret or a google.protobuf.Any binds the fields the
// type's block form names (#349): a Secret's value and hint, an Any's
// own type_url and value. Each fixture's first row is the body its
// comment states.
func TestDataset_PathsIntoSecretAndAny(t *testing.T) {
	sch, err := loadSchema([]string{pathsProto()}, nil, registryRef{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ fixture, body string }{
		{"dotted-into-secret.pxf", `id = "x" pw { value = "s3cret" hint = "db password" }`},
		{"dotted-into-any.pxf", `id = "x" payload { type_url = "type.googleapis.com/dataset.v1.Pick" value = b"CgF4" }`},
	} {
		data, err := os.ReadFile(filepath.Join(datasetDir, tc.fixture))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := pxf.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		ds := doc.Datasets[0]
		md := sch.find(ds.Type)
		if md == nil {
			t.Fatalf("%s: type %s not found", tc.fixture, ds.Type)
		}
		got := dynamicpb.NewMessage(md)
		if err := pxf.BindRow(got, ds.Columns, ds.Rows[0]); err != nil {
			t.Fatalf("%s: %v", tc.fixture, err)
		}
		want := dynamicpb.NewMessage(md)
		if err := pxf.Unmarshal([]byte(tc.body), want); err != nil {
			t.Fatalf("%s: the expected body: %v", tc.fixture, err)
		}
		if !proto.Equal(got, want) {
			t.Fatalf("%s: bound %v, want %v", tc.fixture, got, want)
		}
	}
}
