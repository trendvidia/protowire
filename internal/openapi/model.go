// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func protoMapKey(s string) protoreflect.MapKey {
	return protoreflect.ValueOfString(s).MapKey()
}

// The intermediate model: the lowered image walked as
// FileDescriptorProtos (an image may legitimately omit transitive
// imports, so nothing here hydrates through protodesc) with every §8.1
// carrier decoded once up front.

type model struct {
	files []*descriptorpb.FileDescriptorProto

	messages map[string]*messageInfo // FQN → message
	enums    map[string]*enumInfo    // FQN → enum
	aliases  map[string]*aliasInfo   // FQN → §8.2 TypeDecl
	services []*serviceInfo          // declaration order across files

	// functions indexes FunctionDecl carriers by FQN, for error-code
	// attribution of expression rules.
	functions map[string]*functionInfo

	// chains maps "<messageFQN>.<fieldName>" to the field's type-alias
	// chain (base-most first, most-derived last), from the §8.3
	// source-map TYPE_REFINEMENT entries. The field's lowered
	// AnnotationList carries the chain's expanded validate rules FIRST,
	// in chain order, before any field-level rules — attribution used
	// to avoid emitting one rule on both the alias $ref and the field.
	chains map[string][]string
}

type messageInfo struct {
	fqn     string
	file    string
	desc    *descriptorpb.DescriptorProto
	anns    []dmsg // message_annotations entries
	fields  []*fieldInfo
	mapKV   [2]*descriptorpb.FieldDescriptorProto // set when this is a map entry
	isEntry bool
}

type fieldInfo struct {
	desc *descriptorpb.FieldDescriptorProto
	anns []dmsg // field_annotations entries
}

type enumInfo struct {
	fqn  string
	file string
	desc *descriptorpb.EnumDescriptorProto
	anns []dmsg
}

type aliasInfo struct {
	fqn  string
	file string
	base string // TypeDecl.base_type_fqn: primitive name or FQN
	anns []dmsg // TypeDecl.annotations entries
}

type serviceInfo struct {
	fqn     string
	name    string
	file    string
	anns    []dmsg
	methods []*methodInfo
}

type methodInfo struct {
	name   string
	input  string // FQN without leading dot
	output string
	anns   []dmsg
}

type functionInfo struct {
	fqn       string
	errorCode string
}

// generatedFile reports whether a file is toolchain baggage rather than
// user schema surface: the bundled canonical libraries and the
// well-known types travel in the image for resolution but are not
// boundary elements.
func generatedFile(f *descriptorpb.FileDescriptorProto) bool {
	name := f.GetName()
	if strings.HasPrefix(name, "google/protobuf/") {
		return true
	}
	pkg := f.GetPackage()
	return pkg == "google.protobuf" || strings.HasPrefix(pkg, "protowire.") || pkg == "pxf" || pkg == "sbe"
}

// loadModel reads a lowered image from disk and decodes every carrier.
func loadModel(path string) (*model, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, fds); err != nil {
		return nil, fmt.Errorf("%s is not a FileDescriptorSet image: %w", path, err)
	}
	return buildModel(fds)
}

func buildModel(fds *descriptorpb.FileDescriptorSet) (*model, error) {
	ct, err := carriers()
	if err != nil {
		return nil, err
	}
	m := &model{
		files:     fds.GetFile(),
		messages:  make(map[string]*messageInfo),
		enums:     make(map[string]*enumInfo),
		aliases:   make(map[string]*aliasInfo),
		functions: make(map[string]*functionInfo),
		chains:    make(map[string][]string),
	}
	// Deterministic walk order regardless of image file order.
	files := append([]*descriptorpb.FileDescriptorProto(nil), fds.GetFile()...)
	sort.Slice(files, func(i, j int) bool { return files[i].GetName() < files[j].GetName() })

	for _, f := range files {
		pkg := f.GetPackage()
		for _, md := range f.GetMessageType() {
			if err := m.indexMessage(ct, f.GetName(), pkg, md); err != nil {
				return nil, err
			}
		}
		for _, ed := range f.GetEnumType() {
			if err := m.indexEnum(ct, f.GetName(), join(pkg, ed.GetName()), ed); err != nil {
				return nil, err
			}
		}
		for _, sd := range f.GetService() {
			if err := m.indexService(ct, f.GetName(), pkg, sd); err != nil {
				return nil, err
			}
		}
		if f.GetOptions() != nil {
			// §8.2 type aliases (50403).
			decls, err := ct.reparse(f.GetOptions(), "type_decls")
			if err != nil {
				return nil, err
			}
			for _, d := range decls.msgs("declarations") {
				a := &aliasInfo{
					fqn:  d.str("name"),
					file: f.GetName(),
					base: d.str("base_type_fqn"),
					anns: d.sub("annotations").msgs("entries"),
				}
				m.aliases[a.fqn] = a
			}
			// §8.2 function declarations (50401), for @error_code.
			fns, err := ct.reparse(f.GetOptions(), "functions")
			if err != nil {
				return nil, err
			}
			for _, d := range fns.msgs("declarations") {
				fn := &functionInfo{fqn: d.str("name")}
				// FunctionDecl.options is a map<string, AnnotationArg>;
				// error_code arrives under its unqualified key.
				if d.valid() {
					fd := d.m.Descriptor().Fields().ByName("options")
					if fd != nil {
						mp := d.m.Get(fd).Map()
						v := mp.Get(protoMapKey("error_code"))
						if v.IsValid() {
							fn.errorCode = wrap(v.Message()).str("string_value")
						}
					}
				}
				m.functions[fn.fqn] = fn
			}
			// §8.3 source map: field → alias chain.
			sm, err := ct.reparse(f.GetOptions(), "source_map")
			if err != nil {
				return nil, err
			}
			for _, e := range sm.msgs("entries") {
				if e.enumName("kind") != "TYPE_REFINEMENT" {
					continue
				}
				var chain []string
				for _, link := range e.msgs("type_chain") {
					chain = append(chain, link.str("type_fqn"))
				}
				if len(chain) > 0 {
					m.chains[e.str("descriptor_path")] = chain
				}
			}
		}
	}
	return m, nil
}

