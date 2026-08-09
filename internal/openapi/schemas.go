// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

// The schema half of the boundary (§#080): messages, enums and §8.2
// type aliases become components/schemas entries keyed by FQN. Property
// names are the proto field names as written — the same names the §5.2
// binding rules speak in.

// schemaBuilder accumulates the components/schemas map.
type schemaBuilder struct {
	m *model
	// include gates emission by audience; nil admits everything.
	include func(fqn string) bool
	// out collects components keyed by FQN.
	out map[string]*omap
	// sensitiveMsgs caches transitive message sensitivity.
	sensitiveMsgs map[string]bool
}

func newSchemaBuilder(m *model, include func(string) bool) *schemaBuilder {
	return &schemaBuilder{
		m:             m,
		include:       include,
		out:           make(map[string]*omap),
		sensitiveMsgs: make(map[string]bool),
	}
}

func (b *schemaBuilder) included(fqn string) bool {
	return b.include == nil || b.include(fqn)
}

// buildAll emits every user-schema message, enum and alias that passes
// the audience filter.
func (b *schemaBuilder) buildAll() error {
	byFile := make(map[string]bool)
	for _, f := range b.m.files {
		byFile[f.GetName()] = generatedFile(f)
	}
	var fqns []string
	for fqn, mi := range b.m.messages {
		if !byFile[mi.file] && !mi.isEntry {
			fqns = append(fqns, fqn)
		}
	}
	for fqn, ei := range b.m.enums {
		if !byFile[ei.file] {
			fqns = append(fqns, fqn)
		}
	}
	for fqn, ai := range b.m.aliases {
		if !byFile[ai.file] {
			fqns = append(fqns, fqn)
		}
	}
	sort.Strings(fqns)
	for _, fqn := range fqns {
		if !b.included(fqn) {
			continue
		}
		if err := b.component(fqn); err != nil {
			return err
		}
	}
	return nil
}

// component emits one named schema (message, enum or alias) if not done.
func (b *schemaBuilder) component(fqn string) error {
	if _, done := b.out[fqn]; done {
		return nil
	}
	// Reserve the slot before recursing so reference cycles terminate.
	b.out[fqn] = nil
	var s *omap
	var err error
	switch {
	case b.m.aliases[fqn] != nil:
		s, err = b.aliasSchema(b.m.aliases[fqn])
	case b.m.messages[fqn] != nil:
		s, err = b.messageSchema(b.m.messages[fqn])
	case b.m.enums[fqn] != nil:
		s = b.enumSchema(b.m.enums[fqn])
	default:
		err = fmt.Errorf("schema closure references unknown type %q", fqn)
	}
	if err != nil {
		delete(b.out, fqn)
		return err
	}
	b.out[fqn] = s
	return nil
}

func ref(fqn string) *omap {
	return newOmap().set("$ref", "#/components/schemas/"+fqn)
}

// ── Enums ─────────────────────────────────────────────────────────────────

func (b *schemaBuilder) enumSchema(ei *enumInfo) *omap {
	s := newOmap()
	if d := description(ei.anns); d != "" {
		s.set("description", d)
	}
	s.set("type", "string")
	names := make([]any, 0, len(ei.desc.GetValue()))
	for _, v := range ei.desc.GetValue() {
		names = append(names, v.GetName())
	}
	s.set("enum", names)
	b.commonAnnotations(s, ei.anns, false)
	return s
}

// ── Aliases (§8.2) ────────────────────────────────────────────────────────

