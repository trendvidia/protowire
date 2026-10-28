// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// anyDir is the cross-port Any block fixture corpus (testdata/any/,
// issue #354, draft -01 "Any Blocks"), run here through the reference
// implementation with a type resolver over the fixture schema.
const anyDir = "../../testdata/any"

// anyVerdict is what a fixture asserts: the error Parse (no schema) and
// a decoder bound to any.v1.Holder must give, "" for acceptance.
type anyVerdict struct{ parse, decode string }

var anyVerdicts = map[string]anyVerdict{
	"block.pxf":            {},
	"assignment.pxf":       {},
	"url-verbatim.pxf":     {},
	"list-element.pxf":     {},
	"map-value.pxf":        {},
	"plain-fields.pxf":     {},
	"err-not-first.pxf":    {"@type is allowed only as the first entry", `unknown field "tag" in google.protobuf.Any`},
	"err-twice.pxf":        {"@type is allowed only as the first entry", "@type is allowed only as the first entry"},
	"err-not-any.pxf":      {"", "@type is allowed only as the first entry"},
	"err-unresolved.pxf":   {"", `cannot resolve Any type "any.v1.Nope"`},
	"err-unquoted-url.pxf": {"expected a string type URL after @type =", "expected a string type URL after @type ="},
}

// anyGap is a fixture the pinned protowire-go does not yet meet: by is
// the change that closes it, and parse / decode are the pinned release's
// errors, "" where it accepts. Each is asserted to still hold, so the
// go.mod bump that closes the gap fails the test until it is removed.
type anyGap struct{ by, parse, decode string }

// Empty since protowire-go v1.10.0, which implements the type entry of
// an Any block (protowire-go#176).
var anyKnownGaps = map[string]anyGap{}

func anySetup(t *testing.T) (protoreflect.MessageDescriptor, pxf.UnmarshalOptions) {
	t.Helper()
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    []string{filepath.Join(anyDir, "any.proto")},
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	md := reg.Find("any.v1.Holder")
	if md == nil {
		t.Fatal("any.v1.Holder not found")
	}
	return md, pxf.UnmarshalOptions{TypeResolver: typeResolver{find: reg.Find}}
}

func errMatches(err error, want string) bool {
	if want == "" {
		return err == nil
	}
	return err != nil && strings.Contains(err.Error(), want)
}

func TestAny_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(anyDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(anyVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(anyVerdicts))
	}
	md, opts := anySetup(t)
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := anyVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, perr := pxf.Parse(data)
			msg := dynamicpb.NewMessage(md)
			derr := opts.Unmarshal(data, msg)
			if gap, known := anyKnownGaps[name]; known {
				if !errMatches(perr, gap.parse) || !errMatches(derr, gap.decode) {
					t.Fatalf("the pinned protowire-go no longer treats this fixture as before (Parse: %v; decode: %v); gap %s is closed, remove it from anyKnownGaps", perr, derr, gap.by)
				}
				t.Skipf("known gap, closed by %s", gap.by)
			}
			if !errMatches(perr, want.parse) {
				t.Errorf("Parse: want %q, got %v", want.parse, perr)
			}
			if !errMatches(derr, want.decode) {
				t.Errorf("decode: want %q, got %v", want.decode, derr)
			}
		})
	}
}