// chainOf returns the alias chain recorded for a field, if any.
func (m *model) chainOf(msgFQN, field string) []string {
	return m.chains[msgFQN+"."+field]
}

// chainRuleCount sums the validate rules the chain expands into a
// field's annotation list.
func (m *model) chainRuleCount(chain []string) int {
	n := 0
	for _, fqn := range chain {
		if a := m.aliases[fqn]; a != nil {
			n += len(allAnns(a.anns, annValidate))
		}
	}
	return n
}

// fieldOwnAnns filters a chained field's annotation list down to the
// entries the field wrote itself. Aliases are macros (§6.3): every
// alias annotation expands into the field's list, and the expanded
// copies keep the alias declaration's source location — an entry whose
// (name, location) matches one on a chain alias is alias-owned. The
// alias's own component already renders those; re-emitting them on the
// field would double every constraint.
func (m *model) fieldOwnAnns(anns []dmsg, chain []string) []dmsg {
	if len(chain) == 0 {
		return anns
	}
	type key struct {
		name       string
		file       string
		line, col  int64
		hasLocInfo bool
	}
	keyOf := func(e dmsg) key {
		loc := e.sub("location")
		return key{
			name:       e.str("name"),
			file:       loc.str("file"),
			line:       loc.i64("line"),
			col:        loc.i64("column"),
			hasLocInfo: loc.valid(),
		}
	}
	aliasOwned := map[key]bool{}
	locations := true
	for _, fqn := range chain {
		a := m.aliases[fqn]
		if a == nil {
			continue
		}
		for _, e := range a.anns {
			k := keyOf(e)
			if !k.hasLocInfo {
				locations = false
			}
			aliasOwned[k] = true
		}
	}
	if !locations {
		// No source locations to attribute by: fall back to skipping
		// the chain's leading validate rules and keep the rest.
		skip := m.chainRuleCount(chain)
		var out []dmsg
		for _, e := range anns {
			if e.str("name") == annValidate && skip > 0 {
				skip--
				continue
			}
			out = append(out, e)
		}
		return out
	}
	var out []dmsg
	for _, e := range anns {
		if aliasOwned[keyOf(e)] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (m *model) indexMessage(ct *carrierTypes, file, scope string, md *descriptorpb.DescriptorProto) error {
	fqn := join(scope, md.GetName())
	info := &messageInfo{fqn: fqn, file: file, desc: md, isEntry: md.GetOptions().GetMapEntry()}
	if md.GetOptions() != nil {
		anns, err := ct.annList(md.GetOptions(), "message_annotations")
		if err != nil {
			return err
		}
		info.anns = anns
	}
	for _, fd := range md.GetField() {
		fi := &fieldInfo{desc: fd}
		if fd.GetOptions() != nil {
			anns, err := ct.fieldAnnotations(fd)
			if err != nil {
				return err
			}
			fi.anns = anns
		}
		info.fields = append(info.fields, fi)
	}
	if info.isEntry && len(md.GetField()) == 2 {
		info.mapKV = [2]*descriptorpb.FieldDescriptorProto{md.GetField()[0], md.GetField()[1]}
	}
	m.messages[fqn] = info
	for _, nested := range md.GetNestedType() {
		if err := m.indexMessage(ct, file, fqn, nested); err != nil {
			return err
		}
	}
	for _, ed := range md.GetEnumType() {
		if err := m.indexEnum(ct, file, join(fqn, ed.GetName()), ed); err != nil {
			return err
		}
	}
	return nil
}

func (m *model) indexEnum(ct *carrierTypes, file, fqn string, ed *descriptorpb.EnumDescriptorProto) error {
	info := &enumInfo{fqn: fqn, file: file, desc: ed}
	if ed.GetOptions() != nil {
		anns, err := ct.annList(ed.GetOptions(), "enum_annotations")
		if err != nil {
			return err
		}
		info.anns = anns
	}
	m.enums[fqn] = info
	return nil
}

func (m *model) indexService(ct *carrierTypes, file, pkg string, sd *descriptorpb.ServiceDescriptorProto) error {
	info := &serviceInfo{fqn: join(pkg, sd.GetName()), name: sd.GetName(), file: file}
	if sd.GetOptions() != nil {
		anns, err := ct.annList(sd.GetOptions(), "service_annotations")
		if err != nil {
			return err
		}
		info.anns = anns
	}
	for _, md := range sd.GetMethod() {
		mi := &methodInfo{
			name:   md.GetName(),
			input:  strings.TrimPrefix(md.GetInputType(), "."),
			output: strings.TrimPrefix(md.GetOutputType(), "."),
		}
		if md.GetOptions() != nil {
			anns, err := ct.annList(md.GetOptions(), "method_annotations")
			if err != nil {
				return err
			}
			mi.anns = anns
		}
		info.methods = append(info.methods, mi)
	}
	m.services = append(m.services, info)
	return nil
}

func join(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "." + name
}

// description returns the @description text from an entry list, or "".
func description(entries []dmsg) string {
	return argStr(findAnn(entries, annDescription), "text", 0)
}

// firstSentence trims a description to its first sentence, the §5.2
// fallback for an empty @http summary.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
