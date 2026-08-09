// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// Source positions for diagnostics (#187).
//
// loadSources already parses every topic file to a PXF AST — with a
// position on every node — before binding it to the typed model; the
// positions used to be dropped there, forcing the editor to re-parse
// the same sources to place a squiggle. This file keeps just enough of
// the AST to answer "where was that entry written": each topic element's
// entry list, indexed by the (key, locale) identity the compiler already
// resolves topics by. Identity-keyed rather than index-paired so the
// mapping cannot drift when the authored form and the bound model
// disagree about element order.

// topicAST is one topic element's source shape.
type topicAST struct {
	// line/column of the topic's `key` entry — the baseline position
	// every diagnostic about this topic gets for free. Falls back to
	// the element itself when the key entry is missing.
	line, column int

	entries []pxf.Entry
}

// astIndex holds what position lookup needs from one parsed source.
type astIndex struct {
	topics map[topicRef]*topicAST

	// redirect element positions, in document order. Redirects have no
	// in-model identity to key by, but collectRedirects walks them in
	// exactly this order, so pairing is positional and guarded by count.
	redirects []pxf.Position
}

// indexAST extracts the topic and redirect element positions from a
// parsed topic file. Both the list form (`topics = [ {...} ]`) and the
// repeated-block form (`topics { ... }` per element) are recognized;
// anything else simply yields no positions — locations degrade to the
// file, never fail the build.
func indexAST(doc *pxf.Document) *astIndex {
	idx := &astIndex{topics: map[topicRef]*topicAST{}}
	for _, e := range doc.Entries {
		switch n := e.(type) {
		case *pxf.Assignment:
			list, ok := n.Value.(*pxf.ListVal)
			if !ok {
				continue
			}
			for _, elem := range list.Elements {
				block, ok := elem.(*pxf.BlockVal)
				if !ok {
					continue
				}
				switch n.Key {
				case "topics":
					idx.addTopic(block.Entries, block.Pos)
				case "redirects":
					idx.redirects = append(idx.redirects, block.Pos)
				}
			}
		case *pxf.Block:
			switch n.Name {
			case "topics":
				idx.addTopic(n.Entries, n.Pos)
			case "redirects":
				idx.redirects = append(idx.redirects, n.Pos)
			}
		}
	}
	return idx
}

func (idx *astIndex) addTopic(entries []pxf.Entry, elemPos pxf.Position) {
	t := &topicAST{line: elemPos.Line, column: elemPos.Column, entries: entries}
	ref := topicRef{}
	if kp, ok := entryAt(entries, "key"); ok {
		t.line, t.column = kp.Line, kp.Column
	}
	ref.key = stringEntry(entries, "key")
	ref.locale = stringEntry(entries, "locale")
	// First writer wins on a duplicate identity: the duplicate is a
	// compile error in its own right, and the first element is the one
	// the error message points back at.
	if _, dup := idx.topics[ref]; !dup {
		idx.topics[ref] = t
	}
}

// entryAt returns the position of the named entry in an entry list.
func entryAt(entries []pxf.Entry, name string) (pxf.Position, bool) {
	for _, e := range entries {
		switch n := e.(type) {
		case *pxf.Assignment:
			if n.Key == name {
				return n.Pos, true
			}
		case *pxf.Block:
			if n.Name == name {
				return n.Pos, true
			}
		}
	}
	return pxf.Position{}, false
}

// childEntries returns the nested entry list of the named entry, for
// both authored forms (`review { ... }` and `review = { ... }`).
func childEntries(entries []pxf.Entry, name string) ([]pxf.Entry, bool) {
	for _, e := range entries {
		switch n := e.(type) {
		case *pxf.Assignment:
			if n.Key != name {
				continue
			}
			if bv, ok := n.Value.(*pxf.BlockVal); ok {
				return bv.Entries, true
			}
			return nil, false
		case *pxf.Block:
			if n.Name == name {
				return n.Entries, true
			}
		}
	}
	return nil, false
}

// stringEntry reads a string assignment's value from an entry list.
func stringEntry(entries []pxf.Entry, name string) string {
	for _, e := range entries {
		if a, ok := e.(*pxf.Assignment); ok && a.Key == name {
			if s, ok := a.Value.(*pxf.StringVal); ok {
				return s.Value
			}
		}
	}
	return ""
}

// at returns the topic's Loc pointed at a named entry — e.g.
// at("review", "approved_digest") — falling back to the key-entry
// baseline whenever the source does not record the path. Intermediate
// path segments traverse nested messages in either authored form.
func (t *topic) at(path ...string) Loc {
	loc := t.loc
	if t.ast == nil || len(path) == 0 {
		return loc
	}
	entries := t.ast.entries
	for i, seg := range path {
		if i == len(path)-1 {
			if p, ok := entryAt(entries, seg); ok {
				loc.Line, loc.Column = p.Line, p.Column
			}
			return loc
		}
		next, ok := childEntries(entries, seg)
		if !ok {
			return loc
		}
		entries = next
	}
	return loc
}

// atListElem returns the topic's Loc pointed at the i-th element of a
// topic-level list entry — used for topic-level anchors, whose bound
// order is the authored order. Falls back to the baseline when the
// source records fewer elements than the model (a shape mismatch means
// pairing is not trustworthy).
func (t *topic) atListElem(name string, i int) Loc {
	loc := t.loc
	if t.ast == nil {
		return loc
	}
	for _, e := range t.ast.entries {
		a, ok := e.(*pxf.Assignment)
		if !ok || a.Key != name {
			continue
		}
		list, ok := a.Value.(*pxf.ListVal)
		if !ok || i >= len(list.Elements) {
			return loc
		}
		p, _ := pxf.ValueSpan(list.Elements[i])
		loc.Line, loc.Column = p.Line, p.Column
		return loc
	}
	return loc
}
