// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const httpFixture = fixtureDir + "/21_http_operation.proto"

// httpRules reads an image the way a REST binder does — build the file
// registry with stock protodesc, then ask each method's options for the
// google.api.http extension — and returns "FQN: VERB path[ body]" lines,
// sorted. This is the exact call path connect vanguard and grpc-gateway
// take, so a rule these tests can see is a rule they can bind (#213).
func httpRules(t *testing.T, image []byte) []string {
	t.Helper()
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(image, fds); err != nil {
		t.Fatalf("image is not a FileDescriptorSet: %v", err)
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("stock protodesc rejects the lowered image: %v", err)
	}

	var out []string
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			methods := svcs.Get(i).Methods()
			for j := range methods.Len() {
				md := methods.Get(j)
				rule, _ := proto.GetExtension(md.Options(), annotations.E_Http).(*annotations.HttpRule)
				if rule == nil {
					continue
				}
				out = append(out, fmt.Sprintf("%s: %s", md.FullName(), ruleString(rule)))
				for _, extra := range rule.GetAdditionalBindings() {
					out = append(out, fmt.Sprintf("%s: %s", md.FullName(), ruleString(extra)))
				}
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}

func ruleString(r *annotations.HttpRule) string {
	var verb, path string
	switch p := r.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		verb, path = "GET", p.Get
	case *annotations.HttpRule_Put:
		verb, path = "PUT", p.Put
	case *annotations.HttpRule_Post:
		verb, path = "POST", p.Post
	case *annotations.HttpRule_Delete:
		verb, path = "DELETE", p.Delete
	case *annotations.HttpRule_Patch:
		verb, path = "PATCH", p.Patch
	case *annotations.HttpRule_Custom:
		verb, path = p.Custom.GetKind(), p.Custom.GetPath()
	default:
		return "<no pattern>"
	}
	if body := r.GetBody(); body != "" {
		return fmt.Sprintf("%s %s body=%s", verb, path, body)
	}
	return fmt.Sprintf("%s %s", verb, path)
}

