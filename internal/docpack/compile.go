// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package docpack compiles authored documentation topics into a doc pack
// — the typed interchange artifact `pxf docs build` emits (issue #170).
//
// The pipeline mirrors `pxf build`'s: sources in, one deterministic
// binary artifact out, `--check` for CI. What it compiles is prose and
// anchors rather than schemas, and its two data inputs are the lowered
// schema image (#164) and the appviewer registry export
// (trendvidia/appviewer#33) — data, never code.
//
//	topics/*.pxf ─┐
//	schema image ─┼─► pxf docs build ─► doc pack
//	registry data ┘
//
// Downstream: appviewer packages the pack as a bundle data section and
// serves runtime help and search (trendvidia/appviewer#364); goed drives
// the review states and runs this compiler on its diagnostics debounce
// (trendvidia/goed#321); the OpenAPI (#173) and static-HTML (#171)
// renderers read pack plus image.
package docpack

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"
)

// packFormatVersion versions the pack layout, not its contents. Bump it
// when proto/docs/v1/pack.proto changes in a way consumers must notice —
// including a change to what the content digest covers.
const packFormatVersion = 1

// Options is the resolved input surface of one compilation.
type Options struct {
	// Inputs are topic roots or files, as given on the command line.
	Inputs []string

	// ImagePath is the lowered FileDescriptorSet schema anchors resolve
	// against. Optional: a corpus with no schema anchors needs no image.
	ImagePath string

	// CatalogPath is the appviewer registry export widget anchors
	// resolve against. Optional, on the same terms.
	CatalogPath string

	// SourceLocale is the locale topics are authored in. Defaults to "en".
	SourceLocale string

	// Release applies release policy: unreviewed topics, unset audience
	// tiers and approvals invalidated by later edits become errors.
	Release bool

	// StaleTranslationsFatal escalates translation drift from warning to
	// error independently of Release, for pipelines that ship no drift.
	StaleTranslationsFatal bool

	// ToolVersion is recorded in the pack's provenance.
	ToolVersion string
}

// Result is one compilation's output.
type Result struct {
	// Pack is the compiled DocPack, or nil when the build had errors.
	Pack proto.Message

	Diagnostics []Diagnostic
	Errors      int
}

// Compile runs the whole pipeline. It returns an error only for problems
// that prevent compiling at all — unreadable inputs, a corrupt image.
// Everything about the documentation itself comes back as diagnostics,
// so one bad topic never hides the rest.
func Compile(opts Options) (*Result, error) {
	if opts.SourceLocale == "" {
		opts.SourceLocale = "en"
	}
	if !localePattern.MatchString(opts.SourceLocale) {
		return nil, fmt.Errorf("source locale %q is not a BCP 47 language tag", opts.SourceLocale)
	}
	c := &compiler{
		opts:       opts,
		d:          &diags{},
		byRef:      map[topicRef]*topic{},
		byKey:      map[string][]*topic{},
		headingIDs: map[topicRef]map[string]bool{},
		redirects:  map[string]*redirectEntry{},
	}
	if err := c.run(); err != nil {
		return nil, err
	}
	return &Result{Pack: c.pack, Diagnostics: c.d.sorted(), Errors: c.d.errors()}, nil
}

// ── Compiler state ────────────────────────────────────────────────────────

type compiler struct {
	opts Options
	d    *diags

	image   *Image
	catalog *Catalog

	sources []*source
	topics  []*topic
	byRef   map[topicRef]*topic
	byKey   map[string][]*topic

	headingIDs map[topicRef]map[string]bool
	redirects  map[string]*redirectEntry

	pack proto.Message
}

type topicRef struct {
	key    string
	locale string
}

type topic struct {
	src    *source
	msg    dmsg
	key    string
	locale string
	digest string
	loc    Loc

	anchors  []collectedAnchor
	resolved []resolvedAnchor
	links    []link
	stale    bool
}

func (t *topic) ref() topicRef { return topicRef{key: t.key, locale: t.locale} }

type collectedAnchor struct {
	origin string
	a      dmsg
}

