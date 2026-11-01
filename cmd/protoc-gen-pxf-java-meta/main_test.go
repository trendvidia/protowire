// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
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
