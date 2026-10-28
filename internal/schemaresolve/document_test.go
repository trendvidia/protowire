// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package schemaresolve

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/known/fieldmaskpb"
	_ "google.golang.org/protobuf/types/known/wrapperspb"
)

// itemSource is a document's own @proto defining reg.v1.Item, the type
// fakeFDS serves from the loopback protoregistry.
func itemSource(fieldType string) VirtualFile {
	return VirtualFile{Name: "__indoc_0.proto", Body: []byte(`syntax = "proto3";
package reg.v1;
message Item { ` + fieldType + ` name = 1; }
`)}
}

// A document's own type shadows a protoregistry type of the same name,
// and a warning names the difference only when the two differ (#371).
// Before, the registry was merged last and replaced the document's type.
func TestResolve_DocumentShadowsRegistry(t *testing.T) {
	addr := startLoopbackServer(t, fakeFDS())
	ref := RegistryRef{Server: addr, Namespace: "ns", Schema: "s"}
	cases := []struct {
		name, fieldType, wantKind string
		warn                      bool
	}{
		{"identical", "string", "string", false},
		{"different", "int64", "int64", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reg, err := Resolve(CompileOptions{VirtualFiles: []VirtualFile{itemSource(c.fieldType)}}, ref)
			if err != nil {
				t.Fatal(err)
			}
			md := reg.Find("reg.v1.Item")
			if got := md.Fields().ByName("name").Kind().String(); got != c.wantKind {
				t.Errorf("reg.v1.Item.name is %s, want the document's %s", got, c.wantKind)
			}
			if !reg.FromDocument("reg.v1.Item") {
				t.Error("FromDocument(reg.v1.Item) = false")
			}
			ws := reg.Warnings()
			if c.warn != (len(ws) == 1) || len(ws) > 1 {
				t.Fatalf("warnings = %q, want one: %v", ws, c.warn)
			}
			if c.warn && !strings.Contains(ws[0], "reg.v1.Item differently from reg/v1/items.proto") {
				t.Errorf("warning %q does not name the type and the shadowed file", ws[0])
			}
		})
	}
}

// What counts as the same definition: the json_name a compiler fills in
// and the generated _null mask are not differences, a field's type, name
// or number and a nested type are.
func TestSameDefinition(t *testing.T) {
	base := "message M { google.protobuf.Int32Value w = 1; string user_name = 2; message N { int32 a = 1; } N n = 3; }"
	cases := []struct {
		name, other string
		same        bool
	}{
		{"identical", base, true},
		{"explicit default json_name", strings.Replace(base, "user_name = 2", `user_name = 2 [json_name = "userName"]`, 1), true},
		{"other json_name", strings.Replace(base, "user_name = 2", `user_name = 2 [json_name = "user"]`, 1), false},
		{"field type", strings.Replace(base, "string user_name", "bytes user_name", 1), false},
		{"field number", strings.Replace(base, "user_name = 2", "user_name = 4", 1), false},
		{"nested type", strings.Replace(base, "int32 a = 1", "int64 a = 1", 1), false},
	}
	const header = "syntax = \"proto3\";\npackage s.v1;\nimport \"google/protobuf/wrappers.proto\";\n"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The external one is compiled with the mask, the document's
			// one is a descriptor set without it, as a registry would
			// serve: the mask must not count either way.
			ext := NewRegistry()
			if err := CompileSources(ext, CompileOptions{VirtualFiles: []VirtualFile{{Name: "a.proto", Body: []byte(header + base)}}}); err != nil {
				t.Fatal(err)
			}
			if ext.Find("s.v1.M").Fields().ByName("_null") == nil {
				t.Fatal("precondition: the compiled M has no generated _null")
			}
			compiled := NewRegistry()
			if err := CompileSources(compiled, CompileOptions{VirtualFiles: []VirtualFile{{Name: "b.proto", Body: []byte(header + c.other)}}}); err != nil {
				t.Fatal(err)
			}
			doc := withoutNullMask(t, compiled.Find("s.v1.M"))
			if got := sameDefinition(ext.Find("s.v1.M"), doc); got != c.same {
				t.Errorf("sameDefinition = %v, want %v", got, c.same)
			}
		})
	}
}

// A document's @proto imports only the well-known types, the
// InDocumentImports and another @proto: not another bundled schema, not
// a file on disk, and not a -p file compiled alongside it (#369).
func TestCompileSources_DocumentImports(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "disk.proto"), []byte("syntax = \"proto3\";\npackage d.v1;\nmessage D { int32 a = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		imp    string
		reject bool
	}{
		{"pxf/secret.proto", false},
		{"pxf/bignum.proto", false},
		{"pxf/annotations.proto", false},
		{"sbe/annotations.proto", false},
		{"envelope/v1/envelope.proto", false},
		{"google/protobuf/timestamp.proto", false},
		{"docs/v1/topic.proto", true},
		{"schema/catalog/v1/catalog.proto", true},
		{"protowire/pxf/secret.proto", true},
		{"disk.proto", true},
	}
	for _, c := range cases {
		t.Run(c.imp, func(t *testing.T) {
			src := "syntax = \"proto3\";\npackage l.v1;\nimport \"" + c.imp + "\";\nmessage L { string user = 1; }\n"
			err := CompileSources(NewRegistry(), CompileOptions{
				UserFiles:    []string{"disk.proto"},
				VirtualFiles: []VirtualFile{{Name: "__indoc_0.proto", Body: []byte(src)}},
			})
			switch {
			case c.reject && err == nil:
				t.Fatal("accepted")
			case c.reject && !strings.Contains(err.Error(), "a @proto may import only"):
				t.Fatalf("rejected for another reason: %v", err)
			case !c.reject && err != nil:
				t.Fatalf("rejected: %v", err)
			}
		})
	}
	// The -p file itself still compiles and is registered.
	reg := NewRegistry()
	if err := CompileSources(reg, CompileOptions{UserFiles: []string{"disk.proto"}}); err != nil || reg.Find("d.v1.D") == nil {
		t.Fatalf("-p disk.proto: err %v, d.v1.D found %v", err, reg.Find("d.v1.D") != nil)
	}
}

// withoutNullMask rebuilds md's file without the generated _null field,
// as a schema compiled by a tool that adds none would be.
func withoutNullMask(t *testing.T, md protoreflect.MessageDescriptor) protoreflect.MessageDescriptor {
	t.Helper()
	fdp := protodesc.ToFileDescriptorProto(md.ParentFile())
	for _, m := range fdp.MessageType {
		m.Field = slices.DeleteFunc(m.Field, func(f *descriptorpb.FieldDescriptorProto) bool { return f.GetName() == "_null" })
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	out := fd.Messages().ByName(md.Name())
	if out.Fields().ByName("_null") != nil {
		t.Fatal("precondition: _null still present")
	}
	return out
}