type resolvedAnchor struct {
	collectedAnchor
	kind      string
	id        string
	stability string
	chain     []string
	since     string
	descPath  string
}

type link struct {
	key      string
	fragment string
	text     string
}

type redirectEntry struct {
	from, to dmsg
	fromID   string
	toID     string
	kind     string
	since    string
	note     string
	loc      Loc
}

func (c *compiler) topic(key, locale string) *topic { return c.byRef[topicRef{key, locale}] }
func (c *compiler) hasTopic(key, locale string) bool {
	return c.byRef[topicRef{key, locale}] != nil
}
func (c *compiler) hasKey(key string) bool { return len(c.byKey[key]) > 0 }

// ── Pipeline ──────────────────────────────────────────────────────────────

func (c *compiler) run() error {
	sources, err := loadSources(c.opts.Inputs, c.d)
	if err != nil {
		return err
	}
	c.sources = sources
	if c.opts.ImagePath != "" {
		if c.image, err = loadImage(c.opts.ImagePath); err != nil {
			return err
		}
	}
	if c.opts.CatalogPath != "" {
		if c.catalog, err = loadCatalog(c.opts.CatalogPath); err != nil {
			return err
		}
		if c.catalog.SchemaVersion > catalogFormatVersion {
			c.d.warnf(Loc{File: c.catalog.Path},
				"registry export is catalog schema version %d; this compiler understands %d — newer entries may not resolve",
				c.catalog.SchemaVersion, catalogFormatVersion)
		}
	}

	c.collectTopics(sources)
	c.collectRedirects(sources)
	for _, t := range c.topics {
		c.validateTopic(t)
	}
	c.validateCrossTopic()
	c.resolveAnchors()

	if c.d.errors() > 0 {
		return nil
	}
	return c.buildPack()
}

// collectTopics indexes every topic by (key, locale) and rejects
// duplicates. Identity is (key, locale), never the file path, so two
// files claiming the same identity is a real ambiguity rather than a
// filesystem detail.
func (c *compiler) collectTopics(sources []*source) {
	for _, src := range sources {
		for _, msg := range src.File.msgs("topics") {
			t := &topic{
				src:    src,
				msg:    msg,
				key:    msg.str("key"),
				locale: msg.str("locale"),
			}
			t.loc = Loc{File: src.Rel, Topic: t.key}
			if !c.validateIdentity(t) {
				continue
			}
			digest, err := contentDigest(msg)
			if err != nil {
				c.d.errorf(t.loc, "computing content digest: %v", err)
				continue
			}
			t.digest = digest

			if prev, dup := c.byRef[t.ref()]; dup {
				c.d.errorf(t.loc, "topic %s/%s is already defined in %s", t.key, t.locale, prev.src.Rel)
				continue
			}
			c.byRef[t.ref()] = t
			c.byKey[t.key] = append(c.byKey[t.key], t)
			c.topics = append(c.topics, t)
		}
	}
	// Sorted by (key, locale) from here on: the pack's order, the index's
	// document order, and the order diagnostics come out in.
	sort.Slice(c.topics, func(i, j int) bool {
		if c.topics[i].key != c.topics[j].key {
			return c.topics[i].key < c.topics[j].key
		}
		return c.topics[i].locale < c.topics[j].locale
	})
}

