// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// strictTestSchema compiles a tiny inline .proto for the validator
// tests. Two messages with a deliberate field-name set so the typo
// hints have somewhere to land.
func strictTestSchema(t *testing.T) *schema {
	t.Helper()
	p := writeProto(t, `syntax = "proto3";
package strict.v1;
message Inner {
  string s = 1;
  int32 i = 2;
}
message Outer {
  string name = 1;
  int32 age = 2;
  Inner inner = 3;
  repeated string tags = 4;
}
`)
	sch, err := loadSchema([]string{p}, nil, registryRef{})
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	return sch
}

// --- effectiveMode flag-routing ---

func TestEffectiveMode_AutoBoundIsStrict(t *testing.T) {
	strict, err := effectiveMode(modeAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strict {
		t.Error("auto + bound should be strict")
	}
}

func TestEffectiveMode_AutoUnboundIsLoose(t *testing.T) {
	strict, err := effectiveMode(modeAuto, false)
	if err != nil {
		t.Fatal(err)
	}
	if strict {
		t.Error("auto + unbound should be loose")
	}
}

func TestEffectiveMode_StrictUnboundErrors(t *testing.T) {
	_, err := effectiveMode(modeStrict, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--strict requires") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEffectiveMode_LooseAlwaysSucceeds(t *testing.T) {
	for _, bound := range []bool{true, false} {
		strict, err := effectiveMode(modeLoose, bound)
		if err != nil {
			t.Errorf("loose+bound=%v errored: %v", bound, err)
		}
		if strict {
			t.Errorf("loose+bound=%v should be loose", bound)
		}
	}
}

// --- validator ---

func validate(t *testing.T, query string, sch *schema, typeName string) error {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return validateQueryStrict(q, sch.find(typeName))
}

func TestValidate_KnownField_OK(t *testing.T) {
	sch := strictTestSchema(t)
	for _, q := range []string{".name", ".age", ".tags"} {
		if err := validate(t, q, sch, "strict.v1.Outer"); err != nil {
			t.Errorf("%s: unexpected error: %v", q, err)
		}
	}
}

func TestValidate_UnknownField_Errors(t *testing.T) {
	sch := strictTestSchema(t)
	err := validate(t, ".aeg", sch, "strict.v1.Outer")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), `"aeg"`) {
		t.Errorf("error should name the offending field: %v", err)
	}
	if !strings.Contains(err.Error(), `"age"`) {
		t.Errorf("expected did-you-mean hint: %v", err)
	}
}

func TestValidate_NoSuggestionWhenTooFar(t *testing.T) {
	sch := strictTestSchema(t)
	// "completely_unrelated" is too far from any declared field for
	// the typo heuristic to surface a hint.
	err := validate(t, ".completely_unrelated", sch, "strict.v1.Outer")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Errorf("should not produce a suggestion when nothing is close: %v", err)
	}
}

func TestValidate_NestedFieldChain_OK(t *testing.T) {
	sch := strictTestSchema(t)
	for _, q := range []string{".inner.s", ".inner.i"} {
		if err := validate(t, q, sch, "strict.v1.Outer"); err != nil {
			t.Errorf("%s: unexpected error: %v", q, err)
		}
	}
}

func TestValidate_NestedFieldChain_UnknownLeaf(t *testing.T) {
	sch := strictTestSchema(t)
	err := validate(t, ".inner.t", sch, "strict.v1.Outer")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `"t"`) {
		t.Errorf("error should name the leaf: %v", err)
	}
	if !strings.Contains(err.Error(), "strict.v1.Inner") {
		t.Errorf("error should name the parent message: %v", err)
	}
}

func TestValidate_ChainPastScalar_Permissive(t *testing.T) {
	// `.name.x` — `name` is a scalar (string); a `.x` past a scalar is
	// a runtime mismatch but the validator can't infer that without
	// tracking element types. Stage E permits it.
	sch := strictTestSchema(t)
	err := validate(t, ".name.x", sch, "strict.v1.Outer")
	if err != nil {
		t.Errorf("scalar-then-field should be permitted at Stage E: %v", err)
	}
}

func TestValidate_PipeBothHalvesValidated(t *testing.T) {
	sch := strictTestSchema(t)
	// LHS valid, RHS valid.
	if err := validate(t, ".name | length", sch, "strict.v1.Outer"); err != nil {
		t.Errorf("pipe with valid sides: %v", err)
	}
	// RHS uses unknown field on Outer — `length` doesn't rebind the
	// type, so Stage E validates against root in both halves.
	err := validate(t, ". | .aeg", sch, "strict.v1.Outer")
	if err == nil {
		t.Fatal("expected error on pipe RHS")
	}
}

func TestValidate_NonPathTerms_Permissive(t *testing.T) {
	sch := strictTestSchema(t)
	// String literal, function call, object construction — none of
	// these reference Outer's fields directly; validator passes.
	cases := []string{
		`"hello"`,
		`42`,
		`{a: 1}`,
		`[1, 2, 3]`,
		`pxf_directive("dataset")`,
	}
	for _, q := range cases {
		if err := validate(t, q, sch, "strict.v1.Outer"); err != nil {
			t.Errorf("%s: unexpected error: %v", q, err)
		}
	}
}

func TestValidate_NilRoot_Permissive(t *testing.T) {
	// No bound type → validator is a no-op.
	q, err := gojq.Parse(".anything.goes.here")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateQueryStrict(q, nil); err != nil {
		t.Errorf("nil root should disable validation: %v", err)
	}
}

