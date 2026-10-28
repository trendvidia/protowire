// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	openapiFixtureDir = "../../testdata/openapi"
	openapiSchema     = openapiFixtureDir + "/store.proto"
	openapiConfig     = openapiFixtureDir + "/protowire.openapi.textproto"
)

// storeImage builds the store fixture into a lowered image once per
// call, in a temp dir so config discovery never sees the fixture dir.
func storeImage(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "store.binpb")
	if err := runPxf(t, "build", "-o", out, openapiSchema); err != nil {
		t.Fatalf("building store image: %v", err)
	}
	return out
}

// renderOpenAPI runs `pxf openapi` and returns the document bytes.
func renderOpenAPI(t *testing.T, args ...string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "openapi.out")
	full := append([]string{"openapi", "-o", out}, args...)
	if err := runPxf(t, full...); err != nil {
		t.Fatalf("pxf %s: %v", strings.Join(full, " "), err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestOpenAPISchemaMapping pins the §#080 schema half on the rendered
// YAML: keyword mapping, aliases, sensitivity minima, presence.
func TestOpenAPISchemaMapping(t *testing.T) {
	doc := string(renderOpenAPI(t, "--config", openapiConfig, storeImage(t)))

	for _, want := range []string{
		// Mappable @validate shapes → native keywords.
		"pattern: ^[^@]+@[^@]+$",
		"minLength: 2",
		"maxLength: 32",
		"minimum: 18",
		"exclusiveMaximum: 130",
		"maxItems: 10",
		// In-list rule → enum, with @default / @example beside it.
		"default: US",
		"example: CA",
		// Non-mappable rules carried through, never dropped.
		"x-validation:",
		`rule: this != "admin"`,
		"rule: is_zip(this)",
		// §8.2 aliases as named components, chained via allOf.
		"demo.store.Email:",
		"demo.store.Handle:",
		"$ref: '#/components/schemas/demo.store.ShortName'",
		// Presence: @required array, optional/wrapper nullability.
		"required:\n        - email",
		"- \"null\"",
		// WKT and 64-bit mappings.
		"format: date-time",
		"format: int64",
		// §6.7 sensitivity marker.
		"x-sensitive: true",
		"x-sensitive-class: store.pii",
		// Deprecation with reason.
		"x-deprecated-reason: use handle instead",
		// §7 report closure present for the derived default response.
		"protowire.schema.v1.Report:",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document is missing %q", want)
		}
	}

	// §6.7 doc-emit minimum: the @example on the sensitive field MUST
	// NOT reach the document.
	if strings.Contains(doc, "000-00-0000") {
		t.Error("sensitive field's @example leaked into the document")
	}
}

// TestOpenAPIOperations pins the §5.2 operation surface and the
// derived-response rule (GH #177).
func TestOpenAPIOperations(t *testing.T) {
	doc := string(renderOpenAPI(t, "--config", openapiConfig, storeImage(t)))

	for _, want := range []string{
		"/customers/{customer_id}:",
		"operationId: Store_GetCustomer",     // derived <Service>_<Method>
		"operationId: createCustomer",        // explicit override
		"summary: Fetch one customer by id.", // first sentence of @description
		"in: path",
		"in: query",
		"name: include_closed",
		"- bearerAuth: []",
		"bearerFormat: JWT",
		// Derived responses: 200 + default report, error codes surfaced.
		`"200":`,
		"default:",
		"x-error-codes:",
		"- protowire.required",
		"- store.email.malformed",
		"- store.zip.invalid",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document is missing %q", want)
		}
	}

	// The DELETE with an Empty return renders 200 with no content.
	if !strings.Contains(doc, "operationId: Store_DeleteCustomer") {
		t.Error("missing DELETE operation")
	}
}

// TestOpenAPIAudienceFilter pins Gap 2 filtering: internal elements are
// absent at the default (public) tier and present at --audience
// internal; descriptors are never rewritten, so the image is reusable.
func TestOpenAPIAudienceFilter(t *testing.T) {
	img := storeImage(t)

	public := string(renderOpenAPI(t, "--config", openapiConfig, img))
	if strings.Contains(public, "/audit") || strings.Contains(public, "AuditRecord") {
		t.Error("internal elements leaked into the public document")
	}

	internal := string(renderOpenAPI(t, "--config", openapiConfig, "--audience", "internal", img))
	for _, want := range []string{"/audit:", "demo.store.AuditRecord:", "operationId: Store_ListAudit"} {
		if !strings.Contains(internal, want) {
			t.Errorf("--audience internal document is missing %q", want)
		}
	}
}

