// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const (
	optionalLabel = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeatedLabel = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	stringType    = descriptorpb.FieldDescriptorProto_TYPE_STRING
	int32Type     = descriptorpb.FieldDescriptorProto_TYPE_INT32
	messageType   = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	groupType     = descriptorpb.FieldDescriptorProto_TYPE_GROUP
	requiredLabel = descriptorpb.FieldDescriptorProto_LABEL_REQUIRED
)

func field(
	name string,
	num int32,
	label descriptorpb.FieldDescriptorProto_Label,
	typ descriptorpb.FieldDescriptorProto_Type,
	typeName string,
) *descriptorpb.FieldDescriptorProto {
	f := &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(name),
		Number: proto.Int32(num),
		Label:  label.Enum(),
		Type:   typ.Enum(),
	}
	if typeName != "" {
		f.TypeName = proto.String(typeName)
	}
	return f
}

// keyed sets (pxf.key) the way a plugin receives it: as unknown option bytes.
func keyed(f *descriptorpb.FieldDescriptorProto, key string) *descriptorpb.FieldDescriptorProto {
	b := protowire.AppendTag(nil, protowire.Number(extPxfKey), protowire.BytesType)
	b = protowire.AppendString(b, key)
	f.Options = &descriptorpb.FieldOptions{}
	f.Options.ProtoReflect().SetUnknown(b)
	return f
}

func protoFile(name string, deps []string, msgs ...*descriptorpb.DescriptorProto) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:        proto.String(name),
		Package:     proto.String("keyed.v1"),
		Syntax:      proto.String("proto3"),
		Dependency:  deps,
		MessageType: msgs,
		Options:     &descriptorpb.FileOptions{JavaPackage: proto.String("org.example.keyed")},
	}
}

// generate drives the plugin the way protoc does, over the wire, and generates
// the last file: requests list a file's imports before it.
func generate(t *testing.T, files ...*descriptorpb.FileDescriptorProto) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	req := &pluginpb.CodeGeneratorRequest{
		ProtoFile:      files,
		FileToGenerate: []string{files[len(files)-1].GetName()},
	}
	raw, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(bytes.NewReader(raw), &out); err != nil {
		t.Fatal(err)
	}
	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(out.Bytes(), resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func generated(t *testing.T, resp *pluginpb.CodeGeneratorResponse, name string) string {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("generation failed: %s", resp.GetError())
	}
	for _, f := range resp.File {
		if strings.HasSuffix(f.GetName(), "/"+name) {
			return f.GetContent()
		}
	}
	t.Fatalf("%s was not generated", name)
	return ""
}

func TestKeyFieldsEmitted(t *testing.T) {
	// Region is imported, so the check has to resolve across files. Tree is a
	// nested type, to reach a keyed field below the top level.
	imported := protoFile("region.proto", nil, &descriptorpb.DescriptorProto{
		Name:  proto.String("Region"),
		Field: []*descriptorpb.FieldDescriptorProto{field("name", 1, optionalLabel, stringType, "")},
	})
	node := &descriptorpb.DescriptorProto{
		Name: proto.String("Node"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("id", 1, optionalLabel, stringType, ""),
			keyed(field("children", 4, repeatedLabel, messageType, ".keyed.v1.Node"), "id"),
			keyed(field("regions", 7, repeatedLabel, messageType, ".keyed.v1.Region"), "name"),
			field("plain", 8, repeatedLabel, messageType, ".keyed.v1.Region"),
		},
		NestedType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Tree"),
			Field: []*descriptorpb.FieldDescriptorProto{
				keyed(field("nodes", 2, repeatedLabel, messageType, ".keyed.v1.Node"), "id"),
			},
		}},
	}
	resp := generate(t, imported, protoFile("node.proto", []string{"region.proto"}, node))

	meta := generated(t, resp, "NodePxfMeta.java")
	for _, want := range []string{
		"Map.entry(4, \"id\"),\n",
		"Map.entry(7, \"name\")\n",
		"public Map<Integer, String>  keyFields()         { return KEY_FIELDS; }",
	} {
		if !strings.Contains(meta, want) {
			t.Errorf("NodePxfMeta.java lacks %q:\n%s", want, meta)
		}
	}
	_, table, _ := strings.Cut(meta, "KEY_FIELDS = ")
	table, _, _ = strings.Cut(table, ";")
	if strings.Contains(table, "Map.entry(8,") {
		t.Errorf("the unkeyed field reached KEY_FIELDS: %s", table)
	}
	if nested := generated(t, resp, "Node_TreePxfMeta.java"); !strings.Contains(nested, "Map.entry(2, \"id\")\n") {
		t.Errorf("nested message lost its keyed field:\n%s", nested)
	}
}

