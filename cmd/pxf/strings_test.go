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

// stringsDir is the cross-port simple-string fixture corpus
// (testdata/strings/, issue #350, draft -01 § String Literals: a raw LF
// MUST NOT appear in a simple string), run here through the reference
// implementation: Parse, which has no schema, and a decoder bound to
// strings.v1.Holder, through `pxf validate`.
const stringsDir = "../../testdata/strings"

// stringsVerdicts is what each fixture asserts: the error substring
// Parse and the bound decoder must give, "" for acceptance, and for an
// accept fixture the values s and t bind to.
var stringsVerdicts = map[string]struct{ reject, s, t string }{
	"err-raw-newline.pxf":      {reject: "newline in string literal"},
	"escaped-newline.pxf":      {s: "x\ny"},
	"triple-quoted.pxf":        {s: "x\ny"},
	"newline-after-string.pxf": {s: "x", t: "y"},
}

// stringsKnownGaps maps fixtures the pinned protowire-go does not yet
// meet to the change that closes the gap and what the pinned release
// does instead ("" for acceptance). Each is asserted to still hold, so
// the go.mod bump that closes it fails the test until it is removed.
var stringsKnownGaps = map[string]datasetGap{
	// protowire-go v1.9.0 keeps the raw LF in the string;
	// protowire-go#167 rejects it.
	"err-raw-newline.pxf": {by: "protowire-go#167", old: ""},
}

func TestStrings_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(stringsDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(stringsVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(stringsVerdicts))
	}
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{filepath.Join(stringsDir, "strings.proto")},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("strings.v1.Holder")
	if md == nil {
		t.Fatal("strings.v1.Holder not found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := stringsVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, perr := pxf.Parse(data)
			msg, derr := pxf.UnmarshalDescriptor(data, md)
			_, verr := runCLI(t, "validate", "-p", filepath.Join(stringsDir, "strings.proto"), "-m", "strings.v1.Holder", f)
			if gap, known := stringsKnownGaps[name]; known {
				for _, err := range []error{perr, derr, verr} {
					if !errMatches(err, gap.old) {
						t.Fatalf("the pinned protowire-go no longer treats this fixture as before (Parse: %v; decode: %v; validate: %v); gap %s is closed, remove it from stringsKnownGaps", perr, derr, verr, gap.by)
					}
				}
				t.Skipf("known gap, closed by %s", gap.by)
			}
			for what, err := range map[string]error{"Parse": perr, "decode": derr, "validate": verr} {
				if !errMatches(err, want.reject) {
					t.Errorf("%s: want %q, got %v", what, want.reject, err)
				}
			}
			if want.reject != "" || derr != nil {
				return
			}
			fields := md.Fields()
			r := msg.ProtoReflect()
			if got := r.Get(fields.ByName("s")).String(); got != want.s {
				t.Errorf("s = %q, want %q", got, want.s)
			}
			if got := r.Get(fields.ByName("t")).String(); got != want.t {
				t.Errorf("t = %q, want %q", got, want.t)
			}
		})
	}
}
