// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package schemaresolve is the shared implementation behind the
// schema-resolution chain that the `pxf` CLI
// surface. cmd/pxf/QUERY.md documents the chain as:
//
//  1. Bundled canonical schemas      (`pxf/*`, `sbe/*`, `envelope/v1/*`)
//  2. In-document `@proto` directives (source / named / descriptor shapes)
//  3. -p schema.proto                 (user-supplied .proto sources)
//  4. -s server -n namespace --schema  (protoregistry gRPC fetch)
//
// Each item is independent and additive: callers compose the steps
// they need. The output is always a *Registry — a flat
// FullName → MessageDescriptor map — that the consumers query.
//
// Two CLIs share the same code paths so a bug fix in one place fixes
// both. The package has no main()-package dependencies — only the
// protobuf reflection + gRPC plumbing every protowire CLI needs
// anyway.
package schemaresolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/trendvidia/protocompile"
	_ "github.com/trendvidia/protocompile/experimentalcompile" // registers the fork's compile pipeline
	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
	"github.com/trendvidia/protowire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Registry is a flat map of every message reachable from a Resolve
// call, keyed by FullName. The zero value is usable but empty.
type Registry struct {
	byFullName map[protoreflect.FullName]protoreflect.MessageDescriptor
	// enums holds every enum merged, so that a document's enum can be
	// compared with an external one of the same name.
	enums map[protoreflect.FullName]protoreflect.EnumDescriptor
	// fromDocument holds the messages and enums a document's own @proto
	// defined, which shadow any external definition of the same name.
	fromDocument map[protoreflect.FullName]bool
	// differs holds the external types a document's type shadowed with
	// a different definition, and the types nested in them, until
	// warnReferrers reads it.
	differs  map[protoreflect.FullName]bool
	warnings []string
}

// NewRegistry returns an empty Registry. Useful when callers need to
// hand-merge descriptors that didn't come through the standard
// resolution chain (e.g. tests).
func NewRegistry() *Registry {
	r := &Registry{}
	r.init()
	return r
}

func (r *Registry) init() {
	if r.byFullName == nil {
		r.byFullName = map[protoreflect.FullName]protoreflect.MessageDescriptor{}
	}
	if r.enums == nil {
		r.enums = map[protoreflect.FullName]protoreflect.EnumDescriptor{}
	}
	if r.fromDocument == nil {
		r.fromDocument = map[protoreflect.FullName]bool{}
	}
}

// Find returns the descriptor for `name`, or nil if not registered.
// Lookup is by fully-qualified name (`trades.v1.Trade`), not by leaf.
func (r *Registry) Find(name string) protoreflect.MessageDescriptor {
	if r == nil {
		return nil
	}
	return r.byFullName[protoreflect.FullName(name)]
}

// Len reports the number of registered messages. Useful for tests
// and for the loose-mode signal (zero ⇒ nothing resolved).
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byFullName)
}

// FromDocument reports whether name was defined by a document's own
// @proto directive (a VirtualFiles or DescriptorSet source).
func (r *Registry) FromDocument(name string) bool {
	return r != nil && r.fromDocument[protoreflect.FullName(name)]
}

// Warnings returns what resolution noticed without failing: an
// in-document type whose definition differs from an external one it
// shadows, and an external message that still refers to the external
// definition (#371).
func (r *Registry) Warnings() []string {
	if r == nil {
		return nil
	}
	return r.warnings
}

// mergeDocument folds fd, a file a document's own @proto defined, into
// the registry. The embedded schema is authoritative (draft § The
// "@proto" Directive, "Precedence with external schemas"), so its
// messages and enums replace any external type of the same name, -p or
// protoregistry alike. Where the two definitions differ, it records a
// warning: the document still decodes against its own type, and a
// stale -p does not go unnoticed (decided by the owner on #371).
//
// Two definitions differ as sameDefinition says.
func (r *Registry) mergeDocument(fd protoreflect.FileDescriptor) {
	r.init()
	r.shadow(fd)
	walkTypes(fd, func(d protoreflect.Descriptor) {
		r.put(d)
		r.fromDocument[d.FullName()] = true
	})
}

