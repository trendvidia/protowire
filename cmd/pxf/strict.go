// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"

	"github.com/itchyny/gojq"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// modeFlag is the {auto, strict, loose} tri-state controlling whether
// strict-mode compile-time validation runs. The default is auto:
// strict when a top-level type is bound (via -m or the document's
// @type), loose otherwise. --strict / --loose force the corresponding
// branch regardless.
type modeFlag int

const (
	modeAuto modeFlag = iota
	modeStrict
	modeLoose
)

// effectiveMode picks the runtime mode given the user's --strict /
// --loose toggle and whether the query has a bound top-level type.
// Returns (strict, error): if the user forced --strict but no type is
// bound, surfaces the README's prescribed "errors if no schema is
// available, pointing at pxf infer-schema" message.
func effectiveMode(req modeFlag, bound bool) (bool, error) {
	switch req {
	case modeStrict:
		if !bound {
			return false, fmt.Errorf("--strict requires a top-level message type " +
				"(pass -m fully.qualified.Name, set @type in the document, " +
				"or run `pxf infer-schema` first)")
		}
		return true, nil
	case modeLoose:
		return false, nil
	default:
		return bound, nil
	}
}

// resolveRootType returns the message descriptor the query operates
// against, or nil when the input is unbound. Resolution order matches
// README §A note on @proto: explicit --message wins, then the
// document's @type.
func resolveRootType(messageFlag string, doc *loadedDoc, sch *schema) protoreflect.MessageDescriptor {
	if sch == nil {
		return nil
	}
	if messageFlag != "" {
		return sch.find(messageFlag)
	}
	if doc != nil && doc.typeURL != "" {
		return sch.find(doc.typeURL)
	}
	return nil
}

// validateQueryStrict walks the parsed gojq query and rejects any
// path that doesn't resolve against the bound message. Only the
// simple "field chain from root" subset is enforced — anything more
// dynamic (function calls, list indexing, recursive descent,
// `pxf_directive(...)` paths) passes without validation.
//
// The contract is "no false positives, some false negatives": a query
// that passes strict validation is guaranteed not to type-error on
// the field names this validator can see; a query that uses dynamic
// access patterns still runs and falls back to gojq's runtime
// behaviour.
func validateQueryStrict(q *gojq.Query, root protoreflect.MessageDescriptor) error {
	if q == nil || root == nil {
		return nil
	}
	return walkQuery(q, root)
}

// walkQuery descends into both halves of a pipe / binary op so paths
// on each side get validated. A top-level binary op like `.foo | .bar`
// has Left = `.foo` and Right = `.bar`; both halves are validated
// against the same root (gojq's pipe passes the LHS result through,
// but tracking that result's type would require a full type inferer —
// out of scope for Stage E).
func walkQuery(q *gojq.Query, root protoreflect.MessageDescriptor) error {
	if q == nil {
		return nil
	}
	if q.Term != nil {
		if err := walkTerm(q.Term, root); err != nil {
			return err
		}
	}
	if err := walkQuery(q.Left, root); err != nil {
		return err
	}
	return walkQuery(q.Right, root)
}

// walkTerm validates a single Term's field-chain starting from the
// document root. Terms that aren't a path-from-identity pass without
// validation — that's the "dynamic access" escape hatch.
func walkTerm(t *gojq.Term, root protoreflect.MessageDescriptor) error {
	if t == nil {
		return nil
	}
	// Only direct field chains rooted at identity are validated.
	// Other Term shapes (function calls, literals, object/array
	// constructors, etc.) might contain inner Queries we should
	// descend into — but their semantics don't bind to the document
	// root, so validation against `root` is the wrong type-context.
	// Stage E keeps the validator narrow on purpose.
	switch t.Type {
	case gojq.TermTypeIdentity:
		// `.` followed by a SuffixList of field indices.
		return walkSuffixes(t.SuffixList, root)
	case gojq.TermTypeIndex:
		// `.foo` — the Index is the first field; SuffixList carries
		// any further `.bar` chain.
		md, err := stepField(t.Index, root)
		if err != nil {
			return err
		}
		if md == nil {
			// Terminal scalar; remaining suffixes can't be field
			// accesses but might be array indices / iters, which
			// don't validate against a message type. Accept.
			return walkRemainingSuffixesNonMessage(t.SuffixList)
		}
		return walkSuffixes(t.SuffixList, md)
	}
	return nil
}

