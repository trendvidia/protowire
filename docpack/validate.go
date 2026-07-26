// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"
	"strings"
)

// Structural validation.
//
// These are the rules a v1.2 `@validate` would carry if the doc model
// were written in v1.2 grammar. It is not: the model is stock proto3 so
// every port and every editor can parse it without a v1.2-capable
// parser, the same trade already made for schema/v1/report.proto. The
// rules therefore live here, in the compiler — which is also where the
// issue asks for the load-bearing ones to live: the revisor gate is a
// compiler policy, not IDE machinery, precisely so that no authoring
// tool can decide to skip it.

// validateIdentity checks the facts that make a topic addressable. It
// runs before anything else, because a topic without a usable key and
// locale cannot be reported against coherently.
func (c *compiler) validateIdentity(t *topic) bool {
	ok := true
	if t.key == "" {
		c.d.errorf(t.loc, "topic has no key")
		ok = false
	} else if !topicPattern.MatchString(t.key) {
		c.d.errorf(t.loc, "topic key %q must be dot-separated lowercase segments of [a-z0-9_]", t.key)
		ok = false
	}
	if t.locale == "" {
		c.d.errorf(t.loc, "topic has no locale (the pack's source locale is %s)", c.opts.SourceLocale)
		ok = false
	} else if !localePattern.MatchString(t.locale) {
		c.d.errorf(t.loc, "locale %q is not a BCP 47 language tag", t.locale)
		ok = false
	}
	return ok
}

// validateTopic runs every within-topic rule and collects the anchors the
// resolution pass will need.
func (c *compiler) validateTopic(t *topic) {
	if t.msg.str("title") == "" {
		c.d.errorf(t.loc, "topic has no title")
	}
	c.validateMeta(t)
	c.validateReview(t)
	c.validateBody(t)

	// Topic-level anchors come first, in authored order, so the pack's
	// anchor list reads the way the topic does. Each carries the source
	// position of its own list element; prose anchors are nested too deep
	// for the retained AST and keep the topic baseline (#187).
	for i, a := range t.msg.msgs("anchors") {
		t.anchors = append(t.anchors, collectedAnchor{origin: originTopic, a: a, loc: t.atListElem("anchors", i)})
	}
	walkProse(t.msg.sub("body"), proseVisitor{
		anchor: func(origin string, a dmsg) {
			t.anchors = append(t.anchors, collectedAnchor{origin: origin, a: a, loc: t.loc})
		},
	})
}

func (c *compiler) validateMeta(t *topic) {
	meta := t.msg.sub("meta")
	if !meta.valid() || meta.enumNum("audience") == 0 {
		// Unset audience is a decision nobody made. Tolerated while
		// drafting (it defaults to public), refused at release.
		c.policyf(t.at("meta"), "topic does not set meta.audience; defaulting to AUDIENCE_PUBLIC")
	}
	seen := map[string]bool{}
	for _, tag := range meta.strs("tags") {
		switch {
		case !tagPattern.MatchString(tag):
			c.d.errorf(t.at("meta", "tags"), "tag %q must be lowercase [a-z0-9] words joined by %q", tag, "-")
		case seen[tag]:
			c.d.errorf(t.at("meta", "tags"), "tag %q is listed twice", tag)
		}
		seen[tag] = true
	}
	if parent := meta.str("parent"); parent != "" {
		if !topicPattern.MatchString(parent) {
			c.d.errorf(t.at("meta", "parent"), "meta.parent %q is not a topic key", parent)
		} else if parent == t.key {
			c.d.errorf(t.at("meta", "parent"), "meta.parent points at the topic itself")
		}
	}
	if sup := meta.str("superseded_by"); sup != "" && !topicPattern.MatchString(sup) {
		c.d.errorf(t.at("meta", "superseded_by"), "meta.superseded_by %q is not a topic key", sup)
	}
}

