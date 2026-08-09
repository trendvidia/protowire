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

// TestOpenAPINestedBindingSelfReferentialRequest pins the cycle guard at
// the request message itself. A request type that contains itself is on
// the descent chain from the first step, so `{self.x}` is refused by the
// same rule as the deeper `{self.self.x}` — §5.2 says a binding that
// descends *through* a self-referential type MUST be refused, and the
// depth at which the type recurs is not part of that rule.
func TestOpenAPINestedBindingSelfReferentialRequest(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Req {
  Req self = 1;
  string x = 2;
}

message Res { string ok = 1; }

service S {
  @http("PATCH", "/x/{self.x}")
  rpc Do(Req) returns (Res);
}
`)

	err := runPxf(t, "openapi", "--check", image)
	if err == nil {
		t.Fatal("expected a refusal for a self-referential request message, got a document")
	}
	for _, want := range []string{"recursive", "t.Req"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestOpenAPINestedBindingBodyKeepsContainerKeywords covers what the
// inlining must not lose. A container rendered as a `$ref` composes the
// field's own keywords beside it with `allOf`; inlined minus its bound
// leaf there is no reference left, so they have to land on the object
// itself. `x-validation` is the sharpest case — §#080 promises no rule
// ever disappears at the boundary.
func TestOpenAPINestedBindingBodyKeepsContainerKeywords(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";

message Shelf {
  string id = 1;
  string display_name = 2;
}

message UpdateShelfRequest {
  Shelf shelf = 1
    @description("the shelf being updated")
    @deprecated("use shelf_ref")
    @validate(populated(this));
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
	shelf, ok := props["shelf"].(map[string]any)
	if !ok {
		t.Fatalf("body lost the container of the bound leaf: %v", props)
	}
	if got := shelf["description"]; got != "the shelf being updated" {
		t.Errorf("the inlined container lost its @description: %v", shelf)
	}
	if shelf["deprecated"] != true || shelf["x-deprecated-reason"] != "use shelf_ref" {
		t.Errorf("the inlined container lost its @deprecated: %v", shelf)
	}
	if _, ok := shelf["x-validation"]; !ok {
		t.Errorf("the inlined container dropped a rule §#080 says is never dropped: %v", shelf)
	}
}

// TestOpenAPINestedBindingSensitiveContainer covers §6.7 across both
// halves of the binding. Flattening dissolves the reference that would
// have carried a sensitive container's marker, so the marker travels to
// what the flattening leaves behind — and the doc-emit minima follow it,
// which is why the member's @example must not survive.
func TestOpenAPINestedBindingSensitiveContainer(t *testing.T) {
	const decls = `
message Shelf {
  string id = 1;
  string display_name = 2 @example("main");
}

message Res { string ok = 1; }
`

	t.Run("query", func(t *testing.T) {
		image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";
`+decls+`
message GetShelfRequest {
  Shelf shelf = 1 @sensitive(class = "pii");
}

service S {
  @http("GET", "/shelves/{shelf.id}")
  rpc Get(GetShelfRequest) returns (Res);
}
`)
		// Only the operation: the `t.Shelf` *component* is not itself
		// sensitive — the marker sits on the reference to it — so its
		// own example stays where the reference can still carry one.
		doc := string(renderOpenAPI(t, image))
		ops, _, _ := strings.Cut(doc, "\ncomponents:")
		if strings.Contains(ops, "example: main") {
			t.Errorf("§6.7: an example survived the flattening out of a @sensitive container:\n%s", ops)
		}
		if n := strings.Count(ops, "x-sensitive: true"); n < 2 {
			t.Errorf("want the marker on both the path leaf and the flattened sibling, got %d:\n%s", n, ops)
		}
	})

	t.Run("body", func(t *testing.T) {
		image := buildTempSchema(t, `syntax = "proto3";
package t;

import "protowire/schema/v1/annotations.proto";
`+decls+`
message UpdateShelfRequest {
  Shelf shelf = 1 @sensitive(class = "pii");
}

service S {
  @http("PATCH", "/shelves/{shelf.id}")
  rpc Do(UpdateShelfRequest) returns (Res);
}
`)
		schema := bodySchema(t, image, "/shelves/{shelf.id}", "patch")
		props, _ := schema["properties"].(map[string]any)
		shelf, ok := props["shelf"].(map[string]any)
		if !ok {
			t.Fatalf("body lost the container of the bound leaf: %v", props)
		}
		if shelf["x-sensitive"] != true || shelf["x-sensitive-class"] != "pii" {
			t.Errorf("the inlined container lost its §6.7 markers: %v", shelf)
		}
		member, _ := shelf["properties"].(map[string]any)["display_name"].(map[string]any)
		if _, ok := member["example"]; ok {
			t.Errorf("§6.7: an example survived inside a @sensitive container: %v", member)
		}
		if member["x-sensitive"] != true {
			t.Errorf("the marker did not reach the inlined member: %v", member)
		}
	})
}
