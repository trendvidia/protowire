// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"strings"
	"testing"
)

// TestBundled_PxfProtoWithoutFlags verifies the README's promise:
// pxf_proto on a canonical schema works without -p, in-doc @proto,
// or anything else. The user pays nothing for the baseline.
func TestBundled_PxfProtoWithoutFlags(t *testing.T) {
	// Building an envelope.v1.AppError with no flags whatsoever.
	got := runE2EFullPipeline(t, "x = 1\n",
		`pxf_proto("envelope.v1.AppError"; {code: "INTERNAL", message: "boom"})`)
	if !strings.Contains(got, `@type envelope.v1.AppError`) {
		t.Errorf("expected @type directive: %s", got)
	}
	if !strings.Contains(got, `code = "INTERNAL"`) {
		t.Errorf("expected code field: %s", got)
	}
	if !strings.Contains(got, `message = "boom"`) {
		t.Errorf("expected message field: %s", got)
	}
}

func TestBundled_AllCanonicalFilesCompile(t *testing.T) {
	sch, err := loadSchema(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// One sentinel message per bundled file: if any file failed to
	// compile (missing import, bad syntax, etc.) at least one of
	// these would be missing.
	wants := []string{
		"pxf.Decimal",          // proto/pxf/bignum.proto
		"envelope.v1.Envelope", // proto/envelope/v1/envelope.proto
		"envelope.v1.AppError", // same file, exercises nested-message walk
	}
	for _, w := range wants {
		if sch.find(w) == nil {
			t.Errorf("bundled type %q missing", w)
		}
	}
}

func TestBundled_CoexistsWithDashP(t *testing.T) {
	// User-supplied .proto + bundled both visible from the same registry.
	p := writeProto(t, `syntax = "proto3";
package mine.v1;
message Mine { string s = 1; }
`)
	sch, err := loadSchema([]string{p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sch.find("mine.v1.Mine") == nil {
		t.Error("user type not registered")
	}
	if sch.find("envelope.v1.Envelope") == nil {
		t.Error("bundled type not registered alongside user -p")
	}
}

func TestBundled_CoexistsWithInDocProto(t *testing.T) {
	// In-doc @proto and bundled both visible from the same registry.
	input := `@proto x.v1.X { string s = 1; }
n = 1
`
	doc, err := loadPXF([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	sch, err := loadSchema(nil, doc.protos)
	if err != nil {
		t.Fatal(err)
	}
	if sch.find("x.v1.X") == nil {
		t.Error("in-doc type not registered")
	}
	if sch.find("envelope.v1.Envelope") == nil {
		t.Error("bundled type not registered alongside in-doc @proto")
	}
}

func TestBundled_DashPCanCrossImportCanonical(t *testing.T) {
	// User .proto importing a canonical bundled schema should compile
	// in the same pass — the accessor serves the canonical file out
	// of embed.FS when protocompile follows the import statement.
	p := writeProto(t, `syntax = "proto3";
package mine.v1;
import "envelope/v1/envelope.proto";
message Container {
  envelope.v1.AppError err = 1;
}
`)
	sch, err := loadSchema([]string{p}, nil)
	if err != nil {
		t.Fatalf("cross-import compile failed: %v", err)
	}
	if sch.find("mine.v1.Container") == nil {
		t.Error("user message with bundled-import not registered")
	}
}
