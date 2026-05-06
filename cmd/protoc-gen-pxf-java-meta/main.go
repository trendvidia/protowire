// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Command protoc-gen-pxf-java-meta generates Java companion classes that
// expose protobuf descriptor metadata as compile-time constants, so PXF and
// SBE codecs can run on protobuf-javalite (Android) without descriptor
// reflection at runtime.
//
// For each input proto message Foo it emits FooPxfMeta.java in the package
// declared by the file's java_package option. The generated class implements
// the {@code org.protowire.pxf.PxfMeta} interface (defined in protowire-java's
// :pxf-runtime module) and exposes a {@code public static final INSTANCE}
// singleton for runtime-side interface dispatch. The same data is also
// exposed as {@code public static final} fields for direct-access consumers.
// Each instance contains:
//
//   - FIELD_NUMBERS — field name → field number, in declaration order.
//   - FIELD_KINDS   — field number → FieldDescriptorProto.Type integer (1..18).
//   - WIRE_TYPES    — field number → wire type integer (0=varint, 1=i64,
//                     2=length-delimited, 5=i32).
//   - REPEATED_FIELDS / PACKED_FIELDS — sets of field numbers.
//   - MESSAGE_TYPES — for message-typed fields, the fully-qualified Java
//                     class name (resolves java_package + java_outer_classname
//                     + java_multiple_files conventions).
//   - ENUM_TYPES    — same shape, for enum-typed fields.
//   - NESTED_METAS  — for message-typed fields, a direct reference to the
//                     target type's {@code <Sub>PxfMeta.INSTANCE}. The
//                     runtime walks the meta graph through this map without
//                     a registry lookup.
//   - ENUM_METAS    — same shape, for enum-typed fields, pointing at
//                     {@code <Sub>PxfEnum.INSTANCE}.
//
// For each enum it emits BarPxfEnum.java implementing {@code PxfEnum} (in
// :pxf-runtime) with:
//
//   - VALUES — name → int.
//   - NAMES  — int → name.
//
// Annotation-derived tables (lifted from .proto field/message options at
// codegen time):
//
//   - REQUIRED_FIELDS      — Set<Integer> of fields with (pxf.required).
//   - DEFAULTS             — Map<Integer, String> of (pxf.default) literals
//                            (PXF text literals; the runtime parses them).
//   - SBE_TEMPLATE_ID      — int constant from (sbe.template_id), or -1 if
//                            the message is not an SBE template.
//   - SBE_FIELD_LENGTHS    — Map<Integer, Integer> from (sbe.length).
//   - SBE_FIELD_ENCODINGS  — Map<Integer, String> from (sbe.encoding).
//   - ONEOF_OF             — Map<Integer, String> of field number → declaring
//                            oneof name. Synthetic oneofs around proto3
//                            `optional` fields are excluded.
//   - MAP_FIELDS           — Set<Integer> of fields declared as
//                            `map<K, V>` (vs. plain `repeated SomeMessage`).
//                            Wire shapes are identical; this codegen-time
//                            distinction is what lets the runtime dispatch
//                            to its map encoder.
//   - WELL_KNOWN_KINDS     — Map<Integer, Integer> from field number to a
//                            WKT-kind constant defined on PxfMeta
//                            (WKT_TIMESTAMP, WKT_BIG_INT, etc.). The lite
//                            decoder uses this to emit canonical bare
//                            literals (e.g. an RFC3339 timestamp string)
//                            instead of recursing into the WKT submessage
//                            as a generic block.
//
// Per-file SBE companion: when a .proto declares (sbe.schema_id) or
// (sbe.version) at file scope, an additional <Pascal>SbeFileMeta.java is
// emitted alongside the per-message metas with SCHEMA_ID and VERSION int
// constants (-1 individually when unset).
//
// Lite mode (off by default; opt in via the `lite` plugin parameter, e.g.
// `--pxf-java-meta_out=lite:DIR` or `opt: lite` in buf.gen.yaml): for each
// message the plugin also emits a <Message>PxfCodec.java — a typed
// convenience wrapper composing Parser → LiteWireWriter → MessageLite
// .parseFrom into a single static `unmarshal(text)` call. The codec
// references the user's typed message class by fully-qualified name so it
// works on both flat and nested generated layouts. Compile-time deps that
// the codec brings in: org.protowire.pxf.Parser + Position + PxfException
// (in :pxf-runtime), org.protowire.pxf.android.LiteWireWriter (in
// :pxf-android), and com.google.protobuf.InvalidProtocolBufferException
// (in protobuf-javalite). Off by default so non-lite consumers don't pay
// for those imports.
//
// Consume via `buf generate` (recommended) or plain `protoc`:
//
//	# buf.gen.yaml
//	version: v2
//	plugins:
//	  - remote: buf.build/protocolbuffers/java-lite
//	    out: build/generated/buf/java
//	  - local: protoc-gen-pxf-java-meta
//	    out: build/generated/buf/java
//
//	# or, directly:
//	protoc --plugin=$(go build -o /tmp/p ./cmd/protoc-gen-pxf-java-meta) \
//	       --pxf-java-meta_out=/tmp/out test.proto
//
// Unlike Go-output protoc plugins, this one does not require go_package on
// the input .proto files. It only reads java_package; absent that, it errors
// with a descriptive message.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// pluginOptions captures the comma-separated key[=value] parameters passed
// via `--pxf-java-meta_out=<opt>:<dir>` (protoc) or `opt: <opt>` (buf.gen.yaml).
type pluginOptions struct {
	// liteMode is set by `lite` (or `lite=true`). When enabled, the plugin
	// emits a `<Message>PxfCodec.java` per message: a typed convenience
	// wrapper that composes Parser → LiteWireWriter → MessageLite.parseFrom
	// for the lite-runtime tier. Off by default — the per-message PxfMeta
	// + per-enum PxfEnum tables are usable on their own without dragging in
	// the :pxf-android runtime dependency.
	liteMode bool
}

func parseParameters(raw string) pluginOptions {
	var o pluginOptions
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		switch key {
		case "lite":
			o.liteMode = !hasValue || strings.EqualFold(value, "true")
		}
	}
	return o
}

