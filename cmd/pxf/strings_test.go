// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
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
// Empty since protowire-go v1.10.0, which rejects a raw newline in a
// simple string (protowire-go#167).
var stringsKnownGaps = map[string]datasetGap{}

func TestStrings_Corpus(t *testing.T) {
	runCorpus(t, stringsDir, []string{"strings.proto"}, stringsVerdicts, func(t *testing.T, f fixture[struct{ reject, s, t string }]) {
		md := mustFind(t, f.reg, "strings.v1.Holder")
		_, perr := pxf.Parse(f.data)
		msg, derr := pxf.UnmarshalDescriptor(f.data, md)
		verr := f.validate(t, "strings.v1.Holder")
		if gap, known := stringsKnownGaps[f.name]; known {
			skipGap(t, "stringsKnownGaps", gap, f.want.reject == "", perr, derr, verr)
		}
		for what, err := range map[string]error{"Parse": perr, "decode": derr, "validate": verr} {
			if !errMatches(err, f.want.reject) {
				t.Errorf("%s: want %q, got %v", what, f.want.reject, err)
			}
		}
		if f.want.reject != "" || derr != nil {
			return
		}
		fields := md.Fields()
		r := msg.ProtoReflect()
		if got := r.Get(fields.ByName("s")).String(); got != f.want.s {
			t.Errorf("s = %q, want %q", got, f.want.s)
		}
		if got := r.Get(fields.ByName("t")).String(); got != f.want.t {
			t.Errorf("t = %q, want %q", got, f.want.t)
		}
	})
}
