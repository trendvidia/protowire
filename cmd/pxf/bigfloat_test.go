// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// bigfloatDir is the cross-port pxf.BigFloat text-form corpus
// (testdata/bigfloat/, issues #393 and #406, draft -01 § PXF
// Annotations). generate.py computes every expected value with exact
// rational arithmetic, independently of any port. An enc- case is PB
// bytes and the PXF text an encoder writes for them; a dec- case is a
// PXF document and the PB bytes a decoder produces; an err- case is a
// document a decoder rejects.
const bigfloatDir = "../../testdata/bigfloat"

// bigfloatVerdicts lists every fixture. An err- case's verdict is a
// substring its error must contain, "" for any error; the draft has a
// rejection name the field only for a suffix on a field that isn't a
// BigFloat. The other cases' expected values are their .pxf/.binpb pair.
var bigfloatVerdicts = map[string]string{
	"enc-2pow1024-prec64.pxf":           "",
	"enc-integer.pxf":                   "",
	"enc-long-mantissa.pxf":             "",
	"enc-one-and-a-half.pxf":            "",
	"enc-prec-zero.pxf":                 "",
	"enc-tenth-as-double.pxf":           "",
	"enc-three-quarters-negative.pxf":   "",
	"enc-tiny-negative-exponent.pxf":    "",
	"dec-1e4932.pxf":                    "",
	"dec-2pow1024-p64.pxf":              "",
	"dec-exponent-only-p24.pxf":         "",
	"dec-hundred-digits-p333.pxf":       "",
	"dec-hundred-digits-unsuffixed.pxf": "",
	"dec-integer-p64.pxf":               "",
	"dec-minus-two-and-a-half.pxf":      "",
	"dec-negative-zero.pxf":             "",
	"dec-one-at-max-precision.pxf":      "",
	"dec-rounds-up-to-power-p8.pxf":     "",
	"dec-small-positional.pxf":          "",
	"dec-tenth-p53.pxf":                 "",
	"dec-tenth-unsuffixed.pxf":          "",
	"dec-zero.pxf":                      "",
	"err-precision-overflow.pxf":        "",
	"err-precision-past-limit.pxf":      "",
	"err-precision-zero.pxf":            "",
	"err-suffix-on-int32.pxf":           "count",
}

// bigfloatKnownGaps are cases the pinned protowire-go does not yet meet,
// mapped to the change that closes them. Each is asserted to still fail,
// so the go.mod bump that closes one fails the test until it is removed.
//
// v1.11.0 writes no precision suffix and one digit too few, cannot lex a
// suffixed literal, and stores a zero literal as exponent -256 at 256
// bits (protowire-go#194). It rejects every err- case, but only because a
// suffix does not lex at all, which bigfloatSuffixControl tells apart.
var bigfloatKnownGaps = map[string]string{
	"enc-2pow1024-prec64.pxf":         "protowire-go#194",
	"enc-integer.pxf":                 "protowire-go#194",
	"enc-long-mantissa.pxf":           "protowire-go#194",
	"enc-one-and-a-half.pxf":          "protowire-go#194",
	"enc-tenth-as-double.pxf":         "protowire-go#194",
	"enc-three-quarters-negative.pxf": "protowire-go#194",
	"enc-tiny-negative-exponent.pxf":  "protowire-go#194",
	"dec-2pow1024-p64.pxf":            "protowire-go#194",
	"dec-exponent-only-p24.pxf":       "protowire-go#194",
	"dec-hundred-digits-p333.pxf":     "protowire-go#194",
	"dec-integer-p64.pxf":             "protowire-go#194",
	"dec-negative-zero.pxf":           "protowire-go#194",
	"dec-one-at-max-precision.pxf":    "protowire-go#194",
	"dec-rounds-up-to-power-p8.pxf":   "protowire-go#194",
	"dec-tenth-p53.pxf":               "protowire-go#194",
	"dec-zero.pxf":                    "protowire-go#194",
	"err-precision-overflow.pxf":      "protowire-go#194",
	"err-precision-past-limit.pxf":    "protowire-go#194",
	"err-precision-zero.pxf":          "protowire-go#194",
	"err-suffix-on-int32.pxf":         "protowire-go#194",
}

// bigfloatSuffixControl is a well-formed suffixed document. A decoder
// that rejects it cannot lex a precision suffix at all, so its rejection
// of an err- case says nothing about the rule that case pins.
const bigfloatSuffixControl = "@type bf.v1.H\n\nbf = 1.5p53\n"

func TestBigFloat_Corpus(t *testing.T) {
	runCorpus(t, bigfloatDir, []string{"bigfloat.proto"}, bigfloatVerdicts, func(t *testing.T, f fixture[string]) {
		md := mustFind(t, f.reg, "bf.v1.H")
		var problem string
		switch {
		case strings.HasPrefix(f.name, "enc-"):
			problem = bigfloatEncode(t, md, f)
		case strings.HasPrefix(f.name, "dec-"):
			problem = bigfloatDecode(t, md, f)
		case strings.HasPrefix(f.name, "err-"):
			problem = bigfloatReject(md, f)
		default:
			t.Fatalf("fixture %s is neither enc-, dec- nor err-", f.name)
		}
		if by, known := bigfloatKnownGaps[f.name]; known {
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

// bigfloatWire reads the PB half of an enc- or dec- case.
func bigfloatWire(t *testing.T, f fixture[string]) []byte {
	t.Helper()
	wire, err := os.ReadFile(strings.TrimSuffix(f.path, ".pxf") + ".binpb")
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func bigfloatEncode(t *testing.T, md protoreflect.MessageDescriptor, f fixture[string]) string {
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(bigfloatWire(t, f), msg); err != nil {
		t.Fatal(err)
	}
	out, err := pxf.Marshal(msg)
	switch {
	case err != nil:
		return "Marshal: " + err.Error()
	case string(out) != string(f.data):
		return "Marshal wrote " + strings.TrimSpace(string(out)) + ", want " + strings.TrimSpace(string(f.data))
	}
	return ""
}

func bigfloatDecode(t *testing.T, md protoreflect.MessageDescriptor, f fixture[string]) string {
	msg, err := pxf.UnmarshalDescriptor(f.data, md)
	if err != nil {
		return "decode: " + err.Error()
	}
	got, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if want := bigfloatWire(t, f); !bytes.Equal(got, want) {
		return "decode wrote PB " + hex.EncodeToString(got) + ", want " + hex.EncodeToString(want)
	}
	return ""
}

func bigfloatReject(md protoreflect.MessageDescriptor, f fixture[string]) string {
	_, err := pxf.UnmarshalDescriptor(f.data, md)
	switch {
	case err == nil:
		return "decode accepted a document it must reject"
	case !strings.Contains(err.Error(), f.want):
		return "decode rejected without naming " + f.want + ": " + err.Error()
	}
	if _, cerr := pxf.UnmarshalDescriptor([]byte(bigfloatSuffixControl), md); cerr != nil {
		return "decode rejected it (" + err.Error() + "), but it rejects every precision suffix, the well-formed bf = 1.5p53 too (" + cerr.Error() + ")"
	}
	return ""
}
