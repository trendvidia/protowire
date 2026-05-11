// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package pxfschema_test

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/bufbuild/protocompile"

	"github.com/trendvidia/protowire/internal/pxfschema"
)

// fixturesDir is the testdata/lint/ corpus, addressed relative to this file's
// package (cmd-style "internal/" packages run from their own directory).
const fixturesDir = "../../testdata/lint"

func compileFixture(t *testing.T, name string) []pxfschema.Violation {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	rel := filepath.Join("testdata/lint", name)
	comp := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{filepath.Dir(filepath.Dir(filepath.Dir(abs)))},
		}),
	}
	result, err := comp.Compile(context.Background(), rel)
	if err != nil {
		t.Fatalf("compile %s: %v", rel, err)
	}
	var out []pxfschema.Violation
	for _, f := range result {
		out = append(out, pxfschema.ValidateReflect(f)...)
	}
	return out
}

func TestFixtures(t *testing.T) {
	cases := []struct {
		file string
		want []string // sorted fully-qualified element names; empty = clean
	}{
		{
			file: "reserved-enum-name.proto",
			want: []string{"test.lint.v1.null"},
		},
		{
			file: "reserved-field-name.proto",
			want: []string{"test.lint.v1.Flag.true"},
		},
		{
			file: "reserved-oneof-name.proto",
			want: []string{"test.lint.v1.Choice.false"},
		},
		{
			file: "reserved-uppercase-ok.proto",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			got := compileFixture(t, tc.file)
			gotNames := make([]string, len(got))
			for i, v := range got {
				gotNames[i] = v.Element
			}
			sort.Strings(gotNames)
			if !sameStrings(gotNames, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, gotNames)
			}
		})
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
