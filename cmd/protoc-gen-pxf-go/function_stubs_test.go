// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/gofeaturespb"
	"google.golang.org/protobuf/types/pluginpb"
)

// responseContent returns every file gen wrote in name order, each ended by a
// NUL so a caller can split them again.
func responseContent(t *testing.T, gen *protogen.Plugin) string {
	t.Helper()
	files := gen.Response().GetFile()
	sort.Slice(files, func(i, j int) bool { return files[i].GetName() < files[j].GetName() })
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f.GetContent())
		b.WriteByte(0)
	}
	return b.String()
}

// generateOne runs the plugin over a single FileDescriptorProto and returns what
// it wrote, the .pb.go and any stub file together.
func generateOne(t *testing.T, fdp *descriptorpb.FileDescriptorProto) string {
	t.Helper()
	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{fdp.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	})
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	generateFile(gen, gen.FilesByPath[fdp.GetName()], newSchemaTypes(gen))
	return responseContent(t, gen)
}

// basicFileDescriptor mirrors protowire's
// testdata/schema-extensions/01_basic.proto after lowering: the v1.2
// constructs are carried as extensions 1327/1328/1330 in the options,
// exactly as the protocompile lowering pass embeds them.
func basicFileDescriptor(t *testing.T) *descriptorpb.FileDescriptorProto {
	t.Helper()

	fileOpts := &descriptorpb.FileOptions{GoPackage: proto.String("example.com/fixtures/basicpb")}
	fileCarrier := appendBytesField(nil, schemaFunctionsFieldNumber, encodeFileFunctions(
		encodeFunctionDecl("fixtures.basic.is_email",
			[][]byte{encodeFunctionParam("value", "string")},
			[][]byte{
				encodeFunctionOption("description", encodeStringArg("", "reports whether value is an RFC 5321 address")),
			}),
		encodeFunctionDecl("fixtures.basic.is_valid",
			[][]byte{
				encodeFunctionParam("addr", "fixtures.basic.Email"),
				encodeFunctionParam("strict", "bool"),
			},
			[][]byte{
				encodeFunctionOption("deprecated", encodeStringArg("", "use is_email")),
			}),
	))
	fileCarrier = appendBytesField(fileCarrier, schemaTypeDeclsFieldNumber, encodeFileTypeDecls(
		encodeTypeDecl("fixtures.basic.Email", "string"),
	))
	fileOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(fileCarrier))

	msgOpts := &descriptorpb.MessageOptions{}
	msgOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(
		appendBytesField(nil, schemaAnnotationsFieldNumber, encodeAnnotationList(
			encodeAnnotation(schemaDescriptionAnnotation, encodeStringArg("", "a registered user")),
		))))

	fieldOpts := &descriptorpb.FieldOptions{}
	fieldOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(
		appendBytesField(nil, schemaAnnotationsFieldNumber, encodeAnnotationList(
			encodeAnnotation(schemaDescriptionAnnotation, encodeStringArg("", "primary contact address")),
			encodeAnnotation(schemaDeprecatedAnnotation, encodeStringArg("", "use contact_email")),
		))))

	enumOpts := &descriptorpb.EnumOptions{}
	enumOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(
		appendBytesField(nil, schemaAnnotationsFieldNumber, encodeAnnotationList(
			encodeAnnotation(schemaDescriptionAnnotation, encodeStringArg("", "account lifecycle state")),
		))))

	enumValueOpts := &descriptorpb.EnumValueOptions{}
	enumValueOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(
		appendBytesField(nil, schemaAnnotationsFieldNumber, encodeAnnotationList(
			encodeAnnotation(schemaDeprecatedAnnotation),
		))))

	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("fixtures/basic.proto"),
		Package: proto.String("fixtures.basic"),
		Syntax:  proto.String("proto3"),
		Options: fileOpts,
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name:    proto.String("Status"),
			Options: enumOpts,
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: proto.String("STATUS_LEGACY"), Number: proto.Int32(1), Options: enumValueOpts},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name:    proto.String("User"),
			Options: msgOpts,
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     proto.String("email"),
				Number:   proto.Int32(1),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				JsonName: proto.String("email"),
				Options:  fieldOpts,
			}},
		}},
	}
}

