// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"maps"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/compiler/protogen"
)

// The protocheck module hosts the RFC-001 §9.1 engine SPI (Engine, Function,
// Violation) that generated function stubs program against. The import is
// resolved lazily through QualifiedGoIdent, so files without `function`
// declarations do not depend on it.
var protocheckPackage = protogen.GoImportPath("github.com/trendvidia/protocheck/v2")

// genFunctionStubs generates the RFC-001 §9.3 stubs for the `function`
// declarations carried in the file's options (extension 1328): a <File>Functions
// interface, an Unimplemented<File>Functions placeholder, and a
// Register<File>Functions helper binding implementations to a validation
// engine by fully-qualified name. <File> comes from the file's name, so several
// files in one Go package can each declare functions. Files without function
// declarations generate nothing.
func genFunctionStubs(g *protogen.GeneratedFile, f *protogen.File, types schemaTypes) {
	decls := functionStubDecls(f)
	if len(decls) == 0 {
		return
	}

	prefix := functionStubPrefix(f)
	ifaceName := prefix + "Functions"
	unimplName := "Unimplemented" + ifaceName
	registerName := "Register" + ifaceName

	// Method names: Go-camel-cased final FQN segment, "_"-suffixed on clash.
	methods := make([]string, len(decls))
	usedMethods := make(map[string]bool)
	for i, d := range decls {
		name := d.Name
		if j := strings.LastIndexByte(name, '.'); j >= 0 {
			name = name[j+1:]
		}
		name = goCamelCase(name)
		for name == "" || usedMethods[name] {
			name += "_"
		}
		usedMethods[name] = true
		methods[i] = name
	}

	violationIdent := protocheckPackage.Ident("Violation")

	g.P("// ", ifaceName, " declares the schema `function` implementations required by")
	g.P("// ", f.Desc.Path(), " (RFC-001 §9.3). A validation engine invokes these at")
	g.P("// runtime for @validate rules that call them. Implementations typically")
	g.P("// embed ", unimplName, " and override the functions they provide.")
	g.P("type ", ifaceName, " interface {")
	for i, d := range decls {
		if i > 0 {
			g.P()
		}
		g.P("// ", methods[i], " implements the schema function ", strconv.Quote(d.Name), ".")
		if opt, ok := d.Options["description"]; ok && opt.Kind == schemaArgString && opt.String != "" {
			g.P("//")
			for _, line := range strings.Split(opt.String, "\n") {
				g.P("// ", line)
			}
		}
		if reason, ok := functionDeprecationReason(d); ok {
			g.P("//")
			g.P("// Deprecated: ", reason)
		}
		g.P(methods[i], "(", functionParamSignature(g, d, types, true), ") (bool, *", violationIdent, ")")
	}
	g.P("}")
	g.P()

	g.P("// ", unimplName, " is a placeholder implementation of ", ifaceName, " whose")
	g.P("// methods all fail with the reserved ", protocheckPackage.Ident("CodeFunctionUnimplemented"), " violation")
	g.P("// (RFC-001 §7), per RFC-001 §9.2's lenient registration model.")
	g.P("type ", unimplName, " struct{}")
	g.P()
	for i, d := range decls {
		g.P("func (", unimplName, ") ", methods[i], "(", functionParamSignature(g, d, types, false), ") (bool, *", violationIdent, ") {")
		g.P("return false, &", violationIdent, "{Code: ", protocheckPackage.Ident("CodeFunctionUnimplemented"), ", FallbackMessage: ", protocheckPackage.Ident("MsgFunctionUnimplemented"), "(", strconv.Quote(d.Name), ")}")
		g.P("}")
		g.P()
	}
	g.P("var _ ", ifaceName, " = ", unimplName, "{}")
	g.P()

	g.P("// ", registerName, " binds each schema function declared in ", f.Desc.Path())
	g.P("// to eng by its fully-qualified name, adapting the typed methods of impl to")
	g.P("// the engine's []any calling convention. It returns the first registration")
	g.P("// error encountered.")
	g.P("func ", registerName, "(eng ", protocheckPackage.Ident("Engine"), ", impl ", ifaceName, ") error {")
	for i, d := range decls {
		g.P("if err := eng.Register(", strconv.Quote(d.Name), ", func(args []any) (bool, *", violationIdent, ") {")
		g.P("if len(args) != ", len(d.Params), " {")
		g.P("return false, &", violationIdent, "{Code: ", protocheckPackage.Ident("CodeFunctionInvalidArgument"), ", FallbackMessage: ", protocheckPackage.Ident("MsgFunctionArity"), "(", strconv.Quote(d.Name), ", ", len(d.Params), ")}")
		g.P("}")
		var callArgs []string
		for j, p := range d.Params {
			pt := types.paramType(p.Type, d.Name)
			goType := pt.goType(g)
			if pt.kind == paramAny {
				callArgs = append(callArgs, fmt.Sprintf("args[%d]", j))
				continue
			}
			callArg := fmt.Sprintf("a%d", j)
			switch pt.kind {
			case paramEnum:
				// RFC-001 §9.1 delivers an enum as its number.
				g.P("a", j, ", ok := args[", j, "].(int)")
				callArg = fmt.Sprintf("%s(a%d)", goType, j)
			case paramMessage:
				g.P("a", j, ", ok := ", protocheckPackage.Ident("MessageArg"), "[", goType, "](args[", j, "])")
			default:
				g.P("a", j, ", ok := args[", j, "].(", goType, ")")
			}
			// The guard asserts the resolved Go type, but the message names
			// the parameter type as declared in the schema (RFC-001 §7's
			// MsgFunctionArgType contract) — never the host-language type.
			g.P("if !ok {")
			g.P("return false, &", violationIdent, "{Code: ", protocheckPackage.Ident("CodeFunctionInvalidArgument"), ", FallbackMessage: ", protocheckPackage.Ident("MsgFunctionArgType"), "(", strconv.Quote(d.Name), ", ", j, ", ", strconv.Quote(p.Type), ")}")
			g.P("}")
			callArgs = append(callArgs, callArg)
		}
		g.P("return impl.", methods[i], "(", strings.Join(callArgs, ", "), ")")
		g.P("}); err != nil {")
		g.P("return err")
		g.P("}")
	}
	g.P("return nil")
	g.P("}")
	g.P()
}

