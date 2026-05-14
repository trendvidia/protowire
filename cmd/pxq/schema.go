// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// schema is the in-scope descriptor set the query layer can resolve
// against. Stage C only loads from `-p schema.proto`; in-doc `@proto`
// directives and the protoregistry chain land in follow-up stages.
//
// A nil *schema is the loose-mode signal — pxf_proto / pxf_fieldnames
// raise a query-time error pointing at the README's strict-mode
// section, and pxf_directive / pxf_has fall back to the unbound graph
// shape (raw cell tuples instead of schema-bound row objects).
type schema struct {
	byFullName map[protoreflect.FullName]protoreflect.MessageDescriptor
}

// loadSchema compiles protoFiles and returns a registry of every
// message reachable from them. Stage C entry point.
func loadSchema(protoFiles []string) (*schema, error) {
	if len(protoFiles) == 0 {
		return nil, nil
	}
	comp := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{}),
	}
	result, err := comp.Compile(context.Background(), protoFiles...)
	if err != nil {
		return nil, fmt.Errorf("compile -p schemas: %w", err)
	}

	s := &schema{byFullName: map[protoreflect.FullName]protoreflect.MessageDescriptor{}}
	for _, f := range result {
		walkMessages(f.Messages(), s.byFullName)
	}
	return s, nil
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