// aliasSchema renders one TypeDecl: the base schema (a $ref for named
// bases, an inline scalar otherwise) narrowed by the alias's own rules —
// composition is pure AND (§6.3), so a chained alias is allOf over its
// immediate base.
func (b *schemaBuilder) aliasSchema(ai *aliasInfo) (*omap, error) {
	own := newOmap()
	if d := description(ai.anns); d != "" {
		own.set("description", d)
	}

	c := &constraints{}
	sk, numeric := b.sizeKindOf(ai.base)
	for _, v := range allAnns(ai.anns, annValidate) {
		c.mapValidate(v, sk, numeric)
	}

	base, named, err := b.baseSchema(ai.base)
	if err != nil {
		return nil, fmt.Errorf("type alias %s: %w", ai.fqn, err)
	}
	if !named {
		// Scalar/WKT base: fold the constraints into one flat schema.
		for _, k := range base.keys {
			own.set(k, base.vals[k])
		}
		c.apply(own)
		b.commonAnnotations(own, ai.anns, b.aliasSensitive(ai))
		return own, nil
	}
	// Named base: allOf composition preserves the base's identity.
	c.apply(own)
	b.commonAnnotations(own, ai.anns, b.aliasSensitive(ai))
	if own.len() == 0 {
		return base, nil
	}
	all := newOmap()
	all.set("allOf", []any{base, own})
	return all, nil
}

// baseSchema resolves an alias base: a primitive name or wrapper/WKT
// yields an inline schema (named=false); a message, enum or alias FQN
// yields a $ref and forces the referenced component (named=true).
func (b *schemaBuilder) baseSchema(base string) (*omap, bool, error) {
	if s, ok := scalarBase(base); ok {
		return s, false, nil
	}
	if s, ok := wktSchema(base); ok {
		return s, false, nil
	}
	if b.m.aliases[base] != nil || b.m.messages[base] != nil || b.m.enums[base] != nil {
		if err := b.component(base); err != nil {
			return nil, false, err
		}
		return ref(base), true, nil
	}
	return nil, false, fmt.Errorf("unresolved base type %q", base)
}

// aliasSensitive reports whether the alias itself carries @sensitive.
func (b *schemaBuilder) aliasSensitive(ai *aliasInfo) bool {
	return findAnn(ai.anns, annSensitive).valid()
}

// sizeKindOf classifies an alias base for the rule mapper by walking
// the chain to its concrete type.
func (b *schemaBuilder) sizeKindOf(base string) (sizeKind, bool) {
	seen := map[string]bool{}
	for {
		if seen[base] {
			return sizeNone, false
		}
		seen[base] = true
		if a := b.m.aliases[base]; a != nil {
			base = a.base
			continue
		}
		if w, ok := wrapperScalar(base); ok {
			base = w
			continue
		}
		switch base {
		case "string", "bytes":
			return sizeLength, false
		case "int32", "int64", "uint32", "uint64", "sint32", "sint64",
			"fixed32", "fixed64", "sfixed32", "sfixed64", "float", "double":
			return sizeNone, true
		}
		return sizeNone, false
	}
}

// ── Messages ──────────────────────────────────────────────────────────────

func (b *schemaBuilder) messageSchema(mi *messageInfo) (*omap, error) {
	s := newOmap()
	if d := description(mi.anns); d != "" {
		s.set("description", d)
	}
	s.set("type", "object")

	msgSensitive := b.messageSensitive(mi.fqn)

	props := newOmap()
	var required []any
	for _, fi := range mi.fields {
		name := fi.desc.GetName()
		ps, err := b.propertySchema(mi, fi, msgSensitive)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", mi.fqn, name, err)
		}
		props.set(name, ps)
		if findAnn(fi.anns, annRequired).valid() {
			required = append(required, name)
		}
	}
	if props.len() > 0 {
		s.set("properties", props)
	}
	if len(required) > 0 {
		s.set("required", required)
	}

	// Message-level rules bind `this` to the whole message: never
	// mappable to a property keyword, always carried through.
	c := &constraints{}
	for _, v := range allAnns(mi.anns, annValidate) {
		c.mapValidate(v, sizeNone, false)
	}
	c.apply(s)
	b.commonAnnotations(s, mi.anns, msgSensitive)
	return s, nil
}

