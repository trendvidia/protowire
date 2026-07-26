// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"strconv"
	"strings"
)

// The §#080 rule mapper: common @validate shapes become native OpenAPI
// keywords; everything else is carried through verbatim under
// x-validation so no constraint silently disappears at the boundary.
//
// Mapped shapes (subject `this`, conjuncts split at top-level `&&`):
//
//	matches(this, "RE")            → pattern
//	this.size() OP N / size(this)  → minLength/maxLength (strings, bytes)
//	                                 minItems/maxItems  (repeated)
//	                                 minProperties/maxProperties (maps)
//	this in [ ... ]                → enum
//	this OP N (numeric)            → minimum/maximum/exclusive*
//
// The Expression carrier is source text (§8.1): the capture is verbatim
// and string literals are opaque, so all scanning here tracks quotes
// and bracket depth rather than trusting separators.

// sizeKind selects which OpenAPI length family a size() bound maps to.
type sizeKind int

const (
	sizeLength sizeKind = iota // strings, bytes
	sizeItems                  // repeated fields
	sizeProps                  // map fields
	sizeNone                   // size() is not mappable for this subject
)

// constraints accumulates mapped keywords for one schema target.
type constraints struct {
	pattern  string
	hasPat   bool
	min, max *int64 // size bounds
	sk       sizeKind
	nMin     *float64 // numeric bounds
	nMax     *float64
	exclMin  bool
	exclMax  bool
	enum     []any
	// unmapped carries the x-validation entries: rule source plus the
	// optional code/message overrides from the @validate use site.
	unmapped []*omap
}

// mapValidate folds one @validate entry into c. numeric selects the
// numeric-bounds family (integer/float subjects); sk selects the size
// family.
func (c *constraints) mapValidate(ann dmsg, sk sizeKind, numeric bool) {
	rule := arg(ann, "rule", 0)
	if !rule.valid() || rule.which("value") != "expression" {
		return
	}
	src := strings.TrimSpace(rule.sub("expression").str("source"))
	code := argStr(ann, "code", 1)
	message := argStr(ann, "message", 2)

	var leftover []string
	for _, conj := range splitTopLevel(src, "&&") {
		conj = strings.TrimSpace(conj)
		if conj == "" || !c.mapConjunct(conj, sk, numeric) {
			leftover = append(leftover, conj)
		}
	}
	for _, rule := range leftover {
		e := newOmap().set("rule", rule)
		if code != "" {
			e.set("code", code)
		}
		if message != "" {
			e.set("message", message)
		}
		c.unmapped = append(c.unmapped, e)
	}
}

// mapConjunct attempts one conjunct; false means unmappable.
func (c *constraints) mapConjunct(s string, sk sizeKind, numeric bool) bool {
	// matches(this, "RE") → pattern.
	if args, ok := callArgs(s, "matches"); ok && len(args) == 2 && args[0] == "this" {
		if re, ok := unquote(args[1]); ok && !c.hasPat {
			c.pattern, c.hasPat = re, true
			return true
		}
		return false
	}
	// this in [ ... ] → enum.
	if rest, ok := strings.CutPrefix(s, "this in "); ok {
		return c.mapEnum(strings.TrimSpace(rest))
	}
	// Comparison forms: subject OP literal.
	subj, op, rhs, ok := splitComparison(s)
	if !ok {
		return false
	}
	switch subj {
	case "this.size()", "size(this)":
		if sk == sizeNone {
			return false
		}
		n, err := strconv.ParseInt(rhs, 10, 64)
		if err != nil {
			return false
		}
		return c.mapSize(op, n, sk)
	case "this":
		if !numeric {
			return false
		}
		n, err := strconv.ParseFloat(rhs, 64)
		if err != nil {
			return false
		}
		return c.mapNumeric(op, n)
	}
	return false
}

func (c *constraints) mapEnum(list string) bool {
	if len(list) < 2 || list[0] != '[' || list[len(list)-1] != ']' {
		return false
	}
	var vals []any
	for _, el := range splitTopLevel(list[1:len(list)-1], ",") {
		el = strings.TrimSpace(el)
		if el == "" {
			return false
		}
		if s, ok := unquote(el); ok {
			vals = append(vals, s)
			continue
		}
		if n, err := strconv.ParseInt(el, 10, 64); err == nil {
			vals = append(vals, n)
			continue
		}
		return false
	}
	if len(vals) == 0 || c.enum != nil {
		return false
	}
	c.enum = vals
	return true
}

func (c *constraints) mapSize(op string, n int64, sk sizeKind) bool {
	c.sk = sk
	set := func(p **int64, v int64) bool {
		if *p != nil {
			return false
		}
		*p = &v
		return true
	}
	switch op {
	case "<=":
		return set(&c.max, n)
	case "<":
		return set(&c.max, n-1)
	case ">=":
		return set(&c.min, n)
	case ">":
		return set(&c.min, n+1)
	case "==":
		return set(&c.min, n) && set(&c.max, n)
	}
	return false
}