// functionStubDecls returns the function declarations of f that get stubs.
func functionStubDecls(f *protogen.File) []schemaFunctionDecl {
	var decls []schemaFunctionDecl
	for _, d := range fileFunctions(f) {
		if d.Name != "" {
			decls = append(decls, d)
		}
	}
	return decls
}

// functionStubPrefix returns the <File> part of the stub names: the
// Go-camel-cased base name of the file, with an X in front when that would not
// start an exported identifier (01_basic.proto gives X01Basic).
func functionStubPrefix(f *protogen.File) string {
	name := goSanitized(goCamelCase(path.Base(f.GeneratedFilenamePrefix)))
	if r, _ := utf8.DecodeRuneInString(name); !unicode.IsUpper(r) {
		name = "X" + strings.TrimLeft(name, "_")
	}
	return name
}

// checkFunctionStubConflict reports a plugin error if another file known to
// this run would generate the same stub names into the same Go package as
// file, which happens when two file names camel-case alike (foo_bar.proto and
// fooBar.proto). A plugin error naming both files beats a compile error in
// generated code.
//
// Files in the same import path but different output directories are separate
// Go packages on disk (paths=source_relative), so they do not conflict.
func checkFunctionStubConflict(gen *protogen.Plugin, file *protogen.File) {
	if len(functionStubDecls(file)) == 0 {
		return
	}
	dir := path.Dir(file.GeneratedFilenamePrefix)
	prefix := functionStubPrefix(file)
	for _, other := range gen.Files {
		if other == file || other.GoImportPath != file.GoImportPath || path.Dir(other.GeneratedFilenamePrefix) != dir {
			continue
		}
		if other.GeneratedFilenamePrefix == file.GeneratedFilenamePrefix+"_functions" {
			gen.Error(fmt.Errorf("the function stubs of %s and the code of %s both generate %s.pb.go: rename one of the files",
				file.Desc.Path(), other.Desc.Path(), other.GeneratedFilenamePrefix))
			return
		}
		if functionStubPrefix(other) == prefix && len(functionStubDecls(other)) > 0 {
			gen.Error(fmt.Errorf("%s and %s both declare functions and generate into Go package %s "+
				"under the same RFC-001 §9.3 stub name %sFunctions: rename one of the files",
				file.Desc.Path(), other.Desc.Path(), file.GoImportPath, prefix))
			return
		}
	}
}

// functionParamSignature renders the Go parameter list for a function
// declaration. withNames includes parameter names (for the interface);
// without, only types are emitted (for UnimplementedFunctions methods).
func functionParamSignature(g *protogen.GeneratedFile, d schemaFunctionDecl, types schemaTypes, withNames bool) string {
	var parts []string
	usedNames := make(map[string]bool)
	for i, p := range d.Params {
		goType := types.paramType(p.Type, d.Name).goType(g)
		if !withNames {
			parts = append(parts, goType)
			continue
		}
		name := goSanitized(p.Name)
		if p.Name == "" || usedNames[name] {
			name = fmt.Sprintf("arg%d", i)
		}
		usedNames[name] = true
		parts = append(parts, name+" "+goType)
	}
	return strings.Join(parts, ", ")
}

// schemaTypes is what a function parameter's type can name across the files
// of a run: `type` aliases, enums and messages, by full name.
type schemaTypes struct {
	aliases  map[string]string
	enums    map[string]*protogen.Enum
	messages map[string]*protogen.Message
}

