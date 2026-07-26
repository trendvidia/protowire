// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire/docpack"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

const (
	docsFixtureDir = "../../testdata/docs"
	// The documentation corpus anchors against an image built from this
	// schema fixture, so the anchor tests exercise the real lowering
	// pipeline rather than a hand-written descriptor set.
	docsAnchorSchema = "../../testdata/schema-extensions/01_basic.proto"
)

// docsImage builds the lowered image the topic corpus anchors against.
func docsImage(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "image.binpb")
	if err := runPxf(t, "build", "-o", out, docsAnchorSchema); err != nil {
		t.Fatalf("pxf build: %v", err)
	}
	return out
}

// buildPack compiles the valid corpus and returns the pack path.
func buildPack(t *testing.T, extra ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "docs.binpb")
	args := append([]string{
		"docs", "build",
		"-o", out,
		"--image", docsImage(t),
		"--registry", filepath.Join(docsFixtureDir, "registry.json"),
		filepath.Join(docsFixtureDir, "topics"),
	}, extra...)
	if err := runPxf(t, args...); err != nil {
		t.Fatalf("pxf %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// TestDocsBuildCorpus is the happy path: the whole authored corpus
// compiles, with every anchor kind resolving against the data inputs.
func TestDocsBuildCorpus(t *testing.T) {
	if err := runPxf(t, "docs", "build", "--check",
		"--image", docsImage(t),
		"--registry", filepath.Join(docsFixtureDir, "registry.json"),
		filepath.Join(docsFixtureDir, "topics"),
	); err != nil {
		t.Fatalf("docs build --check: %v", err)
	}
}

// TestDocsBuildReleasePolicy asserts the corpus is releasable: every
// topic approved by a revisor who is not its author, against a digest
// that still matches its content.
func TestDocsBuildReleasePolicy(t *testing.T) {
	if err := runPxf(t, "docs", "build", "--check", "--release",
		"--image", docsImage(t),
		"--registry", filepath.Join(docsFixtureDir, "registry.json"),
		filepath.Join(docsFixtureDir, "topics"),
	); err != nil {
		t.Fatalf("docs build --check --release: %v", err)
	}
}

// TestDocsBuildDeterministic pins the byte-stability bar the pack is
// cached, diffed and shipped against.
func TestDocsBuildDeterministic(t *testing.T) {
	image := docsImage(t)
	dir := t.TempDir()
	var runs [2][]byte
	for i := range runs {
		out := filepath.Join(dir, "pack", string(rune('a'+i))+".binpb")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := runPxf(t, "docs", "build", "-o", out,
			"--image", image,
			"--registry", filepath.Join(docsFixtureDir, "registry.json"),
			filepath.Join(docsFixtureDir, "topics"),
		); err != nil {
			t.Fatalf("docs build: %v", err)
		}
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		runs[i] = raw
	}
	if string(runs[0]) != string(runs[1]) {
		t.Fatalf("doc pack is not byte-stable across runs: %d vs %d bytes", len(runs[0]), len(runs[1]))
	}
}

// TestDocsPackContents reads the emitted pack back and checks that the
// compiler wrote down what it decided: resolved anchors, the derived
// descriptor path with its image digest, the redirect terminus, the
// translation status, and a usable index.
func TestDocsPackContents(t *testing.T) {
	pack := decodePack(t, buildPack(t))

	// Provenance names both data inputs.
	prov := pack.Get(field(t, pack, "provenance")).Message()
	if got := prov.Get(field(t, prov, "format_version")).Uint(); got == 0 {
		t.Error("provenance carries no pack format version")
	}
	if img := prov.Get(field(t, prov, "image")).Message(); img.Get(field(t, img, "digest")).String() == "" {
		t.Error("provenance carries no image digest")
	}
	if cat := prov.Get(field(t, prov, "catalog")).Message(); cat.Get(field(t, cat, "widget_count")).Uint() != 3 {
		t.Errorf("catalog provenance widget_count = %d, want 3", cat.Get(field(t, cat, "widget_count")).Uint())
	}

	// Topics are sorted by (key, locale) and carry their source file.
	topics := pack.Get(field(t, pack, "topics")).List()
	var keys []string
	byKey := map[string]protoreflect.Message{}
	for i := 0; i < topics.Len(); i++ {
		ct := topics.Get(i).Message()
		topic := ct.Get(field(t, ct, "topic")).Message()
		key := topic.Get(field(t, topic, "key")).String() + "/" + topic.Get(field(t, topic, "locale")).String()
		keys = append(keys, key)
		byKey[key] = ct
	}
	want := []string{
		"guide.anchors/de", "guide.anchors/en",
		"guide.overview/de", "guide.overview/en",
		"schema.user/en", "widgets.button/en",
	}
	if !sort.StringsAreSorted(keys) {
		t.Errorf("topics are not sorted by (key, locale): %v", keys)
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("topics = %v, want %v", keys, want)
	}

	// The stale translation is recorded as stale; the current one is not.
	if got := enumName(t, byKey["guide.anchors/de"], "translation_status"); got != "TRANSLATION_STATUS_STALE" {
		t.Errorf("guide.anchors/de translation_status = %s, want STALE", got)
	}
	if got := enumName(t, byKey["guide.overview/de"], "translation_status"); got != "TRANSLATION_STATUS_CURRENT" {
		t.Errorf("guide.overview/de translation_status = %s, want CURRENT", got)
	}

	// Anchors on the schema topic: the redirect resolved, the derived
	// path was re-derived and stamped, and body anchors are marked as
	// such.
	anchors := anchorIndex(t, byKey["schema.user/en"])
	imageDigest := func() string {
		img := prov.Get(field(t, prov, "image")).Message()
		return img.Get(field(t, img, "digest")).String()
	}()

	// Two anchors legitimately share a resolved id here — the direct one
	// and the one that got there through the redirect — so assertions
	// pick the arm they mean rather than trusting map order.
	for _, tc := range []struct {
		id        string
		stability string
		count     int
	}{
		// User is anchored directly and again via the Person redirect.
		{"fixtures.basic.User", "ANCHOR_STABILITY_STABLE", 2},
		{"fixtures.basic.Email", "ANCHOR_STABILITY_STABLE", 2}, // topic anchor + body reference
		{"fixtures.basic.User.email[protowire.schema.v1.validate#0]", "ANCHOR_STABILITY_DERIVED", 1},
	} {
		got := anchors[tc.id]
		if len(got) != tc.count {
			t.Errorf("resolved anchors for %q = %d, want %d", tc.id, len(got), tc.count)
		}
		for _, a := range got {
			if s := enumName(t, a, "stability"); s != tc.stability {
				t.Errorf("anchor %s stability = %s, want %s", tc.id, s, tc.stability)
			}
		}
	}

	// The derived anchor is self-describing about what it is valid
	// against — the whole point of the fragile-anchor contract.
	derived := only(t, anchors, "fixtures.basic.User.email[protowire.schema.v1.validate#0]")
	if got := derived.Get(field(t, derived, "descriptor_path")).String(); got == "" {
		t.Error("derived anchor carries no descriptor_path")
	}
	if got := derived.Get(field(t, derived, "image_digest")).String(); got != imageDigest {
		t.Errorf("derived anchor image_digest = %q, want the provenance digest %q", got, imageDigest)
	}

	// Exactly one of the two User anchors travelled through the redirect,
	// and it records where it came from.
	var chains [][]string
	for _, a := range anchors["fixtures.basic.User"] {
		list := a.Get(field(t, a, "redirect_chain")).List()
		if list.Len() == 0 {
			continue
		}
		var hops []string
		for i := 0; i < list.Len(); i++ {
			hops = append(hops, list.Get(i).String())
		}
		chains = append(chains, hops)
	}
	if len(chains) != 1 || strings.Join(chains[0], "→") != "fixtures.basic.Person→fixtures.basic.User" {
		t.Errorf("redirect chains = %v, want exactly [[fixtures.basic.Person fixtures.basic.User]]", chains)
	}

	// Origins distinguish topic-level anchors from ones written in prose.
	for id, wantOrigin := range map[string]string{
		"fixtures.basic.User.email[protowire.schema.v1.validate#0]": "ANCHOR_ORIGIN_TOPIC",
	} {
		if got := enumName(t, only(t, anchors, id), "origin"); got != wantOrigin {
			t.Errorf("anchor %s origin = %s, want %s", id, got, wantOrigin)
		}
	}
	var bodyAnchors int
	for _, list := range anchors {
		for _, a := range list {
			if enumName(t, a, "origin") == "ANCHOR_ORIGIN_BODY" {
				bodyAnchors++
			}
		}
	}
	if bodyAnchors != 2 {
		t.Errorf("schema.user/en has %d body anchors, want 2", bodyAnchors)
	}

	// Widget anchors carry the catalog's since-version — the fact that
	// makes a topic's runtime applicability checkable.
	widgetAnchors := anchorIndex(t, byKey["widgets.button/en"])
	if got := widgetAnchors["Button#prop:icon"]; got != nil {
		t.Error("unexpected anchor for a prop the corpus does not document")
	}
	// onTapped is anchored twice — once on the topic, once in prose — and
	// both carry the catalog's since-version. A structural widget's child
	// prop resolves against its parent, and carries that entry's since.
	for id, wantSince := range map[string]string{
		"Button#event:onTapped": "0.1.0",
		"Border#prop:position":  "0.2.0",
	} {
		got := widgetAnchors[id]
		if len(got) == 0 {
			t.Errorf("no anchor resolved to %q", id)
		}
		for _, a := range got {
			if since := a.Get(field(t, a, "target_since")).String(); since != wantSince {
				t.Errorf("%s target_since = %q, want %q", id, since, wantSince)
			}
		}
	}

	// The index is present, ordered, and finds words from prose and code.
	index := pack.Get(field(t, pack, "index")).Message()
	postings := index.Get(field(t, index, "postings")).List()
	var terms []string
	for i := 0; i < postings.Len(); i++ {
		p := postings.Get(i).Message()
		terms = append(terms, p.Get(field(t, p, "term")).String())
	}
	if !sort.StringsAreSorted(terms) {
		t.Error("index postings are not sorted by term")
	}
	for _, want := range []string{"doc", "anchors", "button", "ontapped"} {
		if sort.SearchStrings(terms, want) >= len(terms) || terms[sort.SearchStrings(terms, want)] != want {
			t.Errorf("index has no posting for %q", want)
		}
	}
	docs := index.Get(field(t, index, "documents")).List()
	if docs.Len() != topics.Len() {
		t.Errorf("index has %d documents, pack has %d topics", docs.Len(), topics.Len())
	}
}

// TestDocsInvalidFixtures pins the diagnostics for every way a corpus can
// be wrong. Each fixture compiles alone.
func TestDocsInvalidFixtures(t *testing.T) {
	image := docsImage(t)
	registry := filepath.Join(docsFixtureDir, "registry.json")

	cases := map[string]string{
		"approved_without_digest.pxf":         "REVIEW_STATE_APPROVED with no review.approved_digest",
		"audience_leak.pxf":                   "may not point at a more restricted one",
		"dangling_schema_anchor.pxf":          "resolves to nothing",
		"dangling_widget_prop.pxf":            `has no property "colour"`,
		"descriptor_path_missing.pxf":         "is not in the image's source map",
		"descriptor_path_unknown_element.pxf": "names element fixtures.basic.User.nickname, which is not in",
		"duplicate_topic.pxf":                 "is already defined in",
		"heading_skip.pxf":                    "jumps from level 1 to 3",
		"image_without_alt.pxf":               "has no alt text",
		"link_unknown_fragment.pxf":           "is not a heading in that topic",
		"redirect_cycle.pxf":                  "cycles through",
		"self_approval.pxf":                   "self-approval defeats the gate",
		"translation_without_source.pxf":      "has no source-locale topic",
	}

	entries, err := os.ReadDir(filepath.Join(docsFixtureDir, "invalid"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".pxf") {
			continue
		}
		if _, ok := cases[entry.Name()]; !ok {
			t.Errorf("invalid fixture %s has no expected diagnostic; add one", entry.Name())
		}
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(docsFixtureDir, "invalid", name)
			result, err := docpack.Compile(docpack.Options{
				Inputs:      []string{path},
				ImagePath:   image,
				CatalogPath: registry,
			})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if result.Errors == 0 {
				t.Fatalf("%s compiled clean; expected an error containing %q", name, want)
			}
			if result.Pack != nil {
				t.Error("a build with errors emitted a pack")
			}
			var found bool
			for _, d := range result.Diagnostics {
				if d.Severity == docpack.SeverityError && strings.Contains(d.Message, want) {
					found = true
				}
			}
			if !found {
				t.Errorf("no error matched %q; got:\n%s", want, formatDiags(result.Diagnostics))
			}
		})
	}
}

