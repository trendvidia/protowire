// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire/internal/schemaresolve"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// schema is the per-invocation registry the query layer resolves
// against. The README's 4-step resolution chain lives entirely inside
// loadSchema; consumers see a single Find(name) accessor.
//
// A nil *schema is the loose-mode signal — pxf_proto / pxf_fieldnames
// raise a query-time error, and pxf_directive / pxf_has fall back to
// the unbound graph shape (raw cell tuples instead of schema-bound
// row objects).
type schema struct {
	reg *schemaresolve.Registry
}

// find returns the descriptor for name, or nil if not registered.
func (s *schema) find(name string) protoreflect.MessageDescriptor {
	if s == nil {
		return nil
	}
	return s.reg.Find(name)
}

// registryRef is the local alias the CLI plumbs through from flags.
// It exists so the rest of cmd/pxf doesn't need to import the shared
// package directly; conversion happens here at the boundary.
type registryRef = schemaresolve.RegistryRef

// loadSchema runs the README's resolution chain and returns a registry.
// The protoFiles slice comes from -p; inDoc carries any @proto
// directives the parser surfaced; reg carries the protoregistry
// coordinate triple.
//
// Three of the four `@proto` body shapes are supported as schema
// sources here: source, named, and descriptor. Anonymous is the
// fourth shape; resolveAnonymousProtos rewrites those into named
// before loadSchema sees them.
func loadSchema(protoFiles []string, inDoc []pxf.ProtoDirective, reg registryRef) (*schema, error) {
	opts, err := buildCompileOptions(protoFiles, inDoc)
	if err != nil {
		return nil, err
	}
	r, err := schemaresolve.Resolve(opts, reg)
	if err != nil {
		return nil, err
	}
	return &schema{reg: r}, nil
}

// buildCompileOptions translates the cmd/pxf view of the world
// (-p paths + in-doc directives) into the shared CompileOptions
// shape. Synthesises virtual filenames for the named/source shapes
// and accumulates descriptor blobs for the descriptor shape.
func buildCompileOptions(protoFiles []string, inDoc []pxf.ProtoDirective) (schemaresolve.CompileOptions, error) {
	opts := schemaresolve.CompileOptions{
		UserFiles:    protoFiles,
		BundledFiles: schemaresolve.CompileBundledAll,
	}
	for i, pd := range inDoc {
		switch pd.Shape {
		case pxf.ProtoSource:
			opts.VirtualFiles = append(opts.VirtualFiles, schemaresolve.VirtualFile{
				Name: fmt.Sprintf("__pxq_indoc_%d.proto", i),
				Body: pd.Body,
			})
		case pxf.ProtoNamed:
			opts.VirtualFiles = append(opts.VirtualFiles, schemaresolve.VirtualFile{
				Name: fmt.Sprintf("__pxq_indoc_%d.proto", i),
				Body: synthesizeNamedMessageFile(pd.TypeName, pd.Body),
			})
		case pxf.ProtoDescriptor:
			opts.DescriptorSet = append(opts.DescriptorSet, pd.Body)
		case pxf.ProtoAnonymous:
			return opts, fmt.Errorf("pxf: internal — anonymous @proto reached schema loader; " +
				"call resolveAnonymousProtos on the loadedDoc first")
		default:
			return opts, fmt.Errorf("pxf: unknown @proto shape %v", pd.Shape)
		}
	}
	return opts, nil
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

// resolveAnonymousProtos applies the v1.0 spec's bind-to-next-typeless
// rule (draft §3.4.4 / §3.4.5): each anonymous `@proto { body }`
// directive consumes its right-following typeless `@dataset` in
// document order, supplying it as the row message type. We synthesise
// a unique name per pair (`_pxq_anon_N`), rewrite the proto in place
// from anonymous to named, and copy that name onto the matched
// dataset. The rest of the pipeline (loadSchema, pxf_directive) then
// treats the pair as if it had been written with an explicit name.
//
// The binding is strict: every anonymous proto must have a matching
// typeless dataset and vice versa, in lockstep document order. An
// orphan in either direction is a parse-time error from the user's
// perspective.
//
// Document order is established by Pos.Offset since pxf.Document
// exposes protos and datasets as separate slices.
func resolveAnonymousProtos(doc *loadedDoc) error {
	if doc == nil {
		return nil
	}
	type ent struct {
		off     int
		isProto bool
		idx     int
	}
	ents := make([]ent, 0, len(doc.protos)+len(doc.datasets))
	for i, p := range doc.protos {
		ents = append(ents, ent{p.Pos.Offset, true, i})
	}
	for i, ds := range doc.datasets {
		ents = append(ents, ent{ds.Pos.Offset, false, i})
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].off < ents[j].off })

	var pending []int
	counter := 0
	for _, e := range ents {
		if e.isProto {
			if doc.protos[e.idx].Shape == pxf.ProtoAnonymous {
				pending = append(pending, e.idx)
			}
			continue
		}
		if doc.datasets[e.idx].Type != "" {
			continue
		}
		if len(pending) == 0 {
			return fmt.Errorf("@dataset at %s has no type and no preceding anonymous @proto to bind to (draft §3.4.4)",
				doc.datasets[e.idx].Pos)
		}
		protoIdx := pending[0]
		pending = pending[1:]
		name := fmt.Sprintf("_pxq_anon_%d", counter)
		counter++
		doc.protos[protoIdx].Shape = pxf.ProtoNamed
		doc.protos[protoIdx].TypeName = name
		doc.datasets[e.idx].Type = name
	}
	if len(pending) > 0 {
		return fmt.Errorf("anonymous @proto at %s has no following typeless @dataset to bind to (draft §3.4.5)",
			doc.protos[pending[0]].Pos)
	}
	return nil
}
