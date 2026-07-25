// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protocompile/fdp"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// The lowered schema image (#164) is one of the compiler's two data
// inputs. Schema anchors resolve against the names it defines;
// descriptor-path anchors resolve against the §8.3.1 source map it
// embeds.
//
// The image is walked as FileDescriptorProtos rather than hydrated
// through protodesc: an image is a self-contained interchange artifact
// that may legitimately omit files it does not need at runtime, and a
// documentation build must not fail because a transitive import is
// absent from an image whose names it can read perfectly well.

// Image is the resolved view of a lowered FileDescriptorSet.
type Image struct {
	Path      string
	Digest    string
	FileCount int

	// Every addressable schema element, by fully-qualified name:
	// messages, fields, oneofs, enums, enum values (parent-scoped, per
	// protobuf name resolution), services, methods, plus the v1.2 `type`
	// aliases preserved in the FileTypeDecls carrier (§8.2).
	fqns map[string]elementKind

	// Canonical §8.3.1 descriptor paths, from the embedded source maps.
	paths map[string]bool

	// Annotation use sites by element FQN, for diagnostics that can say
	// what the element actually carries instead of only that the path
	// missed.
	annotations map[string][]string
}

type elementKind string

const (
	kindMessage   elementKind = "message"
	kindField     elementKind = "field"
	kindOneof     elementKind = "oneof"
	kindEnum      elementKind = "enum"
	kindEnumValue elementKind = "enum value"
	kindService   elementKind = "service"
	kindMethod    elementKind = "method"
	kindTypeAlias elementKind = "type alias"
)

// loadImage reads a lowered image and indexes everything anchors can
// point at.
func loadImage(path string) (*Image, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &fds); err != nil {
		return nil, fmt.Errorf("%s: not a FileDescriptorSet image: %w", path, err)
	}
	im := &Image{
		Path:        path,
		Digest:      digestOf(raw),
		FileCount:   len(fds.GetFile()),
		fqns:        map[string]elementKind{},
		paths:       map[string]bool{},
		annotations: map[string][]string{},
	}
	for _, fd := range fds.GetFile() {
		im.indexFile(fd)
	}
	return im, nil
}

func (im *Image) indexFile(fd *descriptorpb.FileDescriptorProto) {
	prefix := fd.GetPackage()
	for _, m := range fd.GetMessageType() {
		im.indexMessage(prefix, m)
	}
	for _, e := range fd.GetEnumType() {
		im.indexEnum(prefix, e)
	}
	for _, s := range fd.GetService() {
		name := qualify(prefix, s.GetName())
		im.fqns[name] = kindService
		for _, mth := range s.GetMethod() {
			im.fqns[qualify(name, mth.GetName())] = kindMethod
		}
	}
	im.indexFileCarriers(fd)
}

func (im *Image) indexMessage(prefix string, m *descriptorpb.DescriptorProto) {
	name := qualify(prefix, m.GetName())
	im.fqns[name] = kindMessage
	for _, f := range m.GetField() {
		im.fqns[qualify(name, f.GetName())] = kindField
	}
	for _, o := range m.GetOneofDecl() {
		im.fqns[qualify(name, o.GetName())] = kindOneof
	}
	for _, n := range m.GetNestedType() {
		im.indexMessage(name, n)
	}
	for _, e := range m.GetEnumType() {
		im.indexEnum(name, e)
	}
}

// indexEnum records the enum and its values. Enum values are scoped to
// the enum's *parent*, not to the enum — "pkg.OK", never "pkg.Status.OK"
// — which is both protobuf's name-resolution rule and the spelling
// fdp.DescriptorPath expects.
func (im *Image) indexEnum(prefix string, e *descriptorpb.EnumDescriptorProto) {
	name := qualify(prefix, e.GetName())
	im.fqns[name] = kindEnum
	for _, v := range e.GetValue() {
		im.fqns[qualify(prefix, v.GetName())] = kindEnumValue
	}
}

