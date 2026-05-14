// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package schemaresolve is the shared implementation behind the
// schema-resolution chain that both `cmd/protowire` and `cmd/pxq`
// surface. The README at cmd/pxq describes the chain as:
//
//	1. Bundled canonical schemas      (`pxf/*`, `sbe/*`, `envelope/v1/*`)
//	2. In-document `@proto` directives (source / named / descriptor shapes)
//	3. -p schema.proto                 (user-supplied .proto sources)
//	4. -s server -n namespace --schema  (protoregistry gRPC fetch)
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
	"os"
	"time"

	"github.com/bufbuild/protocompile"
	"github.com/trendvidia/protowire"
	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
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
}

// NewRegistry returns an empty Registry. Useful when callers need to
// hand-merge descriptors that didn't come through the standard
// resolution chain (e.g. tests).
func NewRegistry() *Registry {
	return &Registry{byFullName: map[protoreflect.FullName]protoreflect.MessageDescriptor{}}
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

// Merge folds every message reachable from fd into the registry.
// Used by every source step (compile result, registry fetch,
// descriptor-form bodies).
func (r *Registry) Merge(fd protoreflect.FileDescriptor) {
	walkMessages(fd.Messages(), r.byFullName)
}

func walkMessages(msgs protoreflect.MessageDescriptors, out map[protoreflect.FullName]protoreflect.MessageDescriptor) {
	for i := range msgs.Len() {
		md := msgs.Get(i)
		out[md.FullName()] = md
		walkMessages(md.Messages(), out)
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

// CompileSources resolves UserFiles + VirtualFiles + BundledFiles via
// protocompile in a single pass (so cross-imports work) and folds the
// result into reg. Returns reg for call-chain convenience.
//
// File-name routing inside the accessor:
//   - virtual filename match     → serve from VirtualFiles
//   - bundled canonical name     → serve from protowire.BundledProto
//   - otherwise                  → os.Open
//
// The cross-import promise: a user .proto can `import
// "envelope/v1/envelope.proto"` and have it resolve from the embed.FS
// without an explicit -p flag.
func CompileSources(reg *Registry, opts CompileOptions) error {
	hasInputs := len(opts.UserFiles) > 0 || len(opts.VirtualFiles) > 0 || len(opts.BundledFiles) > 0
	if hasInputs {
		virtual := map[string][]byte{}
		for _, v := range opts.VirtualFiles {
			virtual[v.Name] = v.Body
		}
		accessor := func(filename string) (io.ReadCloser, error) {
			if data, ok := virtual[filename]; ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			if data, err := protowire.BundledProto.ReadFile("proto/" + filename); err == nil {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			return os.Open(filename)
		}
		comp := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: accessor}),
		}
		files := make([]string, 0, len(opts.BundledFiles)+len(opts.UserFiles)+len(opts.VirtualFiles))
		files = append(files, opts.BundledFiles...)
		files = append(files, opts.UserFiles...)
		for _, v := range opts.VirtualFiles {
			files = append(files, v.Name)
		}
		result, err := comp.Compile(context.Background(), files...)
		if err != nil {
			return fmt.Errorf("compile schemas: %w", err)
		}
		for _, f := range result {
			reg.Merge(f)
		}
	}

	// Descriptor-form sources (FileDescriptorSet bytes) bypass
	// protocompile — protodesc.NewFiles hydrates them directly.
	for _, blob := range opts.DescriptorSet {
		if err := MergeDescriptorBlob(reg, blob, "descriptor body"); err != nil {
			return err
		}
	}
	return nil
}

// MergeDescriptorBlob unmarshals a FileDescriptorSet and merges every
// message it carries into reg. The label disambiguates error messages
// when callers have multiple descriptor sources in flight (e.g.
// "@proto descriptor body" vs "protoregistry bundle").
func MergeDescriptorBlob(reg *Registry, b []byte, label string) error {
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return fmt.Errorf("%s: unmarshal FileDescriptorSet: %w", label, err)
	}
	return MergeFileDescriptorSet(reg, &fds, label)
}

// MergeFileDescriptorSet folds every message reachable from fds into
// reg. Used by the @proto descriptor shape and the protoregistry
// fetch.
func MergeFileDescriptorSet(reg *Registry, fds *descriptorpb.FileDescriptorSet, label string) error {
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		reg.Merge(fd)
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
// same configuration cmd/protowire uses today.
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
	if err := CompileSources(reg, opts); err != nil {
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
	return reg, nil
}
