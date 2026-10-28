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
	base := "message M { google.protobuf.Int32Value w = 1; string user_name = 2; message N { int32 a = 1; google.protobuf.Int32Value v = 2; } N n = 3; }"
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
			if ext.Find("s.v1.M").Fields().ByName("_null") == nil || ext.Find("s.v1.M.N").Fields().ByName("_null") == nil {
				t.Fatal("precondition: the compiled M or M.N has no generated _null")
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
		{"google/protobuf/descriptor.proto", false},
		{"google/protobuf/field_mask.proto", false},
		{"google/protobuf/struct.proto", false},
		{"google/protobuf/compiler/plugin.proto", true},
		{"google/protobuf/go_features.proto", true},
		{"google/protobuf/cpp_features.proto", true},
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
// nested messages included, as a schema compiled by a tool that adds
// none would be.
func withoutNullMask(t *testing.T, md protoreflect.MessageDescriptor) protoreflect.MessageDescriptor {
	t.Helper()
	fdp := protodesc.ToFileDescriptorProto(md.ParentFile())
	var strip func([]*descriptorpb.DescriptorProto)
	strip = func(ms []*descriptorpb.DescriptorProto) {
		for _, m := range ms {
			m.Field = slices.DeleteFunc(m.Field, func(f *descriptorpb.FieldDescriptorProto) bool { return f.GetName() == "_null" })
			strip(m.NestedType)
		}
	}
	strip(fdp.MessageType)
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	out := fd.Messages().ByName(md.Name())
	if out.Fields().ByName("_null") != nil || out.Messages().ByName("N").Fields().ByName("_null") != nil {
		t.Fatal("precondition: _null still present")
	}
	return out
}

// shadowed compiles ext as a -p file and doc as a document's @proto into
// one registry.
func shadowed(t *testing.T, ext, doc string) *Registry {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.WriteFile("ext.proto", []byte("syntax = \"proto3\";\n"+ext), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	if err := CompileSources(reg, CompileOptions{
		UserFiles:    []string{"ext.proto"},
		VirtualFiles: []VirtualFile{{Name: "__indoc_0.proto", Body: []byte("syntax = \"proto3\";\n" + doc)}},
	}); err != nil {
		t.Fatal(err)
	}
	return reg
}

// The types a document's @proto shadows besides a top-level message, and
// the warnings each gives (#371). Every case before the review of #385
// either gave no warning or gave one that was not true.
func TestMergeDocument_Shadowing(t *testing.T) {
	cases := []struct {
		name, ext, doc string
		warnings       []string
		check          func(t *testing.T, reg *Registry)
	}{
		{
			name:     "identical enum",
			ext:      "package e.v1;\nenum Color { COLOR_UNSPECIFIED = 0; RED = 1; }\nmessage M { Color c = 1; }\n",
			doc:      "package e.v1;\nenum Color { COLOR_UNSPECIFIED = 0; RED = 1; }\nmessage M { Color c = 1; }\n",
			warnings: nil,
		},
		{
			// M's DescriptorProto names .e.v1.Color either way, so only the
			// enum's own comparison can see this.
			name:     "different enum",
			ext:      "package e.v1;\nenum Color { COLOR_UNSPECIFIED = 0; RED = 1; }\nmessage M { Color c = 1; }\n",
			doc:      "package e.v1;\nenum Color { COLOR_UNSPECIFIED = 0; BLUE = 1; }\nmessage M { Color c = 1; }\n",
			warnings: []string{"@proto defines e.v1.Color differently from ext.proto; using the @proto definition"},
		},
		{
			name:     "nested name over a top-level one",
			ext:      "package n.A;\nmessage B { int32 s = 1; }\n",
			doc:      "package n;\nmessage A { message B { string s = 1; } }\n",
			warnings: []string{"@proto defines n.A.B differently from ext.proto; using the @proto definition"},
			check: func(t *testing.T, reg *Registry) {
				if k := reg.Find("n.A.B").Fields().ByName("s").Kind(); k != protoreflect.StringKind {
					t.Errorf("n.A.B.s is %v, want the document's string", k)
				}
			},
		},
		{
			name:     "external nested type the document's lacks",
			ext:      "package l.v1;\nmessage L { message X { int32 a = 1; } X x = 1; }\n",
			doc:      "package l.v1;\nmessage L { string user = 1; }\n",
			warnings: []string{"@proto defines l.v1.L differently from ext.proto; using the @proto definition"},
			check: func(t *testing.T, reg *Registry) {
				if reg.Find("l.v1.L.X") != nil {
					t.Error("l.v1.L.X, nested in the shadowed external L, is still registered")
				}
			},
		},
		{
			name:     "identical type an external message refers to",
			ext:      "package l.v1;\nmessage L { string user = 1; }\nmessage W { L l = 1; }\n",
			doc:      "package l.v1;\nmessage L { string user = 1; }\n",
			warnings: nil,
			check: func(t *testing.T, reg *Registry) {
				if err := reg.Check(reg.Find("l.v1.W")); err != nil {
					t.Errorf("Check(l.v1.W) = %v, want nil: the definitions are identical", err)
				}
			},
		},
		{
			// The referrer's field keeps the external L, so decoding
			// through it is rejected, not warned about (decided on #387).
			// Only from a type that reaches it: an unrelated one passes.
			name:     "different type an external message refers to",
			ext:      "package l.v1;\nmessage L { int32 user = 1; }\nmessage W { map<string, L> m = 2; }\nmessage V { W w = 1; }\nmessage U { int32 n = 1; }\n",
			doc:      "package l.v1;\nmessage L { string user = 1; }\n",
			warnings: []string{"@proto defines l.v1.L differently from ext.proto; using the @proto definition"},
			check: func(t *testing.T, reg *Registry) {
				const want = "l.v1.W.m, from ext.proto, has type l.v1.L as ext.proto defines it, but the document's @proto defines l.v1.L differently; add l.v1.W to the @proto, or bring ext.proto in line with it"
				for _, name := range []string{"l.v1.W", "l.v1.V"} {
					if err := reg.Check(reg.Find(name)); err == nil || err.Error() != want {
						t.Errorf("Check(%s) = %v, want %q", name, err, want)
					}
				}
				for _, name := range []string{"l.v1.L", "l.v1.U"} {
					if err := reg.Check(reg.Find(name)); err != nil {
						t.Errorf("Check(%s) = %v, want nil: it reaches no external L", name, err)
					}
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reg := shadowed(t, c.ext, c.doc)
			if got := reg.Warnings(); !slices.Equal(got, c.warnings) {
				t.Errorf("warnings:\n got %q\nwant %q", got, c.warnings)
			}
			if c.check != nil {
				c.check(t, reg)
			}
		})
	}
}

// The zero Registry is usable, as its doc comment says: Merge fills its
// maps rather than writing to nil ones.
func TestRegistry_ZeroValueMerge(t *testing.T) {
	ext := NewRegistry()
	if err := CompileSources(ext, CompileOptions{VirtualFiles: []VirtualFile{itemSource("string")}}); err != nil {
		t.Fatal(err)
	}
	var reg Registry
	reg.Merge(ext.Find("reg.v1.Item").ParentFile())
	if reg.Find("reg.v1.Item") == nil {
		t.Fatal("reg.v1.Item not found after Merge into a zero Registry")
	}
}