func TestUnkeyedMessageDoesNotOverrideKeyFields(t *testing.T) {
	resp := generate(t, protoFile("plain.proto", nil, &descriptorpb.DescriptorProto{
		Name:  proto.String("Plain"),
		Field: []*descriptorpb.FieldDescriptorProto{field("id", 1, optionalLabel, stringType, "")},
	}))
	meta := generated(t, resp, "PlainPxfMeta.java")
	if !strings.Contains(meta, "KEY_FIELDS = Map.of();") {
		t.Errorf("no empty table:\n%s", meta)
	}
	// This is what lets the companion compile against an older runtime.
	if strings.Contains(meta, "keyFields()") {
		t.Errorf("unkeyed companion overrides keyFields():\n%s", meta)
	}
}

func TestMisplacedKeyFailsGeneration(t *testing.T) {
	elem := &descriptorpb.DescriptorProto{
		Name: proto.String("Elem"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("id", 1, optionalLabel, stringType, ""),
			field("count", 2, optionalLabel, int32Type, ""),
			field("tags", 3, repeatedLabel, stringType, ""),
		},
	}
	entry := &descriptorpb.DescriptorProto{
		Name:    proto.String("LabelsEntry"),
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		Field: []*descriptorpb.FieldDescriptorProto{
			field("key", 1, optionalLabel, stringType, ""),
			field("value", 2, optionalLabel, stringType, ""),
		},
	}
	cases := []struct {
		name string
		f    *descriptorpb.FieldDescriptorProto
		want string
	}{
		{"singular message", keyed(field("f", 1, optionalLabel, messageType, ".keyed.v1.Elem"), "id"), "valid only on repeated message-typed fields"},
		{"repeated scalar", keyed(field("f", 1, repeatedLabel, stringType, ""), "id"), "valid only on repeated message-typed fields"},
		{"map", keyed(field("f", 1, repeatedLabel, messageType, ".keyed.v1.Holder.LabelsEntry"), "key"), "valid only on repeated message-typed fields"},
		{"group", keyed(field("f", 1, repeatedLabel, groupType, ".keyed.v1.Elem"), "id"), "valid only on repeated message-typed fields"},
		{"no such field", keyed(field("f", 1, repeatedLabel, messageType, ".keyed.v1.Elem"), "nope"), `element message keyed.v1.Elem has no field "nope"`},
		{"empty value", keyed(field("f", 1, repeatedLabel, messageType, ".keyed.v1.Elem"), ""), `element message keyed.v1.Elem has no field ""`},
		{"key not a string", keyed(field("f", 1, repeatedLabel, messageType, ".keyed.v1.Elem"), "count"), "key field keyed.v1.Elem.count must be a singular string field"},
		{"key repeated", keyed(field("f", 1, repeatedLabel, messageType, ".keyed.v1.Elem"), "tags"), "key field keyed.v1.Elem.tags must be a singular string field"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			holder := &descriptorpb.DescriptorProto{
				Name:       proto.String("Holder"),
				Field:      []*descriptorpb.FieldDescriptorProto{c.f},
				NestedType: []*descriptorpb.DescriptorProto{entry},
			}
			resp := generate(t, protoFile("bad.proto", nil, elem, holder))
			msg := resp.GetError()
			if !strings.Contains(msg, `bad.proto: field "keyed.v1.Holder.f"`) || !strings.Contains(msg, c.want) {
				t.Fatalf("error = %q, want the file, the field and %q", msg, c.want)
			}
			if len(resp.File) != 0 {
				t.Errorf("a refused schema generated %d files", len(resp.File))
			}
		})
	}
}

