// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// durationDir is the cross-port signed-duration fixture corpus
// (testdata/duration/, spec issue #234, draft -01 §3.3 and §"Timestamps
// and Durations"), run here through the reference implementation so the
// files assert what a port must do rather than what a README says.
const durationDir = "../../testdata/duration"

func durationProto() string { return filepath.Join(durationDir, "duration.proto") }

func durationFixture(name string) string { return filepath.Join(durationDir, name) }

// decodeDurationFixture binds one fixture against duration.v1.Holder with
// the same resolution pipeline the CLI uses and returns Holder.d's
// (seconds, nanos).
func decodeDurationFixture(t *testing.T, name string) (int64, int32) {
	t.Helper()
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{durationProto()},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("duration.v1.Holder")
	if md == nil {
		t.Fatal("duration.v1.Holder not found")
	}
	data, err := os.ReadFile(durationFixture(name))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := pxf.UnmarshalDescriptor(data, md)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	fd := md.Fields().ByName("d")
	sub := msg.ProtoReflect().Get(fd).Message()
	dd := fd.Message()
	return sub.Get(dd.Fields().ByName("seconds")).Int(),
		int32(sub.Get(dd.Fields().ByName("nanos")).Int())
}

func TestDuration_NegativeBinds(t *testing.T) {
	// The values each fixture's leading comment promises: one leading "-"
	// applies to the whole literal, and a fractional segment yields
	// seconds and nanos of the same sign, as google.protobuf.Duration
	// requires.
	cases := []struct {
		file  string
		secs  int64
		nanos int32
	}{
		{"negative-seconds.pxf", -30, 0},
		{"negative-fractional.pxf", -1, -500000000},
		{"negative-multi-segment.pxf", -5400, 0},
		{"negative-zero.pxf", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			if _, err := runCLI(t, "validate", "-p", durationProto(), "-m", "duration.v1.Holder", durationFixture(c.file)); err != nil {
				t.Fatalf("validate: %v", err)
			}
			secs, nanos := decodeDurationFixture(t, c.file)
			if secs != c.secs || nanos != c.nanos {
				t.Errorf("%s: got (%d, %d), want (%d, %d)", c.file, secs, nanos, c.secs, c.nanos)
			}
		})
	}
}

func TestDuration_Rejects(t *testing.T) {
	// A sign inside the literal, and a DIGIT-led token that runs past a
	// unit into more letters, are errors — the latter an invalid duration,
	// never an identifier (draft -01 §3.3, corrected by #234).
	//
	// Rejection is what the draft requires and what is asserted. The
	// reference lexer produces ILLEGAL "invalid duration: 5seconds", but
	// the parser reports it as `expected '{' for message field "d"` —
	// the diagnostic, not the verdict, is tracked at protowire-go#77.
	for _, file := range []string{
		"err-sign-per-segment.pxf",
		"err-digit-led-identifier.pxf",
		"err-unit-then-alpha.pxf",
	} {
		t.Run(file, func(t *testing.T) {
			_, err := runCLI(t, "validate", "-p", durationProto(), "-m", "duration.v1.Holder", durationFixture(file))
			if err == nil {
				t.Fatalf("validate %s: expected an error", file)
			}
		})
	}
}

func TestDuration_FmtNegativeIsFixedPoint(t *testing.T) {
	// The encoder writes a negative Duration as one leading "-" before the
	// first segment — the form the grammar now derives — and formatting the
	// expected file reproduces it byte for byte.
	want, err := os.ReadFile(durationFixture("roundtrip-negative.expected.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"fmt", "-p", durationProto(), "-m", "duration.v1.Holder"}
	got, err := runCLI(t, append(args, durationFixture("roundtrip-negative.pxf"))...)
	if err != nil {
		t.Fatalf("fmt: %v", err)
	}
	if got != string(want) {
		t.Errorf("fmt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	fp, err := runCLI(t, append(args, durationFixture("roundtrip-negative.expected.pxf"))...)
	if err != nil {
		t.Fatalf("fmt (fixed point): %v", err)
	}
	if fp != string(want) {
		t.Errorf("not a fixed point:\n--- got ---\n%s\n--- want ---\n%s", fp, want)
	}
	if !strings.Contains(got, "d = -1.5s") {
		t.Errorf("negative Duration not emitted with one leading '-': %q", got)
	}
}