// TestAny_Values checks what the accept fixtures decode to: the type URL
// as written, and the body packed as an any.v1.Leaf.
func TestAny_Values(t *testing.T) {
	md, opts := anySetup(t)
	f := md.Fields()
	payload := func(m *dynamicpb.Message) protoreflect.Message { return m.Get(f.ByName("payload")).Message() }
	for _, tc := range []struct {
		file, url, tag string
		n              int64
		any            func(*dynamicpb.Message) protoreflect.Message
	}{
		{"block.pxf", "any.v1.Leaf", "t", 7, payload},
		{"assignment.pxf", "any.v1.Leaf", "t", 0, payload},
		{"url-verbatim.pxf", "type.googleapis.com/any.v1.Leaf", "t", 0, payload},
		{"plain-fields.pxf", "any.v1.Leaf", "t", 0, payload},
		{"list-element.pxf", "any.v1.Leaf", "", 2, func(m *dynamicpb.Message) protoreflect.Message {
			return m.Get(f.ByName("anys")).List().Get(1).Message()
		}},
		{"map-value.pxf", "any.v1.Leaf", "t", 0, func(m *dynamicpb.Message) protoreflect.Message {
			return m.Get(f.ByName("by_key")).Map().Get(protoreflect.ValueOfString("k").MapKey()).Message()
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			if _, known := anyKnownGaps[tc.file]; known {
				t.Skip("known gap")
			}
			data, err := os.ReadFile(filepath.Join(anyDir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			m := dynamicpb.NewMessage(md)
			if err := opts.Unmarshal(data, m); err != nil {
				t.Fatal(err)
			}
			a := tc.any(m)
			af := a.Descriptor().Fields()
			if got := a.Get(af.ByName("type_url")).String(); got != tc.url {
				t.Errorf("type_url = %q, want %q", got, tc.url)
			}
			leaf := dynamicpb.NewMessage(md.ParentFile().Messages().ByName("Leaf"))
			if err := proto.Unmarshal(a.Get(af.ByName("value")).Bytes(), leaf); err != nil {
				t.Fatal(err)
			}
			lf := leaf.Descriptor().Fields()
			if got := leaf.Get(lf.ByName("tag")).String(); got != tc.tag {
				t.Errorf("tag = %q, want %q", got, tc.tag)
			}
			if got := leaf.Get(lf.ByName("n")).Int(); got != tc.n {
				t.Errorf("n = %d, want %d", got, tc.n)
			}
		})
	}
}

func anyProto() string { return filepath.Join(anyDir, "any.proto") }

// The CLI reads an Any block that names its type through the loaded
// schema (#356): validate meets every fixture's decode verdict, encode
// writes the accept fixtures, and a @dataset cell resolves the same way.
// Before, every Any block failed with "no TypeResolver".
func TestAny_CLIValidateEncode(t *testing.T) {
	for name, want := range anyVerdicts {
		t.Run(name, func(t *testing.T) {
			f := filepath.Join(anyDir, name)
			// validate parses before it decodes, so a parse verdict is
			// the error it reports.
			verdict := want.decode
			if want.parse != "" {
				verdict = want.parse
			}
			_, err := runCLI(t, "validate", "-p", anyProto(), f)
			if !errMatches(err, verdict) {
				t.Errorf("validate: want %q, got %v", verdict, err)
			}
			if want.decode != "" {
				return
			}
			out, err := runCLI(t, "encode", "-p", anyProto(), "-m", "any.v1.Holder", f)
			if err != nil || out == "" {
				t.Errorf("encode: %d bytes, err %v", len(out), err)
			}
		})
	}

	row := func(url string) string {
		return "@dataset any.v1.Holder (payload)\n({ @type = \"" + url + "\" tag = \"t\" })\n"
	}
	if err := validateDoc(t, row("any.v1.Leaf"), "-p", anyProto(), "-m", "any.v1.Holder"); err != nil {
		t.Errorf("dataset cell: %v", err)
	}
	if err := validateDoc(t, row("any.v1.Nope"), "-p", anyProto(), "-m", "any.v1.Holder"); !errMatches(err, `cannot resolve Any type "any.v1.Nope"`) {
		t.Errorf("dataset cell with an unknown type: want it rejected, got %v", err)
	}
}

// pxf query shows an Any block's type entry as the "@type" key, as
// protojson does, and writes it back as the block's first entry. Strict
// mode checks a path up to the Any and not past it, since only the
// document says what type the Any holds (#356).
func TestAny_CLIQuery(t *testing.T) {
	block := filepath.Join(anyDir, "block.pxf")
	out, err := runCLI(t, "query", "-p", anyProto(), `.payload["@type"]`, block)
	if err != nil || out != "value = \"any.v1.Leaf\"\n" {
		t.Errorf(`query .payload["@type"]: %q, %v`, out, err)
	}
	if out, err := runCLI(t, "query", "-p", anyProto(), ".payload.tag", block); err != nil || !strings.Contains(out, `"t"`) {
		t.Errorf("strict query past an Any: %q, %v", out, err)
	}
	if _, err := runCLI(t, "query", "-p", anyProto(), ".paylod", block); !errMatches(err, `unknown field "paylod"`) {
		t.Errorf("strict mode still checks up to the Any: want unknown field, got %v", err)
	}

	// The whole document written back is valid PXF that decodes the same.
	whole, err := runCLI(t, "query", "-p", anyProto(), ".", block)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDoc(t, whole, "-p", anyProto(), "-m", "any.v1.Holder"); err != nil {
		t.Errorf("query output does not validate: %v\n%s", err, whole)
	}
}