// Extension field numbers from proto/pxf/annotations.proto and
// proto/sbe/annotations.proto. Wire-format-stable per the project's
// "load-bearing — do not change" comment in those files.
const (
	extPxfRequired    int32 = 50000 // bool   on FieldOptions
	extPxfDefault     int32 = 50001 // string on FieldOptions
	extSbeSchemaID    int32 = 50100 // uint32 on FileOptions
	extSbeVersion     int32 = 50101 // uint32 on FileOptions
	extSbeTemplateID  int32 = 50200 // uint32 on MessageOptions
	extSbeFieldLength int32 = 50300 // uint32 on FieldOptions
	extSbeFieldEncode int32 = 50301 // string on FieldOptions
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "protoc-gen-pxf-java-meta: %v\n", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read CodeGeneratorRequest: %w", err)
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(raw, req); err != nil {
		return fmt.Errorf("unmarshal CodeGeneratorRequest: %w", err)
	}

	resp := &pluginpb.CodeGeneratorResponse{}
	resp.SupportedFeatures = proto.Uint64(uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL))

	opts := parseParameters(req.GetParameter())

	// Index every file in the request so cross-file references (e.g. a field
	// of type .other.Foo) can resolve naming conventions in the file that
	// declares the target message.
	byName := make(map[string]*descriptorpb.FileDescriptorProto, len(req.ProtoFile))
	for _, f := range req.ProtoFile {
		byName[f.GetName()] = f
	}

	generate := make(map[string]bool, len(req.FileToGenerate))
	for _, name := range req.FileToGenerate {
		generate[name] = true
	}

	for _, file := range req.ProtoFile {
		if !generate[file.GetName()] {
			continue
		}
		if err := generateFile(file, byName, opts, resp); err != nil {
			resp.Error = proto.String(err.Error())
			break
		}
	}

	outBytes, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal CodeGeneratorResponse: %w", err)
	}
	if _, err := out.Write(outBytes); err != nil {
		return fmt.Errorf("write CodeGeneratorResponse: %w", err)
	}
	return nil
}

func generateFile(
	file *descriptorpb.FileDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
	opts pluginOptions,
	resp *pluginpb.CodeGeneratorResponse,
) error {
	javaPkg := file.GetOptions().GetJavaPackage()
	if javaPkg == "" {
		return fmt.Errorf("file %q has no java_package option; protoc-gen-pxf-java-meta requires one", file.GetName())
	}
	pkgDir := strings.ReplaceAll(javaPkg, ".", "/")

	for _, msg := range allMessages(file.MessageType, nil) {
		filename := fmt.Sprintf("%s/%sPxfMeta.java", pkgDir, msg.classStem)
		content := emitMeta(file, byName, msg)
		resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String(filename),
			Content: proto.String(content),
		})
		// Skip PxfCodec emission for synthetic map-entry submessages.
		// protobuf-javalite collapses map<K,V> to a native Map<K,V> on the
		// containing class; the entry submessage is internal-only and the
		// typed reference (e.g. Foo.LabelsEntry) the codec wraps doesn't
		// exist on the public API. Map values still round-trip — encoder
		// + decoder reach the entry's PxfMeta through nestedMetas() — so
		// only the user-facing convenience class is affected.
		if opts.liteMode && !msg.desc.GetOptions().GetMapEntry() {
			codecFilename := fmt.Sprintf("%s/%sPxfCodec.java", pkgDir, msg.classStem)
			codecContent := emitCodec(file, byName, msg)
			resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
				Name:    proto.String(codecFilename),
				Content: proto.String(codecContent),
			})
			// Lite-mode SBE emit. Both helpers gate on (sbe.template_id) and
			// return ("", false) for untemplated messages, so non-SBE
			// schemas pay nothing.
			if metaContent, ok := emitSbeMeta(file, byName, msg); ok {
				resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
					Name:    proto.String(fmt.Sprintf("%s/%sSbeMeta.java", pkgDir, msg.classStem)),
					Content: proto.String(metaContent),
				})
			}
			if codecContent, ok := emitSbeCodec(file, byName, msg); ok {
				resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
					Name:    proto.String(fmt.Sprintf("%s/%sSbeCodec.java", pkgDir, msg.classStem)),
					Content: proto.String(codecContent),
				})
			}
		}
	}
	for _, en := range allEnums(file) {
		filename := fmt.Sprintf("%s/%sPxfEnum.java", pkgDir, en.classStem)
		content := emitEnum(file, en)
		resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String(filename),
			Content: proto.String(content),
		})
	}
	if filename, content, ok := emitSbeFile(file); ok {
		resp.File = append(resp.File, &pluginpb.CodeGeneratorResponse_File{
			Name:    proto.String(pkgDir + "/" + filename),
			Content: proto.String(content),
		})
	}
	return nil
}

// flatMsg captures a message together with the synthetic Java class-name
// stem used for its companion file. The stem joins nested types with "_"
// (e.g. message Person.Address yields stem "Person_Address", companion file
// Person_AddressPxfMeta.java with public class Person_AddressPxfMeta).
type flatMsg struct {
	desc      *descriptorpb.DescriptorProto
	fullName  string // e.g. "sample.Person.Address" (proto-package-rooted)
	classStem string // e.g. "Person_Address"
}

// flatEnum mirrors flatMsg for enum types.
type flatEnum struct {
	desc      *descriptorpb.EnumDescriptorProto
	fullName  string
	classStem string
}

// allMessages flattens nested message types into a single slice, including
// the synthetic map-entry submessages protoc generates for `map<K, V>`
// fields. Their PxfMeta is what the runtime's writeMap reads to figure out
// the key/value field shapes, so even though users never reference these
// classes by hand, the lite-runtime encoder needs the metadata available
// via `meta.nestedMetas().get(mapFieldNum)`.
func allMessages(top []*descriptorpb.DescriptorProto, parent *flatMsg) []*flatMsg {
	var out []*flatMsg
	for _, m := range top {
		var fullName, classStem string
		if parent != nil {
			fullName = parent.fullName + "." + m.GetName()
			classStem = parent.classStem + "_" + m.GetName()
		} else {
			fullName = m.GetName()
			classStem = m.GetName()
		}
		fm := &flatMsg{desc: m, fullName: fullName, classStem: classStem}
		out = append(out, fm)
		out = append(out, allMessages(m.NestedType, fm)...)
	}
	return out
}

// allEnums collects every enum (top-level and nested inside any message) in
// the file, in declaration order.
func allEnums(file *descriptorpb.FileDescriptorProto) []*flatEnum {
	var out []*flatEnum
	for _, e := range file.EnumType {
		out = append(out, &flatEnum{desc: e, fullName: e.GetName(), classStem: e.GetName()})
	}
	for _, msg := range allMessages(file.MessageType, nil) {
		for _, e := range msg.desc.EnumType {
			out = append(out, &flatEnum{
				desc:       e,
				fullName:   msg.fullName + "." + e.GetName(),
				classStem: msg.classStem + "_" + e.GetName(),
			})
		}
	}
	return out
}

// --- Java naming -------------------------------------------------------------

// javaContext bundles the file-scoped facts that drive Java class-name
// resolution.
type javaContext struct {
	javaPackage   string
	outerClass    string // "Sample" if java_outer_classname unset and filename was "sample.proto"
	multipleFiles bool
	protoPackage  string // "sample" — to strip from message FQNs
}

func javaContextFor(file *descriptorpb.FileDescriptorProto) javaContext {
	opts := file.GetOptions()
	return javaContext{
		javaPackage:   opts.GetJavaPackage(),
		outerClass:    javaOuterClass(file),
		multipleFiles: opts.GetJavaMultipleFiles(),
		protoPackage:  file.GetPackage(),
	}
}

// javaOuterClass returns the file's java_outer_classname option, or — when
// unset — the protoc-default: the filename basename converted to PascalCase
// (foo_bar.proto → "FooBar"). Matches protoc-gen-java's algorithm.
func javaOuterClass(file *descriptorpb.FileDescriptorProto) string {
	if v := file.GetOptions().GetJavaOuterClassname(); v != "" {
		return v
	}
	base := strings.TrimSuffix(filepath.Base(file.GetName()), filepath.Ext(file.GetName()))
	return toPascalCase(base)
}