// TestDocsPolicyFixtures asserts the release gate: each fixture is a
// warning while authoring and a refusal at release. The authoring loop
// has to stay usable — goed runs this compiler on its diagnostics
// debounce — while release stays strict.
func TestDocsPolicyFixtures(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(docsFixtureDir, "policy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".pxf") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			path := filepath.Join(docsFixtureDir, "policy", entry.Name())

			draft, err := docpack.Compile(docpack.Options{Inputs: []string{path}})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if draft.Errors != 0 {
				t.Errorf("fixture fails without --release:\n%s", formatDiags(draft.Diagnostics))
			}
			if len(draft.Diagnostics) == 0 {
				t.Error("fixture produced no warning at all")
			}

			release, err := docpack.Compile(docpack.Options{Inputs: []string{path}, Release: true})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if release.Errors == 0 {
				t.Error("fixture passes --release; the gate is not gating")
			}
		})
	}
}

// TestDocsDigestMatchesApprovals ties the `docs digest` output to the
// digests recorded in the corpus: the value the authoring flow would
// write is the value the compiler checks.
func TestDocsDigestMatchesApprovals(t *testing.T) {
	digests, diagnostics, err := docpack.Digests([]string{filepath.Join(docsFixtureDir, "topics")})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diagnostics {
		if d.Severity == docpack.SeverityError {
			t.Fatalf("digest pass reported an error: %s", d)
		}
	}
	if len(digests) != 6 {
		t.Fatalf("got %d digests, want 6", len(digests))
	}

	// Every source-locale topic in the corpus is approved against its
	// current content, so the digest listing must reproduce exactly the
	// values the sources record.
	raw, err := os.ReadFile(filepath.Join(docsFixtureDir, "topics", "overview.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range digests {
		if d.Locale != "en" || !strings.HasPrefix(d.Key, "guide.") {
			continue
		}
		if !strings.Contains(string(raw), d.Digest) {
			t.Errorf("%s/%s digest %s is not recorded in overview.pxf", d.Key, d.Locale, d.Digest)
		}
	}
}

// TestImageQuerySurface pins the anchor-target sets LoadImage exposes
// (#185), against an image from the real lowering pipeline: the FQN
// set behind schema anchors, the canonical descriptor-path set behind
// derived anchors, and the per-element annotation FQNs completion
// filters by.
func TestImageQuerySurface(t *testing.T) {
	im, err := docpack.LoadImage(docsImage(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, fqn := range []string{"fixtures.basic.User", "fixtures.basic.User.email"} {
		if !im.Has(fqn) {
			t.Errorf("Has(%q) = false", fqn)
		}
	}
	if im.Has("fixtures.basic.Nope") {
		t.Error(`Has("fixtures.basic.Nope") = true`)
	}

	fqns := im.FQNs()
	if !sort.StringsAreSorted(fqns) {
		t.Error("FQNs() is not sorted")
	}
	if i := sort.SearchStrings(fqns, "fixtures.basic.User.email"); i >= len(fqns) || fqns[i] != "fixtures.basic.User.email" {
		t.Error("FQNs() does not list fixtures.basic.User.email")
	}

	// The corpus resolves this derived anchor, so the path set must
	// carry its canonical spelling.
	const path = "fixtures.basic.User.email[protowire.schema.v1.validate#0]"
	if !im.HasPath(path) {
		t.Errorf("HasPath(%q) = false", path)
	}
	paths := im.Paths()
	if !sort.StringsAreSorted(paths) {
		t.Error("Paths() is not sorted")
	}
	if i := sort.SearchStrings(paths, path); i >= len(paths) || paths[i] != path {
		t.Errorf("Paths() does not list %q", path)
	}

	var found bool
	for _, a := range im.AnnotationsOn("fixtures.basic.User.email") {
		if a == "protowire.schema.v1.validate" {
			found = true
		}
	}
	if !found {
		t.Errorf("AnnotationsOn(fixtures.basic.User.email) = %v, want it to list protowire.schema.v1.validate",
			im.AnnotationsOn("fixtures.basic.User.email"))
	}
}

// TestDocsPreloadedInputs compiles the corpus with both data inputs
// preloaded (#185) and checks the result matches the path-based build
// byte for byte — the debounce optimization must not change the pack.
func TestDocsPreloadedInputs(t *testing.T) {
	imagePath := docsImage(t)
	image, err := docpack.LoadImage(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := docpack.LoadCatalog(filepath.Join(docsFixtureDir, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}

	var packs [2][]byte
	for i, opts := range []docpack.Options{
		{Inputs: []string{filepath.Join(docsFixtureDir, "topics")}, Image: image, Catalog: catalog},
		{Inputs: []string{filepath.Join(docsFixtureDir, "topics")}, ImagePath: imagePath, CatalogPath: filepath.Join(docsFixtureDir, "registry.json")},
	} {
		result, err := docpack.Compile(opts)
		if err != nil {
			t.Fatal(err)
		}
		if result.Errors > 0 {
			t.Fatalf("compile errors:\n%s", formatDiags(result.Diagnostics))
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(result.Pack)
		if err != nil {
			t.Fatal(err)
		}
		packs[i] = raw
	}
	if !bytes.Equal(packs[0], packs[1]) {
		t.Error("preloaded-input build differs from the path-based build")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func decodePack(t *testing.T, path string) protoreflect.Message {
	t.Helper()
	reg := schemaresolve.NewRegistry()
	if err := schemaresolve.CompileSources(reg, schemaresolve.CompileOptions{
		BundledFiles: docpack.BundledDocsSchemas,
	}); err != nil {
		t.Fatal(err)
	}
	md := reg.Find(docpack.DocPackMessage)
	if md == nil {
		t.Fatalf("bundled schemas do not define %s", docpack.DocPackMessage)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(raw, msg); err != nil {
		t.Fatalf("unmarshaling pack: %v", err)
	}
	return msg
}

func field(t *testing.T, m protoreflect.Message, name string) protoreflect.FieldDescriptor {
	t.Helper()
	fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		t.Fatalf("%s has no field %q", m.Descriptor().FullName(), name)
	}
	return fd
}

func enumName(t *testing.T, m protoreflect.Message, name string) string {
	t.Helper()
	fd := field(t, m, name)
	v := fd.Enum().Values().ByNumber(m.Get(fd).Enum())
	if v == nil {
		return ""
	}
	return string(v.Name())
}

// anchorIndex groups a compiled topic's resolved anchors by resolved id.
// Several anchors may share an id — a direct reference and one that
// arrived through a redirect, a topic-level anchor and a prose reference
// to the same target — so the index is one-to-many by construction.
func anchorIndex(t *testing.T, compiled protoreflect.Message) map[string][]protoreflect.Message {
	t.Helper()
	out := map[string][]protoreflect.Message{}
	list := compiled.Get(field(t, compiled, "anchors")).List()
	for i := 0; i < list.Len(); i++ {
		a := list.Get(i).Message()
		id := a.Get(field(t, a, "resolved_id")).String()
		out[id] = append(out[id], a)
	}
	return out
}

// only returns the single anchor resolved to id, failing when there is
// not exactly one.
func only(t *testing.T, index map[string][]protoreflect.Message, id string) protoreflect.Message {
	t.Helper()
	got := index[id]
	if len(got) != 1 {
		t.Errorf("want exactly one anchor resolved to %q, got %d", id, len(got))
		return nil
	}
	return got[0]
}

func formatDiags(ds []docpack.Diagnostic) string {
	var sb strings.Builder
	for _, d := range ds {
		sb.WriteString("  " + d.String() + "\n")
	}
	return sb.String()
}
