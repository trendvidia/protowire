// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/trendvidia/protowire/internal/docpack"
)

// Audience tiers (§#080 Gap 2): assigned by generator configuration
// (FQN globs → tier, defaulting to public), restricted further by any
// doc-pack topic anchored to the element, filtered — never stripped —
// at emission, with transitive inconsistency a hard error naming both
// ends.

type audienceIndex struct {
	rules []audienceRule
	// packTiers carries the anchored-topic contribution: FQN → rank.
	packTiers map[string]int
	packKeys  map[string]string // FQN → topic key, for error messages
}

func newAudienceIndex(rules []audienceRule) *audienceIndex {
	return &audienceIndex{
		rules:     rules,
		packTiers: make(map[string]int),
		packKeys:  make(map[string]string),
	}
}

const publicRank = 1

// rankOf resolves an element's effective tier: the first matching
// config glob (authored order) or public, restricted further by the
// most restrictive topic anchored to it.
func (ai *audienceIndex) rankOf(fqn string) int {
	rank := publicRank
	for _, r := range ai.rules {
		if globMatch(r.glob, fqn) {
			rank = r.rank
			break
		}
	}
	if pr, ok := ai.packTiers[fqn]; ok && pr > rank {
		rank = pr
	}
	return rank
}

// globMatch applies path.Match over the dotted FQN; `*` crosses dots
// (the FQN is one path segment), and a trailing ".*" glob also matches
// nested scopes.
func globMatch(glob, fqn string) bool {
	if ok, err := path.Match(glob, fqn); err == nil && ok {
		return true
	}
	if prefix, found := strings.CutSuffix(glob, ".*"); found {
		return fqn == prefix || strings.HasPrefix(fqn, prefix+".")
	}
	return false
}

// contribute records a doc-pack topic's tier against the schema
// element it anchors.
func (ai *audienceIndex) contribute(fqn, topicKey, tierName string) {
	rank, ok := docpack.AudienceRank(tierName)
	if !ok {
		return
	}
	if cur, exists := ai.packTiers[fqn]; !exists || rank > cur {
		ai.packTiers[fqn] = rank
		ai.packKeys[fqn] = topicKey
	}
}

// tierName renders a rank for diagnostics.
func tierName(rank int) string {
	for _, n := range []string{"AUDIENCE_PUBLIC", "AUDIENCE_COMMUNITY", "AUDIENCE_PARTNER", "AUDIENCE_ENTERPRISE", "AUDIENCE_INTERNAL"} {
		if r, _ := docpack.AudienceRank(n); r == rank {
			return n
		}
	}
	return fmt.Sprintf("rank %d", rank)
}

// checkTransitive walks every element's direct references and errors
// when a reference is more restricted than its referrer: filtering such
// an element would leave a dangling $ref at some tier, and silently
// inlining a restricted definition is worse than refusing (§#080).
func (ai *audienceIndex) checkTransitive(m *model) error {
	type edge struct{ from, to string }
	var edges []edge
	add := func(from, to string) {
		to = strings.TrimPrefix(to, ".")
		if strings.HasPrefix(to, "google.protobuf.") {
			return
		}
		edges = append(edges, edge{from, to})
	}

	gen := make(map[string]bool)
	for _, f := range m.files {
		gen[f.GetName()] = generatedFile(f)
	}
	for fqn, mi := range m.messages {
		if gen[mi.file] || mi.isEntry {
			continue
		}
		for _, fi := range mi.fields {
			if tn := fi.desc.GetTypeName(); tn != "" {
				if entry := m.messages[strings.TrimPrefix(tn, ".")]; entry != nil && entry.isEntry {
					if vt := entry.mapKV[1].GetTypeName(); vt != "" {
						add(fqn, vt)
					}
					continue
				}
				add(fqn, tn)
			}
			for _, alias := range m.chainOf(fqn, fi.desc.GetName()) {
				add(fqn, alias)
			}
		}
	}
	for fqn, a := range m.aliases {
		if gen[a.file] {
			continue
		}
		if !isPrimitive(a.base) {
			add(fqn, a.base)
		}
	}
	for _, svc := range m.services {
		if gen[svc.file] {
			continue
		}
		for _, mth := range svc.methods {
			// A service may contain more-restricted methods — filtering
			// omits the operation, nothing dangles. The schema closure
			// is what must stay consistent: an emitted operation's
			// request and response types must be visible wherever the
			// operation is.
			from := svc.fqn + "." + mth.name
			add(from, mth.input)
			if mth.output != "google.protobuf.Empty" {
				add(from, mth.output)
			}
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		return edges[i].to < edges[j].to
	})
	for _, e := range edges {
		fromRank, toRank := ai.rankOf(e.from), ai.rankOf(e.to)
		if toRank > fromRank {
			hint := ""
			if key, ok := ai.packKeys[e.to]; ok && ai.packTiers[e.to] == toRank {
				hint = fmt.Sprintf(" (tier contributed by doc topic %q)", key)
			}
			return fmt.Errorf("audience inconsistency: %s element %s references %s element %s%s; a schema closure may not reach a more restricted element",
				tierName(fromRank), e.from, tierName(toRank), e.to, hint)
		}
	}
	return nil
}

func isPrimitive(name string) bool {
	switch name {
	case "string", "bytes", "bool", "int32", "int64", "uint32", "uint64",
		"sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64",
		"float", "double":
		return true
	}
	return false
}
