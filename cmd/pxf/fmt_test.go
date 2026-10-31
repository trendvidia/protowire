// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trendvidia/protocompile/ast"
)

const fmtFixtureDir = "../../testdata/fmt"

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fmtFixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// copyFixture copies a fixture into a temporary directory, so -w can
// rewrite it.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(readFixture(t, name)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFmtProtoKeepsAnnotations: a v1.2 source keeps every annotation use
// and comment through formatting, the output parses, and formatting it
// again changes nothing (#425).
func TestFmtProtoKeepsAnnotations(t *testing.T) {
	in := readFixture(t, "annotated.proto")
	out, err := runCLI(t, "fmt", filepath.Join(fmtFixtureDir, "annotated.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if out == in {
		t.Fatal("annotated.proto came back unchanged; the fixture is meant to need formatting")
	}
	if got, want := strings.Count(out, "@"), strings.Count(in, "@"); got != want || want != 9 {
		t.Errorf("annotation uses: %d in the output, %d in the input; want 9 in both", got, want)
	}
	for _, c := range []string{"// Tempo is a closed range", "// min doc.", "// trailing on max", "/* Mode block comment. */", "// Get doc."} {
		if !strings.Contains(out, c) {
			t.Errorf("the output lost the comment %q", c)
		}
	}

	path := filepath.Join(t.TempDir(), "annotated.proto")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := runCLI(t, "fmt", path)
	if err != nil {
		t.Fatalf("formatting the formatted output: %v", err)
	}
	if again != out {
		t.Errorf("formatting is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

// TestFmtProtoMatchesBufFormat: on a file with no v1.2 syntax, pxf fmt
// writes what buf format writes, so a project switching over does not
// reformat its untouched files (#425). Measured beyond this fixture: on
// every stock .proto in this repository that buf format accepts (39
// files, protocompile v0.36.0), the two were byte-identical.
func TestFmtProtoMatchesBufFormat(t *testing.T) {
	out, err := runCLI(t, "fmt", filepath.Join(fmtFixtureDir, "stock.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if want := readFixture(t, "stock.buf-format.proto"); out != want {
		t.Errorf("pxf fmt differs from buf format:\n got:\n%s\nwant:\n%s", out, want)
	}
}

// TestFmtCheckAndWrite: --check fails with a diff on an unformatted file
// and passes on a formatted one; -w rewrites in place.
func TestFmtCheckAndWrite(t *testing.T) {
	path := copyFixture(t, "stock.proto")

	out, err := runCLI(t, "fmt", "--check", path)
	if err == nil || !strings.Contains(err.Error(), "1 of 1 file(s) not formatted") {
		t.Errorf("--check on an unformatted file: err = %v, want 1 of 1 file(s) not formatted", err)
	}
	if !strings.Contains(out, "--- "+path) || !strings.Contains(out, "+message Req {") {
		t.Errorf("--check printed no diff for %s:\n%s", path, out)
	}
	if got := readFile(t, path); got != readFixture(t, "stock.proto") {
		t.Error("--check rewrote the file")
	}

	if _, err := runCLI(t, "fmt", "-w", path); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), readFixture(t, "stock.buf-format.proto"); got != want {
		t.Errorf("-w wrote:\n%s\nwant:\n%s", got, want)
	}
	if out, err := runCLI(t, "fmt", "--check", path); err != nil || out != "" {
		t.Errorf("--check on the formatted file: err = %v, output %q", err, out)
	}
}

// TestFmtFixtureCompiles: pxf fmt does not link what it formats, so nothing
// else notices when the compiler starts refusing annotated.proto (#469).
func TestFmtFixtureCompiles(t *testing.T) {
	src := filepath.Join(fmtFixtureDir, "annotated.proto")
	image := filepath.Join(t.TempDir(), "annotated.binpb")
	if _, err := runCLI(t, "build", src, "-o", image); err != nil {
		t.Fatalf("pxf build %s: %v", src, err)
	}
	if info, err := os.Stat(image); err != nil || info.Size() == 0 {
		t.Fatalf("pxf build wrote no image: %v", err)
	}
}

// TestFmtDirectory: a directory argument covers every .proto and .pxf
// beneath it, and printing more than one file to stdout is refused.
func TestFmtDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"stock.proto", "annotated.proto"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(readFixture(t, name)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runCLI(t, "fmt", dir); err == nil || !strings.Contains(err.Error(), "2 files to format") {
		t.Errorf("stdout with a directory of two files: err = %v, want 2 files to format", err)
	}
	if _, err := runCLI(t, "fmt", "--check", dir); err == nil || !strings.Contains(err.Error(), "2 of 2 file(s) not formatted") {
		t.Errorf("--check on the directory: err = %v, want 2 of 2", err)
	}
	if _, err := runCLI(t, "fmt", "-w", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "fmt", "--check", dir); err != nil {
		t.Errorf("--check after -w: %v", err)
	}
}

// TestFmtKeepsCommentAfterBody: the printer used to drop a comment after
// `};` with no error (trendvidia/protocompile#273), and pxf fmt refused
// the file. Since protocompile v0.36.0 the comment survives, so the file
// formats.
func TestFmtKeepsCommentAfterBody(t *testing.T) {
	src := "syntax = \"proto3\";\npackage fmt.v1;\n\nmessage Foo {\n  oneof choice {\n    string a = 1;\n  }; // this comment is dropped\n}\n"
	path := filepath.Join(t.TempDir(), "drop.proto")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "fmt", "-w", path); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.Contains(got, "// this comment is dropped") {
		t.Errorf("the comment after }; was lost:\n%s", got)
	}
}

// TestLostTokens: the check pxf fmt runs on the printer's output names
// what the output lacks, so a printer that drops source again (as
// protocompile#258 and #273 did) is refused instead of written.
func TestLostTokens(t *testing.T) {
	parse := func(src string) *ast.File {
		t.Helper()
		f, err := parseProto("x.proto", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	before := parse("syntax = \"proto3\";\npackage p;\n// doc\nmessage M {\n  int32 a = 1 @required; // trailing\n}\n")
	for _, c := range []struct{ after, want string }{
		{"syntax = \"proto3\";\npackage p;\n// doc\nmessage M {\n  int32 a = 1 @required; // trailing\n}\n", ""},
		{"syntax = \"proto3\";\npackage p;\n// doc\nmessage M {\n  int32 a = 1; // trailing\n}\n", "Ident required (1 of 1), annotation use @ (1 of 1)"},
		{"syntax = \"proto3\";\npackage p;\nmessage M {\n  int32 a = 1 @required;\n}\n", "comment //doc (1 of 1), comment //trailing (1 of 1)"},
	} {
		if got := lostTokens(before, parse(c.after)); got != c.want {
			t.Errorf("lostTokens = %q, want %q", got, c.want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
