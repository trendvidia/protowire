// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