func TestEveryMisplacedKeyIsReported(t *testing.T) {
	resp := generate(t, protoFile("bad.proto", nil, &descriptorpb.DescriptorProto{
		Name: proto.String("Holder"),
		Field: []*descriptorpb.FieldDescriptorProto{
			keyed(field("a", 1, optionalLabel, stringType, ""), "x"),
			keyed(field("b", 2, optionalLabel, stringType, ""), "y"),
		},
	}))
	if n := strings.Count(resp.GetError(), "invalid (pxf.key)"); n != 2 {
		t.Fatalf("reported %d violations, want 2: %q", n, resp.GetError())
	}
}

// table returns the text of one generated constant, from its name to its ';'.
func table(t *testing.T, meta, name string) string {
	t.Helper()
	_, rest, ok := strings.Cut(meta, " "+name+" = ")
	if !ok {
		t.Fatalf("no %s in:\n%s", name, meta)
	}
	rest, _, _ = strings.Cut(rest, ";")
	return rest
}

func TestNullableFields(t *testing.T) {
	optional := field("opt", 2, optionalLabel, int32Type, "")
	optional.Proto3Optional = proto.Bool(true)
	optional.OneofIndex = proto.Int32(1)
	member := field("member", 3, optionalLabel, stringType, "")
	member.OneofIndex = proto.Int32(0)
	entry := &descriptorpb.DescriptorProto{
		Name:    proto.String("MEntry"),
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		Field: []*descriptorpb.FieldDescriptorProto{
			field("key", 1, optionalLabel, stringType, ""),
			field("value", 2, optionalLabel, int32Type, ""),
		},
	}
	resp := generate(t, protoFile("p3.proto", nil,
		&descriptorpb.DescriptorProto{Name: proto.String("Inner")},
		&descriptorpb.DescriptorProto{
			Name: proto.String("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{
				field("plain", 1, optionalLabel, int32Type, ""),
				optional,
				member,
				field("msg", 4, optionalLabel, messageType, ".keyed.v1.Inner"),
				field("list", 5, repeatedLabel, int32Type, ""),
				field("m", 6, repeatedLabel, messageType, ".keyed.v1.Holder.MEntry"),
			},
			OneofDecl:  []*descriptorpb.OneofDescriptorProto{{Name: proto.String("k")}, {Name: proto.String("_opt")}},
			NestedType: []*descriptorpb.DescriptorProto{entry},
		}))
	if got, want := table(t, generated(t, resp, "HolderPxfMeta.java"), "NULLABLE_FIELDS"), "Set.of(2, 3, 4)"; got != want {
		t.Errorf("proto3: NULLABLE_FIELDS = %s, want %s", got, want)
	}

	proto2 := protoFile("p2.proto", nil, &descriptorpb.DescriptorProto{
		Name: proto.String("Legacy"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("opt", 1, optionalLabel, int32Type, ""),
			field("req", 2, requiredLabel, int32Type, ""),
		},
	})
	proto2.Syntax = proto.String("proto2")
	if got, want := table(t, generated(t, generate(t, proto2), "LegacyPxfMeta.java"), "NULLABLE_FIELDS"), "Set.of(1)"; got != want {
		t.Errorf("proto2: NULLABLE_FIELDS = %s, want %s", got, want)
	}
}

