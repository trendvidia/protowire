// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"strings"
	"testing"
)

// runE2EWithFormat is the format-aware sibling of runE2E from main_test.go.
func runE2EWithFormat(t *testing.T, format, query, input string) string {
	t.Helper()
	doc, err := loadByFormat(format, []byte(input))
	if err != nil {
		t.Fatalf("loadByFormat(%s): %v", format, err)
	}
	results, err := runQuery(query, doc, nil, strictOpts{})
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	var sb strings.Builder
	for _, r := range results {
		if err := emitPXF(&sb, r); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}
	return sb.String()
}

// --- JSON ---

func TestJSON_IntVsFloat_PreservesLexical(t *testing.T) {
	input := `{"i": 1, "f": 1.0, "s": "1", "e": 1e2}`
	if got := runE2EWithFormat(t, "json", ".i", input); !strings.Contains(got, "value = 1\n") {
		t.Errorf("i: %s", got)
	}
	// 1.0 should round-trip as a float; 'g' formatting of 1.0 is "1"
	// so we re-check via type: a number that participates in /2 must
	// stay a float (jq's / always yields float on float operands).
	if got := runE2EWithFormat(t, "json", ".f | . * 2.0", input); !strings.Contains(got, "value = 2") {
		t.Errorf("f*2: %s", got)
	}
	if got := runE2EWithFormat(t, "json", ".s", input); !strings.Contains(got, `"1"`) {
		t.Errorf("s: %s", got)
	}
	if got := runE2EWithFormat(t, "json", ".e", input); !strings.Contains(got, "100") {
		t.Errorf("e: %s", got)
	}
}

func TestJSON_EmptyStringNotNull(t *testing.T) {
	input := `{"empty": "", "absent": null}`
	if got := runE2EWithFormat(t, "json", ".empty | type", input); !strings.Contains(got, `"string"`) {
		t.Errorf("empty is not string: %s", got)
	}
	if got := runE2EWithFormat(t, "json", ".absent | type", input); !strings.Contains(got, `"null"`) {
		t.Errorf("absent is not null: %s", got)
	}
}

func TestJSON_BigInt(t *testing.T) {
	// 2^65 — doesn't fit in int64.
	input := `{"huge": 36893488147419103232}`
	got := runE2EWithFormat(t, "json", ".huge", input)
	if !strings.Contains(got, "36893488147419103232") {
		t.Errorf("expected big.Int passthrough: %s", got)
	}
}

func TestJSON_NestedArrayObject(t *testing.T) {
	input := `{"endpoints": [{"path": "/x"}, {"path": "/y"}]}`
	got := runE2EWithFormat(t, "json", ".endpoints[0].path", input)
	if !strings.Contains(got, `"/x"`) {
		t.Errorf("expected /x: %s", got)
	}
}

// --- YAML ---

func TestYAML_NoLegacyBoolCoercion(t *testing.T) {
	// YAML 1.2 Core Schema: yes/no/on/off are strings, not bools.
	input := "yes_v: yes\nno_v: no\non_v: on\noff_v: off\ntrue_v: true\n"
	for _, k := range []string{".yes_v", ".no_v", ".on_v", ".off_v"} {
		got := runE2EWithFormat(t, "yaml", k+" | type", input)
		if !strings.Contains(got, `"string"`) {
			t.Errorf("%s should be string: %s", k, got)
		}
	}
	if got := runE2EWithFormat(t, "yaml", ".true_v | type", input); !strings.Contains(got, `"boolean"`) {
		t.Errorf(".true_v should be boolean: %s", got)
	}
}

func TestYAML_ExplicitTagsAuthoritative(t *testing.T) {
	input := `i: !!str 1
n: !!int "01"
`
	if got := runE2EWithFormat(t, "yaml", ".i | type", input); !strings.Contains(got, `"string"`) {
		t.Errorf(".i should be string under !!str tag: %s", got)
	}
	if got := runE2EWithFormat(t, "yaml", ".n | type", input); !strings.Contains(got, `"number"`) {
		t.Errorf(".n should be number under !!int tag: %s", got)
	}
}