func toPascalCase(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		// Both '_' and '-' are word separators per protoc-gen-java's
		// java_outer_classname default. `sbe-bench.proto` becomes
		// `SbeBench`, not `Sbe-bench` — the latter is invalid Java.
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// javaTypeName returns the dotted Java class name for a proto type. The input
// is a leading-dot-prefixed proto FQN as produced by protoc, e.g.
// ".sample.Person.Address". The returned string is suitable as a Java type
// reference like "com.example.sample.Person.Address".
func javaTypeName(byName map[string]*descriptorpb.FileDescriptorProto, protoFQN string) string {
	// Strip the leading dot.
	name := strings.TrimPrefix(protoFQN, ".")

	// Find the file that declares this type so we can read its java_package
	// + outer-class + multiple-files options.
	declFile := findDeclaringFile(byName, name)
	if declFile == nil {
		// Fallback: the type name unqualified. Resolution will fail at compile
		// time but a clear stub is more useful than silently emitting nothing.
		return name
	}
	ctx := javaContextFor(declFile)
	rel := strings.TrimPrefix(name, ctx.protoPackage+".")
	rel = strings.TrimPrefix(rel, ctx.protoPackage) // when message lives in default package
	rel = strings.TrimPrefix(rel, ".")
	if !ctx.multipleFiles {
		rel = ctx.outerClass + "." + rel
	}
	if ctx.javaPackage != "" {
		return ctx.javaPackage + "." + rel
	}
	return rel
}

// javaCompanionName returns the fully-qualified Java class name of the
// codegen companion for a proto type, e.g. ".sample.Person.Address" with
// suffix "PxfMeta" yields "com.example.sample.Person_AddressPxfMeta".
// Nested type chains are flattened to underscore-separated form to match
// the file-and-class-name layout that emitMeta / emitEnum produce.
func javaCompanionName(byName map[string]*descriptorpb.FileDescriptorProto, protoFQN, suffix string) string {
	name := strings.TrimPrefix(protoFQN, ".")
	declFile := findDeclaringFile(byName, name)
	if declFile == nil {
		// Fallback: best-effort name; downstream javac would surface the issue.
		return strings.ReplaceAll(name, ".", "_") + suffix
	}
	pkg := declFile.GetPackage()
	rel := name
	if pkg != "" {
		rel = strings.TrimPrefix(rel, pkg+".")
		rel = strings.TrimPrefix(rel, ".")
	}
	rel = strings.ReplaceAll(rel, ".", "_") + suffix
	javaPkg := declFile.GetOptions().GetJavaPackage()
	if javaPkg == "" {
		return rel
	}
	return javaPkg + "." + rel
}

// findDeclaringFile locates the FileDescriptorProto that declares the message
// or enum named protoFullName (no leading dot).
func findDeclaringFile(byName map[string]*descriptorpb.FileDescriptorProto, protoFullName string) *descriptorpb.FileDescriptorProto {
	for _, file := range byName {
		pkg := file.GetPackage()
		var rel string
		if pkg != "" {
			if !strings.HasPrefix(protoFullName, pkg+".") {
				continue
			}
			rel = protoFullName[len(pkg)+1:]
		} else {
			rel = protoFullName
		}
		if descriptorContains(file.MessageType, file.EnumType, rel) {
			return file
		}
	}
	return nil
}

// findMessage resolves a proto-package-qualified message name (no leading
// dot, e.g. "sample.Person.LabelsEntry") to its DescriptorProto. Returns
// nil when the type isn't in any known input file.
func findMessage(byName map[string]*descriptorpb.FileDescriptorProto, protoFullName string) *descriptorpb.DescriptorProto {
	file := findDeclaringFile(byName, protoFullName)
	if file == nil {
		return nil
	}
	rel := protoFullName
	if pkg := file.GetPackage(); pkg != "" {
		rel = strings.TrimPrefix(rel, pkg+".")
	}
	return findMessageRel(file.MessageType, rel)
}

func findMessageRel(msgs []*descriptorpb.DescriptorProto, rel string) *descriptorpb.DescriptorProto {
	parts := strings.SplitN(rel, ".", 2)
	head := parts[0]
	tail := ""
	if len(parts) == 2 {
		tail = parts[1]
	}
	for _, m := range msgs {
		if m.GetName() != head {
			continue
		}
		if tail == "" {
			return m
		}
		return findMessageRel(m.NestedType, tail)
	}
	return nil
}

// isMapField reports whether the field is a proto3 `map<K, V>` declaration
// (vs. plain `repeated SomeMessage`). The wire shape is identical; the
// distinction lives in the target message's MessageOptions.map_entry flag,
// which protoc sets on the synthetic entry submessage. Cross-file map types
// resolve via the byName index.
func isMapField(f *descriptorpb.FieldDescriptorProto, byName map[string]*descriptorpb.FileDescriptorProto) bool {
	if f.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	if f.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return false
	}
	target := findMessage(byName, strings.TrimPrefix(f.GetTypeName(), "."))
	if target == nil {
		return false
	}
	return target.GetOptions().GetMapEntry()
}

// descriptorContains walks message + enum descriptors checking whether the
// dot-delimited relative name resolves inside them.
func descriptorContains(msgs []*descriptorpb.DescriptorProto, enums []*descriptorpb.EnumDescriptorProto, rel string) bool {
	parts := strings.SplitN(rel, ".", 2)
	head := parts[0]
	tail := ""
	if len(parts) == 2 {
		tail = parts[1]
	}
	for _, m := range msgs {
		if m.GetName() != head {
			continue
		}
		if tail == "" {
			return true
		}
		return descriptorContains(m.NestedType, m.EnumType, tail)
	}
	if tail == "" {
		for _, e := range enums {
			if e.GetName() == head {
				return true
			}
		}
	}
	return false
}

// --- Wire types --------------------------------------------------------------

// wireTypeFor returns the protobuf wire type integer (0/1/2/5) for a given
// FieldDescriptorProto.Type. Group fields (TYPE_GROUP) report 2 — protowire
// does not target proto2 groups, so the value is a placeholder. Length-
// delimited covers string, bytes, message, and packed-repeated scalars.
func wireTypeFor(t descriptorpb.FieldDescriptorProto_Type) int {
	switch t {
	case descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_BOOL,
		descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return 1
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return 5
	default:
		// STRING, BYTES, MESSAGE, GROUP — all length-delimited.
		return 2
	}
}

// isPacked reflects whether a repeated field uses packed encoding.
// Defaults: proto3 packs repeated scalars unless [packed=false]; proto2 does
// not pack unless [packed=true]. String, bytes, and message-typed fields are
// never packed.
func isPacked(field *descriptorpb.FieldDescriptorProto, syntax string) bool {
	if field.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	switch field.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING,
		descriptorpb.FieldDescriptorProto_TYPE_BYTES,
		descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
		descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return false
	}
	if field.Options != nil && field.Options.Packed != nil {
		return *field.Options.Packed
	}
	return syntax == "proto3"
}