// collectRedirects builds the redirect table and rejects the two ways it
// can be malformed: a target that means nothing, and a chain that loops.
func (c *compiler) collectRedirects(sources []*source) {
	for _, src := range sources {
		loc := Loc{File: src.Rel}
		for _, r := range src.File.msgs("redirects") {
			from, to := r.sub("from"), r.sub("to")
			if !from.valid() || !to.valid() {
				c.d.errorf(loc, "redirect must set both from and to")
				continue
			}
			fromKind, fromID, err := anchorID(from)
			if err != nil {
				c.d.errorf(loc, "redirect from: %v", err)
				continue
			}
			toKind, toID, err := anchorID(to)
			if err != nil {
				c.d.errorf(loc, "redirect to: %v", err)
				continue
			}
			if fromKind != toKind {
				// A rename moves a thing; it does not change what kind of
				// thing it is. Allowing it would make the redirect table
				// untypeable and every consumer's lookup ambiguous.
				c.d.errorf(loc, "redirect %s → %s crosses anchor kinds (%s → %s)", fromID, toID, fromKind, toKind)
				continue
			}
			if fromID == toID {
				c.d.errorf(loc, "redirect %s points at itself", fromID)
				continue
			}
			if prev, dup := c.redirects[fromID]; dup {
				c.d.errorf(loc, "redirect for %s is already declared in %s", fromID, prev.loc.File)
				continue
			}
			c.redirects[fromID] = &redirectEntry{
				from: from, to: to,
				fromID: fromID, toID: toID, kind: fromKind,
				since: r.str("since_version"), note: r.str("note"),
				loc: loc,
			}
		}
	}
	for _, id := range sortedRedirectKeys(c.redirects) {
		if _, _, err := c.followRedirect(id); err != nil {
			c.d.errorf(c.redirects[id].loc, "%v", err)
		}
	}
}

// followRedirect resolves a redirect chain to its terminus, returning the
// hops travelled. Cycles are an error: the compiler resolves chains so
// consumers never have to, and a consumer that had to would loop.
func (c *compiler) followRedirect(id string) (terminus string, chain []string, err error) {
	seen := map[string]bool{id: true}
	chain = []string{id}
	cur := id
	for {
		entry, ok := c.redirects[cur]
		if !ok {
			return cur, chain, nil
		}
		if seen[entry.toID] {
			return "", nil, fmt.Errorf("redirect chain from %s cycles through %s", id, entry.toID)
		}
		seen[entry.toID] = true
		chain = append(chain, entry.toID)
		cur = entry.toID
	}
}

