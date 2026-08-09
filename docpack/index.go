// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"sort"
	"strings"
	"unicode"
)

// The embedded full-text index.
//
// Indexing lives in the compiler because it needs parsed content, and
// because the index must be consumable beyond appviewer — goed's help,
// the static-HTML export's client-side search (#171), a docs-site
// export. Every one of those loads the index as data; none of them runs
// a search service.
//
// The index records how it was built (Tokenization) so a consumer can
// tokenize a query identically. A query tokenized differently from the
// index is a search that quietly misses, and "quietly" is the part that
// makes it a bug nobody files.

const (
	// tokenizerAlgorithm identifies the tokenization contract. Bump it
	// when a change alters results, so a consumer holding an older pack
	// can tell.
	tokenizerAlgorithm = "protowire.docs.v1.simple"

	// minTokenLength drops single characters, which carry no selectivity
	// and inflate the index.
	minTokenLength = 2
)

// tokenize splits text into index terms.
//
// Terms are lowercased and split on everything that is not a letter, a
// digit, or one of the identifier joiners `_ . -`. A token containing a
// joiner is emitted whole *and* split into its parts at the same
// position: "Button.text" is findable as "button.text", "button" and
// "text", which is what makes searching documentation about code work
// without a second, code-specific index.
func tokenize(s string) []string {
	var out []string
	for _, raw := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '.' && r != '-'
	}) {
		tok := strings.Trim(strings.ToLower(raw), "_.-")
		if len(tok) < minTokenLength {
			continue
		}
		out = append(out, tok)
		if strings.ContainsAny(tok, "_.-") {
			for _, part := range strings.FieldsFunc(tok, func(r rune) bool {
				return r == '_' || r == '.' || r == '-'
			}) {
				if len(part) >= minTokenLength && part != tok {
					out = append(out, part)
				}
			}
		}
	}
	return out
}

// indexDoc is one document in the index under construction.
type indexDoc struct {
	key      string
	locale   string
	title    string
	audience string
	tokens   uint32
}

type occKey struct {
	doc   uint32
	class string
}

type occurrence struct {
	count     uint32
	positions []uint32
}

// indexBuilder accumulates postings across topics.
type indexBuilder struct {
	docs     []indexDoc
	postings map[string]map[occKey]*occurrence

	// cursor state for the document being added
	current  uint32
	position map[string]uint32 // per-class token position within the document
	locales  map[string]bool
}

func newIndexBuilder() *indexBuilder {
	return &indexBuilder{
		postings: map[string]map[occKey]*occurrence{},
		locales:  map[string]bool{},
	}
}

// begin starts a document. Documents are added in DocPack.topics order,
// which is what SearchOccurrence.document indexes into.
func (b *indexBuilder) begin(key, locale, title, audience string) {
	b.docs = append(b.docs, indexDoc{key: key, locale: locale, title: title, audience: audience})
	b.current = uint32(len(b.docs) - 1)
	b.position = map[string]uint32{}
	b.locales[locale] = true
}

// add indexes one run of text under a field class.
func (b *indexBuilder) add(class, text string) {
	if text == "" {
		return
	}
	doc := &b.docs[b.current]
	for _, term := range tokenize(text) {
		pos := b.position[class]
		b.position[class] = pos + 1
		doc.tokens++

		byKey, ok := b.postings[term]
		if !ok {
			byKey = map[occKey]*occurrence{}
			b.postings[term] = byKey
		}
		k := occKey{doc: b.current, class: class}
		occ, ok := byKey[k]
		if !ok {
			occ = &occurrence{}
			byKey[k] = occ
		}
		occ.count++
		occ.positions = append(occ.positions, pos)
	}
}

// emit writes the accumulated index into a SearchIndex message.
//
// Every repeated field is sorted: terms lexicographically, occurrences by
// (document, class, first position). The pack is diffed and cached, so a
// map-iteration order leaking into the output would be a defect, not a
// cosmetic issue.
func (b *indexBuilder) emit(out dmsg) {
	tok := out.newSub("tokenization")
	tok.setStr("algorithm", tokenizerAlgorithm)
	tok.setBool("case_folded", true)
	tok.setU32("min_token_length", minTokenLength)
	for _, l := range sortedKeys(b.locales) {
		tok.appendStr("locales", l)
	}

	for _, d := range b.docs {
		doc := out.appendMsg("documents")
		doc.setStr("topic_key", d.key)
		doc.setStr("locale", d.locale)
		doc.setStr("title", d.title)
		if d.audience != "" {
			doc.setEnum("audience", d.audience)
		}
		doc.setU32("token_count", d.tokens)
	}

	terms := make([]string, 0, len(b.postings))
	for t := range b.postings {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	for _, term := range terms {
		posting := out.appendMsg("postings")
		posting.setStr("term", term)

		keys := make([]occKey, 0, len(b.postings[term]))
		for k := range b.postings[term] {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].doc != keys[j].doc {
				return keys[i].doc < keys[j].doc
			}
			return keys[i].class < keys[j].class
		})
		for _, k := range keys {
			occ := b.postings[term][k]
			e := posting.appendMsg("occurrences")
			e.setU32("document", k.doc)
			e.setEnum("field", k.class)
			e.setU32("count", occ.count)
			sort.Slice(occ.positions, func(i, j int) bool { return occ.positions[i] < occ.positions[j] })
			for _, p := range occ.positions {
				e.appendU32("positions", p)
			}
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