// --- Extension scanning -----------------------------------------------------
//
// Custom options like (pxf.required) and (sbe.template_id) reach this plugin
// as bytes inside the unknown-fields region of the unmarshaled
// FieldOptions / MessageOptions / FileOptions, because descriptorpb only
// knows about the standard descriptor.proto fields. The scanner walks those
// bytes once per message and pulls out the values we care about by their
// well-known field numbers (see `proto/{pxf,sbe}/annotations.proto` for the
// authoritative declarations).
//
// Mirrors the runtime-side reader in
// `protowire-go/encoding/pxf/annotations.go` — the wire layout is the same
// in both directions, the plugin just runs at codegen time.

// scanExt walks the unknown-fields region of `opts` once and reports the
// raw value-bytes plus wire type for the requested field number. Returns
// (nil, 0, false) if absent.
func scanExt(opts proto.Message, fieldNumber int32) ([]byte, protowire.Type, bool) {
	b := extBytes(opts)
	for len(b) > 0 {
		num, typ, tagLen := protowire.ConsumeTag(b)
		if tagLen < 0 {
			return nil, 0, false
		}
		valLen := protowire.ConsumeFieldValue(num, typ, b[tagLen:])
		if valLen < 0 {
			return nil, 0, false
		}
		if int32(num) == fieldNumber {
			return b[tagLen : tagLen+valLen], typ, true
		}
		b = b[tagLen+valLen:]
	}
	return nil, 0, false
}

func extBool(opts proto.Message, fieldNumber int32) bool {
	val, typ, ok := scanExt(opts, fieldNumber)
	if !ok || typ != protowire.VarintType {
		return false
	}
	v, n := protowire.ConsumeVarint(val)
	if n < 0 {
		return false
	}
	return v != 0
}

func extString(opts proto.Message, fieldNumber int32) (string, bool) {
	val, typ, ok := scanExt(opts, fieldNumber)
	if !ok || typ != protowire.BytesType {
		return "", false
	}
	v, n := protowire.ConsumeBytes(val)
	if n < 0 {
		return "", false
	}
	return string(v), true
}

func extUint32(opts proto.Message, fieldNumber int32) (uint32, bool) {
	val, typ, ok := scanExt(opts, fieldNumber)
	if !ok || typ != protowire.VarintType {
		return 0, false
	}
	v, n := protowire.ConsumeVarint(val)
	if n < 0 {
		return 0, false
	}
	return uint32(v), true
}

// extBytes returns the unknown-fields wire bytes for the given options
// message, or nil if opts is nil.
func extBytes(opts proto.Message) []byte {
	if opts == nil {
		return nil
	}
	return opts.ProtoReflect().GetUnknown()
}

// --- Java emission -----------------------------------------------------------

func emitMeta(
	file *descriptorpb.FileDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
	msg *flatMsg,
) string {
	cls := msg.classStem + "PxfMeta"
	javaPkg := file.GetOptions().GetJavaPackage()
	syntax := file.GetSyntax()
	if syntax == "" {
		syntax = "proto2"
	}

	var b strings.Builder
	p := lineWriter(&b)

	p("// Generated by protoc-gen-pxf-java-meta. DO NOT EDIT.")
	p("// source: ", file.GetName())
	p()
	p("package ", javaPkg, ";")
	p()
	p("import java.util.Map;")
	p("import java.util.Set;")
	p()
	p("import org.protowire.pxf.PxfEnum;")
	p("import org.protowire.pxf.PxfMeta;")
	p()
	p("/**")
	p(" * PXF / SBE descriptor metadata for {@code ", msg.fullName, "}, lifted at")
	p(" * codegen time so the runtime never needs descriptor reflection.")
	p(" *")
	p(" * <p>Implements {@link PxfMeta} so the protobuf-javalite runtime can dispatch")
	p(" * through the interface — pass {@code ", cls, ".INSTANCE} where a {@code PxfMeta}")
	p(" * is expected. The static {@code FIELD_NUMBERS}, {@code FIELD_KINDS}, etc.")
	p(" * fields remain public for direct access by static-only consumers.")
	p(" */")
	p("public final class ", cls, " implements PxfMeta {")
	p("    private ", cls, "() {}")
	p()
	p("    /** Singleton; pass to runtime APIs that take a {@link PxfMeta}. */")
	p("    public static final ", cls, " INSTANCE = new ", cls, "();")
	p()

	emitFieldNumbers(p, msg.desc.Field)
	p()
	emitFieldKinds(p, msg.desc.Field)
	p()
	emitWireTypes(p, msg.desc.Field)
	p()
	emitFieldSet(p, "REPEATED_FIELDS", msg.desc.Field, func(f *descriptorpb.FieldDescriptorProto) bool {
		return f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	})
	p()
	emitFieldSet(p, "PACKED_FIELDS", msg.desc.Field, func(f *descriptorpb.FieldDescriptorProto) bool {
		return isPacked(f, syntax)
	})
	p()
	emitTypeNames(p, "MESSAGE_TYPES", msg.desc.Field, byName, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE)
	p()
	emitTypeNames(p, "ENUM_TYPES", msg.desc.Field, byName, descriptorpb.FieldDescriptorProto_TYPE_ENUM)
	p()
	emitRequiredFields(p, msg.desc.Field)
	p()
	emitDefaults(p, msg.desc.Field)
	p()
	emitSbeMessageMeta(p, msg.desc)
	p()
	emitSbeFieldLengths(p, msg.desc.Field)
	p()
	emitSbeFieldEncodings(p, msg.desc.Field)
	p()
	emitOneofs(p, msg.desc)
	p()
	emitMapFields(p, msg.desc.Field, byName)
	p()
	emitWellKnownKinds(p, msg.desc.Field)
	p()
	emitNestedMetas(p, msg.desc.Field, byName)
	p()
	emitEnumMetas(p, msg.desc.Field, byName)
	p()
	emitAccessors(p, qualifiedProtoName(file, msg))
	p("}")
	return b.String()
}

// emitMapFields emits MAP_FIELDS — the set of field numbers declared as
// {@code map<K, V>}. Wire-shape-wise these are indistinguishable from a
// plain {@code repeated SomeMessage}, so the runtime relies on this codegen-
// time distinction to dispatch to the map encoder.
func emitMapFields(
	p func(args ...any),
	fields []*descriptorpb.FieldDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
) {
	var nums []int32
	for _, f := range fields {
		if isMapField(f, byName) {
			nums = append(nums, f.GetNumber())
		}
	}
	p("    /** Field numbers declared as {@code map<K, V>} in the source .proto. */")
	if len(nums) == 0 {
		p("    public static final Set<Integer> MAP_FIELDS = Set.of();")
		return
	}
	args := make([]string, len(nums))
	for i, n := range nums {
		args[i] = fmt.Sprintf("%d", n)
	}
	p("    public static final Set<Integer> MAP_FIELDS = Set.of(", strings.Join(args, ", "), ");")
}