func TestGenFunctionStubs(t *testing.T) {
	content := generateOne(t, basicFileDescriptor(t))

	for _, want := range []string{
		// BasicFunctions interface with typed signatures; the type alias
		// fixtures.basic.Email resolves through FileTypeDecls to string.
		"type BasicFunctions interface {",
		"IsEmail(value string) (bool, *v2.Violation)",
		"IsValid(addr string, strict bool) (bool, *v2.Violation)",
		// Bracket-form options surface as doc comments.
		"// reports whether value is an RFC 5321 address",
		"// Deprecated: use is_email",
		// UnimplementedBasicFunctions placeholder fails with the reserved
		// RFC-001 §7 code and spec-pinned fallback message, both referenced
		// through the runtime's typed constants and template helpers.
		"type UnimplementedBasicFunctions struct{}",
		`{Code: v2.CodeFunctionUnimplemented, FallbackMessage: v2.MsgFunctionUnimplemented("fixtures.basic.is_email")}`,
		"var _ BasicFunctions = UnimplementedBasicFunctions{}",
		// Registration helper with []any adapters.
		"func RegisterBasicFunctions(eng v2.Engine, impl BasicFunctions) error {",
		`if err := eng.Register("fixtures.basic.is_email", func(args []any) (bool, *v2.Violation) {`,
		`{Code: v2.CodeFunctionInvalidArgument, FallbackMessage: v2.MsgFunctionArity("fixtures.basic.is_email", 1)}`,
		"a0, ok := args[0].(string)",
		// The arg-type guard message names the schema-declared parameter
		// type (the alias FQN), not the resolved Go type it asserts.
		`{Code: v2.CodeFunctionInvalidArgument, FallbackMessage: v2.MsgFunctionArgType("fixtures.basic.is_valid", 0, "fixtures.basic.Email")}`,
		"return impl.IsValid(a0, a1)",
		// Engine SPI import.
		`v2 "github.com/trendvidia/protocheck/v2"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated output missing %q", want)
		}
	}

	// The generated file must be syntactically valid Go.
	fset := token.NewFileSet()
	for _, part := range strings.SplitAfter(content, "\x00") {
		part = strings.TrimSuffix(part, "\x00")
		if part == "" {
			continue
		}
		if _, err := parser.ParseFile(fset, "basic.pb.go", part, parser.SkipObjectResolution); err != nil {
			t.Errorf("generated output does not parse: %v", err)
		}
	}

	if t.Failed() {
		t.Logf("generated output:\n%s", content)
	}
}

func TestAnnotationDocComments(t *testing.T) {
	content := generateOne(t, basicFileDescriptor(t))

	for _, want := range []string{
		// @description on message, field, enum.
		"// a registered user\ntype User struct {",
		"// primary contact address",
		"// account lifecycle state\ntype Status int32",
		// @deprecated on the email field and the STATUS_LEGACY value.
		"// Deprecated: use contact_email",
		"// Deprecated: Do not use.\n\tStatus_STATUS_LEGACY Status = 1",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated output missing %q", want)
		}
	}
	if t.Failed() {
		t.Logf("generated output:\n%s", content)
	}
}

// TestOneofAnnotationDocComments covers the two places a oneof's comments are
// emitted: the struct field, which an opaque struct hides, and the builder.
func TestOneofAnnotationDocComments(t *testing.T) {
	fdp := basicFileDescriptor(t)
	oneofOpts := &descriptorpb.OneofOptions{}
	oneofOpts.ProtoReflect().SetUnknown(protoreflect.RawFields(
		appendBytesField(nil, schemaAnnotationsFieldNumber, encodeAnnotationList(
			encodeAnnotation(schemaDescriptionAnnotation, encodeStringArg("", "how to reach the user")),
			encodeAnnotation(schemaDeprecatedAnnotation, encodeStringArg("", "use channels")),
		))))
	msg := fdp.MessageType[0]
	msg.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("contact"), Options: oneofOpts}}
	msg.Field[0].OneofIndex = proto.Int32(0)

	// want counts the sites in the first generated file: the open struct and
	// the builder, of which an opaque struct has only the builder.
	for _, tt := range []struct {
		level gofeaturespb.GoFeatures_APILevel
		want  int
	}{
		{gofeaturespb.GoFeatures_API_OPEN, 1},
		{gofeaturespb.GoFeatures_API_HYBRID, 2},
		{gofeaturespb.GoFeatures_API_OPAQUE, 1},
	} {
		t.Run(tt.level.String(), func(t *testing.T) {
			gen, err := protogen.Options{DefaultAPILevel: tt.level}.New(&pluginpb.CodeGeneratorRequest{
				FileToGenerate: []string{fdp.GetName()},
				ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
			})
			if err != nil {
				t.Fatalf("protogen.Options.New: %v", err)
			}
			generateFile(gen, gen.FilesByPath[fdp.GetName()], newSchemaTypes(gen))
			content := gen.Response().GetFile()[0].GetContent()
			for _, text := range []string{"// how to reach the user", "// Deprecated: use channels"} {
				if got := strings.Count(content, text); got != tt.want {
					t.Errorf("%q appears %d times, want %d\n%s", text, got, tt.want, content)
				}
			}
		})
	}
}

// TestNoSchemaExtensionsNoOverhead verifies that a file without carrier
// options generates no stubs and no engine dependency.
func TestNoSchemaExtensionsNoOverhead(t *testing.T) {
	fdp := basicFileDescriptor(t)
	fdp.Options = &descriptorpb.FileOptions{GoPackage: proto.String("example.com/fixtures/basicpb")}
	fdp.MessageType[0].Options = nil
	fdp.MessageType[0].Field[0].Options = nil
	fdp.EnumType[0].Options = nil
	fdp.EnumType[0].Value[1].Options = nil

	content := generateOne(t, fdp)
	// Note: "Deprecated: Use xxx.Descriptor instead." legacy accessors are
	// always generated, so ban only the annotation-derived notices.
	for _, banned := range []string{"Functions", "protocheck", "Deprecated: use", "Deprecated: Do not use."} {
		if strings.Contains(content, banned) {
			t.Errorf("output for carrier-free file unexpectedly contains %q", banned)
		}
	}
}

// functionFile returns a file in package example.com/fixtures/sharedpb. It
// declares the named function, or no function when fn is empty.
func functionFile(name, fn string, deps ...string) *descriptorpb.FileDescriptorProto {
	opts := &descriptorpb.FileOptions{GoPackage: proto.String("example.com/fixtures/sharedpb")}
	if fn != "" {
		opts.ProtoReflect().SetUnknown(protoreflect.RawFields(appendBytesField(nil, schemaFunctionsFieldNumber,
			encodeFileFunctions(encodeFunctionDecl("fixtures.shared."+fn, nil, nil)))))
	}
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String(name),
		Package:    proto.String("fixtures.shared"),
		Syntax:     proto.String("proto3"),
		Dependency: deps,
		Options:    opts,
	}
}

// TestFunctionStubsPerFile pins the fix for protobuf-go#9: two files in one Go
// package each get their own stub names, and neither set depends on the other
// file being in the run.
func TestFunctionStubsPerFile(t *testing.T) {
	a, b := functionFile("fixtures/common.proto", "first"), functionFile("fixtures/descriptor.proto", "second")
	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{a.GetName(), b.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{a, b},
	})
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	for _, tt := range []struct {
		file string
		want []string
	}{
		{a.GetName(), []string{"type CommonFunctions interface {", "type UnimplementedCommonFunctions struct{}", "func RegisterCommonFunctions(eng v2.Engine, impl CommonFunctions) error {"}},
		{b.GetName(), []string{"type DescriptorFunctions interface {", "type UnimplementedDescriptorFunctions struct{}", "func RegisterDescriptorFunctions(eng v2.Engine, impl DescriptorFunctions) error {"}},
	} {
		sub, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
			FileToGenerate: []string{a.GetName(), b.GetName()},
			ProtoFile:      []*descriptorpb.FileDescriptorProto{a, b},
		})
		if err != nil {
			t.Fatalf("protogen.Options.New: %v", err)
		}
		generateFile(sub, sub.FilesByPath[tt.file], newSchemaTypes(sub))
		content := responseContent(t, sub)
		generateFile(gen, gen.FilesByPath[tt.file], newSchemaTypes(gen))
		alone := generateOne(t, functionFile(tt.file, map[string]string{a.GetName(): "first", b.GetName(): "second"}[tt.file]))
		if content != alone {
			t.Errorf("%s: output depends on the sibling file being in the run", tt.file)
		}
		for _, want := range tt.want {
			if !strings.Contains(content, want) {
				t.Errorf("%s: generated output missing %q", tt.file, want)
			}
		}
	}
	if got := gen.Response().GetError(); got != "" {
		t.Errorf("unexpected plugin error: %s", got)
	}
}

func TestFunctionStubPrefix(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{"descriptor.proto", "Descriptor"},
		{"dir/foo_bar.proto", "FooBar"},
		{"foo-bar.proto", "Foo_bar"},
		{"functions.proto", "Functions"},
		{"01_basic.proto", "X01Basic"},
		{"_private.proto", "XPrivate"},
		{"über.proto", "Xüber"},
	} {
		fdp := functionFile(tt.path, "fn")
		gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
			FileToGenerate: []string{tt.path},
			ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
		})
		if err != nil {
			t.Fatalf("protogen.Options.New: %v", err)
		}
		if got := functionStubPrefix(gen.FilesByPath[tt.path]); got != tt.want {
			t.Errorf("functionStubPrefix(%q) = %q, want %q", tt.path, got, tt.want)
		}
		// The stubs must be valid Go for every name, not just the camel-cased ones.
		generateFile(gen, gen.FilesByPath[tt.path], newSchemaTypes(gen))
		if got := gen.Response().GetError(); got != "" {
			t.Errorf("%s: %s", tt.path, got)
		}
	}
}

func TestFunctionStubConflict(t *testing.T) {
	const a, b = "fixtures/foo_bar.proto", "fixtures/fooBar.proto"
	for _, tt := range []struct {
		name      string
		parameter string
		files     []*descriptorpb.FileDescriptorProto
		generate  []string
		wantErr   []string // substrings; empty means no error
	}{{
		name:     "file names camel-case alike",
		files:    []*descriptorpb.FileDescriptorProto{functionFile(a, "first"), functionFile(b, "second")},
		generate: []string{a, b},
		wantErr:  []string{a, b, "example.com/fixtures/sharedpb", "FooBarFunctions"},
	}, {
		name:     "imported sibling generated by an earlier run",
		files:    []*descriptorpb.FileDescriptorProto{functionFile(a, "first"), functionFile(b, "second", a)},
		generate: []string{b},
		wantErr:  []string{a, b},
	}, {
		name:     "sibling without functions",
		files:    []*descriptorpb.FileDescriptorProto{functionFile(a, "first"), functionFile(b, "")},
		generate: []string{a, b},
	}, {
		name:      "same import path in different output directories",
		parameter: "paths=source_relative",
		files: []*descriptorpb.FileDescriptorProto{
			functionFile("one/foo_bar.proto", "first"),
			functionFile("two/fooBar.proto", "second"),
		},
		generate: []string{"one/foo_bar.proto", "two/fooBar.proto"},
	}, {
		name:      "same directory with source-relative paths",
		parameter: "paths=source_relative",
		files:     []*descriptorpb.FileDescriptorProto{functionFile(a, "first"), functionFile(b, "second")},
		generate:  []string{a, b},
		wantErr:   []string{a, b},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
				Parameter:      proto.String(tt.parameter),
				FileToGenerate: tt.generate,
				ProtoFile:      tt.files,
			})
			if err != nil {
				t.Fatalf("protogen.Options.New: %v", err)
			}
			generate(gen)
			got := gen.Response().GetError()
			if len(tt.wantErr) == 0 && got != "" {
				t.Errorf("unexpected plugin error: %s", got)
			}
			if len(tt.wantErr) > 0 && got == "" {
				t.Error("no plugin error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(got, want) {
					t.Errorf("plugin error missing %q: %s", want, got)
				}
			}
		})
	}
}

// TestStubsInTheirOwnFile pins where the output goes: the stubs in
// <name>_functions.pb.go, and a .pb.go that does not need protocheck.
func TestStubsInTheirOwnFile(t *testing.T) {
	fdp := basicFileDescriptor(t)
	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{fdp.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fdp},
	})
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	generate(gen)
	files := make(map[string]string)
	for _, f := range gen.Response().GetFile() {
		files[f.GetName()] = f.GetContent()
	}
	pb, ok := files["example.com/fixtures/basicpb/basic.pb.go"]
	if !ok {
		t.Fatalf("no .pb.go in %v", files)
	}
	stubs, ok := files["example.com/fixtures/basicpb/basic_functions.pb.go"]
	if !ok {
		t.Fatalf("no stub file in %v", files)
	}
	if strings.Contains(pb, "protocheck") || strings.Contains(pb, "BasicFunctions") {
		t.Errorf(".pb.go holds stub code:\n%s", pb)
	}
	for _, want := range []string{"package basicpb", "type BasicFunctions interface {", `"github.com/trendvidia/protocheck/v2"`} {
		if !strings.Contains(stubs, want) {
			t.Errorf("stub file missing %q:\n%s", want, stubs)
		}
	}
}

// TestStubFileCollision pins that a file whose .pb.go would land on another
// file's stub file is a plugin error naming both.
func TestStubFileCollision(t *testing.T) {
	a, b := functionFile("fixtures/foo.proto", "first"), functionFile("fixtures/foo_functions.proto", "")
	gen, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{a.GetName(), b.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{a, b},
	})
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	generate(gen)
	got := gen.Response().GetError()
	for _, want := range []string{a.GetName(), b.GetName()} {
		if !strings.Contains(got, want) {
			t.Errorf("plugin error missing %q: %q", want, got)
		}
	}
}

// TestDeprecatedOptionAndAnnotation pins that a declaration with both the
// classic option and @deprecated gets one notice. The stock suffix is added
// after the annotation is read, so it is the annotation that stands down.
func TestDeprecatedOptionAndAnnotation(t *testing.T) {
	fdp := basicFileDescriptor(t)
	fdp.MessageType[0].Field[0].Options.Deprecated = proto.Bool(true)
	content := generateOne(t, fdp)
	if strings.Contains(content, "// Deprecated: use contact_email") {
		t.Errorf("annotation notice beside the option's:\n%s", content)
	}
	if !strings.Contains(content, "\t// Deprecated: Marked as deprecated in fixtures/basic.proto.\n\tEmail ") {
		t.Errorf("option's notice missing:\n%s", content)
	}
}

// TestGetterCarriesNoAnnotation records the limit of generating on stock
// protobuf: stock getters do not read a field's comments. A getter gets
// neither @description nor @deprecated, on the open API.
func TestGetterCarriesNoAnnotation(t *testing.T) {
	content := generateOne(t, basicFileDescriptor(t))
	getter := strings.Index(content, "func (x *User) GetEmail() string {")
	if getter < 0 {
		t.Fatalf("no getter:\n%s", content)
	}
	before := content[:getter]
	if tail := before[strings.LastIndex(strings.TrimRight(before, "\n"), "\n")+1:]; strings.HasPrefix(tail, "//") {
		t.Errorf("getter has a doc comment: %q", tail)
	}
}
