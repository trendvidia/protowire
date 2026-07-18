// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// keyedDir is the cross-port keyed-repeated-field fixture corpus
// (testdata/keyed/, spec issue #116, draft -01 §3.13), reused here to
// check the cmd/pxf wiring end-to-end rather than re-test the library.
const keyedDir = "../../testdata/keyed"

func keyedProto() string { return filepath.Join(keyedDir, "keyed.proto") }

func keyedFixturePath(name string) string { return filepath.Join(keyedDir, name) }

// runCLI drives a fresh pxf command tree through Execute with args and
// returns whatever it wrote to stdout plus the command error. os.Stdout
// is swapped for a pipe (drained on a goroutine so large output can't
// deadlock); the package flag globals are reset by newRootCmd's flag
// binding on each call, so runs don't leak into each other.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := newRootCmd()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs(args)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	execErr := root.Execute()
	_ = w.Close()
	os.Stdout = old
	return <-done, execErr
}

func TestKeyed_ValidateAccepts(t *testing.T) {
	cases := []struct{ file, msg string }{
		{"roundtrip-keyed.pxf", "keyed.v1.Node"},
		{"roundtrip-quoted.pxf", "keyed.v1.Deployment"},
		{"anonymous-equivalence.pxf", "keyed.v1.Node"},
		{"redundant-key-ok.pxf", "keyed.v1.Node"},
		{"anonymous-duplicate-ok.pxf", "keyed.v1.Node"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			_, err := runCLI(t, "validate", "-p", keyedProto(), "-m", c.msg, keyedFixturePath(c.file))
			if err != nil {
				t.Fatalf("validate %s: unexpected error: %v", c.file, err)
			}
		})
	}
}

func TestKeyed_ValidateRejects(t *testing.T) {
	cases := []struct {
		file string
		msg  string
		kind pxf.KeyedErrorKind
	}{
		{"err-duplicate-key.pxf", "keyed.v1.Node", pxf.KeyedDuplicateKey},
		{"err-duplicate-key-spelling.pxf", "keyed.v1.Node", pxf.KeyedDuplicateKey},
		{"err-key-conflict.pxf", "keyed.v1.Node", pxf.KeyedKeyConflict},
		{"err-empty-key.pxf", "keyed.v1.Node", pxf.KeyedEmptyKey},
		{"err-empty-key-anonymous.pxf", "keyed.v1.Node", pxf.KeyedEmptyKey},
		{"err-quoted-name-unkeyed.pxf", "keyed.v1.Doc", pxf.KeyedQuotedNameUnkeyed},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			_, err := runCLI(t, "validate", "-p", keyedProto(), "-m", c.msg, keyedFixturePath(c.file))
			if err == nil {
				t.Fatalf("validate %s: expected error, got nil", c.file)
			}
			var ke *pxf.KeyedError
			if !errors.As(err, &ke) {
				t.Fatalf("validate %s: want *pxf.KeyedError, got %T: %v", c.file, err, err)
			}
			if ke.Kind != c.kind {
				t.Errorf("validate %s: kind = %v, want %v", c.file, ke.Kind, c.kind)
			}
		})
	}
}

func TestKeyed_FmtCanonicalization(t *testing.T) {
	// fmt-unquote binds via -m; fmt-anonymous-to-keyed relies on the
	// document's @type directive (no -m) to also cover that resolution
	// path. Each input must format to exactly its .expected.pxf, and
	// the expected file must be a fmt fixed point.
	cases := []struct {
		pair string
		args []string
	}{
		{"fmt-unquote", []string{"-p", keyedProto(), "-m", "keyed.v1.Node"}},
		{"fmt-anonymous-to-keyed", []string{"-p", keyedProto()}},
	}
	for _, c := range cases {
		t.Run(c.pair, func(t *testing.T) {
			want, err := os.ReadFile(keyedFixturePath(c.pair + ".expected.pxf"))
			if err != nil {
				t.Fatal(err)
			}

			got, err := runCLI(t, append(append([]string{"fmt"}, c.args...), keyedFixturePath(c.pair+".pxf"))...)
			if err != nil {
				t.Fatalf("fmt %s: %v", c.pair, err)
			}
			if got != string(want) {
				t.Errorf("fmt %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", c.pair, got, want)
			}

			// The expected file is a fmt fixed point.
			fp, err := runCLI(t, append(append([]string{"fmt"}, c.args...), keyedFixturePath(c.pair+".expected.pxf"))...)
			if err != nil {
				t.Fatalf("fmt %s (fixed point): %v", c.pair, err)
			}
			if fp != string(want) {
				t.Errorf("fmt %s not a fixed point:\n--- got ---\n%s\n--- want ---\n%s", c.pair, fp, want)
			}
		})
	}
}

func TestKeyed_EncodeDeterministicAndFormEquivalent(t *testing.T) {
	enc := func(file string) string {
		t.Helper()
		out, err := runCLI(t, "encode", "-p", keyedProto(), "-m", "keyed.v1.Node", keyedFixturePath(file))
		if err != nil {
			t.Fatalf("encode %s: %v", file, err)
		}
		return out
	}

	// Deterministic: the same document yields the same bytes every time.
	keyed := enc("roundtrip-keyed.pxf")
	if again := enc("roundtrip-keyed.pxf"); again != keyed {
		t.Fatal("encode is not deterministic across runs")
	}

	// The keyed block form and the anonymous list form of the same value
	// are wire-equivalent: identical protobuf bytes under deterministic
	// (canonical field-order) marshaling.
	if anon := enc("anonymous-equivalence.pxf"); anon != keyed {
		t.Error("keyed and anonymous forms did not encode to identical bytes")
	}
}