func TestYAML_NumericIntFloatSplit(t *testing.T) {
	input := "i: 1\nf: 1.0\n"
	// 'g' formatting of 1.0 is "1"; identify the type instead.
	if got := runE2EWithFormat(t, "yaml", ".i | type", input); !strings.Contains(got, `"number"`) {
		t.Errorf("i type: %s", got)
	}
	if got := runE2EWithFormat(t, "yaml", ".f | . * 2.0", input); !strings.Contains(got, "value = 2") {
		t.Errorf("f*2: %s", got)
	}
}

func TestYAML_NestedSequenceMap(t *testing.T) {
	input := `services:
  - name: api
    port: 8080
  - name: web
    port: 80
`
	got := runE2EWithFormat(t, "yaml", ".services | map(.port)", input)
	if !strings.Contains(got, "[8080, 80]") {
		t.Errorf("expected port list, got: %s", got)
	}
}

// --- CSV ---

func TestCSV_PerCellClassification(t *testing.T) {
	input := "name,age,price,active\nAlice,30,1.5,true\nBob,25,2.0,false\n"
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows[0].age`, input); !strings.Contains(got, "30") {
		t.Errorf("age: %s", got)
	}
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows[0].active`, input); !strings.Contains(got, "true") {
		t.Errorf("active: %s", got)
	}
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows[0].name`, input); !strings.Contains(got, `"Alice"`) {
		t.Errorf("name: %s", got)
	}
}

func TestCSV_EmptyCellIsAbsent(t *testing.T) {
	// Row 1 has an empty middle column; `has()` should report false
	// for it (consistent with @dataset empty-cell semantic).
	input := "a,b,c\n1,,3\n"
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows[0] | has("b")`, input); !strings.Contains(got, "false") {
		t.Errorf("empty cell should be absent: %s", got)
	}
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows[0] | has("a")`, input); !strings.Contains(got, "true") {
		t.Errorf("present cell should be present: %s", got)
	}
}

func TestCSV_HeaderRow_Default(t *testing.T) {
	input := "symbol,price\nAAPL,188.42\nMSFT,415.10\n"
	got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows | map(.symbol)`, input)
	if !strings.Contains(got, `["AAPL", "MSFT"]`) {
		t.Errorf("expected symbol list, got: %s", got)
	}
}

func TestCSV_DatasetWrapperShape(t *testing.T) {
	input := "x,y\n1,2\n"
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].columns`, input); !strings.Contains(got, `["x", "y"]`) {
		t.Errorf("columns: %s", got)
	}
	if got := runE2EWithFormat(t, "csv", `pxf_directive("dataset")[0].rows | length`, input); !strings.Contains(got, "1") {
		t.Errorf("rows length: %s", got)
	}
}

// --- Format detection ---

func TestFormat_FromExtension(t *testing.T) {
	cases := []struct {
		path, want string
	}{
		{"x.pxf", "pxf"},
		{"x.json", "json"},
		{"x.yaml", "yaml"},
		{"x.yml", "yaml"},
		{"x.csv", "csv"},
	}
	for _, c := range cases {
		got, err := detectFormat(c.path, "")
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.path, got, c.want)
		}
	}
}

func TestFormat_StdinDefaultsToPXF(t *testing.T) {
	got, err := detectFormat("-", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "pxf" {
		t.Errorf("stdin: got %s, want pxf", got)
	}
}

func TestFormat_OverrideWins(t *testing.T) {
	got, err := detectFormat("x.json", "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got != "yaml" {
		t.Errorf("override: got %s, want yaml", got)
	}
}

func TestFormat_UnknownExtensionErrors(t *testing.T) {
	_, err := detectFormat("x.txt", "")
	if err == nil {
		t.Error("expected error for unknown extension")
	}
}

func TestFormat_UnknownOverrideErrors(t *testing.T) {
	_, err := detectFormat("x.json", "xml")
	if err == nil {
		t.Error("expected error for unknown --format")
	}
}
