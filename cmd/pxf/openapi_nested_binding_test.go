// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// bodySchema returns the request-body schema of one operation, decoded.
func bodySchema(t *testing.T, image, path, verb string) map[string]any {
	t.Helper()
	doc := renderOpenAPI(t, "--format", "json", image)

	var rendered struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema map[string]any `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(doc, &rendered); err != nil {
		t.Fatalf("parsing rendered document: %v", err)
	}
	op, ok := rendered.Paths[path][verb]
	if !ok {
		t.Fatalf("no %s %s in the rendered document:\n%s", strings.ToUpper(verb), path, doc)
	}
	return op.RequestBody.Content["application/json"].Schema
}

// TestOpenAPINestedBindingBodyKeepsSiblings covers #218 on the body
// side: `body: "*"` carries every field the path did not bind, so a
// container with a bound leaf is inlined *minus that leaf* rather than
// dropped whole.
func TestOpenAPINestedBindingBodyKeepsSiblings(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Shelf {
  string id = 1;
  string display_name = 2;
}

message UpdateShelfRequest {
  Shelf shelf = 1;
  bool dry_run = 2;
}

message Res { string ok = 1; }

service S {
  @http("PATCH", "/shelves/{shelf.id}")
  rpc UpdateShelf(UpdateShelfRequest) returns (Res);
}
`)

	schema := bodySchema(t, image, "/shelves/{shelf.id}", "patch")
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("body schema has no properties: %v", schema)
	}

	if _, ok := props["dry_run"]; !ok {
		t.Error("body lost the top-level sibling dry_run")
	}
	shelf, ok := props["shelf"].(map[string]any)
	if !ok {
		t.Fatalf("body lost the container of the bound leaf: %v", props)
	}
	shelfProps, _ := shelf["properties"].(map[string]any)
	if _, ok := shelfProps["display_name"]; !ok {
		t.Errorf("body lost the bound leaf's sibling display_name: %v", shelf)
	}
	if _, ok := shelfProps["id"]; ok {
		t.Errorf("the path-bound leaf is also in the body: %v", shelf)
	}
}

// TestOpenAPINestedBindingDeep proves the descent is recursive rather
// than one level of special case.
func TestOpenAPINestedBindingDeep(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Inner {
  string id = 1;
  string note = 2;
}

message Mid {
  Inner inner = 1;
  string label = 2;
}

message Req {
  Mid mid = 1;
  bool flag = 2;
}

message Res { string ok = 1; }

service S {
  @http("GET", "/things/{mid.inner.id}")
  rpc Get(Req) returns (Res);
}
`)

	doc := string(renderOpenAPI(t, image))
	for _, want := range []string{
		"name: mid.inner.id",   // three components deep, in the path
		"name: mid.inner.note", // its sibling, still bound
		"name: mid.label",      // a sibling one level up
		"name: flag",           // and at the top
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered document is missing %q:\n%s", want, doc)
		}
	}
}

// TestOpenAPINestedBindingCycle pins the guard: a binding that descends
// *through* a self-referential type would flatten forever, so the
// renderer refuses it by name instead of recursing until the stack ends.
//
// The shallower `{node.id}` does not reach this guard — `parent` has
// nothing bound below it, so the pre-existing message-typed refusal
// catches it first (see TestOpenAPINestedBindingRefusesUnboundMessage).
func TestOpenAPINestedBindingCycle(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Node {
  string id = 1;
  Node parent = 2;
}

message Req {
  Node node = 1;
}

message Res { string ok = 1; }

service S {
  @http("GET", "/nodes/{node.parent.id}")
  rpc Get(Req) returns (Res);
}
`)

	err := runPxf(t, "openapi", "--check", image)
	if err == nil {
		t.Fatal("expected a refusal for a recursive container, got a document")
	}
	for _, want := range []string{"recursive", "t.Node"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestOpenAPINestedBindingRefusesUnboundMessage pins the edge the
// flattening deliberately does not cross. Descending into a container
// exposes its own message-typed members to the query binding, and one
// with nothing bound below it hits the pre-existing refusal — the
// renderer flattens where a bound leaf forces it to, not everywhere.
//
// Before #218 this rendered, because the whole container was dropped
// and its members were never examined. Nothing released behaved that
// way: nested bindings only became renderable in #217, in this same
// unreleased version.
func TestOpenAPINestedBindingRefusesUnboundMessage(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Deep { string v = 1; }

message Shelf {
  string id = 1;
  Deep deep = 2;
}

message Req { Shelf shelf = 1; }
message Res { string ok = 1; }

service S {
  @http("GET", "/shelves/{shelf.id}")
  rpc Get(Req) returns (Res);
}
`)

	err := runPxf(t, "openapi", "--check", image)
	if err == nil {
		t.Fatal("expected the message-typed refusal for an unbound nested message")
	}
	if !strings.Contains(err.Error(), "message-typed") {
		t.Errorf("want the message-typed refusal, got: %v", err)
	}
}