// shadow compares each type c declares with the external type of the
// same name it is about to replace, and warns where the two differ. The
// external type's nested types go with it, since they are not the
// document's. A type with no external counterpart is descended into: a
// nested type can share its name with an external top-level one.
func (r *Registry) shadow(c typeContainer) {
	visit := func(d protoreflect.Descriptor) {
		old := r.external(d.FullName())
		if old == nil {
			if md, ok := d.(protoreflect.MessageDescriptor); ok {
				r.shadow(md)
			}
			return
		}
		r.drop(old)
		if !sameDefinition(old, d) {
			r.warnings = append(r.warnings, fmt.Sprintf(
				"@proto defines %s differently from %s; using the @proto definition",
				d.FullName(), old.ParentFile().Path()))
			if r.differs == nil {
				r.differs = map[protoreflect.FullName]bool{}
			}
			r.differs[old.FullName()] = true
			if md, ok := old.(protoreflect.MessageDescriptor); ok {
				walkTypes(md, func(n protoreflect.Descriptor) { r.differs[n.FullName()] = true })
			}
		}
	}
	for i := range c.Messages().Len() {
		visit(c.Messages().Get(i))
	}
	for i := range c.Enums().Len() {
		visit(c.Enums().Get(i))
	}
}

// external returns the external message or enum named name, or nil when
// there is none or a document defined it.
func (r *Registry) external(name protoreflect.FullName) protoreflect.Descriptor {
	if r.fromDocument[name] {
		return nil
	}
	if md, ok := r.byFullName[name]; ok {
		return md
	}
	if ed, ok := r.enums[name]; ok {
		return ed
	}
	return nil
}

// drop removes d and every type nested in it.
func (r *Registry) drop(d protoreflect.Descriptor) {
	remove := func(n protoreflect.Descriptor) {
		delete(r.byFullName, n.FullName())
		delete(r.enums, n.FullName())
	}
	remove(d)
	if md, ok := d.(protoreflect.MessageDescriptor); ok {
		walkTypes(md, remove)
	}
}

