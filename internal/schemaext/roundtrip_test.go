// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package schemaext

import (
	"strings"
	"testing"

	"github.com/trendvidia/protocheck/v2"
	pwsv1 "github.com/trendvidia/protocompile/gen/protowire/schema/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestLoweredCarriers pins the descriptor-level conventions the §9.3
// generator assumes, on the real lowering pass's output: unqualified
// FunctionDecl.options keys, fully-qualified Annotation.name values, and
// the positional-vs-named shape of annotation args as written in source.
func TestLoweredCarriers(t *testing.T) {
	h := getHarness(t)

	t.Run("function declarations", func(t *testing.T) {
		decls := fileFunctions(t, h, "01_basic.proto")
		if len(decls) != 1 {
			t.Fatalf("01_basic.proto: got %d function decls, want 1", len(decls))
		}
		d := decls[0]
		if d.GetName() != "fixtures.basic.is_email" {
			t.Errorf("FunctionDecl.name = %q, want %q", d.GetName(), "fixtures.basic.is_email")
		}
		if len(d.GetParams()) != 1 || d.GetParams()[0].GetName() != "value" || d.GetParams()[0].GetType() != "string" {
			t.Errorf("FunctionDecl.params = %v, want [value string]", d.GetParams())
		}
		if len(d.GetOptions()) != 0 {
			t.Errorf("FunctionDecl.options = %v, want empty", d.GetOptions())
		}
	})

	t.Run("function options keys are unqualified", func(t *testing.T) {
		decls := fileFunctions(t, h, "05_error_overrides.proto")
		byName := make(map[string]*pwsv1.FunctionDecl)
		for _, d := range decls {
			byName[d.GetName()] = d
		}
		matches := byName["fixtures.errors.matches"]
		if matches == nil {
			t.Fatalf("05_error_overrides.proto: no decl fixtures.errors.matches (have %v)", keysOfDecls(byName))
		}
		// The generator indexes this map with bare option names
		// ("description", "deprecated"); the lowering pass must therefore
		// record bracket-form options under their unqualified key.
		arg, ok := matches.GetOptions()["error_code"]
		if !ok {
			t.Fatalf("FunctionDecl.options keys = %v, want unqualified %q", optionKeys(matches), "error_code")
		}
		if arg.GetStringValue() != "common.matches.failed" {
			t.Errorf("options[error_code] = %q, want %q", arg.GetStringValue(), "common.matches.failed")
		}
	})

	t.Run("annotation names are fully qualified", func(t *testing.T) {
		fd := h.byPath["01_basic.proto"]
		msg := fd.GetMessageType()[0]
		if msg.GetName() != "User" {
			t.Fatalf("01_basic.proto message[0] = %q, want User", msg.GetName())
		}
		msgAnns := annotationList(t, msg.GetOptions(), pwsv1.E_MessageAnnotations)
		// @description("a registered user") — library annotation lowered
		// under its declaring package's FQN, with its argument positional
		// exactly as written at the use site.
		desc := findAnnotation(msgAnns, "protowire.schema.v1.description")
		if desc == nil {
			t.Fatalf("User message annotations = %v, want protowire.schema.v1.description", annotationNames(msgAnns))
		}
		if len(desc.GetArgs()) != 1 || desc.GetArgs()[0].GetName() != "" {
			t.Errorf("@description args = %v, want one positional arg", desc.GetArgs())
		}
		if got := desc.GetArgs()[0].GetStringValue(); got != "a registered user" {
			t.Errorf("@description arg = %q, want %q", got, "a registered user")
		}
		// @tag(name = "auth") — user annotation under its own FQN, with a
		// named arg preserved as named.
		tag := findAnnotation(msgAnns, "fixtures.basic.tag")
		if tag == nil {
			t.Fatalf("User message annotations = %v, want fixtures.basic.tag", annotationNames(msgAnns))
		}
		if len(tag.GetArgs()) != 1 || tag.GetArgs()[0].GetName() != "name" || tag.GetArgs()[0].GetStringValue() != "auth" {
			t.Errorf("@tag args = %v, want one named arg name=%q", tag.GetArgs(), "auth")
		}

		field := msg.GetField()[0]
		fieldAnns := annotationList(t, field.GetOptions(), pwsv1.E_FieldAnnotations)
		fieldDesc := findAnnotation(fieldAnns, "protowire.schema.v1.description")
		if fieldDesc == nil {
			t.Fatalf("email field annotations = %v, want protowire.schema.v1.description", annotationNames(fieldAnns))
		}
		if findAnnotation(fieldAnns, "protowire.schema.v1.validate") == nil {
			t.Errorf("email field annotations = %v, want expanded protowire.schema.v1.validate from type Email", annotationNames(fieldAnns))
		}
	})

	t.Run("type declarations", func(t *testing.T) {
		fd := h.byPath["01_basic.proto"]
		list := proto.GetExtension(fd.GetOptions(), pwsv1.E_TypeDecls).(*pwsv1.FileTypeDecls)
		if list == nil || len(list.GetDeclarations()) != 1 {
			t.Fatalf("01_basic.proto FileTypeDecls = %v, want one decl", list.GetDeclarations())
		}
		d := list.GetDeclarations()[0]
		if d.GetName() != "fixtures.basic.Email" || d.GetBaseTypeFqn() != "string" {
			t.Errorf("TypeDecl = %s -> %s, want fixtures.basic.Email -> string", d.GetName(), d.GetBaseTypeFqn())
		}
	})
}

// TestGeneratedStubs asserts on the source the forked protoc-gen-go emits
// for the real lowered descriptors: typed Functions interfaces, the
// reserved RFC-001 §7 codes referenced through protocheck's typed
// constants, and annotation-derived doc comments.
func TestGeneratedStubs(t *testing.T) {
	h := getHarness(t)

	assertContains := func(t *testing.T, protoPath string, wants ...string) {
		t.Helper()
		content := h.generated[protoPath]
		for _, want := range wants {
			if !strings.Contains(content, want) {
				t.Errorf("%s: generated output missing %q", protoPath, want)
			}
		}
		if t.Failed() {
			t.Logf("generated output for %s:\n%s", protoPath, content)
		}
	}

	t.Run("01_basic", func(t *testing.T) {
		assertContains(t, "01_basic.proto",
			"type Functions interface {",
			"IsEmail(value string) (bool, *v2.Violation)",
			"type UnimplementedFunctions struct{}",
			`{Code: v2.CodeFunctionUnimplemented, FallbackMessage: v2.MsgFunctionUnimplemented("fixtures.basic.is_email")}`,
			"func RegisterFunctions(eng v2.Engine, impl Functions) error {",
			`if err := eng.Register("fixtures.basic.is_email", func(args []any) (bool, *v2.Violation) {`,
			`{Code: v2.CodeFunctionInvalidArgument, FallbackMessage: v2.MsgFunctionArity("fixtures.basic.is_email", 1)}`,
			`{Code: v2.CodeFunctionInvalidArgument, FallbackMessage: v2.MsgFunctionArgType("fixtures.basic.is_email", 0, "string")}`,
			`v2 "github.com/trendvidia/protocheck/v2"`,
			// @description annotations surface as doc comments — the
			// generator matched protowire.schema.v1.description by FQN and
			// took the positional string arg.
			"// a registered user",
			"// primary contact address",
		)
	})

	t.Run("02_composition", func(t *testing.T) {
		assertContains(t, "02_composition.proto",
			"Matches(value string, pattern string) (bool, *v2.Violation)",
			"EndsWith(value string, suffix string) (bool, *v2.Violation)",
		)
	})

	t.Run("03_message_typed_param", func(t *testing.T) {
		// fixtures.placement.User is a message type: the signature degrades
		// to any and the adapter passes the arg through without assertion.
		assertContains(t, "03_message_and_field_annotations.proto",
			"SameDomain(msg any) (bool, *v2.Violation)",
			"Matches(value string, pattern string) (bool, *v2.Violation)",
			"// free tier",
		)
	})

	t.Run("06_cross_file_lib", func(t *testing.T) {
		assertContains(t, "06_cross_file_lib.proto",
			`eng.Register("fixtures.lib.matches"`,
			`eng.Register("fixtures.lib.is_e164"`,
		)
	})

	t.Run("no stubs without function decls", func(t *testing.T) {
		for _, protoPath := range []string{"04_required_and_default.proto", "06_cross_file_main.proto"} {
			content := h.generated[protoPath]
			if strings.Contains(content, "type Functions interface") || strings.Contains(content, "protocheck") {
				t.Errorf("%s declares no functions but generated stubs / a protocheck import", protoPath)
			}
		}
	})
}

// TestProtocheckInitVerification feeds the lowered descriptors to
// protocheck's init-time function verification (RFC-001 §9.2): the walk
// must find fixtures.basic.is_email in the real carriers, fail strict
// construction while it is unregistered, and accept registration through
// the Engine SPI the generated RegisterFunctions helper drives.
func TestProtocheckInitVerification(t *testing.T) {
	h := getHarness(t)

	reg, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: h.fds.GetFile()})
	if err != nil {
		t.Fatalf("protodesc.NewFiles: %v", err)
	}
	fd, err := reg.FindFileByPath("01_basic.proto")
	if err != nil {
		t.Fatalf("FindFileByPath: %v", err)
	}

	_, err = protocheck.New(
		protocheck.WithDescriptors(fd),
		protocheck.WithStrictValidation(),
	)
	if err == nil {
		t.Fatal("strict New with no registered impls: want error, got nil")
	}
	if !strings.Contains(err.Error(), "fixtures.basic.is_email") {
		t.Errorf("strict New error should name fixtures.basic.is_email, got: %v", err)
	}

	v, err := protocheck.New(protocheck.WithDescriptors(fd))
	if err != nil {
		t.Fatalf("lenient New: %v", err)
	}
	defer v.Close()
	impl := func(args []any) (bool, *protocheck.Violation) { return true, nil }
	if err := v.Engine().Register("fixtures.basic.is_email", impl); err != nil {
		t.Errorf("Register over lenient placeholder: %v", err)
	}
}

