// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"context"
	"fmt"
	"time"

	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/descriptorpb"
)

// registryRef carries the protoregistry coordinates parsed from
// -s/-n/--schema. Empty server is the loose-mode signal — nothing is
// fetched, no error.
type registryRef struct {
	server    string
	namespace string
	schema    string
}

// validated returns nil when the ref is empty (no registry resolution
// requested) or an error describing the missing flag when the user
// supplied partial coordinates.
func (r registryRef) validated() error {
	if r.server == "" && r.namespace == "" && r.schema == "" {
		return nil
	}
	if r.server == "" {
		return fmt.Errorf("--namespace/--schema require --server")
	}
	if r.namespace == "" {
		return fmt.Errorf("--namespace is required with --server")
	}
	if r.schema == "" {
		return fmt.Errorf("--schema is required with --server")
	}
	return nil
}

// active reports whether the ref carries enough state to be worth
// fetching. Used by loadSchema's fetch path.
func (r registryRef) active() bool {
	return r.server != "" && r.namespace != "" && r.schema != ""
}

// fetchRegistry connects to the protoregistry gRPC server, asks for
// the named schema bundle, and returns the FileDescriptorSet the
// server hosts for it. The returned set carries every file the schema
// transitively imports — the schema registry walks all of them.
//
// Mirrors cmd/protowire's resolveFromRegistry helper, simplified for
// the bundle-fetch path (no per-message FullName narrowing).
func fetchRegistry(ref registryRef) (*descriptorpb.FileDescriptorSet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(ref.server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("protoregistry connect %s: %w", ref.server, err)
	}
	defer conn.Close()

	client := registrypb.NewRegistryServiceClient(conn)
	resp, err := client.GetDescriptor(ctx, &registrypb.GetDescriptorRequest{
		NamespaceId: ref.namespace,
		SchemaId:    ref.schema,
	})
	if err != nil {
		return nil, fmt.Errorf("protoregistry GetDescriptor %s/%s: %w", ref.namespace, ref.schema, err)
	}
	if resp.FileDescriptorSet == nil {
		return nil, fmt.Errorf("protoregistry %s/%s: empty FileDescriptorSet in response", ref.namespace, ref.schema)
	}
	return resp.FileDescriptorSet, nil
}
