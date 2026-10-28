// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire/internal/schemaresolve"
	"google.golang.org/protobuf/reflect/protoreflect"
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
// it fails the test until it is removed. Empty since protowire-go
// v1.11.0, which accepts null on a scalar with explicit presence
// (protowire-go#182, #339 option 3).
var nullsKnownGaps = map[string]datasetGap{}

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
			// schemaresolve gives every fixture message the generated
			// `_null` mask (#377), and the decoder records the null there.
			fields := md.Fields()
			for i := range fields.Len() {
				if fd := fields.Get(i); fd.Name() != "_null" && msg.Has(fd) {
					t.Errorf("%s = null left %s set", want.unset, fd.Name())
				}
			}
			if got := nullMaskPaths(t, msg); !slices.Contains(got, want.unset) {
				t.Errorf("_null paths = %q, want them to contain %q", got, want.unset)
			}
			if !res.IsNull(want.unset) {
				t.Errorf("Result.IsNull(%q) = false, want present-but-null", want.unset)
			}
		})
	}
}

// nullMaskPaths returns the paths msg's `_null` FieldMask records,
// failing the test when the message has no such field.
func nullMaskPaths(t *testing.T, msg protoreflect.Message) []string {
	t.Helper()
	fd := msg.Descriptor().Fields().ByName("_null")
	if fd == nil {
		t.Fatalf("%s has no _null field", msg.Descriptor().FullName())
	}
	if fd.Number() != 536870911 || fd.Message() == nil || fd.Message().FullName() != "google.protobuf.FieldMask" {
		t.Fatalf("%s._null is not the generated mask: number %d, type %v", msg.Descriptor().FullName(), fd.Number(), fd.Message())
	}
	list := msg.Get(fd).Message().Get(fd.Message().Fields().ByName("paths")).List()
	out := make([]string, list.Len())
	for i := range list.Len() {
		out[i] = list.Get(i).String()
	}
	return out
}
