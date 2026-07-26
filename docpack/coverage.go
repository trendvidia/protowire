// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

// The doc-coverage policy (#200).
//
// "Every public registry widget and exported @http endpoint has a
// documenting topic" is a release property, and like the revisor gate
// (#170) it belongs in the compiler: defined once, warning-by-default
// while drafting, refused under release policy — never re-derived by
// each consumer with its own denominator. The compiler already holds
// both sides of the diff at build time: the expected surface (catalog
// entries; @http-annotated methods, the same set `pxf openapi` renders
// operations for) and the documented surface (resolved anchors, with
// redirects already folded to their terminus in resolved_id).
//
// The check is opt-in (Options.Coverage): existing packs must not start
// failing on upgrade.

// Coverage granularities accepted by Options.Coverage.
const (
	// CoverageWidgets checks registry widget types and @http-annotated
	// methods.
	CoverageWidgets = "widgets"
	// CoverageMembers additionally checks per-widget props and events
	// and the screen-transition vocabulary.
	CoverageMembers = "members"
)

// uncoveredRank is the sentinel for "no documenting topic at any tier".
const uncoveredRank = int(^uint(0) >> 1)

// checkCoverage diffs the documentable surface against the corpus's
// resolved anchors. Runs only on an otherwise error-free build: a
// denominator diff over a corpus with dangling anchors would report
// noise on top of real errors.
//
// Audience-aware through AudienceRank: an element demands a topic
// visible at the element's own tier, so an AUDIENCE_INTERNAL element
// would not demand a PUBLIC topic. Neither data input carries element
// tiers today, so every element reads as public — the schema-side half
// of the taxonomy is #173's to introduce, and this rule is already
// written against it.
//
// Common node props and composition props are deliberately absent from
// the denominator: they resolve on every widget (#199), so there is no
// single canonical id to require — requiring them per widget would
// count one cross-cutting mechanic dozens of times. They remain fully
// anchorable; they are just not individually demanded. Consumers
// computing their own editor-side coverage cite this rule instead of
// each re-encoding it.
func (c *compiler) checkCoverage() {
	if c.opts.Coverage == "" {
		return
	}
	if c.catalog == nil && c.image == nil {
		c.d.warnf(Loc{}, "coverage policy is enabled but neither --registry nor --image is present; nothing to check")
		return
	}

	// The numerator: for every resolved id, the least-restricted tier
	// among topics anchoring it — once over all topics, once over
	// approved topics only.
	minRank := map[string]int{}
	minRankApproved := map[string]int{}
	record := func(m map[string]int, id string, tier int) {
		if r, ok := m[id]; !ok || tier < r {
			m[id] = tier
		}
	}
	for _, t := range c.topics {
		tier, ok := AudienceRank(t.msg.sub("meta").enumName("audience"))
		if !ok {
			continue
		}
		approved := t.msg.sub("review").enumName("state") == "REVIEW_STATE_APPROVED"
		for _, ra := range t.resolved {
			record(minRank, ra.id, tier)
			if approved {
				record(minRankApproved, ra.id, tier)
			}
		}
	}
	rank := func(m map[string]int, id string) int {
		if r, ok := m[id]; ok {
			return r
		}
		return uncoveredRank
	}

	need := minRank
	if c.opts.CoverageApproved {
		need = minRankApproved
	}
	elemRank, _ := AudienceRank("") // public: inputs carry no element tiers today

	report := func(loc Loc, what, id string) {
		if rank(need, id) <= elemRank {
			return
		}
		switch {
		case c.opts.CoverageApproved && rank(minRank, id) <= elemRank:
			c.policyf(loc, "%s %s has no approved documenting topic (coverage runs at the documented-and-approved level)", what, id)
		case rank(minRank, id) < uncoveredRank:
			c.policyf(loc, "%s %s is documented only by topics more restricted than the element", what, id)
		default:
			c.policyf(loc, "%s %s has no documenting topic", what, id)
		}
	}

	if c.catalog != nil {
		loc := Loc{File: c.catalog.Path}
		for _, typ := range c.catalog.Widgets() {
			report(loc, "widget", typ)
			if c.opts.Coverage == CoverageMembers {
				for _, p := range c.catalog.Props(typ) {
					report(loc, "widget prop", typ+"#prop:"+p)
				}
				for _, e := range c.catalog.Events(typ) {
					report(loc, "widget event", typ+"#event:"+e)
				}
			}
		}
		if c.opts.Coverage == CoverageMembers {
			for _, tr := range c.catalog.Transitions() {
				report(loc, "transition", "transition:"+tr)
			}
		}
	}
	if c.image != nil {
		loc := Loc{File: c.image.Path}
		for _, m := range c.image.HTTPMethods() {
			report(loc, "@http method", m)
		}
	}
}