// emitNestedMetas emits NESTED_METAS — a map from each MESSAGE-typed field
// number to the {@code <Sub>PxfMeta.INSTANCE} companion of the target type.
// Fully-qualifies class references to avoid coupling to a specific import
// layout (the generated code stays robust against code-folding tools).
//
// Initialization order is INSTANCE-first, then field tables, then this map,
// so circular schemas (A → B, B → A) initialize correctly via the same-thread
// re-entry rule in JLS §12.4.2: each class's INSTANCE is non-null by the time
// the other class's NESTED_METAS dereferences it.
func emitNestedMetas(
	p func(args ...any),
	fields []*descriptorpb.FieldDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
) {
	type entry struct {
		num     int32
		instRef string
	}
	var entries []entry
	for _, f := range fields {
		if f.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			continue
		}
		// Skip well-known-typed fields. Their decoder dispatch goes through
		// WELL_KNOWN_KINDS, not NESTED_METAS, and their companion classes
		// (TimestampPxfMeta etc.) aren't part of the canonical codegen surface
		// — the .proto files are imports, never in FileToGenerate, so emitting
		// references to them yields a dangling pointer at javac time.
		fqn := strings.TrimPrefix(f.GetTypeName(), ".")
		if _, isWkt := wellKnownKindByProtoFQN[fqn]; isWkt {
			continue
		}
		entries = append(entries, entry{
			num:     f.GetNumber(),
			instRef: javaCompanionName(byName, f.GetTypeName(), "PxfMeta") + ".INSTANCE",
		})
	}
	p("    /** Field number → nested {@link PxfMeta} INSTANCE for MESSAGE-kind fields. */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, PxfMeta> NESTED_METAS = Map.of();")
		return
	}
	p("    public static final Map<Integer, PxfMeta> NESTED_METAS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", e.instRef, ")", comma)
	}
	p("    );")
}

// emitEnumMetas mirrors emitNestedMetas for ENUM-kind fields, referencing
// {@code <Sub>PxfEnum.INSTANCE} companions.
func emitEnumMetas(
	p func(args ...any),
	fields []*descriptorpb.FieldDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
) {
	type entry struct {
		num     int32
		instRef string
	}
	var entries []entry
	for _, f := range fields {
		if f.GetType() != descriptorpb.FieldDescriptorProto_TYPE_ENUM {
			continue
		}
		entries = append(entries, entry{
			num:     f.GetNumber(),
			instRef: javaCompanionName(byName, f.GetTypeName(), "PxfEnum") + ".INSTANCE",
		})
	}
	p("    /** Field number → {@link PxfEnum} INSTANCE for ENUM-kind fields. */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, PxfEnum> ENUM_METAS = Map.of();")
		return
	}
	p("    public static final Map<Integer, PxfEnum> ENUM_METAS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", e.instRef, ")", comma)
	}
	p("    );")
}

// qualifiedProtoName returns the proto-package-qualified message name, e.g.
// "sample.Person" or "sample.Person.Address" — suitable for the PxfMeta
// fullName() contract. msg.fullName is rooted at the file's proto package
// boundary (no package prefix) by construction in allMessages, so we prepend
// the file's package here.
func qualifiedProtoName(file *descriptorpb.FileDescriptorProto, msg *flatMsg) string {
	pkg := file.GetPackage()
	if pkg == "" {
		return msg.fullName
	}
	return pkg + "." + msg.fullName
}

// emitAccessors writes the @Override interface methods that wrap each static
// data field for runtime-side interface dispatch via PxfMeta. The set must
// stay in lock-step with the PxfMeta interface defined in
// :pxf-runtime/.../PxfMeta.java — adding a method there without updating
// here is a compile error in downstream consumers.
func emitAccessors(p func(args ...any), fullName string) {
	p("    @Override public Map<String, Integer>  fieldNumbers()      { return FIELD_NUMBERS; }")
	p("    @Override public Map<Integer, Integer> fieldKinds()        { return FIELD_KINDS; }")
	p("    @Override public Map<Integer, Integer> wireTypes()         { return WIRE_TYPES; }")
	p("    @Override public Set<Integer>          repeatedFields()    { return REPEATED_FIELDS; }")
	p("    @Override public Set<Integer>          packedFields()      { return PACKED_FIELDS; }")
	p("    @Override public Map<Integer, String>  messageTypes()      { return MESSAGE_TYPES; }")
	p("    @Override public Map<Integer, String>  enumTypes()         { return ENUM_TYPES; }")
	p("    @Override public Set<Integer>          requiredFields()    { return REQUIRED_FIELDS; }")
	p("    @Override public Map<Integer, String>  defaults()          { return DEFAULTS; }")
	p("    @Override public int                   sbeTemplateId()     { return SBE_TEMPLATE_ID; }")
	p("    @Override public Map<Integer, Integer> sbeFieldLengths()   { return SBE_FIELD_LENGTHS; }")
	p("    @Override public Map<Integer, String>  sbeFieldEncodings() { return SBE_FIELD_ENCODINGS; }")
	p("    @Override public Map<Integer, String>  oneofOf()           { return ONEOF_OF; }")
	p("    @Override public String                fullName()          { return ", quoteJava(fullName), "; }")
	p("    @Override public Map<Integer, PxfMeta> nestedMetas()       { return NESTED_METAS; }")
	p("    @Override public Map<Integer, PxfEnum> enumMetas()         { return ENUM_METAS; }")
	p("    @Override public Set<Integer>          mapFields()         { return MAP_FIELDS; }")
	p("    @Override public Map<Integer, Integer> wellKnownKinds()    { return WELL_KNOWN_KINDS; }")
}

// wellKnownKindByProtoFQN maps recognized well-known-type proto FQNs to the
// integer constants defined on PxfMeta in :pxf-runtime. Stable wire-of-the-
// meta-graph contract — adding new entries is fine, renumbering breaks every
// previously-generated PxfMeta.
//
// Java-side constants live in org.protowire.pxf.PxfMeta (e.g. WKT_TIMESTAMP =
// 1). Both files must be updated in lock-step when extending this list.
var wellKnownKindByProtoFQN = map[string]struct {
	kind int
	name string // e.g. "WKT_TIMESTAMP" — emitted as a symbolic reference for readability
}{
	"google.protobuf.Timestamp":   {1, "WKT_TIMESTAMP"},
	"google.protobuf.Duration":    {2, "WKT_DURATION"},
	"google.protobuf.StringValue": {3, "WKT_STRING_VALUE"},
	"google.protobuf.BytesValue":  {4, "WKT_BYTES_VALUE"},
	"google.protobuf.BoolValue":   {5, "WKT_BOOL_VALUE"},
	"google.protobuf.Int32Value":  {6, "WKT_INT32_VALUE"},
	"google.protobuf.Int64Value":  {7, "WKT_INT64_VALUE"},
	"google.protobuf.UInt32Value": {8, "WKT_UINT32_VALUE"},
	"google.protobuf.UInt64Value": {9, "WKT_UINT64_VALUE"},
	"google.protobuf.FloatValue":  {10, "WKT_FLOAT_VALUE"},
	"google.protobuf.DoubleValue": {11, "WKT_DOUBLE_VALUE"},
	"pxf.BigInt":                  {12, "WKT_BIG_INT"},
	"pxf.Decimal":                 {13, "WKT_DECIMAL"},
	"pxf.BigFloat":                {14, "WKT_BIG_FLOAT"},
}

