// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package schemaresolve

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	registrypb "github.com/trendvidia/protoregistry/proto/protoregistry/v1"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// --- RegistryRef flag-validation ---

func TestRegistryRef_Validate(t *testing.T) {
	cases := []struct {
		name string
		ref  RegistryRef
		want string // substring of expected error; "" = no error
	}{
		{"empty allowed", RegistryRef{}, ""},
		{"all three set", RegistryRef{Server: "x:1", Namespace: "ns", Schema: "s"}, ""},
		{"server missing", RegistryRef{Namespace: "ns", Schema: "s"}, "require --server"},
		{"namespace missing", RegistryRef{Server: "x:1", Schema: "s"}, "--namespace is required"},
		{"schema missing", RegistryRef{Server: "x:1", Namespace: "ns"}, "--schema is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ref.Validate()
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

func TestRegistryRef_Active(t *testing.T) {
	if (RegistryRef{}).Active() {
		t.Error("empty ref should not be active")
	}
	if !(RegistryRef{Server: "x:1", Namespace: "n", Schema: "s"}).Active() {
		t.Error("fully-populated ref should be active")
	}
}

// --- MergeFileDescriptorSet ---

func TestMergeFileDescriptorSet(t *testing.T) {
	reg := NewRegistry()
	if err := MergeFileDescriptorSet(reg, fakeFDS(), "test"); err != nil {
		t.Fatal(err)
	}
	if reg.Find("reg.v1.Item") == nil {
		t.Error("Item not registered from FDS")
	}
}

// --- end-to-end through FetchRegistry against an in-process gRPC server ---

func TestFetchRegistry_EndToEnd(t *testing.T) {
	addr := startLoopbackServer(t, fakeFDS())

	fds, err := FetchRegistry(RegistryRef{Server: addr, Namespace: "reg", Schema: "v1"})
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

func TestResolve_WithRegistry(t *testing.T) {
	addr := startLoopbackServer(t, fakeFDS())

	reg, err := Resolve(CompileOptions{BundledFiles: CompileBundledAll}, RegistryRef{
		Server: addr, Namespace: "reg", Schema: "v1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if reg.Find("reg.v1.Item") == nil {
		t.Error("registry type not in resolved schema")
	}
	if reg.Find("envelope.v1.Envelope") == nil {
		t.Error("bundled type missing alongside registry")
	}
}

func TestResolve_ServerErrorPropagates(t *testing.T) {
	ref := RegistryRef{Server: "127.0.0.1:1", Namespace: "ns", Schema: "s"}
	_, err := Resolve(CompileOptions{}, ref)
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if !strings.Contains(err.Error(), "protoregistry") {
		t.Errorf("error should mention protoregistry: %v", err)
	}
}

func TestResolve_PartialFlagsRejectedBeforeFetch(t *testing.T) {
	_, err := Resolve(CompileOptions{}, RegistryRef{Server: "x:1"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--namespace") {
		t.Errorf("expected --namespace mention: %v", err)
	}
}

// --- compile path ---

func TestCompileSources_UserFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.proto")
	if err := os.WriteFile(p, []byte(`syntax = "proto3";
package test.v1;
message M { string s = 1; }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	if err := CompileSources(reg, CompileOptions{UserFiles: []string{p}}); err != nil {
		t.Fatal(err)
	}
	if reg.Find("test.v1.M") == nil {
		t.Error("test.v1.M not registered")
	}
}

func TestCompileSources_Virtual(t *testing.T) {
	reg := NewRegistry()
	err := CompileSources(reg, CompileOptions{
		VirtualFiles: []VirtualFile{{
			Name: "v.proto",
			Body: []byte("syntax = \"proto3\";\npackage v;\nmessage V { string s = 1; }\n"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Find("v.V") == nil {
		t.Error("v.V not registered from virtual file")
	}
}

func TestCompileSources_BundledOnly(t *testing.T) {
	reg := NewRegistry()
	if err := CompileSources(reg, CompileOptions{BundledFiles: CompileBundledAll}); err != nil {
		t.Fatal(err)
	}
	// Spot-check a few canonical types.
	for _, want := range []string{"pxf.Decimal", "envelope.v1.Envelope", "envelope.v1.AppError"} {
		if reg.Find(want) == nil {
			t.Errorf("bundled type %q not registered", want)
		}
	}
}

func TestCompileSources_DashPCanImportBundled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "user.proto")
	if err := os.WriteFile(p, []byte(`syntax = "proto3";
package mine.v1;
import "envelope/v1/envelope.proto";
message Container { envelope.v1.AppError err = 1; }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	err := CompileSources(reg, CompileOptions{
		UserFiles:    []string{p},
		BundledFiles: CompileBundledAll,
	})
	if err != nil {
		t.Fatalf("cross-import compile failed: %v", err)
	}
	if reg.Find("mine.v1.Container") == nil {
		t.Error("user message with bundled-import not registered")
	}
}

// --- gRPC test fixtures (shared with cmd/pxq's integration tests) ---

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

type fakeRegistry struct {
	registrypb.UnimplementedRegistryServiceServer
	fds *descriptorpb.FileDescriptorSet
}

func (r *fakeRegistry) GetDescriptor(_ context.Context, _ *registrypb.GetDescriptorRequest) (*registrypb.GetDescriptorResponse, error) {
	return &registrypb.GetDescriptorResponse{FileDescriptorSet: r.fds}, nil
}

func startLoopbackServer(t *testing.T, fds *descriptorpb.FileDescriptorSet) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	registrypb.RegisterRegistryServiceServer(srv, &fakeRegistry{fds: fds})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })
	return lis.Addr().String()
}
