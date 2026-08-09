// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"encoding/json"
	"os"
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

// TestOpenAPINestedBindingExcludesContainer pins the #218 limitation as
// behaviour rather than an accident: the bound leaf's container drops
// out of the remaining-field binding, while its scalar siblings at the
// top level still bind.
func TestOpenAPINestedBindingExcludesContainer(t *testing.T) {
	doc := string(renderOpenAPI(t, buildTempImage(t, templatesFixture)))

	if !strings.Contains(doc, "name: include_archived") {
		t.Error("a top-level sibling of the bound container lost its query parameter")
	}
	if strings.Contains(doc, "name: shelf.display_name") {
		t.Error("the renderer flattened a nested sibling; #218 says it does not (yet)")
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
