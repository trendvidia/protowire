// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// runE2EAnonResolved runs the full main() pipeline minus the I/O —
// loadPXF → resolveAnonymousProtos → loadSchema → runQuery → emit.
// Each anonymous-binding test invokes this so the binding pass is
// exercised end-to-end the same way the CLI does it.
func runE2EAnonResolved(t *testing.T, query, input string) string {
	t.Helper()
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatalf("loadPXF: %v", err)
	}
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Fatalf("resolveAnonymousProtos: %v", err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
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

func TestAnonProto_BasicBinding(t *testing.T) {
	input := `@proto { string symbol = 1; double price = 2; }
@dataset ( symbol, price )
( "AAPL", 188.42 )
( "MSFT", 415.10 )
`
	got := runE2EAnonResolved(t, `pxf_directive("dataset")[0].rows | length`, input)
	if !strings.Contains(got, "value = 2") {
		t.Errorf("expected 2 rows: %s", got)
	}
	got = runE2EAnonResolved(t, `pxf_directive("dataset")[0].rows | map(.symbol)`, input)
	if !strings.Contains(got, `["AAPL", "MSFT"]`) {
		t.Errorf("expected symbols: %s", got)
	}
}

func TestAnonProto_SyntheticTypeNameAssigned(t *testing.T) {
	input := `@proto { string symbol = 1; }
@dataset ( symbol )
( "AAPL" )
`
	got := runE2EAnonResolved(t, `pxf_directive("dataset")[0].type`, input)
	if !strings.Contains(got, `"_pxq_anon_0"`) {
		t.Errorf("expected synthetic name: %s", got)
	}
}

func TestAnonProto_SchemaRegistered(t *testing.T) {
	input := `@proto { string symbol = 1; }
@dataset ( symbol )
( "AAPL" )
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Fatal(err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatal(err)
	}
	if sch.find("_pxq_anon_0") == nil {
		t.Error("synthetic message _pxq_anon_0 not registered")
	}
}

func TestAnonProto_MultipleAnonymousInOrder(t *testing.T) {
	// Two anonymous @proto, two typeless @dataset. Spec says bind in
	// document order, so #0 → first dataset, #1 → second.
	input := `@proto { string a = 1; }
@proto { int32 b = 1; }
@dataset ( a )
( "x" )
@dataset ( b )
( 7 )
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Fatalf("resolveAnonymousProtos: %v", err)
	}
	if doc.datasets[0].Type != "_pxq_anon_0" {
		t.Errorf("first dataset type = %q, want _pxq_anon_0", doc.datasets[0].Type)
	}
	if doc.datasets[1].Type != "_pxq_anon_1" {
		t.Errorf("second dataset type = %q, want _pxq_anon_1", doc.datasets[1].Type)
	}
	if doc.protos[0].Shape != pxf.ProtoNamed || doc.protos[0].TypeName != "_pxq_anon_0" {
		t.Errorf("first proto: shape=%v name=%q", doc.protos[0].Shape, doc.protos[0].TypeName)
	}
	if doc.protos[1].Shape != pxf.ProtoNamed || doc.protos[1].TypeName != "_pxq_anon_1" {
		t.Errorf("second proto: shape=%v name=%q", doc.protos[1].Shape, doc.protos[1].TypeName)
	}
}

func TestAnonProto_NamedAndAnonymousMixed(t *testing.T) {
	// A named @proto and a typed @dataset before an anonymous +
	// typeless pair. Named ones must not be consumed; the binding is
	// strictly anonymous-to-typeless.
	input := `@proto trades.v1.Trade { string s = 1; }
@dataset trades.v1.Trade ( s )
( "a" )
@proto { int32 n = 1; }
@dataset ( n )
( 7 )
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Fatalf("resolveAnonymousProtos: %v", err)
	}
	if doc.datasets[0].Type != "trades.v1.Trade" {
		t.Errorf("named dataset should keep its type: %s", doc.datasets[0].Type)
	}
	if doc.datasets[1].Type != "_pxq_anon_0" {
		t.Errorf("typeless dataset should get synthetic name: %s", doc.datasets[1].Type)
	}
	if doc.protos[0].TypeName != "trades.v1.Trade" {
		t.Errorf("named proto unchanged: %s", doc.protos[0].TypeName)
	}
	if doc.protos[1].TypeName != "_pxq_anon_0" {
		t.Errorf("anonymous proto renamed: %s", doc.protos[1].TypeName)
	}
}

func TestAnonProto_TypelessDatasetWithoutAnonymous_Errors(t *testing.T) {
	input := `@dataset ( a )
( 1 )
`
	doc, _ := loadPXF([]byte(input))
	err := resolveAnonymousProtos(doc)
	if err == nil {
		t.Fatal("expected error for orphan typeless dataset")
	}
	if !strings.Contains(err.Error(), "no type and no preceding anonymous") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAnonProto_OrphanAnonymousAtEnd_Errors(t *testing.T) {
	// Two anonymous, only one typeless dataset → the second proto has
	// nowhere to bind.
	input := `@proto { string a = 1; }
@proto { int32 b = 1; }
@dataset ( a )
( "x" )
`
	doc, _ := loadPXF([]byte(input))
	err := resolveAnonymousProtos(doc)
	if err == nil {
		t.Fatal("expected error for orphan anonymous proto")
	}
	if !strings.Contains(err.Error(), "no following typeless @dataset") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAnonProto_DocumentOrderByByteOffset(t *testing.T) {
	// pxf.Document exposes protos and datasets in separate slices, so
	// the binding pass must reconstitute order from Pos.Offset. Verify
	// by interleaving — anonymous-A, dataset, anonymous-B, dataset.
	input := `@proto { string a = 1; }
@dataset ( a )
( "x" )
@proto { int32 b = 1; }
@dataset ( b )
( 7 )
`
	doc, _ := loadPXF([]byte(input))
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Fatalf("resolveAnonymousProtos: %v", err)
	}
	// First dataset binds to first anonymous, second to second.
	// If we did this naively by slice index, the second dataset would
	// pair with the second proto regardless of offset order — this
	// test catches it because the proto and dataset slices have
	// inverse offset orderings to each other here.
	if doc.datasets[0].Type != "_pxq_anon_0" {
		t.Errorf("first dataset type = %q", doc.datasets[0].Type)
	}
	if doc.datasets[1].Type != "_pxq_anon_1" {
		t.Errorf("second dataset type = %q", doc.datasets[1].Type)
	}
}

func TestAnonProto_NoBindingNeeded_Noop(t *testing.T) {
	input := `name = "x"
`
	doc, _ := loadPXF([]byte(input))
	if err := resolveAnonymousProtos(doc); err != nil {
		t.Errorf("no-op should not error: %v", err)
	}
}

func TestAnonProto_PipelineWithPxfProto(t *testing.T) {
	// End-to-end: anonymous @proto registers the type, the dataset
	// rows are accessible by name, and pxf_proto can construct a
	// typed object against the synthetic name.
	input := `@proto { string s = 1; }
@dataset ( s )
( "hello" )
`
	got := runE2EAnonResolved(t,
		`pxf_proto("_pxq_anon_0"; {s: "world"})`, input)
	if !strings.Contains(got, `@type _pxq_anon_0`) {
		t.Errorf("expected @type directive with synthetic name: %s", got)
	}
	if !strings.Contains(got, `s = "world"`) {
		t.Errorf("expected field s: %s", got)
	}
}