func TestValidate_ArrayIndexThenField_Permissive(t *testing.T) {
	// `.tags[0]` followed by `.length` is meaningless on a string but
	// Stage E doesn't track element types after array indexing.
	sch := strictTestSchema(t)
	err := validate(t, ".tags[0]", sch, "strict.v1.Outer")
	if err != nil {
		t.Errorf("array index should pass: %v", err)
	}
}

// --- resolveRootType ---

func TestResolveRootType_MessageFlagWins(t *testing.T) {
	sch := strictTestSchema(t)
	doc := &loadedDoc{typeURL: "strict.v1.Inner"}
	got := resolveRootType("strict.v1.Outer", doc, sch)
	if got == nil || string(got.FullName()) != "strict.v1.Outer" {
		t.Errorf("--message should win over @type: %v", got)
	}
}

func TestResolveRootType_FallsBackToAtType(t *testing.T) {
	sch := strictTestSchema(t)
	doc := &loadedDoc{typeURL: "strict.v1.Outer"}
	got := resolveRootType("", doc, sch)
	if got == nil || string(got.FullName()) != "strict.v1.Outer" {
		t.Errorf("@type fallback failed: %v", got)
	}
}

func TestResolveRootType_NoBindingReturnsNil(t *testing.T) {
	sch := strictTestSchema(t)
	if got := resolveRootType("", &loadedDoc{}, sch); got != nil {
		t.Errorf("expected nil for no binding: %v", got)
	}
}

func TestResolveRootType_NoSchemaReturnsNil(t *testing.T) {
	if got := resolveRootType("x.Type", &loadedDoc{typeURL: "x.Type"}, nil); got != nil {
		t.Error("nil schema should yield nil root type")
	}
}

// --- end-to-end through runQuery ---

func TestRunQuery_StrictModeRejectsTypoAtCompileTime(t *testing.T) {
	sch := strictTestSchema(t)
	doc := &loadedDoc{
		typeURL: "strict.v1.Outer",
		body: map[string]any{
			"name": "Alice",
			"age":  30,
		},
	}
	_, err := runQuery(".aeg", doc, sch, strictOpts{
		enabled: true,
		root:    sch.find("strict.v1.Outer"),
	})
	if err == nil {
		t.Fatal("expected compile-time rejection")
	}
	if !strings.Contains(err.Error(), "strict-mode") {
		t.Errorf("expected strict-mode error: %v", err)
	}
}

func TestRunQuery_LooseModeRunsTypoToCompletion(t *testing.T) {
	sch := strictTestSchema(t)
	doc := &loadedDoc{
		typeURL: "strict.v1.Outer",
		body:    map[string]any{"name": "Alice"},
	}
	// Loose mode: validator skipped; .aeg on Alice's body yields null
	// at runtime (jq's standard behaviour for missing keys).
	out, err := runQuery(".aeg", doc, sch, strictOpts{enabled: false})
	if err != nil {
		t.Fatalf("loose mode should not error at compile: %v", err)
	}
	if len(out) != 1 || out[0] != nil {
		t.Errorf("expected single nil result, got: %#v", out)
	}
}

// A strict run fails on a runtime error, as jq does, after writing the
// results produced before it; a loose run degrades the error to null
// (#403). Covers --strict, the implicit strict mode a bound root type
// selects, and --loose.
func TestStrict_RuntimeErrorFails(t *testing.T) {
	p := writeProto(t, `syntax = "proto3"; package l.v1; message W { int32 n = 1; }
`)
	doc := filepath.Join(t.TempDir(), "u.pxf")
	if err := os.WriteFile(doc, []byte("n = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var sink bytes.Buffer
	errSink = &sink
	t.Cleanup(func() { errSink = io.Discard })
	const query = `.n, pxf_proto("l.v1.Missing"; {}), 3`

	for _, mode := range [][]string{{"--strict"}, nil} {
		sink.Reset()
		args := append([]string{"query", "-p", p, "-m", "l.v1.W"}, mode...)
		out, err := runCLI(t, append(args, query, doc)...)
		if !errMatches(err, `strict-mode: runtime error: pxf_proto("l.v1.Missing")`) {
			t.Errorf("%v: err = %v, want a strict-mode runtime error naming pxf_proto", mode, err)
		}
		if !strings.Contains(out, "value = 1") || strings.Contains(out, "value = 3") || strings.Contains(out, "null") {
			t.Errorf("%v: stdout = %q, want the result before the error only", mode, out)
		}
		if sink.Len() != 0 {
			t.Errorf("%v: stderr = %q, want no null hint", mode, sink.String())
		}
	}

	sink.Reset()
	out, err := runCLI(t, "query", "-p", p, "-m", "l.v1.W", "--loose", query, doc)
	if err != nil {
		t.Fatalf("--loose: unexpected error: %v", err)
	}
	if !strings.Contains(out, "value = null") || !strings.Contains(out, "value = 3") {
		t.Errorf("--loose: stdout = %q, want null in place of the error and the result after it", out)
	}
	if hint := sink.String(); !strings.Contains(hint, `pxf_proto("l.v1.Missing")`) || !strings.Contains(hint, "(runtime error → null)") {
		t.Errorf("--loose: stderr = %q, want the pxf_proto error labelled (runtime error → null)", hint)
	}
}
