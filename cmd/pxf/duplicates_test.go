// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
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
// v1.12.0, which rejects a map key bound twice (protowire-go#191), as
// v1.11.0 did a singular field (protowire-go#184).
var duplicatesKnownGaps = map[string]datasetGap{}

func TestDuplicates_Corpus(t *testing.T) {
	runCorpus(t, duplicatesDir, []string{"duplicates.proto"}, duplicatesVerdicts, func(t *testing.T, f fixture[string]) {
		md := mustFind(t, f.reg, "dup.v1.Holder")
		if _, err := pxf.Parse(f.data); err != nil {
			t.Errorf("Parse: a second binding is legal syntax, got %v", err)
		}
		_, _, derr := pxf.UnmarshalFullDescriptor(f.data, md)
		verr := f.validate(t, "dup.v1.Holder")
		if gap, known := duplicatesKnownGaps[f.name]; known {
			skipGap(t, "duplicatesKnownGaps", gap, f.want == "", derr, verr)
		}
		for what, err := range map[string]error{"decode": derr, "validate": verr} {
			if !errMatches(err, f.want) {
				t.Errorf("%s: want %q, got %v", what, f.want, err)
			}
		}
	})
}