// TestOpenAPITransitiveInconsistency pins the Gap 2 hard error: a
// public operation whose closure reaches an internal element fails
// generation naming both ends.
func TestOpenAPITransitiveInconsistency(t *testing.T) {
	err := runPxf(t, "openapi", "--check",
		"--config", openapiFixtureDir+"/inconsistent.textproto", storeImage(t))
	if err == nil {
		t.Fatal("inconsistent audience assignment was accepted")
	}
	// The first violated edge is the response message reaching the
	// restricted record; the error names both ends.
	for _, want := range []string{"demo.store.ListAuditResponse", "demo.store.AuditRecord", "AUDIENCE_PUBLIC", "AUDIENCE_INTERNAL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestOpenAPIPackContributesTier pins the doc-pack half of Gap 2: a
// partner-tier topic anchored to Customer restricts the element, and
// the public GetCustomer closure fails naming the topic.
func TestOpenAPIPackContributesTier(t *testing.T) {
	img := storeImage(t)
	pack := filepath.Join(t.TempDir(), "docs.binpb")
	if err := runPxf(t, "docs", "build", "-o", pack, "--image", img,
		openapiFixtureDir+"/topics/customer.pxf"); err != nil {
		t.Fatalf("building doc pack: %v", err)
	}
	err := runPxf(t, "openapi", "--check", "--config", openapiConfig, "--pack", pack, img)
	if err == nil {
		t.Fatal("pack-contributed tier was ignored")
	}
	for _, want := range []string{"demo.store.Customer", "AUDIENCE_PARTNER", "store.customer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}

	// The consistency check is global, not tier-relative: even at
	// --audience partner the public GetCustomer operation still
	// references the now-partner Customer, and generation refuses
	// until the config raises the operation's tier too.
	if err := runPxf(t, "openapi", "--check", "--config", openapiConfig,
		"--pack", pack, "--audience", "partner", img); err == nil {
		t.Fatal("public operations referencing a partner element should still fail")
	}
}

// TestOpenAPIByteStable: two runs, identical bytes, both formats.
func TestOpenAPIByteStable(t *testing.T) {
	img := storeImage(t)
	for _, format := range []string{"yaml", "json"} {
		a := renderOpenAPI(t, "--config", openapiConfig, "--format", format, img)
		b := renderOpenAPI(t, "--config", openapiConfig, "--format", format, img)
		if !bytes.Equal(a, b) {
			t.Errorf("%s output is not byte-stable across runs", format)
		}
	}
}

// TestOpenAPIJSONFormat: --format json emits valid JSON with the same
// top-level shape, and -o *.json infers the format.
func TestOpenAPIJSONFormat(t *testing.T) {
	img := storeImage(t)
	raw := renderOpenAPI(t, "--config", openapiConfig, "--format", "json", img)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("--format json emitted invalid JSON: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v", doc["openapi"])
	}
	if _, ok := doc["paths"].(map[string]any)["/customers"]; !ok {
		t.Error("JSON document is missing /customers")
	}

	out := filepath.Join(t.TempDir(), "api.json")
	if err := runPxf(t, "openapi", "-o", out, "--config", openapiConfig, img); err != nil {
		t.Fatal(err)
	}
	inferred, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(inferred) {
		t.Error("-o *.json did not infer JSON format")
	}
}

// TestOpenAPICheckWritesNoOutput pins --check as diagnostics-only.
func TestOpenAPICheckWritesNoOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := runPxf(t, "openapi", "--check", "-o", out, "--config", openapiConfig, storeImage(t)); err != nil {
		t.Fatalf("--check over valid image: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("--check wrote an output file (stat err: %v)", err)
	}
}

// TestOpenAPIDanglingScheme pins the security guard: a use site naming
// an undefined scheme is a generation error, not a dangling $ref.
func TestOpenAPIDanglingScheme(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "protowire.openapi.textproto")
	if err := os.WriteFile(cfg, []byte("info { title: \"t\" version: \"1\" }\naudiences { fqn_glob: \"*Audit*\" tier: \"internal\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runPxf(t, "openapi", "--check", "--config", cfg, storeImage(t))
	if err == nil || !strings.Contains(err.Error(), "bearerAuth") {
		t.Fatalf("undefined security scheme not diagnosed: %v", err)
	}
}

// A field is nullable in the generated document exactly where PXF
// accepts null: a singular field with explicit presence (decided by the
// owner on #380). Each case is the property's schema in a body type,
// across proto3, proto2 and editions.
func TestOpenAPINullable(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"n3.proto": `syntax = "proto3";
package nn.v1;
import "protowire/schema/v1/annotations.proto";
message Inner { int32 x = 1; }
message M3 {
  int32 plain = 1;
  optional int32 opt = 2;
  oneof k { string member = 3; }
  Inner msg = 4;
  repeated int32 list = 5;
  map<string, int32> dict = 6;
}
service S {
  @http("POST", "/m3")
  rpc Put3(M3) returns (M3);
}
`,
		"n2.proto": `syntax = "proto2";
package nn.v2;
import "protowire/schema/v1/annotations.proto";
message M2 {
  optional int32 opt = 1;
  required int32 req = 2;
}
service S2 {
  @http("POST", "/m2")
  rpc Put2(M2) returns (M2);
}
`,
		"ne.proto": `edition = "2023";
package nn.ed;
import "protowire/schema/v1/annotations.proto";
message ME {
  int32 explicit_scalar = 1;
  int32 implicit_scalar = 2 [features.field_presence = IMPLICIT];
}
service SE {
  @http("POST", "/me")
  rpc PutE(ME) returns (ME);
}
`,
	}
	var paths []string
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	image := filepath.Join(t.TempDir(), "image.binpb")
	if err := runPxf(t, append([]string{"build", "-o", image}, paths...)...); err != nil {
		t.Fatalf("build: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(renderOpenAPI(t, "--config", openapiConfig, image), &doc); err != nil {
		t.Fatal(err)
	}
	nullable := func(s map[string]any) bool {
		if ty, ok := s["type"].([]any); ok {
			return slices.Contains(ty, any("null"))
		}
		if alts, ok := s["anyOf"].([]any); ok {
			for _, a := range alts {
				if m, ok := a.(map[string]any); ok && m["type"] == "null" {
					return true
				}
			}
		}
		return false
	}
	for _, c := range []struct {
		schema, prop string
		want         bool
	}{
		{"nn.v1.M3", "plain", false},
		{"nn.v1.M3", "opt", true},
		{"nn.v1.M3", "member", true},
		{"nn.v1.M3", "msg", true},
		{"nn.v1.M3", "list", false},
		{"nn.v1.M3", "dict", false},
		{"nn.v2.M2", "opt", true},
		{"nn.v2.M2", "req", false},
		{"nn.ed.ME", "explicit_scalar", true},
		{"nn.ed.ME", "implicit_scalar", false},
	} {
		s, ok := doc.Components.Schemas[c.schema].Properties[c.prop]
		if !ok {
			t.Errorf("%s.%s: not in the document", c.schema, c.prop)
			continue
		}
		if got := nullable(s); got != c.want {
			t.Errorf("%s.%s: nullable = %v, want %v; schema %v", c.schema, c.prop, got, c.want, s)
		}
	}
}

// The generated `_null` mask is left out of the document; a hand-declared
// `_null`, at any other number, is the author's field and stays (decided
// by the owner on #404).
func TestOpenAPIGeneratedNullMaskHidden(t *testing.T) {
	dir := t.TempDir()
	src := `syntax = "proto3";
package nm.v1;
import "google/protobuf/field_mask.proto";
import "google/protobuf/wrappers.proto";
import "protowire/schema/v1/annotations.proto";
message Gen { google.protobuf.Int32Value w = 1; }
message Hand { google.protobuf.Int32Value w = 1; google.protobuf.FieldMask _null = 15; }
service S {
  @http("POST", "/gen")
  rpc PutGen(Gen) returns (Hand);
}
`
	p := filepath.Join(dir, "nm.proto")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(t.TempDir(), "image.binpb")
	if err := runPxf(t, "build", "-o", image, p); err != nil {
		t.Fatalf("build: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(renderOpenAPI(t, "--config", openapiConfig, image), &doc); err != nil {
		t.Fatal(err)
	}
	schemas := doc.Components.Schemas
	if _, ok := schemas["nm.v1.Gen"].Properties["_null"]; ok {
		t.Error("nm.v1.Gen lists the generated _null mask")
	}
	if _, ok := schemas["nm.v1.Gen"].Properties["w"]; !ok {
		t.Error("nm.v1.Gen lost its own field w")
	}
	if _, ok := schemas["nm.v1.Hand"].Properties["_null"]; !ok {
		t.Error("nm.v1.Hand's hand-declared _null was dropped")
	}
}