// emitWellKnownKinds emits WELL_KNOWN_KINDS — a Map<Integer, Integer> from
// field number to a PxfMeta.WKT_* constant, populated for any MESSAGE-typed
// field whose target is one of the recognized well-known types. The lite
// decoder uses this to emit canonical bare literals (RFC3339 timestamps,
// unwrapped scalars, decimal numbers) instead of recursing into the WKT
// submessage as a generic block.
func emitWellKnownKinds(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	type kv struct {
		num    int32
		wktSym string
	}
	var entries []kv
	for _, f := range fields {
		if f.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			continue
		}
		fqn := strings.TrimPrefix(f.GetTypeName(), ".")
		if w, ok := wellKnownKindByProtoFQN[fqn]; ok {
			entries = append(entries, kv{num: f.GetNumber(), wktSym: w.name})
		}
	}
	p("    /**")
	p("     * Field number → {@code PxfMeta.WKT_*} kind constant for fields whose")
	p("     * target message is a recognized well-known type.")
	p("     */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, Integer> WELL_KNOWN_KINDS = Map.of();")
		return
	}
	p("    public static final Map<Integer, Integer> WELL_KNOWN_KINDS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", PxfMeta.", e.wktSym, ")", comma)
	}
	p("    );")
}

// emitRequiredFields walks fields and emits a Set<Integer> of field numbers
// where (pxf.required) = true.
func emitRequiredFields(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	var nums []int32
	for _, f := range fields {
		if extBool(f.GetOptions(), extPxfRequired) {
			nums = append(nums, f.GetNumber())
		}
	}
	p("    /** Field numbers marked {@code [(pxf.required) = true]}. */")
	if len(nums) == 0 {
		p("    public static final Set<Integer> REQUIRED_FIELDS = Set.of();")
		return
	}
	args := make([]string, len(nums))
	for i, n := range nums {
		args[i] = fmt.Sprintf("%d", n)
	}
	p("    public static final Set<Integer> REQUIRED_FIELDS = Set.of(", strings.Join(args, ", "), ");")
}

// emitDefaults walks fields and emits a Map<Integer, String> of field number
// to PXF default literal (the option value is a PXF text literal, not a
// parsed scalar — the runtime is responsible for parsing it through the same
// PXF lexer used for user input).
func emitDefaults(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	type kv struct {
		num int32
		val string
	}
	var entries []kv
	for _, f := range fields {
		if v, ok := extString(f.GetOptions(), extPxfDefault); ok && v != "" {
			entries = append(entries, kv{num: f.GetNumber(), val: v})
		}
	}
	p("    /**")
	p("     * Field number → {@code (pxf.default)} literal. Values are PXF text")
	p("     * literals (e.g. {@code \"42\"}, {@code \"true\"}, {@code \"\\\"hi\\\"\"}); the")
	p("     * runtime feeds them through the PXF lexer to resolve to typed values.")
	p("     */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, String> DEFAULTS = Map.of();")
		return
	}
	p("    public static final Map<Integer, String> DEFAULTS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", quoteJava(e.val), ")", comma)
	}
	p("    );")
}

// emitSbeMessageMeta emits a SBE_TEMPLATE_ID int constant when the message
// has (sbe.template_id) set, marking it as an SBE-templated message. When
// absent the constant is -1, which the runtime can treat as "this is not an
// SBE message".
func emitSbeMessageMeta(p func(args ...any), msg *descriptorpb.DescriptorProto) {
	id, ok := extUint32(msg.GetOptions(), extSbeTemplateID)
	p("    /** {@code (sbe.template_id)} value, or -1 if unset. */")
	if !ok {
		p("    public static final int SBE_TEMPLATE_ID = -1;")
		return
	}
	p("    public static final int SBE_TEMPLATE_ID = ", id, ";")
}

// emitSbeFieldLengths emits Map<Integer, Integer> for fields with
// (sbe.length) set — typically string/bytes fields whose SBE encoding fixes
// a wire length distinct from the proto string/bytes natural length.
func emitSbeFieldLengths(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	type kv struct {
		num int32
		len uint32
	}
	var entries []kv
	for _, f := range fields {
		if v, ok := extUint32(f.GetOptions(), extSbeFieldLength); ok && v != 0 {
			entries = append(entries, kv{num: f.GetNumber(), len: v})
		}
	}
	p("    /** Field number → {@code (sbe.length)} byte length, where set. */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, Integer> SBE_FIELD_LENGTHS = Map.of();")
		return
	}
	p("    public static final Map<Integer, Integer> SBE_FIELD_LENGTHS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", e.len, ")", comma)
	}
	p("    );")
}

// emitSbeFieldEncodings emits Map<Integer, String> for fields with
// (sbe.encoding) set — narrows a proto numeric type to a smaller SBE
// primitive (e.g. uint32 proto field encoded as "uint8").
func emitSbeFieldEncodings(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	type kv struct {
		num int32
		enc string
	}
	var entries []kv
	for _, f := range fields {
		if v, ok := extString(f.GetOptions(), extSbeFieldEncode); ok && v != "" {
			entries = append(entries, kv{num: f.GetNumber(), enc: v})
		}
	}
	p("    /** Field number → {@code (sbe.encoding)} primitive name, where set. */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, String> SBE_FIELD_ENCODINGS = Map.of();")
		return
	}
	p("    public static final Map<Integer, String> SBE_FIELD_ENCODINGS = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", quoteJava(e.enc), ")", comma)
	}
	p("    );")
}

func emitFieldNumbers(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	p("    /** Field name → field number, in declaration order. */")
	if len(fields) == 0 {
		p("    public static final Map<String, Integer> FIELD_NUMBERS = Map.of();")
		return
	}
	p("    public static final Map<String, Integer> FIELD_NUMBERS = Map.ofEntries(")
	for i, fld := range fields {
		comma := ","
		if i == len(fields)-1 {
			comma = ""
		}
		p("        Map.entry(", quoteJava(fld.GetName()), ", ", fld.GetNumber(), ")", comma)
	}
	p("    );")
}

// emitFieldKinds maps field number → FieldDescriptorProto.Type integer (1..18,
// per descriptor.proto). Stable across protobuf versions; safe to compare with
// integer literals or with the analogue enum on the runtime side.
func emitFieldKinds(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	p("    /**")
	p("     * Field number → proto kind integer. Values follow")
	p("     * {@code google.protobuf.FieldDescriptorProto.Type} (1..18, e.g. 9 = STRING,")
	p("     * 11 = MESSAGE, 14 = ENUM).")
	p("     */")
	if len(fields) == 0 {
		p("    public static final Map<Integer, Integer> FIELD_KINDS = Map.of();")
		return
	}
	p("    public static final Map<Integer, Integer> FIELD_KINDS = Map.ofEntries(")
	for i, fld := range fields {
		comma := ","
		if i == len(fields)-1 {
			comma = ""
		}
		p("        Map.entry(", fld.GetNumber(), ", ", int32(fld.GetType()), ")", comma)
	}
	p("    );")
}