// TestBuildEmitsGoogleAPIHTTP is the #213 acceptance path: @http lowers
// to the standard extension as well as the carrier, so an image built
// by `pxf build` binds routes in stock tooling that has never heard of
// protowire.
func TestBuildEmitsGoogleAPIHTTP(t *testing.T) {
	got := httpRules(t, buildImage(t, httpFixture))
	want := []string{
		"fixtures.http.Orders.CreateOrder: POST /orders body=*",
		"fixtures.http.Orders.GetOrder: GET /orders/{order_id}",
		"fixtures.http.Orders.ListOrders: GET /orders",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("google.api.http rules:\ngot:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestBuildGoogleAPIHTTPOptOut verifies --google-api-http=false drops
// the standard extension and leaves the annotation carrier — and every
// other byte of the image — alone.
func TestBuildGoogleAPIHTTPOptOut(t *testing.T) {
	withRules := buildImage(t, httpFixture)
	without := buildImage(t, "--google-api-http=false", httpFixture)

	if rules := httpRules(t, without); len(rules) != 0 {
		t.Errorf("opted out, but the image still carries %d rule(s): %v", len(rules), rules)
	}
	if bytes.Equal(withRules, without) {
		t.Error("--google-api-http=false produced a byte-identical image")
	}

	// The carrier is untouched by the opt-out: the annotation still
	// lowers, so `pxf openapi` and `pxf docs` see exactly what they did.
	for _, image := range [][]byte{withRules, without} {
		if n := countAnnotationCarriers(t, image); n != 3 {
			t.Errorf("expected 3 method annotation carriers, got %d", n)
		}
	}
}

// countAnnotationCarriers counts methods carrying the 50400 annotation
// list. It scans field numbers rather than decoding, so the count needs
// no carrier schema — the same read a stock consumer performs.
func countAnnotationCarriers(t *testing.T, image []byte) int {
	t.Helper()
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(image, fds); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, f := range fds.GetFile() {
		for _, s := range f.GetService() {
			for _, m := range s.GetMethod() {
				if m.GetOptions() == nil {
					continue
				}
				raw, err := proto.Marshal(m.GetOptions())
				if err != nil {
					t.Fatal(err)
				}
				if hasField(raw, annotationCarrierField) {
					n++
				}
			}
		}
	}
	return n
}

// annotationCarrierField is the RFC-001 §8.1 method_annotations carrier.
const annotationCarrierField = protowire.Number(50400)

// hasField reports whether a field number appears at the top level of an
// encoded message.
func hasField(b []byte, want protowire.Number) bool {
	for len(b) > 0 {
		num, kind, tagLen := protowire.ConsumeTag(b)
		if tagLen < 0 {
			return false
		}
		b = b[tagLen:]
		valLen := protowire.ConsumeFieldValue(num, kind, b)
		if valLen < 0 {
			return false
		}
		b = b[valLen:]
		if num == want {
			return true
		}
	}
	return false
}

// TestBuildGoogleAPIHTTPByteStable pins that emission does not cost the
// image its byte-stability across runs — the acceptance bar for caching
// it as a build artifact (#164).
func TestBuildGoogleAPIHTTPByteStable(t *testing.T) {
	if !bytes.Equal(buildImage(t, httpFixture), buildImage(t, httpFixture)) {
		t.Error("image with google.api.http rules is not byte-stable across runs")
	}
}

// TestHTTPRulesMatchOpenAPIPaths is the invariant #213 exists for: the
// document `pxf openapi` renders and the routes an image actually binds
// must describe the same surface. A spec that promises what nothing
// serves is worse than no spec, and the two now derive from one
// annotation, so drift between them is a bug in one of the two readers.
func TestHTTPRulesMatchOpenAPIPaths(t *testing.T) {
	image := storeImage(t)
	raw, err := os.ReadFile(image)
	if err != nil {
		t.Fatal(err)
	}

	fromImage := map[string]bool{}
	for _, line := range httpRules(t, raw) {
		_, rule, _ := strings.Cut(line, ": ")
		verb, rest, _ := strings.Cut(rule, " ")
		path, _, _ := strings.Cut(rest, " body=")
		fromImage[verb+" "+path] = true
	}

	// Rendered at the most permissive tier: audience filtering legitimately
	// omits operations from a *published* document (the store fixture tiers
	// its audit endpoint internal), and that is a publishing decision, not
	// drift. At `internal` nothing is filtered, so the two sets must match
	// exactly.
	fromDoc := renderedOperations(t, image, "internal")
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

	// The published document is a subset of what is bound, never a
	// superset: a route the reader is told about must exist.
	for op := range renderedOperations(t, image, "") {
		if !fromImage[op] {
			t.Errorf("the public document promises %q, which the image does not bind", op)
		}
	}
}

// renderedOperations returns the "VERB path" set of a rendered document
// at the given audience tier ("" = public).
func renderedOperations(t *testing.T, image, audience string) map[string]bool {
	t.Helper()
	args := []string{"--config", openapiConfig, "--format", "json"}
	if audience != "" {
		args = append(args, "--audience", audience)
	}
	doc := renderOpenAPI(t, append(args, image)...)

	var rendered struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(doc, &rendered); err != nil {
		t.Fatalf("parsing rendered document: %v", err)
	}
	out := map[string]bool{}
	for path, verbs := range rendered.Paths {
		for verb := range verbs {
			out[strings.ToUpper(verb)+" "+path] = true
		}
	}
	return out
}

// TestBuildUnboundPathTemplateRejected pins the compile-error side: a
// path template naming no request field is rejected, so an unbindable
// route never reaches an image. The corpus fixture is the normative
// statement; this asserts the CLI surfaces it.
func TestBuildUnboundPathTemplateRejected(t *testing.T) {
	fixture := filepath.Join(fixtureDir, "invalid", "http_unbound_template.proto")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("corpus fixture missing: %v", err)
	}
	if err := runPxf(t, "build", "--check", fixture); err == nil {
		t.Error("expected --check to reject an @http path template that binds no field")
	}
}