// editionsFile is a 2023 file, whose defaults are explicit presence and packed
// repeated scalars.
func editionsFile(msgs ...*descriptorpb.DescriptorProto) *descriptorpb.FileDescriptorProto {
	f := protoFile("ed.proto", nil, msgs...)
	f.Syntax = proto.String("editions")
	f.Edition = descriptorpb.Edition_EDITION_2023.Enum()
	return f
}

func TestEditionsFeaturesDecideTheTables(t *testing.T) {
	implicit := field("implicit", 2, optionalLabel, int32Type, "")
	implicit.Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{
		FieldPresence: descriptorpb.FeatureSet_IMPLICIT.Enum(),
	}}
	expanded := field("expanded", 4, repeatedLabel, int32Type, "")
	expanded.Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{
		RepeatedFieldEncoding: descriptorpb.FeatureSet_EXPANDED.Enum(),
	}}
	legacyRequired := field("legacy", 5, optionalLabel, int32Type, "")
	legacyRequired.Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{
		FieldPresence: descriptorpb.FeatureSet_LEGACY_REQUIRED.Enum(),
	}}
	meta := generated(t, generate(t, editionsFile(&descriptorpb.DescriptorProto{
		Name: proto.String("Ed"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("explicit", 1, optionalLabel, int32Type, ""),
			implicit,
			field("packed", 3, repeatedLabel, int32Type, ""),
			expanded,
			legacyRequired,
		},
	})), "EdPxfMeta.java")
	if got, want := table(t, meta, "NULLABLE_FIELDS"), "Set.of(1)"; got != want {
		t.Errorf("NULLABLE_FIELDS = %s, want %s", got, want)
	}
	if got, want := table(t, meta, "PACKED_FIELDS"), "Set.of(3)"; got != want {
		t.Errorf("PACKED_FIELDS = %s, want %s", got, want)
	}
}

func TestPluginDeclaresEditions(t *testing.T) {
	resp := generate(t, protoFile("plain.proto", nil, &descriptorpb.DescriptorProto{Name: proto.String("Plain")}))
	if resp.GetSupportedFeatures()&uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS) == 0 {
		t.Error("protoc refuses an editions file unless the plugin declares the feature")
	}
	if got := descriptorpb.Edition(resp.GetMaximumEdition()); got != descriptorpb.Edition_EDITION_2023 {
		t.Errorf("maximum edition = %v", got)
	}
}

func TestDelimitedFieldFailsGeneration(t *testing.T) {
	f := field("inner", 1, optionalLabel, messageType, ".keyed.v1.Inner")
	f.Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{
		MessageEncoding: descriptorpb.FeatureSet_DELIMITED.Enum(),
	}}
	resp := generate(t, editionsFile(
		&descriptorpb.DescriptorProto{Name: proto.String("Inner")},
		&descriptorpb.DescriptorProto{Name: proto.String("Holder"), Field: []*descriptorpb.FieldDescriptorProto{f}},
	))
	if !strings.Contains(resp.GetError(), `"keyed.v1.Holder.inner"`) || !strings.Contains(resp.GetError(), "DELIMITED") {
		t.Fatalf("error = %q", resp.GetError())
	}
}

// fixtureFiles reads a testdata/annotations image and, for settings.binpb,
// gives its last file the java_package this plugin requires.
func fixtureFiles(t *testing.T, name string) []*descriptorpb.FileDescriptorProto {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/annotations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	last := set.File[len(set.File)-1]
	if last.GetOptions().GetJavaPackage() == "" {
		if last.Options == nil {
			last.Options = &descriptorpb.FileOptions{}
		}
		last.Options.JavaPackage = proto.String("org.example.settings")
	}
	return set.File
}

