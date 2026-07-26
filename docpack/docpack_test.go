// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// anchor builds an Anchor with one target set, for the identity tests.
func anchor(t *testing.T, kind string, set func(target dmsg)) dmsg {
	t.Helper()
	md, err := message("protowire.docs.v1.Anchor")
	if err != nil {
		t.Fatal(err)
	}
	a := newMsg(md)
	set(a.newSub(kind))
	return a
}

func TestAnchorID(t *testing.T) {
	cases := []struct {
		name string
		kind string
		set  func(dmsg)
		want string
	}{
		{
			name: "schema element",
			kind: anchorSchema,
			set:  func(m dmsg) { m.setStr("fqn", "myco.orders.Order.total") },
			want: "myco.orders.Order.total",
		},
		{
			// Derived through the shared §8.3.1 formatter, never
			// hand-assembled: the grammar belongs to the toolchain.
			name: "descriptor path with annotation ordinal",
			kind: anchorDescriptorPath,
			set: func(m dmsg) {
				m.setStr("element_fqn", "myco.User.email")
				m.setStr("annotation_fqn", "protowire.schema.v1.validate")
				m.setU32("ordinal", 1)
			},
			want: "myco.User.email[protowire.schema.v1.validate#1]",
		},
		{
			name: "descriptor path bare element",
			kind: anchorDescriptorPath,
			set:  func(m dmsg) { m.setStr("element_fqn", "myco.User.email") },
			want: "myco.User.email",
		},
		{
			name: "widget type",
			kind: anchorWidget,
			set:  func(m dmsg) { m.setStr("type", "Button") },
			want: "Button",
		},
		{
			name: "widget prop",
			kind: anchorWidget,
			set: func(m dmsg) {
				m.setStr("type", "Button")
				m.setStr("prop", "text")
			},
			want: "Button#prop:text",
		},
		{
			name: "widget event",
			kind: anchorWidget,
			set: func(m dmsg) {
				m.setStr("type", "Button")
				m.setStr("event", "onTapped")
			},
			want: "Button#event:onTapped",
		},
		{
			name: "route",
			kind: anchorRoute,
			set:  func(m dmsg) { m.setStr("path", "/settings/profile") },
			want: "/settings/profile",
		},
		{
			name: "topic",
			kind: anchorTopic,
			set:  func(m dmsg) { m.setStr("key", "widgets.button.overview") },
			want: "widgets.button.overview",
		},
		{
			// The prefix keeps transition ids disjoint from every other
			// spelling in the shared namespace (#199).
			name: "transition",
			kind: anchorTransition,
			set:  func(m dmsg) { m.setStr("name", "slide") },
			want: "transition:slide",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, id, err := anchorID(anchor(t, tc.kind, tc.set))
			if err != nil {
				t.Fatalf("anchorID: %v", err)
			}
			if kind != tc.kind {
				t.Errorf("kind = %q, want %q", kind, tc.kind)
			}
			if id != tc.want {
				t.Errorf("id = %q, want %q", id, tc.want)
			}
		})
	}
}

