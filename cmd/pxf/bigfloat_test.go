// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// bigfloatDir is the cross-port pxf.BigFloat text-form corpus
// (testdata/bigfloat/, issues #393 and #406, draft -01 § PXF
// Annotations). generate.py computes every expected value with exact
// rational arithmetic, independently of any port. An enc- case is PB
// bytes and the PXF text an encoder writes for them; a dec- case is a
// PXF document and the PB bytes a decoder produces; an err- case is a
// document a decoder rejects.
const bigfloatDir = "../../testdata/bigfloat"

// bigfloatKnownGaps are cases the pinned protowire-go does not yet meet,
// mapped to the change that closes them. Each is asserted to still fail,
// so the go.mod bump that closes one fails the test until it is removed.
//
// v1.11.0 writes no precision suffix and one digit too few, cannot lex a
// suffixed literal, and stores a zero literal as exponent -256 at 256
// bits (protowire-go#194). Its err- verdicts already hold, but only
// because a suffix does not lex at all.
var bigfloatKnownGaps = map[string]string{
	"enc-2pow1024-prec64":         "protowire-go#194",
	"enc-integer":                 "protowire-go#194",
	"enc-long-mantissa":           "protowire-go#194",
	"enc-one-and-a-half":          "protowire-go#194",
	"enc-tenth-as-double":         "protowire-go#194",
	"enc-three-quarters-negative": "protowire-go#194",
	"enc-tiny-negative-exponent":  "protowire-go#194",
	"dec-2pow1024-p64":            "protowire-go#194",
	"dec-hundred-digits-p333":     "protowire-go#194",
	"dec-tenth-p53":               "protowire-go#194",
	"dec-zero":                    "protowire-go#194",
}

func TestBigFloat_Corpus(t *testing.T) {
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{filepath.Join(bigfloatDir, "bigfloat.proto")},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("bf.v1.H")
	if md == nil {
		t.Fatal("bf.v1.H not found")
	}
	pxfs, err := filepath.Glob(filepath.Join(bigfloatDir, "*.pxf"))
	if err != nil || len(pxfs) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, p := range pxfs {
		name := strings.TrimSuffix(filepath.Base(p), ".pxf")
		t.Run(name, func(t *testing.T) {
			text, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			var wire []byte
			if !strings.HasPrefix(name, "err-") {
				if wire, err = os.ReadFile(strings.TrimSuffix(p, ".pxf") + ".binpb"); err != nil {
					t.Fatal(err)
				}
			}
			var problem string
			switch {
			case strings.HasPrefix(name, "enc-"):
				msg := dynamicpb.NewMessage(md)
				if err := proto.Unmarshal(wire, msg); err != nil {
					t.Fatal(err)
				}
				out, err := pxf.Marshal(msg)
				switch {
				case err != nil:
					problem = "Marshal: " + err.Error()
				case string(out) != string(text):
					problem = "Marshal wrote " + strings.TrimSpace(string(out)) + ", want " + strings.TrimSpace(string(text))
				}
			case strings.HasPrefix(name, "dec-"):
				msg, err := pxf.UnmarshalDescriptor(text, md)
				if err != nil {
					problem = "decode: " + err.Error()
					break
				}
				got, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, wire) {
					problem = "decode wrote PB " + hexOf(got) + ", want " + hexOf(wire)
				}
			case strings.HasPrefix(name, "err-"):
				if _, err := pxf.UnmarshalDescriptor(text, md); err == nil {
					problem = "decode accepted a document it must reject"
				}
			default:
				t.Fatalf("fixture %s is neither enc-, dec- nor err-", name)
			}
			if by, known := bigfloatKnownGaps[name]; known {
				if problem == "" {
					t.Fatalf("the pinned protowire-go now meets this case; gap %s is closed, remove it from bigfloatKnownGaps", by)
				}
				t.Skipf("known gap, closed by %s: %s", by, problem)
			}
			if problem != "" {
				t.Error(problem)
			}
		})
	}
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	var sb strings.Builder
	for _, c := range b {
		sb.WriteByte(digits[c>>4])
		sb.WriteByte(digits[c&15])
	}
	return sb.String()
}
