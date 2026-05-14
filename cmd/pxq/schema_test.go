// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProto drops a .proto file in a temp dir and returns its path.
// Each call returns an isolated dir so callers can pass the path
// straight to loadSchema without polluting siblings.
func writeProto(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.proto")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runE2EWithSchema(t *testing.T, query, input string, sch *schema) string {
	t.Helper()
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	results, err := runQuery(query, doc, sch)
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

// --- schema loader ---

func TestSchema_LoadFromProto(t *testing.T) {
	p := writeProto(t, `syntax = "proto3";
package trades.v1;
message Trade {
  string symbol = 1;
  double price = 2;
  int64 qty = 3;
}
`)
	sch, err := loadSchema([]string{p})
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("trades.v1.Trade") == nil {
		t.Error("Trade not found")
	}
	if sch.find("trades.v1.Missing") != nil {
		t.Error("non-existent type should be nil")
	}
}

func TestSchema_NestedMessagesRegistered(t *testing.T) {
	p := writeProto(t, `syntax = "proto3";
package outer.v1;
message Outer {
  message Inner {
    string x = 1;
  }
  Inner inner = 1;
}
`)
	sch, err := loadSchema([]string{p})
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("outer.v1.Outer.Inner") == nil {
		t.Error("nested Inner not registered")
	}
}

func TestSchema_EmptyArgsReturnsNil(t *testing.T) {
	sch, err := loadSchema(nil)
	if err != nil {
		t.Fatal(err)
	}
	if sch != nil {
		t.Error("expected nil schema for empty arg list")
	}
}

func TestSchema_CompileErrorPropagates(t *testing.T) {
	p := writeProto(t, "this is not a valid proto file\n")
	_, err := loadSchema([]string{p})
	if err == nil {
		t.Error("expected compile error")
	}
}

// --- pxf_directive ---

func TestPxfDirective_Dataset_Unschemaed(t *testing.T) {
	input := `@dataset trades.v1.Trade ( symbol, qty )
( "AAPL", 100 )
( "MSFT", 50 )
`
	got := runE2EWithSchema(t,
		`pxf_directive("dataset")[0].rows | map(.symbol)`,
		input, nil)
	if !strings.Contains(got, `["AAPL", "MSFT"]`) {
		t.Errorf("expected symbol list: %s", got)
	}
}

func TestPxfDirective_Proto_FromDocument(t *testing.T) {
	input := `@proto trades.v1.Trade { string symbol = 1; }
@dataset trades.v1.Trade ( symbol )
( "AAPL" )
`
	got := runE2EWithSchema(t,
		`pxf_directive("proto")[0] | { shape, typeName }`,
		input, nil)
	if !strings.Contains(got, `shape = "named"`) {
		t.Errorf("expected named shape: %s", got)
	}
	if !strings.Contains(got, `typeName = "trades.v1.Trade"`) {
		t.Errorf("expected typeName: %s", got)
	}
}

func TestPxfDirective_Generic_FromDocument(t *testing.T) {
	input := `@header pkg.Header { id = "h1" }
body = "x"
`
	got := runE2EWithSchema(t,
		`pxf_directive("header")[0].type`,
		input, nil)
	if !strings.Contains(got, `"pkg.Header"`) {
		t.Errorf("expected type=pkg.Header: %s", got)
	}
}

func TestPxfDirective_Type(t *testing.T) {
	input := "@type trades.v1.Trade\nsymbol = \"AAPL\"\n"
	got := runE2EWithSchema(t, `pxf_directive("type")[0].value`, input, nil)
	if !strings.Contains(got, `"trades.v1.Trade"`) {
		t.Errorf("expected type URL: %s", got)
	}
}

func TestPxfDirective_Missing_ReturnsEmptyList(t *testing.T) {
	input := "name = \"x\"\n"
	got := runE2EWithSchema(t, `pxf_directive("dataset") | length`, input, nil)
	if !strings.Contains(got, "value = 0") {
		t.Errorf("expected empty list for absent directive: %s", got)
	}
}

// --- pxf_fieldnames ---

func TestPxfFieldnames_NoSchemaErrors(t *testing.T) {
	input := `@type trades.v1.Trade
symbol = "AAPL"
`
	doc, _ := loadPXF([]byte(input))
	_, err := runQuery("pxf_fieldnames", doc, nil)
	if err != nil {
		// Compile-time error is also acceptable.
		return
	}
	// If no compile error, the runtime should still have failed; check
	// output for the error string.
	got := runE2EWithSchema(t, "pxf_fieldnames // \"errored\"", input, nil)
	if !strings.Contains(got, `"errored"`) {
		t.Errorf("expected runtime error → loose-mode fallback: %s", got)
	}
}

func TestPxfFieldnames_WithSchema(t *testing.T) {
	p := writeProto(t, `syntax = "proto3";
package trades.v1;
message Trade {
  string symbol = 1;
  double price = 2;
  int64 qty = 3;
}
`)
	sch, err := loadSchema([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	input := `@type trades.v1.Trade
symbol = "AAPL"
`
	got := runE2EWithSchema(t,
		`pxf_fieldnames | sort`,
		input, sch)
	for _, want := range []string{`"symbol"`, `"price"`, `"qty"`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in field names, got: %s", want, got)
		}
	}
}

// --- pxf_type ---

func TestPxfType_LooseValues(t *testing.T) {
	cases := []struct {
		q, want string
	}{
		{`.s | pxf_type`, `"string"`},
		{`.i | pxf_type`, `"int"`},
		{`.f | pxf_type`, `"float"`},
		{`.b | pxf_type`, `"bool"`},
		{`.l | pxf_type`, `"list"`},
		{`.m | pxf_type`, `"object"`},
	}
	input := `s = "x"
i = 1
f = 1.5
b = true
l = [1, 2]
m { k = 1 }
`
	for _, c := range cases {
		got := runE2EWithSchema(t, c.q, input, nil)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s, got %s", c.q, c.want, got)
		}
	}
}

// --- pxf_has ---

func TestPxfHas_SchemaAware(t *testing.T) {
	input := `@dataset x.Row ( a, b, c )
( 1, , 3 )
`
	// Empty cell (absent) → has() false.
	got := runE2EWithSchema(t,
		`pxf_directive("dataset")[0].rows[0] | pxf_has("b")`,
		input, nil)
	if !strings.Contains(got, "false") {
		t.Errorf("absent b: got %s", got)
	}
	// Present cell → true.
	got = runE2EWithSchema(t,
		`pxf_directive("dataset")[0].rows[0] | pxf_has("a")`,
		input, nil)
	if !strings.Contains(got, "true") {
		t.Errorf("present a: got %s", got)
	}
}

// --- pxf_proto ---

func TestPxfProto_NoSchemaErrors(t *testing.T) {
	input := "x = 1\n"
	// Loose-mode fallback via alt-op should surface "errored".
	got := runE2EWithSchema(t,
		`pxf_proto("trades.v1.Trade"; {symbol: "AAPL"}) // "errored"`,
		input, nil)
	if !strings.Contains(got, `"errored"`) {
		t.Errorf("expected loose-mode fallback when no schema: %s", got)
	}
}

func TestPxfProto_BindsTypedObject(t *testing.T) {
	p := writeProto(t, `syntax = "proto3";
package trades.v1;
message Trade {
  string symbol = 1;
  double price = 2;
}
`)
	sch, err := loadSchema([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	input := `t = "AAPL"
p = 188.42
`
	got := runE2EWithSchema(t,
		`pxf_proto("trades.v1.Trade"; {symbol: .t, price: .p})`,
		input, sch)
	// Output should carry the @type directive that the emitter lifts
	// from the constructed object's @type sentinel.
	if !strings.Contains(got, `@type trades.v1.Trade`) {
		t.Errorf("expected @type directive: %s", got)
	}
	if !strings.Contains(got, `symbol = "AAPL"`) {
		t.Errorf("expected symbol: %s", got)
	}
	if !strings.Contains(got, `price = 188.42`) {
		t.Errorf("expected price: %s", got)
	}
}

func TestPxfProto_UnregisteredTypeErrors(t *testing.T) {
	p := writeProto(t, `syntax = "proto3"; package x.v1; message A { }
`)
	sch, _ := loadSchema([]string{p})
	input := "n = 1\n"
	got := runE2EWithSchema(t,
		`pxf_proto("not.a.real.Type"; {}) // "errored"`,
		input, sch)
	if !strings.Contains(got, `"errored"`) {
		t.Errorf("expected error for unknown type: %s", got)
	}
}

// --- pipe form for pxf_proto ---

func TestPxfProto_PipeForm(t *testing.T) {
	p := writeProto(t, `syntax = "proto3"; package x.v1; message A { string s = 1; }
`)
	sch, _ := loadSchema([]string{p})
	input := "t = \"hello\"\n"
	got := runE2EWithSchema(t,
		`{s: .t} | pxf_proto("x.v1.A"; .)`,
		input, sch)
	if !strings.Contains(got, `s = "hello"`) {
		t.Errorf("pipe form: %s", got)
	}
}