// validateReview enforces the review state machine and the revisor gate.
//
// The gate has three parts, and all three are needed for approval to mean
// anything: a state, a revisor who is not the author, and a digest that
// still matches the content. Drop the digest and approval survives any
// later edit; drop the revisor check and a topic can approve itself.
func (c *compiler) validateReview(t *topic) {
	review := t.msg.sub("review")
	state := review.enumName("state")

	if !review.valid() || state == "REVIEW_STATE_UNSPECIFIED" {
		c.policyf(t.at("review"), "topic has no review state; release builds require REVIEW_STATE_APPROVED")
		return
	}
	author, revisor := review.str("author"), review.str("revisor")
	if digest := review.str("approved_digest"); digest != "" && !digestPattern.MatchString(digest) {
		c.d.errorf(t.at("review", "approved_digest"), "review.approved_digest is not a lowercase hex SHA-256")
	}
	if revisor != "" && revisor == author {
		c.d.errorf(t.at("review", "revisor"), "review.revisor %q is also the author; self-approval defeats the gate", revisor)
	}

	if state != "REVIEW_STATE_APPROVED" {
		c.policyf(t.at("review", "state"), "topic is %s; release builds require REVIEW_STATE_APPROVED", state)
		return
	}
	if revisor == "" {
		c.d.errorf(t.at("review"), "topic is REVIEW_STATE_APPROVED with no review.revisor")
	}
	switch approved := review.str("approved_digest"); {
	case approved == "":
		c.d.errorf(t.at("review"), "topic is REVIEW_STATE_APPROVED with no review.approved_digest")
	case approved != t.digest:
		// The normal authoring loop passes through this state: edit an
		// approved topic and the sign-off no longer covers it. That is a
		// warning while working (goed runs this compiler on its
		// diagnostics debounce) and a refusal at release.
		c.policyf(t.at("review", "approved_digest"), "content changed since approval (approved %s, current %s); re-review required",
			shortDigest(approved), shortDigest(t.digest))
	}
}

// validateBody checks heading structure, link targets and image alt text.
func (c *compiler) validateBody(t *topic) {
	var lastLevel uint32
	ids := map[string]bool{}

	walkProse(t.msg.sub("body"), proseVisitor{
		heading: func(h dmsg) {
			level := h.u32("level")
			text := runsText(h.msgs("runs"))
			switch {
			case level == 0:
				c.d.errorf(t.loc, "heading %q has level 0; levels are 1-based below the topic title", text)
			case lastLevel == 0 && level != 1:
				c.d.errorf(t.loc, "first heading %q is level %d; a topic's headings start at level 1", text, level)
			case level > lastLevel+1 && lastLevel != 0:
				c.d.errorf(t.loc, "heading %q jumps from level %d to %d; heading structure is navigation, not styling",
					text, lastLevel, level)
			}
			if level > 0 {
				lastLevel = level
			}

			id := h.str("id")
			if id == "" {
				id = slugify(text)
			}
			switch {
			case id == "":
				c.d.errorf(t.loc, "heading %q yields no fragment id; give it an explicit id", text)
			case ids[id]:
				c.d.errorf(t.loc, "heading fragment id %q is used twice in this topic", id)
			}
			ids[id] = true
		},
		image: func(img dmsg) {
			if img.str("path") == "" {
				c.d.errorf(t.loc, "image block has no path")
			}
			if img.str("alt") == "" {
				// Accessibility is a property of the model, not of one
				// renderer's diligence — so it fails the build, always.
				c.d.errorf(t.loc, "image %q has no alt text", img.str("path"))
			}
		},
		link: func(l dmsg) {
			url, key := l.str("url"), l.str("topic")
			switch {
			case url == "" && key == "":
				c.d.errorf(t.loc, "link %q targets neither a url nor a topic", l.str("text"))
			case url != "" && key != "":
				c.d.errorf(t.loc, "link %q sets both url %q and topic %q", l.str("text"), url, key)
			case url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://"):
				c.d.errorf(t.loc, "link url %q is not an absolute http(s) URL", url)
			case key != "":
				t.links = append(t.links, link{key: key, fragment: l.str("fragment"), text: l.str("text")})
			}
		},
	})
	c.headingIDs[t.ref()] = ids
}

// ── Cross-topic rules ─────────────────────────────────────────────────────

