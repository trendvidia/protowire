// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Loose-mode hint goes to stderr in production; silence it during
	// tests so the output a test inspects is just the query result.
	errSink = io.Discard
	m.Run()
}

func runE2E(t *testing.T, query, input string) string {
	t.Helper()
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	results, err := runQuery(query, doc, nil, strictOpts{})
	if err != nil {
		t.Fatalf("runQuery %q: %v", query, err)
	}
	var buf bytes.Buffer
	for _, r := range results {
		if err := emitPXF(&buf, r); err != nil {
			t.Fatalf("emitPXF: %v", err)
		}
	}
	return buf.String()
}

func TestRoundTrip_Identity(t *testing.T) {
	input := `name = "Alice"
age = 30
`
	got := runE2E(t, ".", input)
	// Field order is sorted on emission, so we compare key/value content
	// rather than byte-equal.
	for _, want := range []string{`name = "Alice"`, `age = 30`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestQuery_FieldAccess(t *testing.T) {
	input := `name = "Alice"
age = 30
`
	if got := runE2E(t, ".name", input); !strings.Contains(got, `"Alice"`) {
		t.Errorf("expected Alice in output: %s", got)
	}
	if got := runE2E(t, ".age", input); !strings.Contains(got, `30`) {
		t.Errorf("expected 30 in output: %s", got)
	}
}

func TestQuery_ListLength(t *testing.T) {
	input := `tags = ["a", "b", "c"]
`
	got := runE2E(t, ".tags | length", input)
	if !strings.Contains(got, `3`) {
		t.Errorf("expected 3, got: %s", got)
	}
}

func TestQuery_NestedBlock(t *testing.T) {
	input := `config {
  host = "localhost"
  port = 8080
}
`
	got := runE2E(t, ".config.port", input)
	if !strings.Contains(got, "8080") {
		t.Errorf("expected 8080: %s", got)
	}
}

func TestQuery_Dataset_FilterAndCount(t *testing.T) {
	input := `@dataset trades.v1.Trade ( symbol, price, qty )
( "AAPL", 188.42, 100 )
( "MSFT", 415.10, 50 )
( "AAPL", 188.55, 75 )
`
	got := runE2E(t,
		`pxf_directive("dataset")[0].rows | map(select(.symbol == "AAPL")) | length`,
		input)
	if !strings.Contains(got, "2") {
		t.Errorf("expected 2 AAPL rows, got: %s", got)
	}
}

func TestQuery_Dataset_EmptyAndNullCells(t *testing.T) {
	input := `@dataset x.Row ( a, b, c )
( 1, , 3 )
( 4, null, 6 )
`
	// First row: b is absent (empty cell) → missing key.
	got := runE2E(t, `pxf_directive("dataset")[0].rows[0] | has("b")`, input)
	if !strings.Contains(got, "false") {
		t.Errorf("expected absent b → has() false: %s", got)
	}
	// Second row: b is present (null literal) → present-with-null.
	got = runE2E(t, `pxf_directive("dataset")[0].rows[1] | has("b")`, input)
	if !strings.Contains(got, "true") {
		t.Errorf("expected null b → has() true: %s", got)
	}
	got = runE2E(t, `pxf_directive("dataset")[0].rows[1].b`, input)
	if !strings.Contains(got, "null") {
		t.Errorf("expected null b value, got: %s", got)
	}
}

func TestQuery_Proto_Directive_Exposed(t *testing.T) {
	input := `@proto trades.v1.Trade { string symbol = 1; }
@dataset trades.v1.Trade ( symbol )
( "AAPL" )
`
	got := runE2E(t, `pxf_directive("proto")[0].shape`, input)
	if !strings.Contains(got, `"named"`) {
		t.Errorf("expected named proto shape: %s", got)
	}
	got = runE2E(t, `pxf_directive("proto")[0].typeName`, input)
	if !strings.Contains(got, `"trades.v1.Trade"`) {
		t.Errorf("expected typeName trades.v1.Trade: %s", got)
	}
}

func TestQuery_LooseMode_RuntimeErrorBecomesNull(t *testing.T) {
	input := `age = 30
`
	// Comparing an int to a string is a jq runtime error; loose mode
	// should degrade it to null instead of aborting.
	got := runE2E(t, `.age > "thirty" // "fallback"`, input)
	if !strings.Contains(got, `"fallback"`) {
		t.Errorf("expected fallback via alt-op: %s", got)
	}
}

func TestEmit_Bytes_RoundTrip(t *testing.T) {
	input := `data = b"aGVsbG8="
`
	got := runE2E(t, ".", input)
	if !strings.Contains(got, `data = b"aGVsbG8="`) {
		t.Errorf("expected bytes round-trip: %s", got)
	}
}

func TestEmit_BigInt(t *testing.T) {
	// 2^65 — doesn't fit in int64, exercises the big.Int fall-back in
	// loadPXF and the emitPXF big.Int branch.
	input := `huge = 36893488147419103232
`
	got := runE2E(t, ".huge", input)
	if !strings.Contains(got, "36893488147419103232") {
		t.Errorf("expected big.Int round-trip: %s", got)
	}
}

func TestEmit_NestedBlock(t *testing.T) {
	input := `tls {
  cert = "a.pem"
  key = "a.key"
}
`
	got := runE2E(t, ".", input)
	// Block emitted with indented inner entries.
	if !strings.Contains(got, "cert = \"a.pem\"") {
		t.Errorf("expected inner cert: %s", got)
	}
	if !strings.Contains(got, "tls = {") {
		t.Errorf("expected block: %s", got)
	}
}
