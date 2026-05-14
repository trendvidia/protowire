// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"strings"
	"testing"
)

// The heavy lifting (FetchRegistry, RegistryRef.Validate, in-process
// gRPC test fixtures) lives in internal/schemaresolve where the
// behaviour is implemented. This file keeps a couple of thin sanity
// checks that exercise the cmd/pxq → schemaresolve wiring — proving
// the alias and the loadSchema entry point still surface the package's
// error messages correctly.

func TestRegistryWiring_PartialFlagsErrorAtLoadSchema(t *testing.T) {
	_, err := loadSchema(nil, nil, registryRef{Server: "x:1"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--namespace") {
		t.Errorf("expected --namespace mention: %v", err)
	}
}

func TestRegistryWiring_UnreachableServerSurfacesError(t *testing.T) {
	ref := registryRef{Server: "127.0.0.1:1", Namespace: "ns", Schema: "s"}
	_, err := loadSchema(nil, nil, ref)
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if !strings.Contains(err.Error(), "protoregistry") {
		t.Errorf("error should mention protoregistry: %v", err)
	}
}
