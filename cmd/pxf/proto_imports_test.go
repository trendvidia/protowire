// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// protoImportsDir is the cross-port fixture corpus for what a
// source-form @proto may import (testdata/proto-imports/, issue #369,
// draft -01 § The "@proto" Directive).
const protoImportsDir = "../../testdata/proto-imports"

// protoImportsVerdicts is each fixture's error substring, "" for accept.
var protoImportsVerdicts = map[string]string{
	"secret.pxf":          "",
	"bignum.pxf":          "",
	"envelope.pxf":        "",
	"well-known.pxf":      "",
	"err-tool-schema.pxf": "a @proto may import only",
	"err-filesystem.pxf":  "a @proto may import only",
	"err-plugin.pxf":      "a @proto may import only",
	"err-features.pxf":    "a @proto may import only",
	"descriptor.pxf":      "",
}

// Runs from the fixture directory, so err-filesystem.pxf's sibling.proto
// is there to be found if pxf looked on disk.
func TestProtoImports_Corpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(protoImportsDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(protoImportsVerdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(protoImportsVerdicts))
	}
	t.Chdir(protoImportsDir)
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := protoImportsVerdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			if _, err := runCLI(t, "validate", name); !errMatches(err, want) {
				t.Errorf("validate: want %q, got %v", want, err)
			}
		})
	}
}