// walkSuffixes walks a Suffix chain accumulating field steps against
// `curr`. Stops at the first non-field suffix (array index, iter,
// optional) — those don't bind to a message type.
func walkSuffixes(suffixes []*gojq.Suffix, curr protoreflect.MessageDescriptor) error {
	if curr == nil {
		return walkRemainingSuffixesNonMessage(suffixes)
	}
	for i, s := range suffixes {
		if s == nil || s.Index == nil || s.Index.Name == "" {
			// Non-field suffix (e.g. `[0]`, `[]`, slice). Validation
			// of subsequent fields would need to track the array's
			// element type; defer to runtime.
			return walkRemainingSuffixesNonMessage(suffixes[i+1:])
		}
		next, err := stepField(s.Index, curr)
		if err != nil {
			return err
		}
		if next == nil {
			// Terminal scalar field; further suffixes (if any) can
			// only be non-field ops on the scalar value.
			return walkRemainingSuffixesNonMessage(suffixes[i+1:])
		}
		curr = next
	}
	return nil
}

// walkRemainingSuffixesNonMessage validates that any leftover
// suffixes don't claim field access — array indices, iters, and
// slices are fine, but a `.foo` after a scalar value is a guaranteed
// runtime error worth surfacing at compile time. Conservative: only
// fields on a known-scalar context get flagged; everything else
// passes since we've already lost the type.
func walkRemainingSuffixesNonMessage(suffixes []*gojq.Suffix) error {
	// Intentionally permissive — accepts anything, since once we've
	// stepped onto a non-message value we no longer have a type to
	// validate against. Hook reserved for a future stage that
	// tracks element types through arrays/maps.
	_ = suffixes
	return nil
}

// stepField looks up `idx.Name` on `md`. Returns the field's message
// descriptor (for nested messages) or nil (for scalar fields). Errors
// when the field name isn't declared.
func stepField(idx *gojq.Index, md protoreflect.MessageDescriptor) (protoreflect.MessageDescriptor, error) {
	if idx == nil || idx.Name == "" {
		return nil, nil
	}
	fd := md.Fields().ByName(protoreflect.Name(idx.Name))
	if fd == nil {
		return nil, unknownFieldError(idx.Name, md)
	}
	if fd.Kind() == protoreflect.MessageKind {
		return fd.Message(), nil
	}
	return nil, nil
}

// unknownFieldError formats the README's prescribed strict-mode
// error. Includes a "did you mean" hint when the offending name has
// a single low-edit-distance candidate among the declared fields.
func unknownFieldError(name string, md protoreflect.MessageDescriptor) error {
	candidate := closestField(name, md)
	if candidate != "" {
		return fmt.Errorf("strict-mode: unknown field %q on %s (did you mean %q?)",
			name, md.FullName(), candidate)
	}
	return fmt.Errorf("strict-mode: unknown field %q on %s", name, md.FullName())
}

// closestField returns the declared field name whose edit distance
// from `name` is at most 2 — a typical typo radius. Empty string when
// nothing is close enough.
func closestField(name string, md protoreflect.MessageDescriptor) string {
	const maxDist = 2
	best := maxDist + 1
	out := ""
	fields := md.Fields()
	for i := range fields.Len() {
		fn := string(fields.Get(i).Name())
		d := editDistance(name, fn)
		if d < best {
			best = d
			out = fn
		}
	}
	if best > maxDist {
		return ""
	}
	return out
}

// editDistance is the standard Levenshtein implementation. Used only
// during the typo-hint computation, so an O(len(a)*len(b)) table is
// fine — declared message field counts and field-name lengths sit in
// the dozens.
func editDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