func emitWireTypes(p func(args ...any), fields []*descriptorpb.FieldDescriptorProto) {
	p("    /** Field number → wire type integer (0=varint, 1=i64, 2=length-delimited, 5=i32). */")
	if len(fields) == 0 {
		p("    public static final Map<Integer, Integer> WIRE_TYPES = Map.of();")
		return
	}
	p("    public static final Map<Integer, Integer> WIRE_TYPES = Map.ofEntries(")
	for i, fld := range fields {
		comma := ","
		if i == len(fields)-1 {
			comma = ""
		}
		p("        Map.entry(", fld.GetNumber(), ", ", wireTypeFor(fld.GetType()), ")", comma)
	}
	p("    );")
}

func emitFieldSet(
	p func(args ...any),
	name string,
	fields []*descriptorpb.FieldDescriptorProto,
	pred func(*descriptorpb.FieldDescriptorProto) bool,
) {
	var picked []int32
	for _, f := range fields {
		if pred(f) {
			picked = append(picked, f.GetNumber())
		}
	}
	p("    /** Field numbers matching the predicate (see Javadoc on each constant). */")
	if len(picked) == 0 {
		p("    public static final Set<Integer> ", name, " = Set.of();")
		return
	}
	args := make([]string, len(picked))
	for i, n := range picked {
		args[i] = fmt.Sprintf("%d", n)
	}
	p("    public static final Set<Integer> ", name, " = Set.of(", strings.Join(args, ", "), ");")
}

func emitTypeNames(
	p func(args ...any),
	name string,
	fields []*descriptorpb.FieldDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
	matchKind descriptorpb.FieldDescriptorProto_Type,
) {
	type entry struct {
		num  int32
		java string
	}
	var entries []entry
	for _, f := range fields {
		if f.GetType() != matchKind {
			continue
		}
		entries = append(entries, entry{
			num:  f.GetNumber(),
			java: javaTypeName(byName, f.GetTypeName()),
		})
	}
	p("    /** Field number → fully-qualified Java class name, for fields of the matching kind. */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, String> ", name, " = Map.of();")
		return
	}
	p("    public static final Map<Integer, String> ", name, " = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", quoteJava(e.java), ")", comma)
	}
	p("    );")
}

func emitEnum(file *descriptorpb.FileDescriptorProto, en *flatEnum) string {
	cls := en.classStem + "PxfEnum"
	javaPkg := file.GetOptions().GetJavaPackage()
	fullName := qualifiedProtoEnumName(file, en)

	var b strings.Builder
	p := lineWriter(&b)

	p("// Generated by protoc-gen-pxf-java-meta. DO NOT EDIT.")
	p("// source: ", file.GetName())
	p()
	p("package ", javaPkg, ";")
	p()
	p("import java.util.Map;")
	p()
	p("import org.protowire.pxf.PxfEnum;")
	p()
	p("/**")
	p(" * PXF enum value tables for {@code ", en.fullName, "}.")
	p(" *")
	p(" * <p>Value-name → int and int → value-name lookups, lifted at codegen time")
	p(" * so the runtime never needs descriptor reflection. Implements")
	p(" * {@link PxfEnum} for runtime-side interface dispatch — pass {@code ", cls, ".INSTANCE}")
	p(" * where a {@link PxfEnum} is expected.")
	p(" */")
	p("public final class ", cls, " implements PxfEnum {")
	p("    private ", cls, "() {}")
	p()
	p("    /** Singleton; pass to runtime APIs that take a {@link PxfEnum}. */")
	p("    public static final ", cls, " INSTANCE = new ", cls, "();")
	p()
	p("    /** Enum value name → integer. */")
	if len(en.desc.Value) == 0 {
		p("    public static final Map<String, Integer> VALUES = Map.of();")
	} else {
		p("    public static final Map<String, Integer> VALUES = Map.ofEntries(")
		for i, v := range en.desc.Value {
			comma := ","
			if i == len(en.desc.Value)-1 {
				comma = ""
			}
			p("        Map.entry(", quoteJava(v.GetName()), ", ", v.GetNumber(), ")", comma)
		}
		p("    );")
	}
	p()
	p("    /** Integer → enum value name. Aliased values map to the first declaration. */")
	if len(en.desc.Value) == 0 {
		p("    public static final Map<Integer, String> NAMES = Map.of();")
	} else {
		// Dedupe by value (proto enum aliasing): keep the first name encountered.
		seen := make(map[int32]bool)
		type kv struct {
			n int32
			s string
		}
		var kept []kv
		for _, v := range en.desc.Value {
			if seen[v.GetNumber()] {
				continue
			}
			seen[v.GetNumber()] = true
			kept = append(kept, kv{n: v.GetNumber(), s: v.GetName()})
		}
		p("    public static final Map<Integer, String> NAMES = Map.ofEntries(")
		for i, e := range kept {
			comma := ","
			if i == len(kept)-1 {
				comma = ""
			}
			p("        Map.entry(", e.n, ", ", quoteJava(e.s), ")", comma)
		}
		p("    );")
	}
	p()
	p("    @Override public String                fullName() { return ", quoteJava(fullName), "; }")
	p("    @Override public Map<String, Integer>  values()   { return VALUES; }")
	p("    @Override public Map<Integer, String>  names()    { return NAMES; }")
	p("}")
	return b.String()
}

// qualifiedProtoEnumName mirrors qualifiedProtoName for enum types.
func qualifiedProtoEnumName(file *descriptorpb.FileDescriptorProto, en *flatEnum) string {
	pkg := file.GetPackage()
	if pkg == "" {
		return en.fullName
	}
	return pkg + "." + en.fullName
}

// lineWriter returns a closure that appends arguments concatenated to the
// given builder, followed by a newline. Keeps the emit code readable by
// hiding the WriteString boilerplate.
func lineWriter(b *strings.Builder) func(args ...any) {
	return func(args ...any) {
		for _, a := range args {
			fmt.Fprint(b, a)
		}
		b.WriteByte('\n')
	}
}

// emitOneofs walks the message's user-declared oneofs (skipping the
// synthetic ones proto3 generates around `optional` fields) and emits a
// Map<Integer, String> of field number → oneof name. The runtime uses this
// to know which sibling fields to clear when a oneof slot is set.
//
// A proto3 synthetic oneof has exactly one field whose proto3_optional flag
// is true; that field's presence is real but the surrounding oneof grouping
// is an implementation artifact, not a user-visible mutual-exclusion choice.
func emitOneofs(p func(args ...any), msg *descriptorpb.DescriptorProto) {
	type kv struct {
		num   int32
		oneof string
	}
	var entries []kv
	for _, f := range msg.Field {
		if f.OneofIndex == nil {
			continue
		}
		idx := int(*f.OneofIndex)
		if idx < 0 || idx >= len(msg.OneofDecl) {
			continue
		}
		if isSyntheticOneof(msg, idx) {
			continue
		}
		entries = append(entries, kv{num: f.GetNumber(), oneof: msg.OneofDecl[idx].GetName()})
	}
	p("    /**")
	p("     * Field number → declaring oneof name, for fields inside a user-declared")
	p("     * {@code oneof} block. Synthetic oneofs around proto3 {@code optional}")
	p("     * fields are excluded.")
	p("     */")
	if len(entries) == 0 {
		p("    public static final Map<Integer, String> ONEOF_OF = Map.of();")
		return
	}
	p("    public static final Map<Integer, String> ONEOF_OF = Map.ofEntries(")
	for i, e := range entries {
		comma := ","
		if i == len(entries)-1 {
			comma = ""
		}
		p("        Map.entry(", e.num, ", ", quoteJava(e.oneof), ")", comma)
	}
	p("    );")
}

