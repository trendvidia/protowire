// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
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
	cat, err := loadCatalog("../../testdata/docs/registry.json")
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
		since, err := cat.resolveWidget(id)
		if err != nil {
			t.Errorf("resolveWidget(%q): %v", id, err)
			continue
		}
		if since != want {
			t.Errorf("resolveWidget(%q) since = %q, want %q", id, since, want)
		}
	}

	for _, id := range []string{"Nope", "Button#prop:colour", "Button#event:onHover"} {
		if _, err := cat.resolveWidget(id); err == nil {
			t.Errorf("resolveWidget(%q) resolved; expected an error", id)
		}
	}
}
