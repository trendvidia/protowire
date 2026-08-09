// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// httpRuleBindings returns one method's rule as the primary pattern plus
// its additional bindings, in wire order. httpRules deliberately
// flattens and sorts both into one set — good for "is this route
// bindable at all", useless for the §5.2 property that source order
// decides which use site is the rule (#215).
func httpRuleBindings(t *testing.T, image []byte, method string) (primary string, additional []string) {
	t.Helper()
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(image, fds); err != nil {
		t.Fatalf("image is not a FileDescriptorSet: %v", err)
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("stock protodesc rejects the lowered image: %v", err)
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(method))
	if err != nil {
		t.Fatalf("looking up %s: %v", method, err)
	}
	md, ok := d.(protoreflect.MethodDescriptor)
	if !ok {
		t.Fatalf("%s is %T, not a method", method, d)
	}
	rule, _ := proto.GetExtension(md.Options(), annotations.E_Http).(*annotations.HttpRule)
	if rule == nil {
		t.Fatalf("%s carries no google.api.http rule", method)
	}
	for _, extra := range rule.GetAdditionalBindings() {
		additional = append(additional, ruleString(extra))
	}
	return ruleString(rule), additional
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
	image := buildImage(t, httpFixture)
	got := httpRules(t, image)
	want := []string{
		"fixtures.http.Orders.CreateOrder: POST /orders body=*",
		"fixtures.http.Orders.GetOrder: GET /orders/{order_id}",
		"fixtures.http.Orders.ListOrders: GET /orders",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("google.api.http rules:\ngot:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	// The rule rides in unknown-field bytes, so the lowered file gains no
	// dependency on google/api/annotations.proto (RFC-001 §5.2,
	// STABILITY.md). protodesc.NewFiles above would fail outright on a
	// dangling one; this pins that none is written at all, because an
	// image obliged to carry the googleapis files is no longer
	// self-contained.
	for _, dep := range fileDeps(t, image, filepath.Base(httpFixture)) {
		if strings.HasPrefix(dep, "google/api/") {
			t.Errorf("lowering added the import %q; the option must ride in unknown-field bytes", dep)
		}
	}
}

// fileDeps returns the dependency list the image records for one file.
func fileDeps(t *testing.T, image []byte, name string) []string {
	t.Helper()
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(image, fds); err != nil {
		t.Fatal(err)
	}
	for _, f := range fds.GetFile() {
		if f.GetName() == name {
			return f.GetDependency()
		}
	}
	t.Fatalf("image has no file %q", name)
	return nil
}

// TestBuildGoogleAPIHTTPOptOut verifies --google-api-http=false drops
// the standard extension and leaves the rest of the image alone.
//
// "The rest" is asserted rather than assumed: strip field 72295728 from
// every method of the emitting image and the two must be equal. Merely
// asserting that the images differ would still pass if the opt-out path
// also perturbed, say, the source-map carrier — which is the shape of
// promise STABILITY.md makes for the flag, and the reason it is worth a
// comparison rather than a count.
func TestBuildGoogleAPIHTTPOptOut(t *testing.T) {
	withRules := buildImage(t, httpFixture)
	without := buildImage(t, "--google-api-http=false", httpFixture)

	if rules := httpRules(t, without); len(rules) != 0 {
		t.Errorf("opted out, but the image still carries %d rule(s): %v", len(rules), rules)
	}
	if bytes.Equal(withRules, without) {
		t.Error("--google-api-http=false produced a byte-identical image")
	}
	if stripped, want := stripHTTPRules(t, withRules), stripHTTPRules(t, without); !bytes.Equal(stripped, want) {
		t.Errorf("the opt-out changed more than the google.api.http rules (%d vs %d bytes with rules removed)",
			len(stripped), len(want))
	}

	// The carrier is untouched by the opt-out: the annotation still
	// lowers, so `pxf openapi` and `pxf docs` see exactly what they did.
	for _, image := range [][]byte{withRules, without} {
		if n := countAnnotationCarriers(t, image); n != 3 {
			t.Errorf("expected 3 method annotation carriers, got %d", n)
		}
	}
}

// stripHTTPRules re-encodes an image with the google.api.http extension
// removed from every method, so two images can be compared for
// everything *but* the rules. Both sides go through the same clear +
// deterministic re-marshal, so any normalization applies equally.
func stripHTTPRules(t *testing.T, image []byte) []byte {
	t.Helper()
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(image, fds); err != nil {
		t.Fatal(err)
	}
	for _, f := range fds.GetFile() {
		for _, s := range f.GetService() {
			for _, m := range s.GetMethod() {
				if opts := m.GetOptions(); opts != nil {
					proto.ClearExtension(opts, annotations.E_Http)
				}
			}
		}
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(fds)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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

// Byte-stability of an image carrying google.api.http rules is not
// asserted here: TestBuildPositiveCorpus builds every top-level fixture
// — 21_http_operation.proto among them — twice and fails on any
// difference, so the rule bytes are already pinned as stable by the
// #164 acceptance test.

// TestHTTPRulesMatchOpenAPIPaths is the invariant #213 exists for: the
// document `pxf openapi` renders and the routes an image actually binds
// must describe the same surface. A spec that promises what nothing
// serves is worse than no spec, and the two now derive from one
// annotation, so drift between them is a bug in one of the two readers.
//
// The invariant holds over the store fixture and is not yet true in
// general: the renderer describes only the first `@http` of a method
// (#215) and rejects the dotted and sub-path template segments the
// compiler accepts (#217). Neither class appears in any corpus fixture,
// which is why this test passes while those gaps stand — adding one
// before its issue is resolved is expected to fail here, and that is
// the signal, not a broken test.
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
	// superset: a route the reader is told about must exist. Guarded for
	// emptiness like the internal tier above — an audience-filter
	// regression that emptied the public document would otherwise make
	// this loop pass by never running.
	public := renderedOperations(t, image, "")
	if len(public) == 0 {
		t.Fatal("the public document describes no operations; the subset check would be vacuous")
	}
	for op := range public {
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
// route never reaches an image. That it is rejected at all is already
// covered for every fixture in invalid/ by TestCheckInvalidFixtures;
// what this adds is the half §5.2 actually promises — the diagnostic
// names the offending segment and points at the annotation that wrote
// it, rather than failing the build from somewhere unattributable.
func TestBuildUnboundPathTemplateRejected(t *testing.T) {
	fixture := filepath.Join(fixtureDir, "invalid", "http_unbound_template.proto")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("corpus fixture missing: %v", err)
	}
	var err error
	diag := captureStderr(t, func() { err = runPxf(t, "build", "--check", fixture) })
	if err == nil {
		t.Fatal("expected --check to reject an @http path template that binds no field")
	}
	for _, want := range []string{
		"{orderId}",                         // the segment that binds nothing
		"fixtures.badhttp.GetOrderRequest",  // the message it was resolved against
		"http_unbound_template.proto:",      // a position in the fixture, not a bare CLI error
		`@http("GET", "/orders/{orderId}")`, // the annotation blamed, quoted back
	} {
		if !strings.Contains(diag, want) {
			t.Errorf("diagnostic does not mention %q:\n%s", want, diag)
		}
	}
}

// captureStderr swaps os.Stderr for a pipe while fn runs and returns
// what was written to it. The compile-diagnostic renderer writes to
// os.Stderr directly rather than to cobra's error writer, so
// runPxfCaptured cannot see it. The reader is drained on a goroutine so
// a report larger than the pipe buffer cannot deadlock — the same shape
// as runCLI's stdout capture.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stderr = old
	}()
	fn()
	_ = w.Close()
	return <-done
}

// TestBuildAuthoredHTTPRuleWins pins the promise STABILITY.md makes
// about the foreign extension number: protowire writes nothing at
// 72295728 when the schema author wrote the option by hand. A second,
// competing rule is not merely redundant — a method carries at most one
// rule, so emitting ours would silently re-route the author's endpoint.
//
// The schema needs a resolvable google/api/annotations.proto, which the
// bundled opener deliberately does not serve (no import is ever added
// to a lowered file), so the test supplies a minimal one in its own
// import root — exactly what an author writing the option by hand has
// to do.
func TestBuildAuthoredHTTPRuleWins(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("google/api/annotations.proto", googleAPIAnnotationsStub)
	write("orders.proto", authoredRuleSchema)

	got := httpRules(t, buildImage(t, root))
	want := []string{"authored.Orders.GetOrder: GET /v2/orders/{order_id}"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the authored rule did not survive alone:\ngot:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// googleAPIAnnotationsStub is enough of google/api/annotations.proto to
// write the option by hand: the extension and the shape it carries.
const googleAPIAnnotationsStub = `syntax = "proto3";
package google.api;
import "google/protobuf/descriptor.proto";
extend google.protobuf.MethodOptions {
  HttpRule http = 72295728;
}
message HttpRule {
  string selector = 1;
  oneof pattern {
    string get = 2;
    string put = 3;
    string post = 4;
    string delete = 5;
    string patch = 6;
    CustomHttpPattern custom = 8;
  }
  string body = 7;
  string response_body = 12;
  repeated HttpRule additional_bindings = 11;
}
message CustomHttpPattern {
  string kind = 1;
  string path = 2;
}
`

// authoredRuleSchema carries both spellings on one method, disagreeing
// on the path so the surviving rule identifies which one won.
const authoredRuleSchema = `syntax = "proto3";
package authored;
import "protowire/schema/v1/annotations.proto";
import "google/api/annotations.proto";
message GetOrderRequest { string order_id = 1; }
message Order { string order_id = 1; }
service Orders {
  @http("GET", "/orders/{order_id}")
  rpc GetOrder(GetOrderRequest) returns (Order) {
    option (google.api.http) = { get: "/v2/orders/{order_id}" };
  }
}
`