// propertySchema renders one field.
func (b *schemaBuilder) propertySchema(mi *messageInfo, fi *fieldInfo, msgSensitive bool) (*omap, error) {
	f := fi.desc
	chain := b.m.chainOf(mi.fqn, f.GetName())
	sensitive := msgSensitive || b.fieldSensitive(fi, chain)

	// Aliases are macros: the field's list carries the chain's expanded
	// annotations too, but those live on the alias components — only
	// the field's own entries are mapped inline here.
	own := b.m.fieldOwnAnns(fi.anns, chain)

	var core *omap
	var err error
	sk := sizeLength
	numeric := false
	switch {
	case b.isMapField(f):
		core, err = b.mapFieldSchema(f)
		sk, numeric = sizeProps, false
	case f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
		var elem *omap
		elem, err = b.elementSchema(f, chain, sensitive)
		if err == nil {
			core = newOmap().set("type", "array").set("items", elem)
		}
		sk, numeric = sizeItems, false
	default:
		core, err = b.singularSchema(f, chain, sensitive)
		sk, numeric = b.fieldSizeKind(f, chain)
	}
	if err != nil {
		return nil, err
	}

	extra := b.fieldKeywords(own, sensitive, sk, numeric)

	// A $ref core with siblings needs allOf; a flat core absorbs them.
	if _, isRef := core.get("$ref"); isRef && extra.len() > 0 {
		merged := newOmap()
		merged.set("allOf", []any{core, extra})
		return b.nullable(merged, f, chain), nil
	}
	for _, k := range extra.keys {
		core.set(k, extra.vals[k])
	}
	return b.nullable(core, f, chain), nil
}

// fieldKeywords collects what a field contributes on its own — its
// description, its @validate rules (mapped, or carried verbatim under
// x-validation), and the @deprecated/§6.7 markers — apart from the
// schema of its type, which the caller composes them with.
func (b *schemaBuilder) fieldKeywords(own []dmsg, sensitive bool, sk sizeKind, numeric bool) *omap {
	extra := newOmap()
	if d := description(own); d != "" {
		extra.set("description", d)
	}
	c := &constraints{}
	for _, v := range allAnns(own, annValidate) {
		c.mapValidate(v, sk, numeric)
	}
	c.apply(extra)
	b.commonAnnotations(extra, own, sensitive)
	if sensitive {
		if _, has := extra.get("x-sensitive"); !has {
			extra.set("x-sensitive", true)
		}
	}
	return extra
}

// mergeFieldKeywords writes a field's own keywords onto a schema the
// caller built in place of propertySchema's. The §5.2 body binding
// inlines a container minus its bound leaf (issue #218), and an inlined
// container has no $ref left to compose with — without this its
// @description, its x-validation carry-through and its §6.7 markers
// would go missing along with the reference.
func (b *schemaBuilder) mergeFieldKeywords(s *omap, mi *messageInfo, fi *fieldInfo, msgSensitive bool) {
	chain := b.m.chainOf(mi.fqn, fi.desc.GetName())
	sensitive := msgSensitive || b.fieldSensitive(fi, chain)
	sk, numeric := b.fieldSizeKind(fi.desc, chain)
	extra := b.fieldKeywords(b.m.fieldOwnAnns(fi.anns, chain), sensitive, sk, numeric)
	for _, k := range extra.keys {
		s.set(k, extra.vals[k])
	}
}

// elementSchema renders the item schema of a repeated field. A chain on
// a repeated field aliases the ELEMENT type (§6.3): the element becomes
// a $ref to the most-derived alias.
func (b *schemaBuilder) elementSchema(f *descriptorpb.FieldDescriptorProto, chain []string, sensitive bool) (*omap, error) {
	if len(chain) > 0 {
		derived := chain[len(chain)-1]
		if err := b.component(derived); err != nil {
			return nil, err
		}
		return ref(derived), nil
	}
	return b.typeSchema(f, sensitive)
}

// singularSchema renders a non-repeated, non-map field's core schema.
func (b *schemaBuilder) singularSchema(f *descriptorpb.FieldDescriptorProto, chain []string, sensitive bool) (*omap, error) {
	if len(chain) > 0 {
		derived := chain[len(chain)-1]
		if err := b.component(derived); err != nil {
			return nil, err
		}
		return ref(derived), nil
	}
	return b.typeSchema(f, sensitive)
}