// A @proto cannot import a -p file either, even under the exact name
// -p compiles it as: -p is an external schema, not one of the draft's
// four import sources.
func TestProtoImports_NotFromProtoFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("dep.proto", []byte("syntax = \"proto3\";\npackage d.v1;\nmessage D { int32 a = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `@proto """
syntax = "proto3";
package l.v1;
import "dep.proto";
message L { d.v1.D d = 1; }
"""
@type l.v1.L
d { a = 1 }
`
	if err := validateDoc(t, doc, "-p", "dep.proto"); !errMatches(err, "a @proto may import only") {
		t.Fatalf("want the import rejected, got %v", err)
	}
}

// shadowDocs returns a body, a body with no @type, and a @dataset
// document, each with its own @proto defining l.v1.L with user of the
// given type.
func shadowDocs(userType string) (body, untyped, dataset string) {
	proto := `@proto """
syntax = "proto3";
package l.v1;
message L { ` + userType + ` user = 1; }
"""
`
	return proto + "@type l.v1.L\nuser = \"ann\"\n", proto + "user = \"ann\"\n", proto + "@dataset l.v1.L (user)\n(\"ann\")\n"
}

// -p and a document's @proto both defining l.v1.L: every command uses
// the document's type, and warns only when -p's definition differs
// (#371). Before, @dataset and query failed with `symbol "l.v1.L"
// already defined` even when the two agreed.
func TestProtoShadowing(t *testing.T) {
	body, untyped, dataset := shadowDocs("string")
	for _, c := range []struct {
		name, pType string
		warn        bool
	}{
		{"identical -p", "string", false},
		{"different -p", "int32", true},
	} {
		pFile := filepath.Join(t.TempDir(), "l.proto")
		src := "syntax = \"proto3\";\npackage l.v1;\nmessage L { " + c.pType + " user = 1; }\n"
		if err := os.WriteFile(pFile, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		write := func(name, doc string) string {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}
		bodyPath, untypedPath, datasetPath := write("body.pxf", body), write("untyped.pxf", untyped), write("dataset.pxf", dataset)
		for _, cmd := range []struct {
			name string
			args []string
		}{
			{"validate body", []string{"validate", "-p", pFile, bodyPath}},
			// -m names the document's type: the embedded schema is
			// authoritative under any name, not only under @type.
			{"validate -m body", []string{"validate", "-p", pFile, "-m", "l.v1.L", untypedPath}},
			{"validate @dataset", []string{"validate", "-p", pFile, "-m", "l.v1.L", datasetPath}},
			{"query", []string{"query", "-p", pFile, ".", datasetPath}},
			{"encode", []string{"encode", "-p", pFile, "-m", "l.v1.L", untypedPath}},
			{"fmt", []string{"fmt", "-p", pFile, bodyPath}},
		} {
			t.Run(c.name+"/"+cmd.name, func(t *testing.T) {
				printedWarnings.Clear()
				var err error
				stderr := captureStderr(t, func() { _, err = runCLI(t, cmd.args...) })
				if err != nil {
					t.Fatalf("want the document's type used, got: %v", err)
				}
				got := strings.Contains(stderr, "warning: @proto defines l.v1.L differently from ")
				if got != c.warn {
					t.Fatalf("warning on stderr = %v, want %v; stderr:\n%s", got, c.warn, stderr)
				}
				if c.warn && !strings.Contains(stderr, filepath.Base(pFile)) {
					t.Errorf("warning does not name the -p file; stderr:\n%s", stderr)
				}
			})
		}
	}
}

// encode and fmt read a self-describing document with no -p: the type
// its @type or -m names is the document's own. Before, both resolved it
// against -p and protoregistry only, and failed with "not found".
func TestProtoShadowing_SelfDescribing(t *testing.T) {
	body, untyped, _ := shadowDocs("string")
	dir := t.TempDir()
	bodyPath, untypedPath := filepath.Join(dir, "body.pxf"), filepath.Join(dir, "untyped.pxf")
	for path, doc := range map[string]string{bodyPath: body, untypedPath: untyped} {
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runCLI(t, "encode", "-m", "l.v1.L", untypedPath)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Field 1, wire type 2 (length-delimited), "ann".
	if want := "\x0a\x03ann"; out != want {
		t.Errorf("encode wrote %q, want %q", out, want)
	}
	out, err = runCLI(t, "fmt", bodyPath)
	if err != nil {
		t.Fatalf("fmt: %v", err)
	}
	if !strings.Contains(out, `user = "ann"`) {
		t.Errorf("fmt output lacks the body:\n%s", out)
	}
}

// An external type that refers to a type the document's @proto redefines
// differently still holds the external definition in that field, so
// decoding through it is rejected, naming both (decided by the owner on
// #387). Before, pxf warned and decoded W.l against l.proto's L. Every
// command that picks a type to decode with checks it; a document whose
// type does not reach W is unaffected.
func TestProtoShadowing_Referrer(t *testing.T) {
	dir := t.TempDir()
	pFile := filepath.Join(dir, "l.proto")
	if err := os.WriteFile(pFile, []byte("syntax = \"proto3\";\npackage l.v1;\nmessage L { int32 user = 1; }\nmessage W { L l = 1; }\nmessage U { int32 n = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const proto = `@proto """
syntax = "proto3";
package l.v1;
message L { string user = 1; }
"""
`
	want := "l.v1.W.l, from " + pFile + ", has type l.v1.L as " + pFile + " defines it, but the document's @proto defines l.v1.L differently; add l.v1.W to the @proto, or bring " + pFile + " in line with it"
	write := func(doc string) string {
		p := filepath.Join(t.TempDir(), "d.pxf")
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	body, dataset := write(proto+"@type l.v1.W\nl { user = \"ann\" }\n"), write(proto+"@dataset l.v1.W (l.user)\n(\"ann\")\n")
	for _, c := range []struct {
		name string
		args []string
	}{
		{"validate body", []string{"validate", "-p", pFile, body}},
		{"validate @dataset", []string{"validate", "-p", pFile, "-m", "l.v1.W", dataset}},
		{"query", []string{"query", "-p", pFile, "-m", "l.v1.W", ".", body}},
		{"encode", []string{"encode", "-p", pFile, "-m", "l.v1.W", body}},
		{"fmt", []string{"fmt", "-p", pFile, body}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := runCLI(t, c.args...); !errMatches(err, want) {
				t.Fatalf("want %q, got %v", want, err)
			}
		})
	}
	if err := validateDoc(t, proto+"@type l.v1.U\nn = 1\n", "-p", pFile); err != nil {
		t.Errorf("a type that does not reach W is rejected: %v", err)
	}
}