func sortedRedirectKeys(m map[string]*redirectEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── Anchor resolution ─────────────────────────────────────────────────────

// resolveAnchors resolves every anchor in the build against the data
// inputs. A dangling anchor is an error and a moved target is a redirect
// — the runtime never shrugs at a miss, because by the time it sees the
// pack there is nothing left to shrug about.
func (c *compiler) resolveAnchors() {
	for _, t := range c.topics {
		for _, ca := range t.anchors {
			kind, id, err := anchorID(ca.a)
			if err != nil {
				c.d.errorf(t.loc, "%v", err)
				continue
			}
			ra := resolvedAnchor{collectedAnchor: ca, kind: kind, id: id, stability: stabilityOf(kind)}

			// An author who declares a redirect means it: follow it even
			// when the retired target still happens to exist.
			if _, declared := c.redirects[id]; declared {
				terminus, chain, ferr := c.followRedirect(id)
				if ferr != nil {
					continue // reported once, at the redirect
				}
				ra.chain = chain
				ra.since = c.redirects[id].since
				ra.id = terminus
			}

			if err := c.resolveTarget(t, &ra); err != nil {
				c.d.errorf(t.loc, "%v", err)
				continue
			}
			t.resolved = append(t.resolved, ra)
		}
	}
}

// resolveTarget checks a resolved id against the data input that owns
// that anchor kind.
func (c *compiler) resolveTarget(t *topic, ra *resolvedAnchor) error {
	switch ra.kind {
	case anchorSchema:
		if c.image == nil {
			return fmt.Errorf("schema anchor %s needs a lowered image (pass --image)", ra.id)
		}
		if _, ok := c.image.lookup(ra.id); !ok {
			return fmt.Errorf("schema anchor %s resolves to nothing in %s%s", ra.id, c.image.Path, redirectHint(ra))
		}
		return nil

	case anchorDescriptorPath:
		if c.image == nil {
			return fmt.Errorf("descriptor-path anchor %s needs a lowered image (pass --image)", ra.id)
		}
		// Re-derived on every build, never trusted from the source: the
		// path grammar is a toolchain artifact (trendvidia/protolsp#260).
		if !c.image.hasPath(ra.id) {
			element := ra.a.sub(anchorDescriptorPath).str("element_fqn")
			// Distinguish "no such element" from "that element carries no
			// such annotation": the first is a renamed or removed target,
			// the second is an ordinal that has drifted.
			if _, ok := c.image.lookup(element); !ok {
				return fmt.Errorf("descriptor-path anchor %s names element %s, which is not in %s%s",
					ra.id, element, c.image.Path, redirectHint(ra))
			}
			return fmt.Errorf("descriptor-path anchor %s is not in the image's source map (annotations recorded on %s: %s)",
				ra.id, element, c.image.annotationsOn(element))
		}
		ra.descPath = ra.id
		return nil

	case anchorWidget:
		if c.catalog == nil {
			return fmt.Errorf("widget anchor %s needs a registry export (pass --registry)", ra.id)
		}
		since, err := c.catalog.resolveWidget(ra.id)
		if err != nil {
			return err
		}
		if since != "" {
			ra.since = since
		}
		return nil

	case anchorRoute:
		// Routes are app-owned strings with no data input to check them
		// against. The shape is validated in anchorID; the pack records
		// the route for the runtime to bind.
		return nil

	case anchorTopic:
		target := c.topic(ra.id, t.locale)
		if target == nil {
			if c.hasKey(ra.id) {
				return fmt.Errorf("topic anchor %s exists but not in locale %s", ra.id, t.locale)
			}
			return fmt.Errorf("topic anchor %s names no topic in this build%s", ra.id, redirectHint(ra))
		}
		if err := c.checkAudience(t, target, "anchors"); err != nil {
			return err
		}
		if since := target.msg.sub("meta").str("since"); since != "" && ra.since == "" {
			ra.since = since
		}
		return nil
	}
	return fmt.Errorf("anchor %s has an unhandled kind %q", ra.id, ra.kind)
}

func redirectHint(ra *resolvedAnchor) string {
	if len(ra.chain) > 1 {
		return fmt.Sprintf(" (via redirect from %s)", ra.chain[0])
	}
	return " (a renamed target needs a redirect entry)"
}

// audienceRank orders the visibility tiers by widening restriction. An
// unset tier reads as public, matching the compiler's documented default.
//
// This map is the single in-repo definition of the taxonomy's ordering
// (protowire.docs.v1.Audience); consumers outside the doc pipeline reach
// it through AudienceRank so the ordering is never forked.
var audienceRank = map[string]int{
	"AUDIENCE_UNSPECIFIED": 1,
	"AUDIENCE_PUBLIC":      1,
	"AUDIENCE_COMMUNITY":   2,
	"AUDIENCE_PARTNER":     3,
	"AUDIENCE_ENTERPRISE":  4,
	"AUDIENCE_INTERNAL":    5,
}

// AudienceRank returns the restriction rank of a protowire.docs.v1
// Audience value name ("AUDIENCE_PUBLIC" → 1 … "AUDIENCE_INTERNAL" → 5),
// with the empty string and AUDIENCE_UNSPECIFIED reading as public. The
// second result is false for a name outside the taxonomy.
func AudienceRank(name string) (int, bool) {
	if name == "" {
		return audienceRank["AUDIENCE_PUBLIC"], true
	}
	r, ok := audienceRank[name]
	return r, ok
}

// checkAudience enforces transitive consistency: a topic must not point
// at a more restricted one. Rendering-time filtering would otherwise
// produce a link to a page the reader is not allowed to see, which is
// both a broken link and an information leak about what exists.
func (c *compiler) checkAudience(from, to *topic, how string) error {
	fromTier := from.msg.sub("meta").enumName("audience")
	toTier := to.msg.sub("meta").enumName("audience")
	if audienceRank[toTier] > audienceRank[fromTier] {
		return fmt.Errorf("%s topic %s %s %s topic %s; a topic may not point at a more restricted one",
			displayAudience(fromTier), from.key, how, displayAudience(toTier), to.key)
	}
	return nil
}

func displayAudience(tier string) string {
	if tier == "" || tier == "AUDIENCE_UNSPECIFIED" {
		return "AUDIENCE_PUBLIC (default)"
	}
	return tier
}
