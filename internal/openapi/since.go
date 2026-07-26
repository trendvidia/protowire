// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/descriptorpb"
)

// x-since (§#080 Gap 3): derived from protoregistry history — the first
// registered revision containing an element — when coordinates are
// configured, omitted otherwise. Availability is never authored.

// sinceIndex maps element FQN → first version label ("v3").
type sinceIndex map[string]string

// versionSets is the pure input: schema version → the set of element
// FQNs present in that version's descriptor.
type versionSets map[uint64]map[string]bool

// firstSeen computes the earliest version containing each element.
func firstSeen(vs versionSets) sinceIndex {
	versions := make([]uint64, 0, len(vs))
	for v := range vs {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	out := make(sinceIndex)
	for _, v := range versions {
		for fqn := range vs[v] {
			if _, seen := out[fqn]; !seen {
				out[fqn] = fmt.Sprintf("v%d", v)
			}
		}
	}
	return out
}

// elementFQNs walks one descriptor set and returns every element FQN
// the generator stamps: messages, enums, aliases, services, methods.
func elementFQNs(fds *descriptorpb.FileDescriptorSet) (map[string]bool, error) {
	m, err := buildModel(fds)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for fqn := range m.messages {
		out[fqn] = true
	}
	for fqn := range m.enums {
		out[fqn] = true
	}
	for fqn := range m.aliases {
		out[fqn] = true
	}
	for _, svc := range m.services {
		out[svc.fqn] = true
		for _, mth := range svc.methods {
			out[svc.fqn+"."+mth.name] = true
		}
	}
	return out, nil
}

// fetchHistory pulls every registered version's descriptor set. It is a
// variable so tests can substitute a fake without a live registry.
var fetchHistory = func(coords registryCoords) (versionSets, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(coords.server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dialing protoregistry %s: %w", coords.server, err)
	}
	defer conn.Close()
	client := registrypb.NewRegistryServiceClient(conn)

	info, err := client.GetSchema(ctx, &registrypb.GetSchemaRequest{
		NamespaceId: coords.namespace,
		SchemaId:    coords.schema,
	})
	if err != nil {
		return nil, fmt.Errorf("protoregistry GetSchema %s/%s: %w", coords.namespace, coords.schema, err)
	}
	out := make(versionSets)
	for _, v := range info.GetSchema().GetVersions() {
		resp, err := client.GetDescriptor(ctx, &registrypb.GetDescriptorRequest{
			NamespaceId: coords.namespace,
			SchemaId:    coords.schema,
			Version:     v,
		})
		if err != nil {
			return nil, fmt.Errorf("protoregistry GetDescriptor v%d: %w", v, err)
		}
		fds := resp.GetFileDescriptorSet()
		if fds == nil {
			return nil, fmt.Errorf("protoregistry v%d: empty descriptor set", v)
		}
		fqns, err := elementFQNs(fds)
		if err != nil {
			return nil, fmt.Errorf("protoregistry v%d: %w", v, err)
		}
		out[v] = fqns
	}
	return out, nil
}

// buildSince resolves the index for configured coordinates.
func buildSince(coords registryCoords) (sinceIndex, error) {
	if !coords.configured() {
		return nil, nil
	}
	vs, err := fetchHistory(coords)
	if err != nil {
		return nil, err
	}
	return firstSeen(vs), nil
}

// stamp adds x-since to a schema/operation omap when the index knows
// the element.
func (idx sinceIndex) stamp(s *omap, fqn string) {
	if idx == nil || s == nil {
		return
	}
	if v, ok := idx[strings.TrimPrefix(fqn, ".")]; ok {
		s.set("x-since", v)
	}
}