// warnReferrers warns about each external message with a field whose
// type a document redefined differently. A descriptor's field types are
// bound when its file is built, so that field still holds the external
// definition: shadowing replaces what a name looks up, not what other
// external types refer to.
func (r *Registry) warnReferrers() {
	if len(r.differs) == 0 {
		return
	}
	names := make([]protoreflect.FullName, 0, len(r.byFullName))
	for name := range r.byFullName {
		if !r.fromDocument[name] {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		md := r.byFullName[name]
		if md.IsMapEntry() {
			continue
		}
		warned := map[protoreflect.FullName]bool{}
		for i := range md.Fields().Len() {
			t := fieldType(md.Fields().Get(i))
			if t == nil || !r.differs[t.FullName()] || warned[t.FullName()] {
				continue
			}
			warned[t.FullName()] = true
			r.warnings = append(r.warnings, fmt.Sprintf(
				"%s from %s still uses %s's %s, not the @proto one",
				name, md.ParentFile().Path(), t.ParentFile().Path(), t.FullName()))
		}
	}
	r.differs = nil
}

// fieldType returns the message or enum f holds, a map's value type for
// a map field, or nil for a scalar.
func fieldType(f protoreflect.FieldDescriptor) protoreflect.Descriptor {
	if f.IsMap() {
		f = f.MapValue()
	}
	if f.Message() != nil {
		return f.Message()
	}
	if f.Enum() != nil {
		return f.Enum()
	}
	return nil
}

// sameDefinition reports whether a and b define the same message or
// enum: equal DescriptorProtos, so names, numbers, types, labels,
// options, oneofs and nested types (or values) agree. Field types
// compare by full name; a top-level type the field refers to is compared
// on its own. The file each lives in does not count, since the
// in-document one is always a synthetic file. Two things a compiler adds
// are normalized away, so that a schema compiled elsewhere (a
// protoregistry bundle) can match: a json_name equal to the default one,
// and the generated `_null` mask (NullMask).
func sameDefinition(a, b protoreflect.Descriptor) bool {
	switch a := a.(type) {
	case protoreflect.MessageDescriptor:
		b, ok := b.(protoreflect.MessageDescriptor)
		return ok && proto.Equal(messageDefinition(a), messageDefinition(b))
	case protoreflect.EnumDescriptor:
		b, ok := b.(protoreflect.EnumDescriptor)
		return ok && proto.Equal(protodesc.ToEnumDescriptorProto(a), protodesc.ToEnumDescriptorProto(b))
	}
	return false
}

// nullMaskNumber is the number protocompile gives the generated `_null`
// field (RFC-001 §8.6).
const nullMaskNumber = 536870911

func messageDefinition(md protoreflect.MessageDescriptor) *descriptorpb.DescriptorProto {
	m := protodesc.ToDescriptorProto(md)
	normalizeDefinition(md, m)
	return m
}

// normalizeDefinition rewrites m, md's DescriptorProto, so that every
// field states its json_name (protoc writes the default one in, a bundle
// may leave it out) and the generated `_null` mask is gone.
// ToDescriptorProto keeps md's order of fields and nested messages, so
// the two walk in step.
func normalizeDefinition(md protoreflect.MessageDescriptor, m *descriptorpb.DescriptorProto) {
	for i, f := range m.Field {
		f.JsonName = proto.String(md.Fields().Get(i).JSONName())
	}
	m.Field = slices.DeleteFunc(m.Field, func(f *descriptorpb.FieldDescriptorProto) bool {
		return f.GetName() == "_null" && f.GetNumber() == nullMaskNumber
	})
	for i, n := range m.NestedType {
		normalizeDefinition(md.Messages().Get(i), n)
	}
}

// Merge folds every message reachable from fd into the registry, and
// records its enums. Used by every source step (compile result,
// registry fetch, descriptor-form bodies).
func (r *Registry) Merge(fd protoreflect.FileDescriptor) {
	r.init()
	walkTypes(fd, r.put)
}

func (r *Registry) put(d protoreflect.Descriptor) {
	switch d := d.(type) {
	case protoreflect.MessageDescriptor:
		r.byFullName[d.FullName()] = d
	case protoreflect.EnumDescriptor:
		r.enums[d.FullName()] = d
	}
}

// typeContainer is a file or a message: what declares messages and
// enums.
type typeContainer interface {
	Messages() protoreflect.MessageDescriptors
	Enums() protoreflect.EnumDescriptors
}

// walkTypes calls f on every message and enum c declares, nested ones
// included.
func walkTypes(c typeContainer, f func(protoreflect.Descriptor)) {
	for i := range c.Enums().Len() {
		f(c.Enums().Get(i))
	}
	for i := range c.Messages().Len() {
		md := c.Messages().Get(i)
		f(md)
		walkTypes(md, f)
	}
}

// VirtualFile is an in-memory .proto served to protocompile under a
// synthetic filename. The synthesised in-doc `@proto` named/source
// shapes lower to VirtualFile entries.
type VirtualFile struct {
	Name string // synthetic path, e.g. "__indoc_0.proto"
	Body []byte // .proto source bytes
}

// CompileOptions carries everything CompileSources needs. UserFiles
// are read from disk; VirtualFiles served from memory; bundled
// canonical schemas (BundledFiles) come from the repo-root embed.FS.
type CompileOptions struct {
	UserFiles     []string      // -p flag values (filesystem paths)
	VirtualFiles  []VirtualFile // in-doc @proto synthesised sources
	BundledFiles  []string      // canonical schemas to compile (read from embed.FS)
	DescriptorSet [][]byte      // serialised FileDescriptorSet blobs to merge
}

// CompileBundledAll is the convenience list every CLI uses to include
// every canonical schema this repo ships. Callers that want to opt
// out of bundled types pass nil for CompileOptions.BundledFiles.
var CompileBundledAll = []string{
	"pxf/annotations.proto",
	"pxf/bignum.proto",
	"pxf/secret.proto",
	"sbe/annotations.proto",
	"envelope/v1/envelope.proto",
}

// InDocumentImports are the canonical schemas a source-form @proto may
// import, besides the well-known types and another @proto in the same
// document (draft § The "@proto" Directive, decided by the owner on
// #369). They are the schemas whose types a document can hold. The rest
// of the bundled tree configures tools, and a filesystem path is never
// one.
var InDocumentImports = []string{
	"pxf/annotations.proto",
	"pxf/bignum.proto",
	"pxf/secret.proto",
	"sbe/annotations.proto",
	"envelope/v1/envelope.proto",
}

// CompileSources compiles opts into reg in two passes.
//
// The external pass compiles BundledFiles and UserFiles. Its imports
// resolve from the bundled canonical schemas, then the filesystem, so a
// user .proto can `import "envelope/v1/envelope.proto"` with no -p
// flag for it.
//
// The document pass compiles VirtualFiles, the sources of a document's
// own @proto directives, and merges DescriptorSet. Its imports resolve
// only from the other VirtualFiles, InDocumentImports and the
// well-known types: a @proto never reads the filesystem or a -p file.
// Its messages shadow the external pass's (see mergeDocument).
func CompileSources(reg *Registry, opts CompileOptions) error {
	if err := compileExternal(reg, opts); err != nil {
		return err
	}
	return compileDocument(reg, opts)
}

func compileExternal(reg *Registry, opts CompileOptions) error {
	if len(opts.UserFiles) == 0 && len(opts.BundledFiles) == 0 {
		return nil
	}
	accessor := func(filename string) (io.ReadCloser, error) {
		if rc, err := readBundled(filename); err == nil {
			return rc, nil
		}
		return os.Open(filename)
	}
	files := make([]string, 0, len(opts.BundledFiles)+len(opts.UserFiles))
	files = append(files, opts.BundledFiles...)
	files = append(files, opts.UserFiles...)
	result, err := compile(accessor, files)
	if err != nil {
		return fmt.Errorf("compile schemas: %w", err)
	}
	for _, f := range result {
		reg.Merge(f)
	}
	return nil
}

func compileDocument(reg *Registry, opts CompileOptions) error {
	if len(opts.VirtualFiles) > 0 {
		virtual := map[string][]byte{}
		files := make([]string, 0, len(opts.VirtualFiles))
		for _, v := range opts.VirtualFiles {
			virtual[v.Name] = v.Body
			files = append(files, v.Name)
		}
		accessor := func(filename string) (io.ReadCloser, error) {
			if data, ok := virtual[filename]; ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			if slices.Contains(InDocumentImports, filename) {
				return readBundled(filename)
			}
			return nil, fmt.Errorf("a @proto may import only the well-known types, %s and another @proto in the document", strings.Join(InDocumentImports, ", "))
		}
		result, err := compile(accessor, files)
		if err != nil {
			return fmt.Errorf("compile schemas: %w", err)
		}
		for _, f := range result {
			reg.mergeDocument(f)
		}
	}

	// Descriptor-form sources (FileDescriptorSet bytes) bypass
	// protocompile — protodesc.NewFiles hydrates them directly.
	for _, blob := range opts.DescriptorSet {
		if err := mergeBlob(blob, "descriptor body", reg.mergeDocument); err != nil {
			return err
		}
	}
	reg.warnReferrers()
	return nil
}

// compile compiles files with the generated `_null` mask where NullMask
// asks for it, resolving imports through accessor and the well-known
// types.
func compile(accessor func(string) (io.ReadCloser, error), files []string) ([]protoreflect.FileDescriptor, error) {
	comp := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: accessor}),
		NullMask: NullMask,
	}
	result, err := comp.Compile(context.Background(), files...)
	if err != nil {
		return nil, err
	}
	out := make([]protoreflect.FileDescriptor, 0, len(result))
	for _, f := range result {
		out = append(out, f)
	}
	return out, nil
}

