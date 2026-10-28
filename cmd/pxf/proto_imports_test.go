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

// A -p type that refers to a type the document redefines still holds
// -p's definition (#387, not decided). The warnings say so, where before
// the first one alone claimed the @proto definition was in use. This
// pins the current behaviour: update it with #387's decision.
func TestProtoShadowing_Referrer(t *testing.T) {
	dir := t.TempDir()
	pFile := filepath.Join(dir, "l.proto")
	if err := os.WriteFile(pFile, []byte("syntax = \"proto3\";\npackage l.v1;\nmessage L { int32 user = 1; }\nmessage W { L l = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `@proto """
syntax = "proto3";
package l.v1;
message L { string user = 1; }
"""
@type l.v1.W
l { user = "ann" }
`
	printedWarnings.Clear()
	var err error
	stderr := captureStderr(t, func() { err = validateDoc(t, doc, "-p", pFile) })
	if !errMatches(err, `expected integer for field "user"`) {
		t.Errorf("want W.l decoded against l.proto's L, got %v", err)
	}
	for _, w := range []string{
		"warning: @proto defines l.v1.L differently from " + pFile,
		"warning: l.v1.W from " + pFile + " still uses " + pFile + "'s l.v1.L, not the @proto one",
	} {
		if !strings.Contains(stderr, w) {
			t.Errorf("stderr lacks %q; stderr:\n%s", w, stderr)
		}
	}
}