// isSyntheticOneof reports whether the oneof at index idx is the wrapper
// proto3 generates around a single `optional` field. The convention: it
// holds exactly one field whose proto3_optional flag is true.
func isSyntheticOneof(msg *descriptorpb.DescriptorProto, idx int) bool {
	var members []*descriptorpb.FieldDescriptorProto
	for _, f := range msg.Field {
		if f.OneofIndex != nil && int(*f.OneofIndex) == idx {
			members = append(members, f)
		}
	}
	return len(members) == 1 && members[0].GetProto3Optional()
}

// emitSbeFile generates a per-file SBE companion class when the file has at
// least one of (sbe.schema_id) or (sbe.version) set. The class lives in the
// file's java_package and is named <Pascal>SbeFileMeta where <Pascal> is the
// filename basename in PascalCase (matching the protoc-gen-java
// java_outer_classname default). Returns ("", "", false) when the file has
// neither option set.
func emitSbeFile(file *descriptorpb.FileDescriptorProto) (filename, content string, ok bool) {
	schemaID, hasSchema := extUint32(file.GetOptions(), extSbeSchemaID)
	version, hasVersion := extUint32(file.GetOptions(), extSbeVersion)
	if !hasSchema && !hasVersion {
		return "", "", false
	}
	stem := javaOuterClass(file)
	cls := stem + "SbeFileMeta"

	var b strings.Builder
	p := lineWriter(&b)
	p("// Generated by protoc-gen-pxf-java-meta. DO NOT EDIT.")
	p("// source: ", file.GetName())
	p()
	p("package ", file.GetOptions().GetJavaPackage(), ";")
	p()
	p("/**")
	p(" * SBE file-level metadata for {@code ", file.GetName(), "}.")
	p(" *")
	p(" * <p>The runtime stamps these onto outbound SBE wire frames so the receiver")
	p(" * can verify it's looking at the expected schema and version.")
	p(" */")
	p("public final class ", cls, " {")
	p("    private ", cls, "() {}")
	p()
	p("    /** {@code (sbe.schema_id)} value, or -1 if unset. */")
	if hasSchema {
		p("    public static final int SCHEMA_ID = ", schemaID, ";")
	} else {
		p("    public static final int SCHEMA_ID = -1;")
	}
	p()
	p("    /** {@code (sbe.version)} value, or -1 if unset. */")
	if hasVersion {
		p("    public static final int VERSION = ", version, ";")
	} else {
		p("    public static final int VERSION = -1;")
	}
	p("}")
	return cls + ".java", b.String(), true
}

// emitCodec generates the typed convenience wrapper <Message>PxfCodec.java.
// Composes both directions of the lite pipeline:
//
//   - unmarshal(text)  := Parser → LiteWireWriter → MessageLite.parseFrom
//   - marshal(msg)     := MessageLite.toByteArray → LiteWireReader → Format
//
// Only emitted when the `lite` plugin parameter is set, since both halves
// pull in compile-time deps on :pxf-android that full-runtime consumers
// don't need.
func emitCodec(
	file *descriptorpb.FileDescriptorProto,
	byName map[string]*descriptorpb.FileDescriptorProto,
	msg *flatMsg,
) string {
	cls := msg.classStem + "PxfCodec"
	javaPkg := file.GetOptions().GetJavaPackage()
	// Fully-qualified Java class name of the user's typed message — what we
	// call .parseFrom() on. e.g. "com.example.sample.Person.Address".
	typedFqn := javaTypeName(byName, "."+qualifiedProtoName(file, msg))

	var b strings.Builder
	p := lineWriter(&b)
	p("// Generated by protoc-gen-pxf-java-meta (lite mode). DO NOT EDIT.")
	p("// source: ", file.GetName())
	p()
	p("package ", javaPkg, ";")
	p()
	p("import com.google.protobuf.InvalidProtocolBufferException;")
	p()
	p("import org.protowire.pxf.Parser;")
	p("import org.protowire.pxf.Position;")
	p("import org.protowire.pxf.PxfException;")
	p("import org.protowire.pxf.android.LitePxf;")
	p("import org.protowire.pxf.android.LiteWireWriter;")
	p()
	p("import java.nio.charset.StandardCharsets;")
	p()
	p("/**")
	p(" * Typed convenience wrappers around the lite-runtime PXF pipeline for")
	p(" * {@code ", qualifiedProtoName(file, msg), "}.")
	p(" *")
	p(" * <ul>")
	p(" *   <li>{@link #unmarshal(String)}: PXF text → typed {@code ", typedFqn, "}.")
	p(" *   <li>{@link #marshal(", typedFqn, ")}: typed message → PXF text.")
	p(" * </ul>")
	p(" *")
	p(" * <p>Failures from any stage surface as {@link PxfException}.")
	p(" */")
	p("public final class ", cls, " {")
	p("    private ", cls, "() {}")
	p()
	p("    /** Parse PXF text into a {@code ", typedFqn, "}. */")
	p("    public static ", typedFqn, " unmarshal(String text) {")
	p("        return unmarshal(text.getBytes(StandardCharsets.UTF_8));")
	p("    }")
	p()
	p("    /** Parse PXF UTF-8 bytes into a {@code ", typedFqn, "}. */")
	p("    public static ", typedFqn, " unmarshal(byte[] textBytes) {")
	p("        byte[] wire = LiteWireWriter.encode(Parser.parse(textBytes), ", msg.classStem, "PxfMeta.INSTANCE);")
	p("        try {")
	p("            return ", typedFqn, ".parseFrom(wire);")
	p("        } catch (InvalidProtocolBufferException e) {")
	p("            throw new PxfException(Position.UNKNOWN,")
	p("                \"decoding wire bytes for ", qualifiedProtoName(file, msg), " failed: \" + e.getMessage(), e);")
	p("        }")
	p("    }")
	p()
	p("    /** Serialize a typed {@code ", typedFqn, "} back to PXF text. */")
	p("    public static String marshal(", typedFqn, " msg) {")
	p("        return LitePxf.marshal(msg, ", msg.classStem, "PxfMeta.INSTANCE);")
	p("    }")
	p("}")
	return b.String()
}

// quoteJava wraps s in Java string literal syntax, escaping the few characters
// that can show up in a proto identifier (none, in valid input — proto field
// and enum names match [A-Za-z_][A-Za-z0-9_]*) plus a defensive backslash and
// quote escape, plus dotted Java class names that may pass through here.
func quoteJava(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
