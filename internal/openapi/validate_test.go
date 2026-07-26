// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"reflect"
	"testing"
)

// TestSplitTopLevel pins the scanner against the §8.1 capture
// guarantees: string literals are opaque, brackets nest.
func TestSplitTopLevel(t *testing.T) {
	cases := []struct {
		in   string
		sep  string
		want []string
	}{
		{`a && b`, "&&", []string{`a `, ` b`}},
		{`matches(this, "a && b") && c`, "&&", []string{`matches(this, "a && b") `, ` c`}},
		{`f(x && y)`, "&&", []string{`f(x && y)`}},
		{`this in ["a,b", "c"]`, ",", []string{`this in ["a,b", "c"]`}},
		{`"US", 2`, ",", []string{`"US"`, ` 2`}},
	}
	for _, c := range cases {
		if got := splitTopLevel(c.in, c.sep); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitTopLevel(%q, %q) = %q, want %q", c.in, c.sep, got, c.want)
		}
	}
}

// mapped runs one rule source through the mapper with string sizing.
func mapped(t *testing.T, src string, sk sizeKind, numeric bool) (*constraints, *omap) {
	t.Helper()
	c := &constraints{}
	for _, conj := range splitTopLevel(src, "&&") {
		conj = trimmed(conj)
		if conj == "" {
			continue
		}
		if !c.mapConjunct(conj, sk, numeric) {
			c.unmapped = append(c.unmapped, newOmap().set("rule", conj))
		}
	}
	s := newOmap()
	c.apply(s)
	return c, s
}

func trimmed(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func TestMapConjuncts(t *testing.T) {
	// The regex containing ")" and "," inside the string literal is the
	// §8.1 opaque-capture edge case from 12_expression_args.proto.
	_, s := mapped(t, `matches(this, "^[a-z),]+$")`, sizeLength, false)
	if got, _ := s.get("pattern"); got != `^[a-z),]+$` {
		t.Errorf("pattern = %v", got)
	}

	_, s = mapped(t, `this.size() >= 2 && this.size() <= 32`, sizeLength, false)
	if got, _ := s.get("minLength"); got != int64(2) {
		t.Errorf("minLength = %v", got)
	}
	if got, _ := s.get("maxLength"); got != int64(32) {
		t.Errorf("maxLength = %v", got)
	}

	// size(this) form, strict bound, items sizing.
	_, s = mapped(t, `size(this) < 11`, sizeItems, false)
	if got, _ := s.get("maxItems"); got != int64(10) {
		t.Errorf("maxItems = %v", got)
	}

	_, s = mapped(t, `this >= 18 && this < 130`, sizeNone, true)
	if got, _ := s.get("minimum"); got != float64(18) {
		t.Errorf("minimum = %v", got)
	}
	if got, _ := s.get("exclusiveMaximum"); got != float64(130) {
		t.Errorf("exclusiveMaximum = %v", got)
	}

	_, s = mapped(t, `this in ["US", "CA"]`, sizeLength, false)
	if got, _ := s.get("enum"); !reflect.DeepEqual(got, []any{"US", "CA"}) {
		t.Errorf("enum = %v", got)
	}

	// Unmappable shapes fall through whole, never half-translated.
	for _, src := range []string{
		`this == 1 || this == 2`,
		`size(split(this, ",")[0]) > 0`,
		`!this.startsWith(" ")`,
		`same_domain(this)`,
	} {
		c, s := mapped(t, src, sizeLength, true)
		if len(c.unmapped) != 1 {
			t.Errorf("%q: unmapped = %d, want 1", src, len(c.unmapped))
		}
		if _, ok := s.get("x-validation"); !ok {
			t.Errorf("%q: no x-validation emitted", src)
		}
	}

	// Mixed: the mappable conjunct maps, the rest is carried through.
	c, s := mapped(t, `this.size() <= 64 && !this.startsWith(" ")`, sizeLength, false)
	if got, _ := s.get("maxLength"); got != int64(64) {
		t.Errorf("maxLength = %v", got)
	}
	if len(c.unmapped) != 1 {
		t.Errorf("unmapped = %d, want 1", len(c.unmapped))
	}

	// A string-typed subject never grows numeric bounds.
	c, _ = mapped(t, `this >= 18`, sizeLength, false)
	if len(c.unmapped) != 1 {
		t.Errorf("string subject accepted numeric bound")
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		glob, fqn string
		want      bool
	}{
		{"*Audit*", "demo.store.AuditRecord", true},
		{"*Audit*", "demo.store.Store.ListAudit", true},
		{"*Audit*", "demo.store.Customer", false},
		{"demo.internal.*", "demo.internal.Secret", true},
		{"demo.internal.*", "demo.internal.deep.Secret", true},
		{"demo.internal.*", "demo.internalish.Thing", false},
		{"demo.store.Customer", "demo.store.Customer", true},
	}
	for _, c := range cases {
		if got := globMatch(c.glob, c.fqn); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.glob, c.fqn, got, c.want)
		}
	}
}

func TestFirstSeen(t *testing.T) {
	vs := versionSets{
		3: {"a.B": true, "a.C": true},
		1: {"a.B": true},
		2: {"a.B": true, "a.D": true},
	}
	idx := firstSeen(vs)
	want := sinceIndex{"a.B": "v1", "a.D": "v2", "a.C": "v3"}
	if !reflect.DeepEqual(idx, want) {
		t.Errorf("firstSeen = %v, want %v", idx, want)
	}
}
