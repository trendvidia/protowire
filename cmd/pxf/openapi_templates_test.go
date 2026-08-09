// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const templatesFixture = fixtureDir + "/23_http_template_paths.proto"

// TestOpenAPIDottedAndSubPathTemplates is the #217 acceptance path: the
// renderer accepts every template shape the compiler does, so a schema
// can no longer compile and bind and then fail to document.
func TestOpenAPIDottedAndSubPathTemplates(t *testing.T) {
	image := buildTempImage(t, templatesFixture)
	doc := string(renderOpenAPI(t, image))

	for _, want := range []string{
		"/shelves/{shelf.id}:", // dotted variable carries through
		"name: shelf.id",
		"/named/{name}:", // sub-path constraint dropped from the key
		"name: name",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered document is missing %q:\n%s", want, doc)
		}
	}

	// The constraint belongs to the routing skeleton, not to OpenAPI: it
	// must not leak into a path key OpenAPI cannot parse.
	if strings.Contains(doc, "shelves/*}") {
		t.Errorf("sub-path constraint leaked into the document:\n%s", doc)
	}
}

// TestOpenAPINestedBindingKeepsSiblings is #218's acceptance path: a
// bound leaf takes only itself out of the remaining-field binding, so
// its siblings still bind — nested ones under dotted names, exactly as
// an HttpRule consumer would place them.
//
// This test asserted the opposite until #218 closed: the drop was
// pinned as behaviour so that fixing it would flip a test rather than
// change a document quietly.
func TestOpenAPINestedBindingKeepsSiblings(t *testing.T) {
	doc := string(renderOpenAPI(t, buildTempImage(t, templatesFixture)))

	for _, want := range []string{
		"name: include_archived",   // top-level sibling of the container
		"name: shelf.display_name", // sibling of the bound leaf
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered document is missing query parameter %q:\n%s", want, doc)
		}
	}
	// The bound leaf itself binds to the path, and must not also appear
	// as a query parameter.
	if strings.Contains(doc, "name: shelf.id\n          in: query") {
		t.Error("the bound leaf was rendered as a query parameter as well as a path one")
	}
}

// TestHTTPRulesMatchOpenAPIPathsTemplates is the #213 parity invariant
// over the template shapes: what the image binds and what the document
// describes must be the same surface. Sub-path constraints are compared
// in their OpenAPI spelling, since that normalisation is the one place
// the two forms legitimately differ (§5.2).
func TestHTTPRulesMatchOpenAPIPathsTemplates(t *testing.T) {
	image := buildTempImage(t, templatesFixture)
	raw, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}

	fromImage := map[string]bool{}
	for _, line := range httpRules(t, raw) {
		_, rule, _ := strings.Cut(line, ": ")
		verb, rest, _ := strings.Cut(rule, " ")
		path, _, _ := strings.Cut(rest, " body=")
		fromImage[verb+" "+openAPIPathSpelling(path)] = true
	}

	doc := renderOpenAPI(t, "--format", "json", image)
	var rendered struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(doc, &rendered); err != nil {
		t.Fatalf("parsing rendered document: %v", err)
	}
	fromDoc := map[string]bool{}
	for path, verbs := range rendered.Paths {
		for verb := range verbs {
			fromDoc[strings.ToUpper(verb)+" "+path] = true
		}
	}

	if len(fromDoc) == 0 {
		t.Fatal("the rendered document describes no operations; the comparison would be vacuous")
	}
	for op := range fromDoc {
		if !fromImage[op] {
			t.Errorf("OpenAPI documents %q, but the image binds no such route", op)
		}
	}
	for op := range fromImage {
		if !fromDoc[op] {
			t.Errorf("the image binds %q, but the OpenAPI document does not describe it", op)
		}
	}
}

// renderOpenAPIError runs `pxf openapi` expecting it to fail, and
// returns the error text. The residual #217 edges below are refusals,
// so they need the failing half of renderOpenAPI's contract.
func renderOpenAPIError(t *testing.T, args ...string) string {
	t.Helper()
	full := append([]string{"openapi", "-o", filepath.Join(t.TempDir(), "openapi.out")}, args...)
	err := runPxf(t, full...)
	if err == nil {
		t.Fatalf("pxf %s: rendered a document, want an error", strings.Join(full, " "))
	}
	return err.Error()
}

