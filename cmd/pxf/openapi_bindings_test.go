// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trendvidia/protowire/docpack"
)

const bindingsFixture = fixtureDir + "/22_http_additional_bindings.proto"

// buildTempSchema compiles one schema written to a temp dir, well away
// from the repo so generator-config discovery finds nothing and the
// defaults apply.
func buildTempSchema(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.proto")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(dir, "image.binpb")
	if err := runPxf(t, "build", "-o", image, path); err != nil {
		t.Fatalf("pxf build: %v", err)
	}
	return image
}

// TestOpenAPIRendersEveryBinding is the #215 acceptance path: a method
// with several @http use sites contributes one operation per binding,
// so the document describes every route the image binds.
func TestOpenAPIRendersEveryBinding(t *testing.T) {
	doc := string(renderOpenAPI(t, "--config", openapiConfig, storeImage(t)))

	for _, want := range []string{
		"/customers/{customer_id}:",
		"/v2/customers/{customer_id}:",
		"operationId: Store_GetCustomer", // primary keeps the derived id
		"operationId: getCustomerV2",     // the alias names its own
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered document is missing %q", want)
		}
	}
}

// TestOpenAPIRepeatedHTTPNeedsOperationID pins the decision recorded on
// #215: a binding after the first names its own operation_id rather
// than inheriting a positional suffix, because the id is a generated
// client's method name and must not move when two annotation lines are
// reordered.
func TestOpenAPIRepeatedHTTPNeedsOperationID(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message R { string id = 1; }

service S {
  @http("GET", "/r/{id}")
  @http("GET", "/v2/r/{id}")
  rpc Get(R) returns (R);
}
`)

	err := runPxf(t, "openapi", "--check", image)
	if err == nil {
		t.Fatal("expected an error for a second binding with no operation_id")
	}
	for _, want := range []string{"operation_id", "/v2/r/{id}", "S_Get"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestOpenAPIDuplicateOperationID covers the property that used to hold
// "by construction" and now has to be checked: ids are unique across
// the document. Two methods claiming one id previously rendered a
// document with colliding operationIds and no complaint.
func TestOpenAPIDuplicateOperationID(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message R { string id = 1; }

service S {
  @http("GET", "/a/{id}", operation_id = "sameName")
  rpc GetA(R) returns (R);

  @http("GET", "/b/{id}", operation_id = "sameName")
  rpc GetB(R) returns (R);
}
`)

	err := runPxf(t, "openapi", "--check", image)
	if err == nil {
		t.Fatal("expected an error for two operations claiming one operationId")
	}
	for _, want := range []string{"sameName", "unique", "t.S.GetA"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestOpenAPISingleBindingKeepsDerivedID guards the additive contract:
// the overwhelmingly common one-binding method still gets its derived
// <Service>_<Method> id with nothing authored.
func TestOpenAPISingleBindingKeepsDerivedID(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message R { string id = 1; }

service S {
  @http("GET", "/r/{id}")
  rpc Get(R) returns (R);
}
`)

	doc := string(renderOpenAPI(t, image))
	if !strings.Contains(doc, "operationId: S_Get") {
		t.Errorf("single-binding method lost its derived operationId:\n%s", doc)
	}
}

// TestBuildAdditionalBindingsFixture pins the corpus fixture's lowering:
// source order decides which use site is the rule and which are
// additional bindings.
func TestBuildAdditionalBindingsFixture(t *testing.T) {
	got := httpRules(t, buildImage(t, bindingsFixture))
	want := []string{
		"fixtures.bindings.Orders.GetOrder: GET /legacy/orders/{order_id}",
		"fixtures.bindings.Orders.GetOrder: GET /orders/{order_id}",
		"fixtures.bindings.Orders.GetOrder: GET /v2/orders/{order_id}",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rules:\ngot:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestCoverageCountsMethodsNotBindings pins that repetition does not
// inflate the #200 doc-coverage denominator: three bindings on one
// method still demand one documenting topic, not three.
func TestCoverageCountsMethodsNotBindings(t *testing.T) {
	image := filepath.Join(t.TempDir(), "image.binpb")
	if err := runPxf(t, "build", "-o", image, bindingsFixture); err != nil {
		t.Fatalf("pxf build: %v", err)
	}
	im, err := docpack.LoadImage(image)
	if err != nil {
		t.Fatal(err)
	}
	if got := im.HTTPMethods(); len(got) != 1 || got[0] != "fixtures.bindings.Orders.GetOrder" {
		t.Errorf("HTTPMethods() = %v, want exactly [fixtures.bindings.Orders.GetOrder]", got)
	}
}