func TestAnchorIDRejects(t *testing.T) {
	cases := []struct {
		name string
		kind string
		set  func(dmsg)
		want string
	}{
		{"empty fqn", anchorSchema, func(m dmsg) {}, "empty fqn"},
		{"fqn with a slash", anchorSchema, func(m dmsg) { m.setStr("fqn", "myco/Order") }, "not a fully-qualified name"},
		{"widget prop and event", anchorWidget, func(m dmsg) {
			m.setStr("type", "Button")
			m.setStr("prop", "text")
			m.setStr("event", "onTapped")
		}, "sets both prop"},
		{"lowercase widget type", anchorWidget, func(m dmsg) { m.setStr("type", "button") }, "PascalCase"},
		{"relative route", anchorRoute, func(m dmsg) { m.setStr("path", "settings") }, `must start with "/"`},
		{"route with fragment", anchorRoute, func(m dmsg) { m.setStr("path", "/a#b") }, "contains a fragment"},
		{"uppercase topic key", anchorTopic, func(m dmsg) { m.setStr("key", "Widgets.Button") }, "dotted lowercase key"},
		{"ordinal without annotation", anchorDescriptorPath, func(m dmsg) {
			m.setStr("element_fqn", "myco.User.email")
			m.setU32("ordinal", 2)
		}, "without an annotation_fqn"},
		{"empty transition name", anchorTransition, func(m dmsg) {}, "empty name"},
		{"dashed transition name", anchorTransition, func(m dmsg) { m.setStr("name", "slide-left") }, "not an identifier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := anchorID(anchor(t, tc.kind, tc.set))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestAnchorStability pins the classification the whole fragile-anchor
// contract rests on.
func TestAnchorStability(t *testing.T) {
	for kind, want := range map[string]string{
		anchorSchema:         "ANCHOR_STABILITY_STABLE",
		anchorWidget:         "ANCHOR_STABILITY_STABLE",
		anchorRoute:          "ANCHOR_STABILITY_STABLE",
		anchorTopic:          "ANCHOR_STABILITY_STABLE",
		anchorTransition:     "ANCHOR_STABILITY_STABLE",
		anchorDescriptorPath: "ANCHOR_STABILITY_DERIVED",
	} {
		if got := stabilityOf(kind); got != want {
			t.Errorf("stabilityOf(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"The doc pack", []string{"the", "doc", "pack"}},
		{"Case FOLDED", []string{"case", "folded"}},
		// Single characters carry no selectivity and are dropped.
		{"a b cd", []string{"cd"}},
		// Identifiers are emitted whole and split, at the same position,
		// so documentation about code is findable either way.
		{"Button.text", []string{"button.text", "button", "text"}},
		{"on_tapped", []string{"on_tapped", "on", "tapped"}},
		{"punctuation, is: dropped!", []string{"punctuation", "is", "dropped"}},
	}
	for _, tc := range cases {
		got := tokenize(tc.in)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Error handling":      "error-handling",
		"  Leading spaces":    "leading-spaces",
		"Anchors & stability": "anchors-stability",
		"§8.3.1 paths":        "8-3-1-paths",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestContentDigestCoversContentOnly pins what the review gate actually
// gates on: editing the body invalidates an approval, re-tagging or
// re-recording the approval itself does not.
func TestContentDigestCoversContentOnly(t *testing.T) {
	md, err := message("protowire.docs.v1.Topic")
	if err != nil {
		t.Fatal(err)
	}
	build := func(title, body, tag, revisor string) dmsg {
		topic := newMsg(md)
		topic.setStr("key", "a.b")
		topic.setStr("locale", "en")
		topic.setStr("title", title)
		para := topic.newSub("body").appendMsg("blocks").newSub("paragraph")
		para.appendMsg("runs").setStr("text", body)
		meta := topic.newSub("meta")
		meta.setEnum("audience", "AUDIENCE_PUBLIC")
		meta.appendStr("tags", tag)
		topic.newSub("review").setStr("revisor", revisor)
		return topic
	}
	digest := func(d dmsg) string {
		t.Helper()
		got, err := contentDigest(d)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	base := digest(build("Title", "Body", "tag-a", "r@example.com"))
	if got := digest(build("Title", "Body", "tag-b", "other@example.com")); got != base {
		t.Error("metadata and review changes moved the content digest; approvals would churn")
	}
	if got := digest(build("Title", "Edited body", "tag-a", "r@example.com")); got == base {
		t.Error("editing the body left the digest unchanged; approval would survive an edit")
	}
	if got := digest(build("Edited title", "Body", "tag-a", "r@example.com")); got == base {
		t.Error("editing the title left the digest unchanged")
	}
}

// TestCatalogJSONBoundary checks the one place JSON exists in the
// pipeline: appviewer's export converts to the typed model, and the
// widget membership rules hold on the far side.
func TestCatalogJSONBoundary(t *testing.T) {
	cat, err := LoadCatalog("../testdata/docs/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	if cat.SchemaVersion != 3 {
		t.Errorf("schema version = %d, want 3", cat.SchemaVersion)
	}
	if cat.WidgetCount != 3 {
		t.Errorf("widget count = %d, want 3", cat.WidgetCount)
	}

	for id, want := range map[string]string{
		"Button":                "0.1.0",
		"Button#prop:text":      "0.1.0",
		"Button#prop:icon":      "0.3.0", // per-prop since overrides the widget's
		"Button#event:onTapped": "0.1.0",
		"Button#prop:id":        "0.1.0", // common node prop
		"Border#prop:position":  "0.2.0", // a structural widget's child prop
	} {
		since, err := cat.ResolveWidget(id)
		if err != nil {
			t.Errorf("resolveWidget(%q): %v", id, err)
			continue
		}
		if since != want {
			t.Errorf("resolveWidget(%q) since = %q, want %q", id, since, want)
		}
	}

	for _, id := range []string{"Nope", "Button#prop:colour", "Button#event:onHover"} {
		if _, err := cat.ResolveWidget(id); err == nil {
			t.Errorf("resolveWidget(%q) resolved; expected an error", id)
		}
	}
}

// TestCatalogV9Boundary pins the schema-v9 mirror (#186) against an
// export shaped like the live appviewer registry: bind resolves
// per-widget (the v8 move off common_props), composition props and
// transitions are carried but never resolvable as widget anchors, and
// the authoring hints survive the JSON boundary into the typed model.
func TestCatalogV9Boundary(t *testing.T) {
	cat, err := LoadCatalog("../testdata/docs/registry_v9.json")
	if err != nil {
		t.Fatal(err)
	}
	if cat.SchemaVersion != 9 {
		t.Errorf("schema version = %d, want 9", cat.SchemaVersion)
	}

	// bind is a capability prop stamped onto Bindable specs: it resolves
	// on the Binder widget and on no other — never commonly.
	if _, err := cat.ResolveWidget("Entry#prop:bind"); err != nil {
		t.Errorf("Entry#prop:bind did not resolve: %v", err)
	}
	if _, err := cat.ResolveWidget("Label#prop:bind"); err == nil {
		t.Error("Label#prop:bind resolved; bind must be per-widget since v8")
	}

	// The rest of the membership rules hold on the v9 shape.
	for _, id := range []string{"Border#prop:position", "Entry#event:onChanged", "Label#prop:help_topic"} {
		if _, err := cat.ResolveWidget(id); err != nil {
			t.Errorf("ResolveWidget(%q): %v", id, err)
		}
	}

	// Composition props are offered by context on any widget node, so a
	// prop anchor naming one resolves on every widget, like a common
	// prop (#199) — with since falling back to the widget's.
	if since, err := cat.ResolveWidget("Label#prop:slot"); err != nil || since != "0.1.0" {
		t.Errorf("Label#prop:slot = (%q, %v), want (\"0.1.0\", nil)", since, err)
	}
	if got := strings.Join(cat.CompositionProps(), ","); got != "content_slot,slot,template" {
		t.Errorf("CompositionProps() = %q", got)
	}
	// Producer order, not sorted: the default leads upstream.
	if got := strings.Join(cat.Transitions(), ","); got != "none,slide,fade" {
		t.Errorf("Transitions() = %q", got)
	}
	// Transitions are catalog-global transition-anchor targets (#199).
	if err := cat.ResolveTransition("slide"); err != nil {
		t.Errorf("ResolveTransition(slide): %v", err)
	}
	if err := cat.ResolveTransition("warp"); err == nil || !strings.Contains(err.Error(), "none, slide, fade") {
		t.Errorf("ResolveTransition(warp) = %v, want an error naming the vocabulary", err)
	}
}

// TestCatalogV9TypedMirror checks the JSON boundary carries the v4–v9
// fields into the typed WidgetCatalog — the mirror a .pxf/.binpb
// producer authors directly.
func TestCatalogV9TypedMirror(t *testing.T) {
	raw, err := os.ReadFile("../testdata/docs/registry_v9.json")
	if err != nil {
		t.Fatal(err)
	}
	md, err := message(WidgetCatalogMessage)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalogFromJSON(raw, md)
	if err != nil {
		t.Fatal(err)
	}

	widgets := map[string]dmsg{}
	for _, w := range cat.msgs("widgets") {
		widgets[w.str("type")] = w
	}
	border := widgets["Border"]
	if !border.valid() {
		t.Fatal("no Border widget in the typed mirror")
	}
	if border.str("icon") != "border" || border.str("category") != "layout" {
		t.Errorf("Border icon/category = %q/%q", border.str("icon"), border.str("category"))
	}
	if border.m.Get(border.fd("variadic_children")).Bool() != true {
		t.Error("Border variadic_children not carried")
	}
	pos := border.msgs("child_props")[0]
	if pos.str("name") != "position" {
		t.Fatalf("Border child_props[0] = %q", pos.str("name"))
	}
	if pos.m.Get(pos.fd("required")).Bool() != true || pos.str("default_value") != "center" {
		t.Errorf("position required/default_value = %v/%q, want true/center",
			pos.m.Get(pos.fd("required")).Bool(), pos.str("default_value"))
	}
	if got := len(cat.msgs("composition_props")); got != 3 {
		t.Errorf("composition_props carried %d entries, want 3", got)
	}
	if got := len(cat.msgs("transitions")); got != 3 {
		t.Errorf("transitions carried %d entries, want 3", got)
	}
}

// TestCatalogVersionGate pins the floor semantics (#186): an export at
// the mirrored version builds silently, an older one still resolves,
// and only a genuinely newer one warns.
func TestCatalogVersionGate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "t.pxf"), []byte(topicSource("a.b")), 0o644); err != nil {
		t.Fatal(err)
	}
	warned := func(registry string) bool {
		t.Helper()
		result, err := Compile(Options{Inputs: []string{root}, CatalogPath: registry})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range result.Diagnostics {
			if strings.Contains(d.Message, "newer entries may not resolve") {
				return true
			}
		}
		return false
	}

	if warned("../testdata/docs/registry_v9.json") {
		t.Error("a v9 export warned on a v9 compiler; the gate must be silent at the floor")
	}
	if warned("../testdata/docs/registry.json") {
		t.Error("an older export warned; the floor gates newer versions only")
	}

	v10 := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(v10, []byte(`{"schema_version": 10, "widgets": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !warned(v10) {
		t.Error("a v10 export did not warn; the floor must flag versions past the mirror")
	}
}

// TestCatalogQuerySurface pins the read-only sets anchor completion
// consumes (#185): exactly what ResolveWidget checks membership
// against, so an editor can only offer anchors the compiler accepts.
func TestCatalogQuerySurface(t *testing.T) {
	cat, err := LoadCatalog("../testdata/docs/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	joined := func(ss []string) string { return strings.Join(ss, ",") }

	if got := cat.Widgets(); joined(got) != "Border,Button,Label" {
		t.Errorf("Widgets() = %v", got)
	}
	if got := cat.Props("Button"); joined(got) != "icon,importance,text" {
		t.Errorf("Props(Button) = %v", got)
	}
	// A structural widget's child_props are prop-anchorable, so they are
	// part of the completion set too.
	if got := cat.Props("Border"); joined(got) != "position" {
		t.Errorf("Props(Border) = %v", got)
	}
	if got := cat.Events("Button"); joined(got) != "onTapped" {
		t.Errorf("Events(Button) = %v", got)
	}
	if got := cat.CommonProps(); joined(got) != "id,type" {
		t.Errorf("CommonProps() = %v", got)
	}
	if got := cat.Props("Nope"); got != nil {
		t.Errorf("Props of an unknown type = %v, want nil", got)
	}
	if got := cat.Events("Nope"); got != nil {
		t.Errorf("Events of an unknown type = %v, want nil", got)
	}
}

// topicSource renders a minimal one-topic file for the overlay tests.
func topicSource(key string) string {
	return `@type protowire.docs.v1.TopicFile
topics = [
  {
    key = "` + key + `"
    locale = "en"
    title = "T"
  }
]
`
}

// compiledKeys lists a compile result's topic keys via the digest pass
// over the same inputs — the pack itself is checked in the cmd tests;
// here the question is only which sources joined the build.
func compiledKeys(t *testing.T, result *Result) []string {
	t.Helper()
	if result.Errors > 0 {
		var sb strings.Builder
		for _, d := range result.Diagnostics {
			sb.WriteString("  " + d.String() + "\n")
		}
		t.Fatalf("compile failed:\n%s", sb.String())
	}
	var keys []string
	for _, ct := range wrap(result.Pack.ProtoReflect()).msgs("topics") {
		keys = append(keys, ct.sub("topic").str("key"))
	}
	return keys
}

// TestOverlay pins the editor seam (#185): a dirty buffer replaces its
// on-disk file, and a buffer with no file yet still joins the build —
// while root-wide checks keep running across the spliced corpus.
func TestOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.pxf"), []byte(topicSource("guide.saved")), 0o644); err != nil {
		t.Fatal(err)
	}

	// The overlay replaces a.pxf (its saved key disappears) and adds a
	// file that exists nowhere on disk.
	result, err := Compile(Options{
		Inputs: []string{root},
		Overlay: map[string][]byte{
			"a.pxf":   []byte(topicSource("guide.edited")),
			"new.pxf": []byte(topicSource("guide.unsaved")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(compiledKeys(t, result), ",")
	if got != "guide.edited,guide.unsaved" {
		t.Errorf("compiled topics = %s, want guide.edited,guide.unsaved", got)
	}

	// Root-wide checks see the overlay: two buffers claiming one (key,
	// locale) is the duplicate-topic error, exactly as on disk.
	dup, err := Compile(Options{
		Inputs: []string{root},
		Overlay: map[string][]byte{
			"new.pxf": []byte(topicSource("guide.saved")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dup.Errors == 0 {
		t.Error("duplicate key across disk and overlay compiled clean")
	}

	// Diagnostics attribute to the overlay path, so the editor can route
	// them to the right buffer.
	bad, err := Compile(Options{
		Inputs:  []string{root},
		Overlay: map[string][]byte{"new.pxf": []byte("not pxf {{{")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var attributed bool
	for _, d := range bad.Diagnostics {
		if d.Severity == SeverityError && d.Loc.File == "new.pxf" {
			attributed = true
		}
	}
	if !attributed {
		t.Errorf("no error attributed to the overlay path; got %v", bad.Diagnostics)
	}
}

// TestDiagnosticPositions pins the Loc coordinates (#187): the baseline
// is the topic's `key` entry, and checks that know their offending
// entry — a review field, a topic-level anchor — point at that entry.
// Positions come from the AST the loader already parses; the editor
// must never need a second parse to place a squiggle.
func TestDiagnosticPositions(t *testing.T) {
	src := `@type protowire.docs.v1.TopicFile
topics = [
  {
    key = "a.b"
    locale = "en"
    review {
      state = REVIEW_STATE_APPROVED
      author = "a@example.com"
      revisor = "a@example.com"
    }
    anchors = [
      { topic { key = "no.such" } }
    ]
  }
]
`
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "t.pxf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(Options{Inputs: []string{root}})
	if err != nil {
		t.Fatal(err)
	}

	find := func(substr string) Diagnostic {
		t.Helper()
		for _, d := range result.Diagnostics {
			if strings.Contains(d.Message, substr) {
				return d
			}
		}
		t.Fatalf("no diagnostic contains %q; got %v", substr, result.Diagnostics)
		return Diagnostic{}
	}

	for substr, wantLine := range map[string]int{
		"topic has no title":           4,  // baseline: the key entry
		"self-approval defeats":        9,  // the revisor entry
		"no review.approved_digest":    6,  // the review block
		"names no topic in this build": 12, // the anchor's list element
		"does not set meta.audience":   4,  // no meta entry to point at → baseline
	} {
		d := find(substr)
		if d.Loc.Line != wantLine {
			t.Errorf("%q at line %d, want %d (%s)", substr, d.Loc.Line, wantLine, d)
		}
		if d.Loc.Column == 0 {
			t.Errorf("%q has no column (%s)", substr, d)
		}
	}

	// String() renders file:line:col so terminals and editors can jump.
	if d := find("topic has no title"); !strings.HasPrefix(d.String(), "t.pxf:4:5: error:") {
		t.Errorf("String() = %q, want a t.pxf:4:5: error: prefix", d.String())
	}
}

// TestPreloadedCatalog pins Options.Catalog (#185): a caller on a
// debounce loads the registry once and hands it in; resolution behaves
// exactly as with CatalogPath.
func TestPreloadedCatalog(t *testing.T) {
	cat, err := LoadCatalog("../testdata/docs/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	src := `@type protowire.docs.v1.TopicFile
topics = [
  {
    key = "widgets.button"
    locale = "en"
    title = "Button"
    anchors = [
      { widget { type = "Button" prop = "text" } }
    ]
  }
]
`
	if err := os.WriteFile(filepath.Join(root, "b.pxf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(Options{Inputs: []string{root}, Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	if keys := compiledKeys(t, result); strings.Join(keys, ",") != "widgets.button" {
		t.Errorf("compiled topics = %v", keys)
	}
}

// TestCompositionAndTransitionAnchors compiles the #199 anchor kinds
// end to end against a v9 catalog: a composition prop resolves on a
// typed widget with the widget's since as fallback, and a transition
// anchor resolves against the catalog-global vocabulary under its
// prefixed canonical id.
func TestCompositionAndTransitionAnchors(t *testing.T) {
	src := `@type protowire.docs.v1.TopicFile
topics = [
  {
    key = "guide.composition"
    locale = "en"
    title = "Templates and transitions"
    anchors = [
      { widget { type = "Label" prop = "slot" } }
      { transition { name = "slide" } }
    ]
  }
]
`
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "c.pxf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(Options{Inputs: []string{root}, CatalogPath: "../testdata/docs/registry_v9.json"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors > 0 {
		t.Fatalf("compile failed:\n%s", formatDiags(result.Diagnostics))
	}
	topic := wrap(result.Pack.ProtoReflect()).msgs("topics")[0]
	got := map[string]dmsg{}
	for _, a := range topic.msgs("anchors") {
		got[a.str("resolved_id")] = a
	}
	slot, ok := got["Label#prop:slot"]
	if !ok {
		t.Fatalf("no anchor resolved to Label#prop:slot; got %v", keysOf(got))
	}
	if since := slot.str("target_since"); since != "0.1.0" {
		t.Errorf("Label#prop:slot target_since = %q, want the widget fallback 0.1.0", since)
	}
	slide, ok := got["transition:slide"]
	if !ok {
		t.Fatalf("no anchor resolved to transition:slide; got %v", keysOf(got))
	}
	if s := slide.enumName("stability"); s != "ANCHOR_STABILITY_STABLE" {
		t.Errorf("transition:slide stability = %s", s)
	}

	// A name outside the vocabulary is a dangling anchor, like any other.
	bad := strings.Replace(src, `name = "slide"`, `name = "warp"`, 1)
	dangling, err := Compile(Options{
		Inputs:      []string{root},
		Overlay:     map[string][]byte{"c.pxf": []byte(bad)},
		CatalogPath: "../testdata/docs/registry_v9.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dangling.Errors == 0 {
		t.Error("a transition outside the catalog vocabulary compiled clean")
	}
}

func formatDiags(ds []Diagnostic) string {
	var sb strings.Builder
	for _, d := range ds {
		sb.WriteString("  " + d.String() + "\n")
	}
	return sb.String()
}

func keysOf(m map[string]dmsg) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