// CompileBundledFiles compiles canonical schemas from the embedded
// proto/ tree and returns their FileDescriptors.
//
// CompileSources folds its results into a flat message Registry, which
// is what the value-oriented subcommands need. Callers that need
// file-level descriptors — extensions, in particular, which a message
// map cannot carry — use this instead: `pxf docs build` builds dynamic
// extension types from schema/v1/descriptor.proto so it can read the
// 1327-1331 carriers out of a lowered image.
//
// NullMask leaves every bundled schema without a mask, so these compile
// as their sources declare them.
func CompileBundledFiles(files ...string) ([]protoreflect.FileDescriptor, error) {
	out, err := compile(readBundled, files)
	if err != nil {
		return nil, fmt.Errorf("compile bundled schemas: %w", err)
	}
	return out, nil
}

// NullMask selects the files whose messages get protocompile's generated
// `_null` FieldMask, the field a PXF null survives protobuf binary in
// (protocompile#248, RFC-001 §8.6). Every file gets one except the
// canonical schemas this repository bundles: those are shared types
// every port generates code for, so they must not grow a field (#377).
// google/protobuf/ files never get one; protocompile excludes them.
func NullMask(path string) bool {
	return !IsBundled(path)
}

// IsBundled reports whether path names a canonical schema served from
// the embedded proto/ tree, under either spelling readBundled accepts.
func IsBundled(filename string) bool {
	rc, err := readBundled(filename)
	if err != nil {
		return false
	}
	rc.Close()
	return true
}