// validateCrossTopic runs the rules that need the whole corpus: parent
// chains, in-pack links, translation provenance, and the audience
// consistency rule.
func (c *compiler) validateCrossTopic() {
	for _, t := range c.topics {
		meta := t.msg.sub("meta")

		if parent := meta.str("parent"); parent != "" && topicPattern.MatchString(parent) && parent != t.key {
			if !c.hasTopic(parent, t.locale) {
				c.d.errorf(t.at("meta", "parent"), "meta.parent %q names no topic in this build (locale %s)", parent, t.locale)
			}
		}
		if sup := meta.str("superseded_by"); sup != "" && topicPattern.MatchString(sup) && !c.hasKey(sup) {
			c.d.errorf(t.at("meta", "superseded_by"), "meta.superseded_by %q names no topic in this build", sup)
		}
		for _, l := range t.links {
			switch {
			case !topicPattern.MatchString(l.key):
				c.d.errorf(t.loc, "link %q targets %q, which is not a topic key", l.text, l.key)
			case !c.hasTopic(l.key, t.locale):
				c.d.errorf(t.loc, "link %q targets topic %q, which is not in this build (locale %s)", l.text, l.key, t.locale)
			case l.fragment != "":
				target := topicRef{key: l.key, locale: t.locale}
				if ids := c.headingIDs[target]; !ids[l.fragment] {
					c.d.errorf(t.loc, "link %q targets %s#%s, which is not a heading in that topic",
						l.text, l.key, l.fragment)
				}
			}
		}
		c.validateParentChain(t)
		c.validateTranslation(t)
	}
}

// validateParentChain rejects cycles in the navigation tree. A cycle is
// not merely a rendering problem: a tree walk over it does not terminate,
// and every consumer would have to defend against it separately.
func (c *compiler) validateParentChain(t *topic) {
	seen := map[string]bool{t.key: true}
	cur := t
	for {
		parent := cur.msg.sub("meta").str("parent")
		if parent == "" {
			return
		}
		if seen[parent] {
			c.d.errorf(t.at("meta", "parent"), "meta.parent chain cycles through %q", parent)
			return
		}
		seen[parent] = true
		next := c.topic(parent, cur.locale)
		if next == nil {
			return // already reported by validateCrossTopic
		}
		cur = next
	}
}

// validateTranslation checks that a localized topic declares what it was
// translated from, and reports drift when the source has moved on.
func (c *compiler) validateTranslation(t *topic) {
	tr := t.msg.sub("translation")
	if t.locale == c.opts.SourceLocale {
		if tr.valid() {
			c.d.errorf(t.at("translation"), "topic is in the source locale %s but carries translation provenance", c.opts.SourceLocale)
		}
		return
	}
	source := c.topic(t.key, c.opts.SourceLocale)
	if source == nil {
		c.d.errorf(t.loc, "translation has no source-locale topic %q in %s", t.key, c.opts.SourceLocale)
		return
	}
	if !tr.valid() || tr.str("source_digest") == "" {
		c.d.errorf(t.at("translation"), "translation does not record translation.source_digest")
		return
	}
	digest := tr.str("source_digest")
	if !digestPattern.MatchString(digest) {
		c.d.errorf(t.at("translation", "source_digest"), "translation.source_digest is not a lowercase hex SHA-256")
		return
	}
	if digest != source.digest {
		t.stale = true
		// A stale translation is real content, correctly typed, merely
		// behind. Warned by default so the i18n workflow has a signal
		// instead of tribal knowledge; escalated by policy for builds
		// that must not ship drift.
		c.staleTranslationf(t.at("translation", "source_digest"), "translation is stale: %s in %s has changed since translation (from %s, now %s)",
			t.key, c.opts.SourceLocale, shortDigest(digest), shortDigest(source.digest))
	}
}

// ── Policy-sensitive diagnostics ──────────────────────────────────────────

// policyf reports something that is a warning while authoring and a
// refusal at release: unset audience, unapproved review state, approval
// invalidated by a later edit. Release mode is the revisor gate.
func (c *compiler) policyf(loc Loc, format string, args ...any) {
	if c.opts.Release {
		c.d.errorf(loc, "%s (release policy)", fmt.Sprintf(format, args...))
		return
	}
	c.d.warnf(loc, format, args...)
}

// staleTranslationf reports translation drift under the caller's chosen
// severity.
func (c *compiler) staleTranslationf(loc Loc, format string, args ...any) {
	if c.opts.StaleTranslationsFatal {
		c.d.errorf(loc, format, args...)
		return
	}
	c.d.warnf(loc, format, args...)
}

func shortDigest(d string) string {
	if len(d) <= 12 {
		return d
	}
	return d[:12]
}
