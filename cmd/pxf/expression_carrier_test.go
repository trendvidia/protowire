// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"path/filepath"
	"testing"

	schemav1 "github.com/trendvidia/protocompile/gen/protowire/schema/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestExpressionLanguageCarrier asserts what fixture 24's header says
// lowers (#431): the enum value names in the rule on `mode` reach
// Expression.enum_values resolved, once per spelling in source order
// (§8.1, #427), and slug_rule's `expression` default lowers like an
// argument, with its declared-function call and no builtin (§5.1,
// #437).
func TestExpressionLanguageCarrier(t *testing.T) {
	const name = "24_expression_language.proto"
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(buildImage(t, filepath.Join(fixtureDir, name)), &set); err != nil {
		t.Fatal(err)
	}
	file := imageFile(t, &set, name)

	var mode *descriptorpb.FieldDescriptorProto
	for _, m := range file.GetMessageType() {
		for _, f := range m.GetField() {
			if m.GetName() == "Profile" && f.GetName() == "mode" {
				mode = f
			}
		}
	}
	if mode == nil {
		t.Fatal("fixture 24 has no Profile.mode")
	}
	al := carrierOf(t, mode.GetOptions())
	if al == nil || len(al.GetEntries()) != 1 || len(al.GetEntries()[0].GetArgs()) != 1 {
		t.Fatalf("Profile.mode carries %v, want one @validate with one argument", al)
	}
	expr := al.GetEntries()[0].GetArgs()[0].GetExpression()
	type ref struct {
		spelling, enum, value string
		number                int32
	}
	want := []ref{
		{"MODE_MATCH", "fixtures.exprlang.Mode", "MODE_MATCH", 1},
		{"fixtures.exprlang.MODE_REGULATE", "fixtures.exprlang.Mode", "MODE_REGULATE", 2},
		{"Mode.MODE_UNSPECIFIED", "fixtures.exprlang.Mode", "MODE_UNSPECIFIED", 0},
	}
	var got []ref
	for _, r := range expr.GetEnumValues() {
		v := r.GetValue()
		got = append(got, ref{r.GetSpelling(), v.GetEnumType(), v.GetValueName(), v.GetNumber()})
	}
	if len(got) != len(want) {
		t.Fatalf("Profile.mode enum_values = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("enum_values[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(expr.GetCalls()) != 0 {
		t.Errorf("Profile.mode calls = %v, want none: defined() is a builtin", expr.GetCalls())
	}

	var decls schemav1.FileAnnotationDecls
	if err := proto.Unmarshal(optionPayload(t, file.GetOptions(), 1329), &decls); err != nil {
		t.Fatal(err)
	}
	var deflt *schemav1.Expression
	for _, d := range decls.GetDeclarations() {
		if d.GetName() == "fixtures.exprlang.slug_rule" && len(d.GetParams()) == 1 {
			deflt = d.GetParams()[0].GetDefaultValue().GetExpression()
		}
	}
	if deflt == nil {
		t.Fatalf("no slug_rule declaration with an expression default in %v", &decls)
	}
	if got, want := deflt.GetSource(), `matches(this, "^[a-z0-9-]+$") && !reserved_slug(this)`; got != want {
		t.Errorf("slug_rule default source = %q, want %q", got, want)
	}
	if calls := deflt.GetCalls(); len(calls) != 1 || calls[0].GetFqn() != "fixtures.exprlang.reserved_slug" || calls[0].GetArity() != 1 {
		t.Errorf("slug_rule default calls = %v, want only fixtures.exprlang.reserved_slug/1", calls)
	}
}