func (c *constraints) mapNumeric(op string, n float64) bool {
	set := func(p **float64, v float64, excl *bool, e bool) bool {
		if *p != nil {
			return false
		}
		*p = &v
		*excl = e
		return true
	}
	switch op {
	case "<=":
		return set(&c.nMax, n, &c.exclMax, false)
	case "<":
		return set(&c.nMax, n, &c.exclMax, true)
	case ">=":
		return set(&c.nMin, n, &c.exclMin, false)
	case ">":
		return set(&c.nMin, n, &c.exclMin, true)
	}
	return false
}

// apply writes the accumulated keywords into a schema omap, in a fixed
// key order for byte stability. x-validation is appended last.
func (c *constraints) apply(schema *omap) {
	if c.hasPat {
		schema.set("pattern", c.pattern)
	}
	if c.enum != nil {
		schema.set("enum", c.enum)
	}
	minKey, maxKey := "minLength", "maxLength"
	switch c.sk {
	case sizeItems:
		minKey, maxKey = "minItems", "maxItems"
	case sizeProps:
		minKey, maxKey = "minProperties", "maxProperties"
	}
	if c.min != nil {
		schema.set(minKey, *c.min)
	}
	if c.max != nil {
		schema.set(maxKey, *c.max)
	}
	if c.nMin != nil {
		if c.exclMin {
			schema.set("exclusiveMinimum", *c.nMin)
		} else {
			schema.set("minimum", *c.nMin)
		}
	}
	if c.nMax != nil {
		if c.exclMax {
			schema.set("exclusiveMaximum", *c.nMax)
		} else {
			schema.set("maximum", *c.nMax)
		}
	}
	if len(c.unmapped) > 0 {
		entries := make([]any, len(c.unmapped))
		for i, e := range c.unmapped {
			entries[i] = e
		}
		schema.set("x-validation", entries)
	}
}

// empty reports whether nothing was accumulated at all.
func (c *constraints) empty() bool {
	return !c.hasPat && c.enum == nil && c.min == nil && c.max == nil &&
		c.nMin == nil && c.nMax == nil && len(c.unmapped) == 0
}

// ── Source-text scanning ──────────────────────────────────────────────────

// splitTopLevel splits s at every occurrence of sep that sits at zero
// bracket depth outside string literals.
func splitTopLevel(s, sep string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); {
		ch := s[i]
		switch ch {
		case '"', '\'':
			i = skipString(s, i)
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if depth == 0 && strings.HasPrefix(s[i:], sep) {
			out = append(out, s[start:i])
			i += len(sep)
			start = i
			continue
		}
		i++
	}
	return append(out, s[start:])
}

// skipString advances past the string literal opening at s[i].
func skipString(s string, i int) int {
	quote := s[i]
	for i++; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case quote:
			return i + 1
		}
	}
	return i
}

// callArgs recognizes `name( ... )` covering the whole conjunct and
// returns its top-level comma-split arguments, trimmed.
func callArgs(s, name string) ([]string, bool) {
	rest, ok := strings.CutPrefix(s, name+"(")
	if !ok || !strings.HasSuffix(rest, ")") {
		return nil, false
	}
	inner := rest[:len(rest)-1]
	// The prefix/suffix test can be fooled by `name(x) > f(y)`; verify
	// the parentheses actually balance across the interior.
	if !balanced(inner) {
		return nil, false
	}
	args := splitTopLevel(inner, ",")
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	return args, true
}

func balanced(s string) bool {
	depth := 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '"', '\'':
			i = skipString(s, i)
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return false
			}
		}
		i++
	}
	return depth == 0
}

// splitComparison splits `lhs OP rhs` at a top-level comparison
// operator. Longer operators are matched first so `<=` never reads as
// `<`.
func splitComparison(s string) (lhs, op, rhs string, ok bool) {
	depth := 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '"', '\'':
			i = skipString(s, i)
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
		if depth == 0 {
			for _, cand := range []string{"<=", ">=", "==", "<", ">"} {
				if strings.HasPrefix(s[i:], cand) {
					return strings.TrimSpace(s[:i]), cand, strings.TrimSpace(s[i+len(cand):]), true
				}
			}
		}
		i++
	}
	return "", "", "", false
}

// unquote strips one layer of matching double quotes.
func unquote(s string) (string, bool) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		body := s[1 : len(s)-1]
		var b strings.Builder
		for i := 0; i < len(body); i++ {
			if body[i] == '\\' && i+1 < len(body) {
				i++
			}
			b.WriteByte(body[i])
		}
		return b.String(), true
	}
	return "", false
}
