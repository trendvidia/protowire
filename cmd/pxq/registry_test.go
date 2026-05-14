// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"context"
	"net"
	"strings"
	"testing"

	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// --- flag validation ---

func TestRegistry_Validated(t *testing.T) {
	cases := []struct {
		name string
		ref  registryRef
		want string // substring of expected error; "" means no error
	}{
		{"empty allowed", registryRef{}, ""},
		{"all three set", registryRef{server: "x:1", namespace: "ns", schema: "s"}, ""},
		{"server missing", registryRef{namespace: "ns", schema: "s"}, "require --server"},
		{"namespace missing", registryRef{server: "x:1", schema: "s"}, "--namespace is required"},
		{"schema missing", registryRef{server: "x:1", namespace: "ns"}, "--schema is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ref.validated()
			switch {
			case c.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case c.want != "" && err == nil:
				t.Errorf("expected error containing %q, got nil", c.want)
			case c.want != "" && !strings.Contains(err.Error(), c.want):
				t.Errorf("error %v does not contain %q", err, c.want)
			}
		})
	}
}

func TestRegistry_Active(t *testing.T) {
	if (registryRef{}).active() {
		t.Error("empty ref should not be active")
	}
	if !(registryRef{server: "x:1", namespace: "n", schema: "s"}).active() {
		t.Error("fully-populated ref should be active")
	}
}

// --- registerFileDescriptorSet: the merge path used by both
//     descriptor-form @proto and the registry fetch ---

func TestRegistry_RegisterFileDescriptorSet(t *testing.T) {
	s := &schema{byFullName: map[protoreflect.FullName]protoreflect.MessageDescriptor{}}
	if err := registerFileDescriptorSet(s, fakeFDS(), "test"); err != nil {
		t.Fatal(err)
	}
	if s.find("reg.v1.Item") == nil {
		t.Error("Item not registered from FDS")
	}
}

// --- end-to-end through fetchRegistry against a loopback gRPC ---

func TestRegistry_FetchEndToEnd(t *testing.T) {
	addr := startLoopbackServer(t, fakeFDS())

	fds, err := fetchRegistry(registryRef{server: addr, namespace: "reg", schema: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if fds == nil || len(fds.File) == 0 {
		t.Fatal("empty FDS in response")
	}
	if name := fds.File[0].GetName(); name != "reg/v1/items.proto" {
		t.Errorf("unexpected filename: %s", name)
	}
}

func TestRegistry_LoadSchemaWithRegistry(t *testing.T) {
	// loadSchema honours the registry leg of the resolution chain
	// when -s/-n/--schema are all set; bundled stays available too.
	addr := startLoopbackServer(t, fakeFDS())

	sch, err := loadSchema(nil, nil, registryRef{
		server: addr, namespace: "reg", schema: "v1",
	})
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}
	if sch.find("reg.v1.Item") == nil {
		t.Error("registry type not in resolved schema")
	}
	if sch.find("envelope.v1.Envelope") == nil {
		t.Error("bundled type missing alongside registry")
	}
}

func TestRegistry_LoadSchemaServerErrorPropagates(t *testing.T) {
	// Server set to a loopback port that nothing listens on — fetch
	// fails fast (the per-call context timeout caps the wait).
	ref := registryRef{server: "127.0.0.1:1", namespace: "ns", schema: "s"}
	_, err := loadSchema(nil, nil, ref)
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if !strings.Contains(err.Error(), "protoregistry") {
		t.Errorf("error should mention protoregistry: %v", err)
	}
}

func TestRegistry_PartialFlagsRejectedAtLoadSchema(t *testing.T) {
	// Validation runs first, before any network call — half-set
	// triples error cleanly even when the server would be unreachable.
	_, err := loadSchema(nil, nil, registryRef{server: "x:1"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--namespace") {
		t.Errorf("expected --namespace mention: %v", err)
	}
}

// --- gRPC test fixtures ---

// fakeFDS returns a minimal FileDescriptorSet — one message,
// reg.v1.Item with a single string field. Enough to exercise the
// merge path without a real .proto compiler.
func fakeFDS() *descriptorpb.FileDescriptorSet {
	syntax := "proto3"
	pkg := "reg.v1"
	fileName := "reg/v1/items.proto"
	msgName := "Item"
	fieldName := "name"
	fieldNum := int32(1)
	fieldType := descriptorpb.FieldDescriptorProto_TYPE_STRING
	fieldLabel := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    &fileName,
			Package: &pkg,
			Syntax:  &syntax,
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: &msgName,
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:   &fieldName,
					Number: &fieldNum,
					Type:   &fieldType,
					Label:  &fieldLabel,
				}},
			}},
		}},
	}
}

// fakeRegistry implements RegistryServiceServer for the tests above.
type fakeRegistry struct {
	registrypb.UnimplementedRegistryServiceServer
	fds *descriptorpb.FileDescriptorSet
}

func (r *fakeRegistry) GetDescriptor(_ context.Context, _ *registrypb.GetDescriptorRequest) (*registrypb.GetDescriptorResponse, error) {
	return &registrypb.GetDescriptorResponse{FileDescriptorSet: r.fds}, nil
}

// startLoopbackServer spins up an in-process gRPC server on a
// loopback port and returns its address. Real network sockets but
// scoped to 127.0.0.1, no dial-override gymnastics — fetchRegistry
// uses grpc.NewClient(addr) unmodified.
func startLoopbackServer(t *testing.T, fds *descriptorpb.FileDescriptorSet) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	registrypb.RegisterRegistryServiceServer(srv, &fakeRegistry{fds: fds})
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
	})
	return lis.Addr().String()
}
