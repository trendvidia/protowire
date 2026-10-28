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

// duplicatesDir is the cross-port fixture corpus for a singular field
// bound twice in one message body (testdata/duplicates/, issue #347,
// draft -01 § Entries and Keys), run here through Parse, a decoder bound
// to dup.v1.Holder, and `pxf validate`.
const duplicatesDir = "../../testdata/duplicates"

// duplicatesVerdicts is each fixture's error substring, "" for accept.
var duplicatesVerdicts = map[string]string{
	"err-scalar-twice.pxf":              "already set",
	"err-scalar-same-value.pxf":         "already set",
	"err-wrapper-twice.pxf":             "already set",
	"err-message-twice.pxf":             "already set",
	"err-message-assign-then-block.pxf": "already set",
	"err-nested-scalar-twice.pxf":       "already set",
	"err-null-then-value.pxf":           "already set",
	"err-optional-twice.pxf":            "already set",
	"err-oneof-member-twice.pxf":        `field "km" conflicts with already-set field "km"`,
	"err-oneof-scalar-twice.pxf":        `field "ks" conflicts with already-set field "ks"`,
	"once-per-body.pxf":                 "",
	"repeated-twice.pxf":                "",
	// A map key bound twice (#396).
	"err-map-key-twice-in-literal.pxf":      "already set",
	"err-map-key-twice-same-value.pxf":      "already set",
	"err-map-key-twice-across-bindings.pxf": "already set",
	"err-map-key-twice-int-spellings.pxf":   "already set",
	"err-map-key-twice-message-values.pxf":  "already set",
	"map-twice-distinct-keys.pxf":           "",
	"map-key-once-per-body.pxf":             "",
}

// duplicatesKnownGaps are fixtures the pinned protowire-go does not yet
// meet. Each is asserted to still hold, so the go.mod bump that closes
// one fails the test until its entry goes. Empty since protowire-go
// v1.11.0, which rejects a singular field bound twice (protowire-go#184).
// v1.11.0 keeps the last value of a map key bound twice, so it accepts
// the #396 reject fixtures (old "").
var duplicatesKnownGaps = map[string]datasetGap{
	"err-map-key-twice-in-literal.pxf":      {by: "protowire-go#191"},
	"err-map-key-twice-same-value.pxf":      {by: "protowire-go#191"},
	"err-map-key-twice-across-bindings.pxf": {by: "protowire-go#191"},
	"err-map-key-twice-int-spellings.pxf":   {by: "protowire-go#191"},
	"err-map-key-twice-message-values.pxf":  {by: "protowire-go#191"},
}

func TestDuplicates_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(duplicatesDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(duplicatesVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(duplicatesVerdicts))
	}
	schemaFile := filepath.Join(duplicatesDir, "duplicates.proto")
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{schemaFile},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("dup.v1.Holder")
	if md == nil {
		t.Fatal("dup.v1.Holder not found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := duplicatesVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pxf.Parse(data); err != nil {
				t.Errorf("Parse: a second binding is legal syntax, got %v", err)
			}
			_, _, derr := pxf.UnmarshalFullDescriptor(data, md)
			_, verr := runCLI(t, "validate", "-p", schemaFile, "-m", "dup.v1.Holder", f)
			if gap, known := duplicatesKnownGaps[name]; known {
				skipGap(t, "duplicatesKnownGaps", gap, want == "", derr, verr)
			}
			for what, err := range map[string]error{"decode": derr, "validate": verr} {
				if !errMatches(err, want) {
					t.Errorf("%s: want %q, got %v", what, want, err)
				}
			}
		})
	}
}
