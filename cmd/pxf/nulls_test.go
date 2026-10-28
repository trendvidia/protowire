// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// nullsDir is the cross-port null-binding fixture corpus
// (testdata/nulls/, issue #339, draft -01 § Booleans, Null, and
// Identifier Values), run here through the reference implementation:
// Parse, which has no schema and accepts every fixture, a full decoder
// bound to nulls.v1.Holder, and `pxf validate`.
const nullsDir = "../../testdata/nulls"

// nullsVerdicts is what each fixture asserts: the error substring the
// bound decoder and validate must give, "" for acceptance, and for an
// accept fixture the field the null leaves unset.
var nullsVerdicts = map[string]struct{ reject, unset string }{
	"err-scalar.pxf":          {reject: `null is not allowed on scalar field "n"`},
	"err-optional-scalar.pxf": {reject: `null is not allowed on scalar field "o"`},
	"err-oneof-scalar.pxf":    {reject: `null is not allowed on scalar field "ks"`},
	"err-repeated.pxf":        {reject: `null is not allowed on repeated field "l"`},
	"err-list-element.pxf":    {reject: `null is not allowed in repeated field "l"`},
	"message.pxf":             {unset: "msg"},
	"wrapper.pxf":             {unset: "w"},
	"oneof-message.pxf":       {unset: "km"},
}

func TestNulls_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(nullsDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(nullsVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(nullsVerdicts))
	}
	schemaFile := filepath.Join(nullsDir, "nulls.proto")
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{schemaFile},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("nulls.v1.Holder")
	if md == nil {
		t.Fatal("nulls.v1.Holder not found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := nullsVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pxf.Parse(data); err != nil {
				t.Errorf("Parse: a null is legal syntax everywhere, got %v", err)
			}
			msg, res, derr := pxf.UnmarshalFullDescriptor(data, md)
			_, verr := runCLI(t, "validate", "-p", schemaFile, "-m", "nulls.v1.Holder", f)
			for what, err := range map[string]error{"decode": derr, "validate": verr} {
				if !errMatches(err, want.reject) {
					t.Errorf("%s: want %q, got %v", what, want.reject, err)
				}
			}
			if want.reject != "" || derr != nil {
				return
			}
			if msg.Has(md.Fields().ByName("msg")) || msg.Has(md.Fields().ByName("w")) ||
				msg.WhichOneof(md.Oneofs().ByName("k")) != nil {
				t.Errorf("%s = null left a field set", want.unset)
			}
			if !res.IsNull(want.unset) {
				t.Errorf("Result.IsNull(%q) = false, want present-but-null", want.unset)
			}
		})
	}
}