// ─── carrier helpers ────────────────────────────────────────────────────────

func fileFunctions(t *testing.T, h *harness, protoPath string) []*pwsv1.FunctionDecl {
	t.Helper()
	fd := h.byPath[protoPath]
	if fd == nil {
		t.Fatalf("no descriptor for %s", protoPath)
	}
	list := proto.GetExtension(fd.GetOptions(), pwsv1.E_Functions).(*pwsv1.FileFunctions)
	if list == nil {
		t.Fatalf("%s carries no FileFunctions (ext 50401)", protoPath)
	}
	return list.GetDeclarations()
}

func annotationList(t *testing.T, opts proto.Message, ext protoreflect.ExtensionType) *pwsv1.AnnotationList {
	t.Helper()
	list := proto.GetExtension(opts, ext).(*pwsv1.AnnotationList)
	if list == nil {
		t.Fatal("options carry no AnnotationList (ext 50400)")
	}
	return list
}

func findAnnotation(list *pwsv1.AnnotationList, name string) *pwsv1.Annotation {
	for _, a := range list.GetEntries() {
		if a.GetName() == name {
			return a
		}
	}
	return nil
}

func annotationNames(list *pwsv1.AnnotationList) []string {
	var names []string
	for _, a := range list.GetEntries() {
		names = append(names, a.GetName())
	}
	return names
}

func optionKeys(d *pwsv1.FunctionDecl) []string {
	var keys []string
	for k := range d.GetOptions() {
		keys = append(keys, k)
	}
	return keys
}

func keysOfDecls(m map[string]*pwsv1.FunctionDecl) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
