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
// bound to the fixture's message, and `pxf validate`.
const nullsDir = "../../testdata/nulls"

// nullsVerdicts is what each fixture asserts: the error substring the
// bound decoder and validate must give, "" for acceptance, and for an
// accept fixture the field the null leaves unset. msg is the message the
// fixture binds to, nulls.v1.Holder when empty.
var nullsVerdicts = map[string]struct{ msg, reject, unset string }{
	"err-scalar.pxf":       {reject: `null is not allowed on scalar field "n"`},
	"err-repeated.pxf":     {reject: `null is not allowed on repeated field "l"`},
	"err-list-element.pxf": {reject: `null is not allowed in repeated field "l"`},
	"err-map.pxf":          {reject: `null is not allowed on map field "m"`},
	// Decided on #380 and #379.
	"err-map-value.pxf":         {reject: `null is not allowed as map value in field "m"`},
	"err-map-message-value.pxf": {reject: `null is not allowed as map value in field "mm"`},
	"err-proto2-required.pxf":   {msg: "nulls.v1.Strict", reject: `null is not allowed on scalar field "r"`},
	"err-nested-scalar.pxf":     {reject: `null is not allowed on scalar field "x"`},
	"err-oneof-sibling.pxf":     {reject: `field "ks" conflicts with already-set field "km"`},
	"message.pxf":               {unset: "msg"},
	"wrapper.pxf":               {unset: "w"},
	"optional-scalar.pxf":       {unset: "o"},
	"oneof-scalar.pxf":          {unset: "ks"},
	"oneof-message.pxf":         {unset: "km"},
	"proto2-scalar.pxf":         {msg: "nulls.v1.Legacy", unset: "p"},
	"required-optional.pxf":     {msg: "nulls.v1.Required", unset: "r"},
	"default-optional.pxf":      {msg: "nulls.v1.Defaulted", unset: "d"},
}

// nullsKnownGaps maps fixtures the pinned protowire-go does not yet meet
// to the change that closes the gap and what the pinned release does
// instead. Each is asserted to still hold, so the go.mod bump that closes
// it fails the test until it is removed. v1.10.0 shipped the stricter
// rule #339 first decided (protowire-go#150), which rejects null on every
// scalar that is not a wrapper; the owner then chose option 3.
var nullsKnownGaps = map[string]datasetGap{
	"optional-scalar.pxf":   {by: "protowire-go#182", old: `null is not allowed on scalar field "o"`},
	"oneof-scalar.pxf":      {by: "protowire-go#182", old: `null is not allowed on scalar field "ks"`},
	"proto2-scalar.pxf":     {by: "protowire-go#182", old: `null is not allowed on scalar field "p"`},
	"required-optional.pxf": {by: "protowire-go#182", old: `null is not allowed on scalar field "r"`},
	"default-optional.pxf":  {by: "protowire-go#182", old: `null is not allowed on scalar field "d"`},
}

func TestNulls_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(nullsDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(nullsVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(nullsVerdicts))
	}
	schemaFiles := []string{filepath.Join(nullsDir, "nulls.proto"), filepath.Join(nullsDir, "nulls_proto2.proto")}
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    schemaFiles,
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := nullsVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			msgName := want.msg
			if msgName == "" {
				msgName = "nulls.v1.Holder"
			}
			md := reg.Find(msgName)
			if md == nil {
				t.Fatalf("%s not found", msgName)
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pxf.Parse(data); err != nil {
				t.Errorf("Parse: a null is legal syntax everywhere, got %v", err)
			}
			msg, res, derr := pxf.UnmarshalFullDescriptor(data, md)
			_, verr := runCLI(t, "validate", "-p", schemaFiles[0], "-p", schemaFiles[1], "-m", msgName, f)
			if gap, known := nullsKnownGaps[name]; known {
				skipGap(t, "nullsKnownGaps", gap, want.reject == "", derr, verr)
			}
			for what, err := range map[string]error{"decode": derr, "validate": verr} {
				if !errMatches(err, want.reject) {
					t.Errorf("%s: want %q, got %v", what, want.reject, err)
				}
			}
			if want.reject != "" || derr != nil {
				return
			}
			fields := md.Fields()
			for i := range fields.Len() {
				if fd := fields.Get(i); msg.Has(fd) {
					t.Errorf("%s = null left %s set", want.unset, fd.Name())
				}
			}
			if !res.IsNull(want.unset) {
				t.Errorf("Result.IsNull(%q) = false, want present-but-null", want.unset)
			}
		})
	}
}