// The annotation form must generate what the bracket form does: nothing tells
// the runtime which spelling a schema used.
func TestAnnotationFormFillsTheTables(t *testing.T) {
	bracket := generated(t, generate(t, fixtureFiles(t, "settings.binpb")...), "SettingsPxfMeta.java")
	annotated := generated(t, generate(t, fixtureFiles(t, "settings_annotated.binpb")...), "SettingsPxfMeta.java")
	if got := table(t, annotated, "REQUIRED_FIELDS"); got != "Set.of(1)" {
		t.Errorf("REQUIRED_FIELDS = %s", got)
	}
	if !strings.Contains(table(t, annotated, "DEFAULTS"), `Map.entry(3, "us-east-1")`) {
		t.Errorf("DEFAULTS = %s", table(t, annotated, "DEFAULTS"))
	}
	for _, name := range []string{"REQUIRED_FIELDS", "DEFAULTS"} {
		if got, want := table(t, annotated, name), table(t, bracket, name); got != want {
			t.Errorf("%s differs between the spellings:\nannotated %s\nbracket   %s", name, got, want)
		}
	}
}

// carrierDefault is one AnnotationList holding @default with a message literal,
// which no single PXF literal can denote.
func carrierDefault() []byte {
	var arg []byte
	arg = protowire.AppendTag(arg, 15, protowire.BytesType) // AnnotationArg.literal
	arg = protowire.AppendBytes(arg, nil)
	var ann []byte
	ann = protowire.AppendTag(ann, 1, protowire.BytesType)
	ann = protowire.AppendString(ann, "protowire.schema.v1.default")
	ann = protowire.AppendTag(ann, 2, protowire.BytesType)
	ann = protowire.AppendBytes(ann, arg)
	var list []byte
	list = protowire.AppendTag(list, 1, protowire.BytesType)
	list = protowire.AppendBytes(list, ann)
	out := protowire.AppendTag(nil, 1327, protowire.BytesType)
	return protowire.AppendBytes(out, list)
}

func TestUnreducibleDefaultFailsGeneration(t *testing.T) {
	f := field("n", 1, optionalLabel, int32Type, "")
	f.Options = &descriptorpb.FieldOptions{}
	f.Options.ProtoReflect().SetUnknown(carrierDefault())
	resp := generate(t, protoFile("bad.proto", nil, &descriptorpb.DescriptorProto{
		Name:  proto.String("Holder"),
		Field: []*descriptorpb.FieldDescriptorProto{f},
	}))
	if !strings.Contains(resp.GetError(), "@default") {
		t.Fatalf("error = %q, want the @default named", resp.GetError())
	}
}

func TestSecretKind(t *testing.T) {
	secret := protoFile("pxf/secret.proto", nil, &descriptorpb.DescriptorProto{
		Name:  proto.String("Secret"),
		Field: []*descriptorpb.FieldDescriptorProto{field("value", 1, optionalLabel, stringType, "")},
	})
	secret.Package = proto.String("pxf")
	secret.Options.JavaPackage = proto.String("org.example.pxf")
	resp := generate(t, secret, protoFile("holder.proto", []string{"pxf/secret.proto"}, &descriptorpb.DescriptorProto{
		Name: proto.String("Holder"),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("pw", 1, optionalLabel, messageType, ".pxf.Secret"),
			field("pws", 2, repeatedLabel, messageType, ".pxf.Secret"),
		},
	}))
	meta := generated(t, resp, "HolderPxfMeta.java")
	if got := table(t, meta, "WELL_KNOWN_KINDS"); !strings.Contains(got, "Map.entry(1, PxfMeta.WKT_SECRET)") ||
		!strings.Contains(got, "Map.entry(2, PxfMeta.WKT_SECRET)") {
		t.Errorf("WELL_KNOWN_KINDS = %s", got)
	}
	// A Secret with metadata is a block, which needs the nested table.
	if got := table(t, meta, "NESTED_METAS"); !strings.Contains(got, "Map.entry(1, org.example.pxf.SecretPxfMeta.INSTANCE)") {
		t.Errorf("NESTED_METAS = %s", got)
	}
}