func newSchemaTypes(gen *protogen.Plugin) schemaTypes {
	t := schemaTypes{
		aliases:  make(map[string]string),
		enums:    make(map[string]*protogen.Enum),
		messages: make(map[string]*protogen.Message),
	}
	addEnums := func(enums []*protogen.Enum) {
		for _, e := range enums {
			t.enums[string(e.Desc.FullName())] = e
		}
	}
	var addMessages func([]*protogen.Message)
	addMessages = func(messages []*protogen.Message) {
		for _, m := range messages {
			t.messages[string(m.Desc.FullName())] = m
			addEnums(m.Enums)
			addMessages(m.Messages)
		}
	}
	for _, f := range gen.Files {
		maps.Copy(t.aliases, fileTypeAliases(f))
		addEnums(f.Enums)
		addMessages(f.Messages)
	}
	return t
}

type paramKind int

const (
	paramAny     paramKind = iota // no static type: passed through as any
	paramPlain                    // args holds the Go type itself
	paramEnum                     // args holds the number
	paramMessage                  // args holds the generated or a dynamic message
)

// paramType is how a parameter appears in generated Go: goName for a built-in
// type, ident otherwise.
type paramType struct {
	kind   paramKind
	goName string
	ident  protogen.GoIdent
}

func (p paramType) goType(g *protogen.GeneratedFile) string {
	switch {
	case p.kind == paramAny:
		return "any"
	case p.kind == paramMessage:
		return "*" + g.QualifiedGoIdent(p.ident)
	case p.goName != "":
		return p.goName
	}
	return g.QualifiedGoIdent(p.ident)
}

// schemaScalarGoTypes maps the types with a built-in Go form: scalars and the
// wrappers that RFC-001 §9.1 delivers as their base type.
var schemaScalarGoTypes = map[string]string{
	"string":                      "string",
	"bool":                        "bool",
	"bytes":                       "[]byte",
	"int32":                       "int32",
	"sint32":                      "int32",
	"sfixed32":                    "int32",
	"int64":                       "int64",
	"sint64":                      "int64",
	"sfixed64":                    "int64",
	"uint32":                      "uint32",
	"fixed32":                     "uint32",
	"uint64":                      "uint64",
	"fixed64":                     "uint64",
	"float":                       "float32",
	"double":                      "float64",
	"google.protobuf.StringValue": "string",
	"google.protobuf.BytesValue":  "[]byte",
	"google.protobuf.BoolValue":   "bool",
	"google.protobuf.Int32Value":  "int32",
	"google.protobuf.Int64Value":  "int64",
	"google.protobuf.UInt32Value": "uint32",
	"google.protobuf.UInt64Value": "uint64",
	"google.protobuf.FloatValue":  "float32",
	"google.protobuf.DoubleValue": "float64",
}

// schemaTimeGoTypes maps the well-known types that §9.1 delivers as time values.
var schemaTimeGoTypes = map[string]protogen.GoIdent{
	"google.protobuf.Timestamp": protogen.GoImportPath("time").Ident("Time"),
	"google.protobuf.Duration":  protogen.GoImportPath("time").Ident("Duration"),
}

// paramType maps a parameter type, named in the scope of the declaration fqn,
// to its generated Go form, following aliases. A type no file of the run
// declares stays any.
func (t schemaTypes) paramType(name, fqn string) paramType {
	seen := make(map[string]bool)
	for {
		if goName, ok := schemaScalarGoTypes[name]; ok {
			return paramType{kind: paramPlain, goName: goName}
		}
		if ident, ok := schemaTimeGoTypes[name]; ok {
			return paramType{kind: paramPlain, ident: ident}
		}
		full := t.resolve(name, fqn)
		if base, ok := t.aliases[full]; ok && !seen[full] {
			seen[full] = true
			name, fqn = base, full
			continue
		}
		if e, ok := t.enums[full]; ok {
			return paramType{kind: paramEnum, ident: e.GoIdent}
		}
		if m, ok := t.messages[full]; ok {
			return paramType{kind: paramMessage, ident: m.GoIdent}
		}
		return paramType{kind: paramAny}
	}
}

// resolve finds what name means in the enclosing scopes of fqn. A compiler
// before protocompile v0.37.0 keeps a parameter's spelling, and an alias's
// base can still be spelled package-locally.
func (t schemaTypes) resolve(name, fqn string) string {
	for dot := strings.LastIndexByte(fqn, '.'); dot >= 0; dot = strings.LastIndexByte(fqn, '.') {
		fqn = fqn[:dot]
		if candidate := fqn + "." + name; t.declares(candidate) {
			return candidate
		}
	}
	return name
}

func (t schemaTypes) declares(full string) bool {
	_, alias := t.aliases[full]
	_, enum := t.enums[full]
	_, message := t.messages[full]
	return alias || enum || message
}

// functionDeprecationReason reports whether the declaration carries a
// bracket-form deprecated option, and the reason to render.
func functionDeprecationReason(d schemaFunctionDecl) (string, bool) {
	opt, ok := d.Options["deprecated"]
	if !ok {
		return "", false
	}
	switch {
	case opt.Kind == schemaArgString && opt.String != "":
		return opt.String, true
	case opt.Kind == schemaArgBool && !opt.Bool:
		return "", false
	default:
		return "Do not use.", true
	}
}