// typeSchema maps a field's descriptor type with no alias awareness.
func (b *schemaBuilder) typeSchema(f *descriptorpb.FieldDescriptorProto, sensitive bool) (*omap, error) {
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		fqn := strings.TrimPrefix(f.GetTypeName(), ".")
		if s, ok := wktSchema(fqn); ok {
			return s, nil
		}
		if err := b.component(fqn); err != nil {
			return nil, err
		}
		return ref(fqn), nil
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		fqn := strings.TrimPrefix(f.GetTypeName(), ".")
		if err := b.component(fqn); err != nil {
			return nil, err
		}
		return ref(fqn), nil
	case descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return nil, fmt.Errorf("proto2 groups are not supported")
	default:
		s, ok := scalarBase(scalarName(f.GetType()))
		if !ok {
			return nil, fmt.Errorf("unmappable field type %v", f.GetType())
		}
		return s, nil
	}
}

func (b *schemaBuilder) isMapField(f *descriptorpb.FieldDescriptorProto) bool {
	if f.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return false
	}
	mi := b.m.messages[strings.TrimPrefix(f.GetTypeName(), ".")]
	return mi != nil && mi.isEntry
}

func (b *schemaBuilder) mapFieldSchema(f *descriptorpb.FieldDescriptorProto) (*omap, error) {
	entry := b.m.messages[strings.TrimPrefix(f.GetTypeName(), ".")]
	val, err := b.typeSchema(entry.mapKV[1], false)
	if err != nil {
		return nil, err
	}
	return newOmap().set("type", "object").set("additionalProperties", val), nil
}

// fieldSizeKind classifies a singular field for the rule mapper.
func (b *schemaBuilder) fieldSizeKind(f *descriptorpb.FieldDescriptorProto, chain []string) (sizeKind, bool) {
	if len(chain) > 0 {
		return b.sizeKindOf(chain[len(chain)-1])
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING,
		descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return sizeLength, false
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		if w, ok := wrapperScalar(strings.TrimPrefix(f.GetTypeName(), ".")); ok {
			return b.sizeKindOf(w)
		}
		return sizeNone, false
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM,
		descriptorpb.FieldDescriptorProto_TYPE_BOOL,
		descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return sizeNone, false
	default:
		return sizeNone, true
	}
}

// nullable wraps a schema when the field opted into explicit null
// (§6.1): proto3 `optional` or a wrapper type.
func (b *schemaBuilder) nullable(s *omap, f *descriptorpb.FieldDescriptorProto, chain []string) *omap {
	isWrapper := false
	if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE && len(chain) == 0 {
		_, isWrapper = wrapperScalar(strings.TrimPrefix(f.GetTypeName(), "."))
	}
	if !f.GetProto3Optional() && !isWrapper {
		return s
	}
	if t, ok := s.get("type"); ok {
		if ts, isStr := t.(string); isStr && ts != "object" {
			s.set("type", []any{ts, "null"})
			return s
		}
	}
	if _, isRef := s.get("$ref"); isRef {
		return newOmap().set("anyOf", []any{s, newOmap().set("type", "null")})
	}
	return s
}

// ── Shared annotation → keyword mapping ──────────────────────────────────

// commonAnnotations maps @deprecated, @example, @default and @sensitive
// on a declaration; §6.7 doc-emit minima suppress values and examples
// on sensitive declarations.
func (b *schemaBuilder) commonAnnotations(s *omap, anns []dmsg, sensitive bool) {
	if dep := findAnn(anns, annDeprecated); dep.valid() {
		s.set("deprecated", true)
		if reason := argStr(dep, "reason", 0); reason != "" {
			s.set("x-deprecated-reason", reason)
		}
	}
	if sens := findAnn(anns, annSensitive); sens.valid() || sensitive {
		s.set("x-sensitive", true)
		if class := argStr(sens, "class", 0); class != "" {
			s.set("x-sensitive-class", class)
		}
		return // §6.7: no values or examples for sensitive declarations.
	}
	if def := findAnn(anns, annDefault); def.valid() {
		if v, ok := argScalar(arg(def, "value", 0)); ok {
			s.set("default", v)
		}
	}
	if ex := findAnn(anns, annExample); ex.valid() {
		if v, ok := argScalar(arg(ex, "value", 0)); ok {
			s.set("example", v)
		}
	}
}

