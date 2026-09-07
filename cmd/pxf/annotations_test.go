// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// annotationsDir is the cross-port (pxf.required) / (pxf.default)
// fixture corpus (testdata/annotations/, issue #244), driven here
// through the CLI end-to-end: the library applies both annotations on
// its full decode path, and the CLI used the plain one, so validate
// accepted a document missing a required field and encode wrote a
// document without its defaults (#269).
const annotationsDir = "../../testdata/annotations"

func annotationsProto() string { return filepath.Join(annotationsDir, "settings.proto") }

func annotationsFixture(name string) string { return filepath.Join(annotationsDir, name) }

func TestAnnotations_ValidateRejectsMissingRequired(t *testing.T) {
	_, err := runCLI(t, "validate", "-p", annotationsProto(), "-m", "settings.v1.Settings",
		annotationsFixture("missing-required.pxf"))
	if err == nil {
		t.Fatal("validate accepted a document whose (pxf.required) field is absent")
	}
	if !strings.Contains(err.Error(), `required field "name" is absent`) {
		t.Fatalf("the error must name the field: %v", err)
	}
}

func TestAnnotations_ValidateAcceptsComplete(t *testing.T) {
	if _, err := runCLI(t, "validate", "-p", annotationsProto(), "-m", "settings.v1.Settings",
		annotationsFixture("ok.pxf")); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestAnnotations_EncodeAppliesDefaults(t *testing.T) {
	out, err := runCLI(t, "encode", "-p", annotationsProto(), "-m", "settings.v1.Settings",
		annotationsFixture("ok.pxf"))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	wantHex, err := os.ReadFile(annotationsFixture("ok.expected.hex"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := hex.DecodeString(strings.TrimSpace(string(wantHex)))
	if err != nil {
		t.Fatal(err)
	}
	if got := []byte(out); string(got) != string(want) {
		t.Fatalf("encode bytes differ from the cross-port golden:\n got %x\nwant %x", got, want)
	}
}

func TestAnnotations_EncodeRejectsMissingRequired(t *testing.T) {
	_, err := runCLI(t, "encode", "-p", annotationsProto(), "-m", "settings.v1.Settings",
		annotationsFixture("missing-required.pxf"))
	if err == nil || !strings.Contains(err.Error(), `required field "name" is absent`) {
		t.Fatalf("encode must refuse a document missing a required field, got: %v", err)
	}
}
