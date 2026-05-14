// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/trendvidia/protowire-go/encoding/pxf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// schema is the in-scope descriptor set the query layer can resolve
// against. Loaded from the README's resolution chain:
//   1. bundled canonical schemas (deferred)
//   2. in-document @proto directives (this stage)
//   3. -p schema.proto                (Stage C)
//   4. protoregistry server           (deferred)
//
// A nil *schema is the loose-mode signal — pxf_proto / pxf_fieldnames
// raise a query-time error, and pxf_directive / pxf_has fall back to
// the unbound graph shape (raw cell tuples instead of schema-bound row
// objects).
type schema struct {
	byFullName map[protoreflect.FullName]protoreflect.MessageDescriptor
}

// loadSchema compiles protoFiles and any in-document @proto directives
// into a single descriptor registry. Returns nil when there's no
// schema material in scope (the loose-mode signal).
//
// Three of the four `@proto` body shapes (draft §3.4.5) are supported
// as schema sources:
//
//   - source     (`@proto """ ... """`)   — full .proto source file
//   - named      (`@proto Name { body }`) — sugar over a single message
//   - descriptor (`@proto b"..."`)        — base64 FileDescriptorSet
//
// Anonymous (`@proto { body }`) is rejected here for now — its binding
// rule (consume as the type of the next typeless directive) is a
// separate piece of logic that lands in a follow-up.
func loadSchema(protoFiles []string, inDoc []pxf.ProtoDirective) (*schema, error) {
	if len(protoFiles) == 0 && len(inDoc) == 0 {
		return nil, nil
	}

	// Synthesize virtual .proto files for the source/named shapes so
	// protocompile can compile them alongside the user-supplied -p
	// files in a single pass — that way cross-references between the
	// two work without extra plumbing.
	virtual := map[string][]byte{}
	virtualNames := make([]string, 0, len(inDoc))
	var descriptorBlobs [][]byte
	for i, pd := range inDoc {
		switch pd.Shape {
		case pxf.ProtoSource:
			name := fmt.Sprintf("__pxq_indoc_%d.proto", i)
			virtual[name] = pd.Body
			virtualNames = append(virtualNames, name)
		case pxf.ProtoNamed:
			name := fmt.Sprintf("__pxq_indoc_%d.proto", i)
			virtual[name] = synthesizeNamedMessageFile(pd.TypeName, pd.Body)
			virtualNames = append(virtualNames, name)
		case pxf.ProtoDescriptor:
			descriptorBlobs = append(descriptorBlobs, pd.Body)
		case pxf.ProtoAnonymous:
			return nil, fmt.Errorf("pxq: anonymous @proto directives aren't yet supported as schema sources; " +
				"give the message a name (e.g. `@proto trades.v1.Trade { ... }`)")
		default:
			return nil, fmt.Errorf("pxq: unknown @proto shape %v", pd.Shape)
		}
	}

	s := &schema{byFullName: map[protoreflect.FullName]protoreflect.MessageDescriptor{}}

	// Compile -p files + virtual in-doc sources together.
	if len(protoFiles) > 0 || len(virtualNames) > 0 {
		accessor := func(filename string) (io.ReadCloser, error) {
			if data, ok := virtual[filename]; ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			return os.Open(filename)
		}
		comp := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: accessor}),
		}
		files := append([]string{}, protoFiles...)
		files = append(files, virtualNames...)
		result, err := comp.Compile(context.Background(), files...)
		if err != nil {
			return nil, fmt.Errorf("compile schemas: %w", err)
		}
		for _, f := range result {
			walkMessages(f.Messages(), s.byFullName)
		}
	}

	// Register descriptor-form in-doc protos. They bypass protocompile
	// entirely — the body is already a serialised FileDescriptorSet
	// that protodesc can hydrate directly.
	for _, blob := range descriptorBlobs {
		if err := registerDescriptorBlob(s, blob); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// synthesizeNamedMessageFile turns `@proto trades.v1.Trade { body }`
// into the equivalent standalone `.proto`:
//
//	syntax = "proto3";
//	package trades.v1;
//	message Trade {
//	  body
//	}
//
// If typeName has no dot, the file gets no `package` declaration.
func synthesizeNamedMessageFile(typeName string, body []byte) []byte {
	pkg, msg := splitDottedName(typeName)
	var b strings.Builder
	b.WriteString("syntax = \"proto3\";\n")
	if pkg != "" {
		fmt.Fprintf(&b, "package %s;\n", pkg)
	}
	fmt.Fprintf(&b, "message %s {\n", msg)
	b.Write(body)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// splitDottedName returns (package, leaf) for a dotted protobuf type
// name. "trades.v1.Trade" → ("trades.v1", "Trade"); "Trade" → ("", "Trade").
func splitDottedName(name string) (string, string) {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return "", name
	}
	return name[:idx], name[idx+1:]
}

// registerDescriptorBlob unmarshals a FileDescriptorSet and merges
// every message reachable from it into the schema registry. Same
// shape as the source-form path's output — callers can't tell which
// route a given descriptor came from.
func registerDescriptorBlob(s *schema, b []byte) error {
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return fmt.Errorf("@proto descriptor body: unmarshal FileDescriptorSet: %w", err)
	}
	files, err := protodesc.NewFiles(&fds)
	if err != nil {
		return fmt.Errorf("@proto descriptor body: %w", err)
	}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		walkMessages(fd.Messages(), s.byFullName)
		return true
	})
	return nil
}

func walkMessages(msgs protoreflect.MessageDescriptors, out map[protoreflect.FullName]protoreflect.MessageDescriptor) {
	for i := range msgs.Len() {
		md := msgs.Get(i)
		out[md.FullName()] = md
		walkMessages(md.Messages(), out) // nested types
	}
}

// find returns the descriptor for name, or nil if not registered.
func (s *schema) find(name string) protoreflect.MessageDescriptor {
	if s == nil {
		return nil
	}
	return s.byFullName[protoreflect.FullName(name)]
}