// fieldSensitive reports §6.7 sensitivity from the field's own list or
// its alias chain or its (transitively sensitive) message type.
func (b *schemaBuilder) fieldSensitive(fi *fieldInfo, chain []string) bool {
	if findAnn(fi.anns, annSensitive).valid() {
		return true
	}
	for _, fqn := range chain {
		if a := b.m.aliases[fqn]; a != nil && findAnn(a.anns, annSensitive).valid() {
			return true
		}
	}
	if fi.desc.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return b.messageSensitive(strings.TrimPrefix(fi.desc.GetTypeName(), "."))
	}
	return false
}

// messageSensitive reports whether a message carries @sensitive.
func (b *schemaBuilder) messageSensitive(fqn string) bool {
	if v, ok := b.sensitiveMsgs[fqn]; ok {
		return v
	}
	mi := b.m.messages[fqn]
	v := mi != nil && findAnn(mi.anns, annSensitive).valid()
	b.sensitiveMsgs[fqn] = v
	return v
}

// ── Scalar and WKT tables ─────────────────────────────────────────────────

func scalarName(t descriptorpb.FieldDescriptorProto_Type) string {
	switch t {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "double"
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "float"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64:
		return "int64"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
		return "uint64"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32:
		return "int32"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return "fixed64"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "fixed32"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "bool"
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "bytes"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
		return "uint32"
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "sfixed32"
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "sfixed64"
	case descriptorpb.FieldDescriptorProto_TYPE_SINT32:
		return "sint32"
	case descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return "sint64"
	}
	return ""
}

// scalarBase maps a protobuf scalar name to its OpenAPI schema,
// following the proto3 JSON mapping (64-bit integers as strings).
func scalarBase(name string) (*omap, bool) {
	s := newOmap()
	switch name {
	case "double", "float":
		s.set("type", "number").set("format", name)
	case "int32", "sint32", "sfixed32":
		s.set("type", "integer").set("format", "int32")
	case "uint32", "fixed32":
		s.set("type", "integer").set("format", "uint32")
	case "int64", "sint64", "sfixed64":
		s.set("type", "string").set("format", "int64")
	case "uint64", "fixed64":
		s.set("type", "string").set("format", "uint64")
	case "bool":
		s.set("type", "boolean")
	case "string":
		s.set("type", "string")
	case "bytes":
		s.set("type", "string").set("format", "byte")
	default:
		return nil, false
	}
	return s, true
}

// wrapperScalars maps google.protobuf wrapper FQNs to their unwrapped
// scalar names.
var wrapperScalars = map[string]string{
	"google.protobuf.DoubleValue": "double",
	"google.protobuf.FloatValue":  "float",
	"google.protobuf.Int64Value":  "int64",
	"google.protobuf.UInt64Value": "uint64",
	"google.protobuf.Int32Value":  "int32",
	"google.protobuf.UInt32Value": "uint32",
	"google.protobuf.BoolValue":   "bool",
	"google.protobuf.StringValue": "string",
	"google.protobuf.BytesValue":  "bytes",
}

func wrapperScalar(fqn string) (string, bool) {
	s, ok := wrapperScalars[fqn]
	return s, ok
}

// wktSchema maps the well-known types with a canonical JSON form. A
// wrapper is its unwrapped scalar plus explicit null (§6.2 rule 1).
func wktSchema(fqn string) (*omap, bool) {
	if scalar, ok := wrapperScalar(fqn); ok {
		s, _ := scalarBase(scalar)
		if t, ok := s.get("type"); ok {
			s.set("type", []any{t, "null"})
		}
		return s, true
	}
	switch fqn {
	case "google.protobuf.Timestamp":
		return newOmap().set("type", "string").set("format", "date-time"), true
	case "google.protobuf.Duration":
		return newOmap().set("type", "string").set("format", "duration"), true
	case "google.protobuf.Any":
		props := newOmap().
			set("type_url", newOmap().set("type", "string")).
			set("value", newOmap().set("type", "string").set("format", "byte"))
		return newOmap().set("type", "object").set("properties", props), true
	case "google.protobuf.Empty":
		return newOmap().set("type", "object"), true
	case "google.protobuf.FieldMask":
		return newOmap().set("type", "string"), true
	}
	return nil, false
}