func qualify(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// indexFileCarriers reads the file-scope schema-extension carriers: the
// `type` declarations that make aliases resolvable by name (50403) and
// the source map whose entries are the only authority on which
// descriptor paths exist (50404).
func (im *Image) indexFileCarriers(fd *descriptorpb.FileDescriptorProto) {
	opts := fd.GetOptions()
	if opts == nil {
		return
	}
	xt, err := carriers()
	if err != nil {
		return
	}
	decoded, err := xt.decodeFileOptions(opts)
	if err != nil {
		return
	}

	if decls := decoded.typeDecls; decls.valid() {
		for _, td := range decls.msgs("declarations") {
			if n := td.str("name"); n != "" {
				im.fqns[n] = kindTypeAlias
			}
		}
	}
	if sm := decoded.sourceMap; sm.valid() {
		for _, entry := range sm.msgs("entries") {
			raw := entry.str("descriptor_path")
			if raw == "" {
				continue
			}
			// Parse and re-render through the shared §8.3.1 helper so the
			// index holds canonical spellings only: anchors are compared
			// against it, and hand-normalized strings would compare
			// unequal for cosmetic reasons.
			p, err := fdp.ParseDescriptorPath(raw)
			if err != nil {
				continue
			}
			im.paths[p.String()] = true
			if p.Annotation != "" {
				im.annotations[p.Element] = appendUnique(im.annotations[p.Element], p.Annotation)
			}
		}
	}
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// lookup reports an element's kind, and whether it exists at all.
func (im *Image) lookup(fqn string) (elementKind, bool) {
	k, ok := im.fqns[fqn]
	return k, ok
}

// hasPath reports whether the image's source maps carry this canonical
// descriptor path.
func (im *Image) hasPath(p string) bool { return im.paths[p] }

// annotationsOn lists the annotation FQNs the image records for an
// element, for diagnostics.
func (im *Image) annotationsOn(fqn string) string {
	list := im.annotations[fqn]
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

// ── Carrier extension types ───────────────────────────────────────────────

// carrierTypes holds the dynamic extension types for the file-scope
// schema-extension carriers. They are built from the bundled
// schema/v1/descriptor.proto: this repository ships no generated Go for
// its own schemas, so the extensions are resolved the same dynamic way
// every other typed artifact in `pxf` is.
type carrierTypes struct {
	types     *protoregistry.Types
	typeDecls protoreflect.ExtensionType
	sourceMap protoreflect.ExtensionType
}

type decodedFileOptions struct {
	typeDecls dmsg
	sourceMap dmsg
}

var (
	carrierOnce sync.Once
	carrierVal  *carrierTypes
	carrierErr  error
)

func carriers() (*carrierTypes, error) {
	carrierOnce.Do(func() {
		files, err := schemaresolve.CompileBundledFiles("schema/v1/descriptor.proto")
		if err != nil {
			carrierErr = err
			return
		}
		ct := &carrierTypes{types: new(protoregistry.Types)}
		for _, fd := range files {
			xs := fd.Extensions()
			for i := 0; i < xs.Len(); i++ {
				xt := dynamicpb.NewExtensionType(xs.Get(i))
				if err := ct.types.RegisterExtension(xt); err != nil {
					carrierErr = err
					return
				}
				switch xs.Get(i).FullName() {
				case "protowire.schema.v1.type_decls":
					ct.typeDecls = xt
				case "protowire.schema.v1.source_map":
					ct.sourceMap = xt
				}
			}
		}
		if ct.typeDecls == nil || ct.sourceMap == nil {
			carrierErr = fmt.Errorf("bundled schema/v1/descriptor.proto is missing the file-scope carriers")
			return
		}
		carrierVal = ct
	})
	return carrierVal, carrierErr
}

// decodeFileOptions re-parses FileOptions with the carrier extensions
// registered. The options arrive from a plain descriptorpb unmarshal, so
// the carriers sit in unknown fields; round-tripping the bytes through a
// resolver-aware unmarshal is what turns them back into messages.
func (ct *carrierTypes) decodeFileOptions(opts *descriptorpb.FileOptions) (decodedFileOptions, error) {
	raw, err := proto.Marshal(opts)
	if err != nil {
		return decodedFileOptions{}, err
	}
	reparsed := &descriptorpb.FileOptions{}
	if err := (proto.UnmarshalOptions{Resolver: ct.types}).Unmarshal(raw, reparsed); err != nil {
		return decodedFileOptions{}, err
	}
	var out decodedFileOptions
	if proto.HasExtension(reparsed, ct.typeDecls) {
		if m, ok := proto.GetExtension(reparsed, ct.typeDecls).(protoreflect.ProtoMessage); ok {
			out.typeDecls = wrap(m.ProtoReflect())
		}
	}
	if proto.HasExtension(reparsed, ct.sourceMap) {
		if m, ok := proto.GetExtension(reparsed, ct.sourceMap).(protoreflect.ProtoMessage); ok {
			out.sourceMap = wrap(m.ProtoReflect())
		}
	}
	return out, nil
}
