// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

// Prose traversal. Three passes need to walk a topic body — anchor
// collection, heading validation and indexing — and they must agree
// exactly on what the body contains, so they share one walker rather
// than each re-deriving the tree shape.

// Field classes, mirroring proto/docs/v1/pack.proto FieldClass. Indexed
// text is tagged with the class it came from; ranking is the consumer's
// business, so the index records provenance and no weights.
const (
	classTitle   = "FIELD_CLASS_TITLE"
	classSummary = "FIELD_CLASS_SUMMARY"
	classHeading = "FIELD_CLASS_HEADING"
	classBody    = "FIELD_CLASS_BODY"
	classCode    = "FIELD_CLASS_CODE"
	classTag     = "FIELD_CLASS_TAG"
)

// Anchor origins, mirroring proto/docs/v1/pack.proto AnchorOrigin.
const (
	originTopic   = "ANCHOR_ORIGIN_TOPIC"
	originBody    = "ANCHOR_ORIGIN_BODY"
	originExample = "ANCHOR_ORIGIN_EXAMPLE"
)

// proseVisitor receives one callback per interesting node. Every hook is
// optional.
type proseVisitor struct {
	// heading is called for each Heading block, in document order.
	heading func(h dmsg)
	// text is called with each run of indexable text and its class.
	text func(class, s string)
	// anchor is called for each anchor appearing in the body, with the
	// origin that describes where it was written.
	anchor func(origin string, a dmsg)
	// link is called for each Link inline run.
	link func(l dmsg)
	// image is called for each ImageBlock.
	image func(img dmsg)
}

// walkProse visits a topic body depth-first in document order.
func walkProse(body dmsg, v proseVisitor) {
	if !body.valid() {
		return
	}
	walkBlocks(body.msgs("blocks"), v)
}

func walkBlocks(blocks []dmsg, v proseVisitor) {
	for _, b := range blocks {
		switch b.which("kind") {
		case "paragraph":
			walkRuns(b.sub("paragraph").msgs("runs"), classBody, v)
		case "heading":
			h := b.sub("heading")
			if v.heading != nil {
				v.heading(h)
			}
			walkRuns(h.msgs("runs"), classHeading, v)
		case "code":
			if v.text != nil {
				c := b.sub("code")
				v.text(classCode, c.str("source"))
				v.text(classBody, c.str("caption"))
			}
		case "list":
			for _, item := range b.sub("list").msgs("items") {
				walkBlocks(item.msgs("blocks"), v)
			}
		case "admonition":
			walkBlocks(b.sub("admonition").msgs("blocks"), v)
		case "image":
			img := b.sub("image")
			if v.image != nil {
				v.image(img)
			}
			if v.text != nil {
				// Alt text is indexed: it is the only textual content of
				// an image, and a reader who searches for it should find
				// the topic that shows it.
				v.text(classBody, img.str("alt"))
				v.text(classBody, img.str("caption"))
			}
		case "table":
			t := b.sub("table")
			for _, group := range [][]dmsg{t.msgs("header"), t.msgs("rows")} {
				for _, row := range group {
					for _, cell := range row.msgs("cells") {
						walkRuns(cell.msgs("runs"), classBody, v)
					}
				}
			}
			if v.text != nil {
				v.text(classBody, t.str("caption"))
			}
		case "example":
			ex := b.sub("example")
			if subject := ex.sub("subject"); subject.valid() && v.anchor != nil {
				v.anchor(originExample, subject)
			}
			if v.text != nil {
				v.text(classCode, ex.str("source"))
				v.text(classBody, ex.str("caption"))
			}
		}
	}
}

func walkRuns(runs []dmsg, class string, v proseVisitor) {
	for _, r := range runs {
		switch kind := r.which("kind"); kind {
		case "text", "emphasis", "strong":
			if v.text != nil {
				v.text(class, r.str(kind))
			}
		case "code":
			if v.text != nil {
				v.text(classCode, r.str("code"))
			}
		case "link":
			l := r.sub("link")
			if v.link != nil {
				v.link(l)
			}
			if v.text != nil {
				v.text(class, l.str("text"))
			}
		case "anchor_ref":
			ref := r.sub("anchor_ref")
			if a := ref.sub("anchor"); a.valid() && v.anchor != nil {
				v.anchor(originBody, a)
			}
			if v.text != nil {
				v.text(class, ref.str("text"))
			}
		}
	}
}

// runsText renders a sequence of inline runs as plain text. Used for
// derived heading ids and for diagnostics that quote a heading.
func runsText(runs []dmsg) string {
	var out string
	for _, r := range runs {
		switch kind := r.which("kind"); kind {
		case "text", "emphasis", "strong", "code":
			out += r.str(kind)
		case "link":
			out += r.sub("link").str("text")
		case "anchor_ref":
			out += r.sub("anchor_ref").str("text")
		}
	}
	return out
}
