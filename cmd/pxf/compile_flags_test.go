// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sbeOrderProto = `syntax = "proto3";
package orders.v1;

import "sbe/annotations.proto";

option (sbe.schema_id) = 1;
option (sbe.version) = 0;

message Order {
  option (sbe.template_id) = 1;
  uint64 id = 1;
}
`

// protoWorkdir chdirs to a temp directory holding name and the annotation
// schemas it may import, since -p resolves imports from the working
// directory.
func protoWorkdir(t *testing.T, name, body string) {
	t.Helper()
	files := map[string]string{name: body}
	for _, lib := range []string{"sbe/annotations.proto", "schema/v1/annotations.proto"} {
		data, err := os.ReadFile(filepath.Join("../../proto", lib))
		if err != nil {
			t.Fatal(err)
		}
		files[lib] = string(data)
	}
	t.Chdir(t.TempDir())
	if err := os.Mkdir("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestProto2SBEPathSpellings pins that every spelling of a -p path names
// the same file. The compiler reports cleaned paths, so a filter on the
// flag's spelling dropped ./order.proto and left nothing to convert (#459).
func TestProto2SBEPathSpellings(t *testing.T) {
	protoWorkdir(t, "order.proto", sbeOrderProto)
	abs, err := filepath.Abs("order.proto")
	if err != nil {
		t.Fatal(err)
	}
	want, err := runCLI(t, "proto2sbe", "-p", "order.proto")
	if err != nil {
		t.Fatalf("order.proto: %v", err)
	}
	if !strings.Contains(want, `package="orders.v1"`) {
		t.Fatalf("no schema in output:\n%s", want)
	}
	for _, path := range []string{"./order.proto", "sub/../order.proto", abs} {
		got, err := runCLI(t, "proto2sbe", "-p", path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
		} else if got != want {
			t.Errorf("%s: output differs from order.proto:\n%s", path, got)
		}
	}
}

// TestProto2SBEOmitsImports pins that only the -p files are converted: an
// imported schema carries no (sbe.schema_id) and would fail the run.
func TestProto2SBEOmitsImports(t *testing.T) {
	protoWorkdir(t, "order.proto", sbeOrderProto)
	if err := os.WriteFile("root.proto", []byte(strings.Replace(sbeOrderProto,
		`import "sbe/annotations.proto";`,
		"import \"sbe/annotations.proto\";\nimport \"plain.proto\";", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("plain.proto", []byte("syntax = \"proto3\";\npackage plain.v1;\nmessage Plain {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "proto2sbe", "-p", "root.proto"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestV12SourceOnProtoFlag pins the behaviour #459 states: lint and
// proto2sbe compile with the reference compiler, so they read v1.2 source
// as encode and validate do, and lint still reports what it exists for.
func TestV12SourceOnProtoFlag(t *testing.T) {
	const annotated = `syntax = "proto3";
package orders.v1;

import "sbe/annotations.proto";
import "schema/v1/annotations.proto";

option (sbe.schema_id) = 1;
option (sbe.version) = 0;

type OrderID = uint64 @validate(this > 0);

message Order {
  option (sbe.template_id) = 1;
  OrderID id = 1;
}
`
	t.Run("lint", func(t *testing.T) {
		protoWorkdir(t, "order.proto", annotated)
		if _, err := runCLI(t, "lint", "-p", "order.proto"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("lint reports a reserved name", func(t *testing.T) {
		protoWorkdir(t, "order.proto", `syntax = "proto3";
package orders.v1;

import "schema/v1/annotations.proto";

message Order {
  string null = 1 @required;
}
`)
		_, err := runCLI(t, "lint", "-p", "order.proto")
		if err == nil || !strings.Contains(err.Error(), "1 reserved-name violation") {
			t.Fatalf("want one reserved-name violation, got %v", err)
		}
	})
	t.Run("proto2sbe", func(t *testing.T) {
		protoWorkdir(t, "order.proto", annotated)
		out, err := runCLI(t, "proto2sbe", "-p", "order.proto")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `package="orders.v1"`) {
			t.Fatalf("no schema in output:\n%s", out)
		}
	})
}
