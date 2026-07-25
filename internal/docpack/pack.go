// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"sort"
)

// Pack assembly.
//
// Everything the compiler decided is written down here, once, in a
// defined order. Consumers read the pack and nothing else: they do not
// re-resolve anchors, re-tokenize prose or re-derive descriptor paths.
// That is what makes a second renderer agree with the first.

func (c *compiler) buildPack() error {
	md, err := message(DocPackMessage)
	if err != nil {
		return err
	}
	pack := newMsg(md)
	c.writeProvenance(pack.newSub("provenance"))

	pack.setStr("source_locale", c.opts.SourceLocale)
	for _, l := range c.localeOrder() {
		pack.appendStr("locales", l)
	}

	index := newIndexBuilder()
	for _, t := range c.topics {
		c.writeTopic(pack.appendMsg("topics"), t)
		c.indexTopic(index, t)
	}
	index.emit(pack.newSub("index"))

	for _, id := range sortedRedirectKeys(c.redirects) {
		c.writeRedirect(pack.appendMsg("redirects"), c.redirects[id])
	}

	c.pack = pack.proto()
	return nil
}

// localeOrder puts the source locale first and sorts the rest. The source
// locale is not one locale among many — it is the origin every
// translation is checked against — so it leads.
func (c *compiler) localeOrder() []string {
	seen := map[string]bool{}
	for _, t := range c.topics {
		seen[t.locale] = true
	}
	out := []string{}
	if seen[c.opts.SourceLocale] {
		out = append(out, c.opts.SourceLocale)
		delete(seen, c.opts.SourceLocale)
	}
	return append(out, sortedKeys(seen)...)
}

func (c *compiler) writeProvenance(prov dmsg) {
	prov.setU32("format_version", packFormatVersion)
	prov.setStr("tool", "pxf docs build")
	prov.setStr("tool_version", c.opts.ToolVersion)
	prov.setBool("release", c.opts.Release)

	if c.image != nil {
		img := prov.newSub("image")
		img.setStr("path", c.image.Path)
		img.setStr("digest", c.image.Digest)
		img.setU32("file_count", uint32(c.image.FileCount))
	}
	if c.catalog != nil {
		cat := prov.newSub("catalog")
		cat.setStr("path", c.catalog.Path)
		cat.setStr("digest", c.catalog.Digest)
		cat.setU32("schema_version", c.catalog.SchemaVersion)
		cat.setU32("widget_count", uint32(c.catalog.WidgetCount))
	}

	sources := append([]*source(nil), c.sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Rel < sources[j].Rel })
	for _, s := range sources {
		f := prov.appendMsg("sources")
		f.setStr("path", s.Rel)
		f.setStr("digest", s.Digest)
	}
}

func (c *compiler) writeTopic(out dmsg, t *topic) {
	// The authored topic travels verbatim: one prose model in the
	// system, and a renderer that reads the pack reads exactly what the
	// author wrote.
	out.setMsg("topic", t.msg)
	out.setStr("content_digest", t.digest)
	out.setStr("source_file", t.src.Rel)

	if t.locale != c.opts.SourceLocale {
		if t.stale {
			out.setEnum("translation_status", "TRANSLATION_STATUS_STALE")
		} else {
			out.setEnum("translation_status", "TRANSLATION_STATUS_CURRENT")
		}
	}

	for _, ra := range t.resolved {
		a := out.appendMsg("anchors")
		a.setMsg("authored", ra.a)
		a.setEnum("origin", ra.origin)
		a.setEnum("stability", ra.stability)
		a.setStr("resolved_id", ra.id)
		if ra.descPath != "" {
			a.setStr("descriptor_path", ra.descPath)
			// Carried per-anchor so an extracted topic stays
			// self-describing about what its fragile paths are valid
			// against.
			if c.image != nil {
				a.setStr("image_digest", c.image.Digest)
			}
		}
		if ra.since != "" {
			a.setStr("target_since", ra.since)
		}
		if len(ra.chain) > 1 {
			for _, hop := range ra.chain {
				a.appendStr("redirect_chain", hop)
			}
		}
	}
}

func (c *compiler) writeRedirect(out dmsg, r *redirectEntry) {
	terminus, chain, err := c.followRedirect(r.fromID)
	if err != nil {
		// Cycles are reported and fail the build before assembly; the
		// single-hop form is the honest fallback if one ever got here.
		terminus, chain = r.toID, nil
	}
	out.setStr("from_id", r.fromID)
	out.setStr("to_id", terminus)
	if len(chain) > 2 {
		for _, hop := range chain {
			out.appendStr("chain", hop)
		}
	}
	if r.since != "" {
		out.setStr("since_version", r.since)
	}
	if r.note != "" {
		out.setStr("note", r.note)
	}
	out.setMsg("from", r.from)
	out.setMsg("to", r.to)
}

// indexTopic feeds one topic to the index builder. Documents are added in
// pack order, which is what SearchOccurrence.document indexes into.
func (c *compiler) indexTopic(index *indexBuilder, t *topic) {
	meta := t.msg.sub("meta")
	index.begin(t.key, t.locale, t.msg.str("title"), meta.enumName("audience"))
	index.add(classTitle, t.msg.str("title"))
	index.add(classSummary, t.msg.str("summary"))
	for _, tag := range meta.strs("tags") {
		index.add(classTag, tag)
	}
	walkProse(t.msg.sub("body"), proseVisitor{
		text: func(class, s string) { index.add(class, s) },
	})
}