// TestOpenAPISubPathConstraintCollision pins the first open edge of
// #217 (RFC-001 §5.2, "What the grammar above still leaves unsettled").
// `{name=shelves/*}` and `{name=shelves/*/books/*}` are distinct routes
// to a binder — the canonical nested-resource shape — but one OpenAPI
// path key, so the pair is refused. The refusal is the behaviour under
// test; what this asserts is that the diagnostic names both source
// paths, since the key it collided on appears in no schema.
func TestOpenAPISubPathConstraintCollision(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;
import "protowire/schema/v1/annotations.proto";

message GetReq { string name = 1; }
message Shelf { string id = 1; }

service S {
  @http("GET", "/v1/{name=shelves/*}")
  rpc GetShelf(GetReq) returns (Shelf);

  @http("GET", "/v1/{name=shelves/*/books/*}")
  rpc GetBook(GetReq) returns (Shelf);
}
`)
	msg := renderOpenAPIError(t, image)

	for _, want := range []string{
		"/v1/{name=shelves/*/books/*}", // the path that was refused
		"/v1/{name}",                   // the key both normalised to
		"/v1/{name=shelves/*}",         // the path that claimed it first
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("collision diagnostic does not name %q: %s", want, msg)
		}
	}
}

// TestOpenAPIMessageTypedLeafRefused pins the second open edge of #217:
// the grammar constrains interior components, so a message-typed leaf
// compiles and binds and then fails to render. This is the residual
// compile-and-bind-then-refuse case, asserted so that closing it at
// either end flips a test rather than passing unnoticed.
func TestOpenAPIMessageTypedLeafRefused(t *testing.T) {
	image := buildTempSchema(t, `syntax = "proto3";
package t;
import "protowire/schema/v1/annotations.proto";

message Inner { string v = 1; }
message Mid { Inner inner = 1; }
message Req { Mid mid = 1; }
message Res { string ok = 1; }

service S {
  @http("GET", "/things/{mid.inner}")
  rpc Get(Req) returns (Res);
}
`)
	if msg := renderOpenAPIError(t, image); !strings.Contains(msg, "message-typed") {
		t.Errorf("want the message-typed refusal, got: %s", msg)
	}
}

// TestOpenAPISubPathConstraintIsNotRepresented pins the third open edge
// of #217: dropping the constraint from the path key drops it from the
// document, so `name` renders as an unconstrained string. Recorded as a
// known loss — a reader of the document cannot tell that the value must
// be a `shelves/…` sub-path.
func TestOpenAPISubPathConstraintIsNotRepresented(t *testing.T) {
	doc := string(renderOpenAPI(t, "--format", "json", buildTempImage(t, templatesFixture)))

	var rendered struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name   string         `json:"name"`
				Schema map[string]any `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(doc), &rendered); err != nil {
		t.Fatalf("parsing rendered document: %v", err)
	}
	op, ok := rendered.Paths["/named/{name}"]["get"]
	if !ok {
		t.Fatalf("no GET /named/{name} in the rendered document:\n%s", doc)
	}
	for _, p := range op.Parameters {
		if p.Name != "name" {
			continue
		}
		if len(p.Schema) != 1 || p.Schema["type"] != "string" {
			t.Errorf("the sub-path constraint now has a representation (%v) — "+
				"#217's third open edge closed; update RFC-001 §5.2 and this test", p.Schema)
		}
		return
	}
	t.Errorf("GET /named/{name} declares no `name` parameter:\n%s", doc)
}

// openAPIPathSpelling drops a sub-path constraint from each template
// segment, mirroring the renderer's path-key normalisation.
func openAPIPathSpelling(path string) string {
	var b strings.Builder
	for {
		open := strings.IndexByte(path, '{')
		if open < 0 {
			b.WriteString(path)
			return b.String()
		}
		close := strings.IndexByte(path[open:], '}')
		if close < 0 {
			b.WriteString(path)
			return b.String()
		}
		name := path[open+1 : open+close]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		b.WriteString(path[:open] + "{" + name + "}")
		path = path[open+close+1:]
	}
}