// readBundled opens a canonical schema from the embedded proto/ tree.
// Import paths map the same way the `pxf build` bundled opener maps
// them: "X" → proto/X, and the canonical "protowire/X" spelling also
// resolves to proto/X.
func readBundled(filename string) (io.ReadCloser, error) {
	if data, err := protowire.BundledProto.ReadFile("proto/" + filename); err == nil {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	if rest, ok := strings.CutPrefix(filename, "protowire/"); ok {
		if data, err := protowire.BundledProto.ReadFile("proto/" + rest); err == nil {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}
	return nil, fs.ErrNotExist
}

// MergeDescriptorBlob unmarshals a FileDescriptorSet and merges every
// message it carries into reg. The label disambiguates error messages
// when callers have multiple descriptor sources in flight (e.g.
// "@proto descriptor body" vs "protoregistry bundle").
func MergeDescriptorBlob(reg *Registry, b []byte, label string) error {
	return mergeBlob(b, label, reg.Merge)
}

// MergeFileDescriptorSet folds every message reachable from fds into
// reg. Used by the protoregistry fetch.
func MergeFileDescriptorSet(reg *Registry, fds *descriptorpb.FileDescriptorSet, label string) error {
	return mergeFiles(fds, label, reg.Merge)
}

// mergeBlob unmarshals a FileDescriptorSet and hands each file to merge:
// Registry.Merge for an external schema, mergeDocument for a
// descriptor-form @proto.
func mergeBlob(b []byte, label string, merge func(protoreflect.FileDescriptor)) error {
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return fmt.Errorf("%s: unmarshal FileDescriptorSet: %w", label, err)
	}
	return mergeFiles(&fds, label, merge)
}

func mergeFiles(fds *descriptorpb.FileDescriptorSet, label string, merge func(protoreflect.FileDescriptor)) error {
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		merge(fd)
		return true
	})
	return nil
}

// RegistryRef is the protoregistry gRPC coordinate triple parsed
// from CLI flags. Empty == not requested.
type RegistryRef struct {
	Server    string
	Namespace string
	Schema    string
}

// Validate reports whether the triple is internally consistent:
// either every field empty (no request) or every field set.
// Partial triples surface as actionable flag errors.
func (r RegistryRef) Validate() error {
	if r.Server == "" && r.Namespace == "" && r.Schema == "" {
		return nil
	}
	if r.Server == "" {
		return fmt.Errorf("--namespace/--schema require --server")
	}
	if r.Namespace == "" {
		return fmt.Errorf("--namespace is required with --server")
	}
	if r.Schema == "" {
		return fmt.Errorf("--schema is required with --server")
	}
	return nil
}

// Active reports whether r carries enough state for Fetch to run.
func (r RegistryRef) Active() bool {
	return r.Server != "" && r.Namespace != "" && r.Schema != ""
}

// FetchRegistry connects to the named server, asks for the schema
// bundle, and returns the FileDescriptorSet the server hosts for it.
// Connection uses insecure credentials with a 10s timeout per call —
// same configuration the legacy cmd/protowire used.
func FetchRegistry(ref RegistryRef) (*descriptorpb.FileDescriptorSet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(ref.Server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("protoregistry connect %s: %w", ref.Server, err)
	}
	defer conn.Close()

	client := registrypb.NewRegistryServiceClient(conn)
	resp, err := client.GetDescriptor(ctx, &registrypb.GetDescriptorRequest{
		NamespaceId: ref.Namespace,
		SchemaId:    ref.Schema,
	})
	if err != nil {
		return nil, fmt.Errorf("protoregistry GetDescriptor %s/%s: %w", ref.Namespace, ref.Schema, err)
	}
	if resp.FileDescriptorSet == nil {
		return nil, fmt.Errorf("protoregistry %s/%s: empty FileDescriptorSet in response", ref.Namespace, ref.Schema)
	}
	return resp.FileDescriptorSet, nil
}

// Resolve is the convenience entry point that runs the full chain.
// Callers that only need a subset construct CompileOptions /
// RegistryRef explicitly and call CompileSources / FetchRegistry +
// MergeFileDescriptorSet themselves.
func Resolve(opts CompileOptions, ref RegistryRef) (*Registry, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	reg := NewRegistry()
	if err := compileExternal(reg, opts); err != nil {
		return nil, err
	}
	if ref.Active() {
		fds, err := FetchRegistry(ref)
		if err != nil {
			return nil, err
		}
		if err := MergeFileDescriptorSet(reg, fds, "protoregistry bundle"); err != nil {
			return nil, err
		}
	}
	// Last, so a document's own types shadow the registry's too (#371).
	if err := compileDocument(reg, opts); err != nil {
		return nil, err
	}
	return reg, nil
}
