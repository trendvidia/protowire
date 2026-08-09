// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

// The operation half (§5.2): every method carrying @http becomes one
// OpenAPI operation. Responses are derived, never authored (§#080/GH
// #177): 200 from the return type, default from the §7 report model.

// httpUse is one parsed @http annotation.
type httpUse struct {
	method      string
	path        string
	summary     string
	operationID string
	tags        []string
	security    []string
}

func parseHTTP(ann dmsg) (*httpUse, bool) {
	if !ann.valid() {
		return nil, false
	}
	u := &httpUse{
		method:      strings.ToUpper(argStr(ann, "method", 0)),
		path:        argStr(ann, "path", 1),
		summary:     argStr(ann, "summary", 2),
		operationID: argStr(ann, "operation_id", 3),
		tags:        argStrings(ann, "tags", 4),
		security:    argStrings(ann, "security", 5),
	}
	return u, u.method != "" && u.path != ""
}

// bodyless per the §5.2 binding rules: remaining request fields bind to
// the query string for these methods, to the body otherwise.
func bodyless(method string) bool {
	switch method {
	case "GET", "HEAD", "DELETE", "OPTIONS":
		return true
	}
	return false
}

// templateParams extracts the variable of each {…} segment, in order of
// appearance. A segment may constrain its variable with a sub-path
// pattern (`{name=shelves/*}`); the variable is what precedes the `=`,
// exactly as the compiler reads it (§5.2, issue #217).
func templateParams(path string) []string {
	var out []string
	for _, seg := range templateSegments(path) {
		out = append(out, seg.variable)
	}
	return out
}

// templateSegment is one {…} of a path: the variable it binds and the
// half-open byte range of the whole segment, braces included.
type templateSegment struct {
	variable   string
	start, end int
}

func templateSegments(path string) []templateSegment {
	var out []templateSegment
	for i := 0; i < len(path); {
		open := strings.IndexByte(path[i:], '{')
		if open < 0 {
			break
		}
		open += i
		close := strings.IndexByte(path[open:], '}')
		if close < 0 {
			break
		}
		name := path[open+1 : open+close]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		out = append(out, templateSegment{
			variable: strings.TrimSpace(name),
			start:    open,
			end:      open + close + 1,
		})
		i = open + close + 1
	}
	return out
}

// openAPIPath rewrites a §5.2 path into an OpenAPI path template. The
// two agree except for sub-path patterns: OpenAPI's template grammar
// has no `{name=pattern}` form, so the constraint is dropped from the
// key and only the variable remains. Everything else — including a
// dotted variable, which OpenAPI allows as a parameter name — is
// carried through verbatim.
func openAPIPath(path string) string {
	segs := templateSegments(path)
	if len(segs) == 0 {
		return path
	}
	var b strings.Builder
	prev := 0
	for _, seg := range segs {
		b.WriteString(path[prev:seg.start])
		b.WriteString("{" + seg.variable + "}")
		prev = seg.end
	}
	b.WriteString(path[prev:])
	return b.String()
}

// operationsBuilder accumulates paths and the security-scheme demand.
type operationsBuilder struct {
	m       *model
	sb      *schemaBuilder
	include func(fqn string) bool
	schemes map[string]bool // config-defined scheme names
	// paths: path template → method(lower) → operation
	paths map[string]map[string]*omap
	// operationIDs maps every emitted operationId to a description of
	// the operation that claimed it, so a collision names both ends.
	operationIDs map[string]string
	// usedSchemes records referenced names for components emission.
	usedSchemes map[string]bool
	needsReport bool
}

func newOperationsBuilder(m *model, sb *schemaBuilder, include func(string) bool, schemes map[string]bool) *operationsBuilder {
	return &operationsBuilder{
		m:            m,
		sb:           sb,
		include:      include,
		schemes:      schemes,
		paths:        make(map[string]map[string]*omap),
		operationIDs: make(map[string]string),
		usedSchemes:  make(map[string]bool),
	}
}

func (ob *operationsBuilder) included(fqn string) bool {
	return ob.include == nil || ob.include(fqn)
}

func (ob *operationsBuilder) buildAll() error {
	for _, svc := range ob.m.services {
		if generatedFileByName(ob.m, svc.file) || !ob.included(svc.fqn) {
			continue
		}
		for _, mth := range svc.methods {
			if !ob.included(svc.fqn + "." + mth.name) {
				continue
			}
			// Every @http use site is an operation. A method carrying
			// several lowers to a rule plus additional_bindings (§5.2),
			// so rendering only the first would describe less than the
			// image binds — the mirror of issue #213 (issue #215).
			//
			// binding counts the use sites that render, not the carrier
			// slots: one that parses to nothing produces no operation, so
			// it must not push the first real one out of the position
			// that owns the derived operationId.
			binding := 0
			for _, ann := range allAnns(mth.anns, annHTTP) {
				use, ok := parseHTTP(ann)
				if !ok {
					continue
				}
				if err := ob.operation(svc, mth, use, binding); err != nil {
					return fmt.Errorf("%s.%s: %w", svc.fqn, mth.name, err)
				}
				binding++
			}
		}
	}
	return nil
}

func generatedFileByName(m *model, name string) bool {
	for _, f := range m.files {
		if f.GetName() == name {
			return generatedFile(f)
		}
	}
	return false
}

// operation renders one @http use site. binding is its index among the
// method's rendered use sites: 0 is the rule, the rest are additional
// bindings.
func (ob *operationsBuilder) operation(svc *serviceInfo, mth *methodInfo, use *httpUse, binding int) error {
	op := newOmap()

	if len(use.tags) > 0 {
		tags := make([]any, len(use.tags))
		for i, t := range use.tags {
			tags[i] = t
		}
		op.set("tags", tags)
	}

	desc := description(mth.anns)
	summary := use.summary
	if summary == "" {
		summary = firstSentence(desc)
	}
	if summary != "" {
		op.set("summary", summary)
	}
	if desc != "" && desc != summary {
		op.set("description", desc)
	}

	// operationId names a method in generated clients, so it must be
	// unique across the document and stable across edits that do not
	// change the API. The derived spelling satisfies both only while a
	// method has one binding — the §#080 "unique by construction"
	// argument. A repeated @http names its own rather than taking a
	// positional suffix, which would rename a client method whenever
	// two annotation lines are reordered (issue #215).
	opID := use.operationID
	if opID == "" {
		if binding > 0 {
			return fmt.Errorf("@http %s %s needs an explicit operation_id: "+
				"it is binding %d on this method, and the derived %q is reserved for the first",
				use.method, use.path, binding+1, svc.name+"_"+mth.name)
		}
		opID = svc.name + "_" + mth.name
	}
	owner := fmt.Sprintf("%s.%s (%s %s)", svc.fqn, mth.name, use.method, use.path)
	if prev, dup := ob.operationIDs[opID]; dup {
		return fmt.Errorf("operationId %q is already used by %s: ids must be unique across the document", opID, prev)
	}
	ob.operationIDs[opID] = owner
	op.set("operationId", opID)

	if dep := findAnn(mth.anns, annDeprecated); dep.valid() {
		op.set("deprecated", true)
		if reason := argStr(dep, "reason", 0); reason != "" {
			op.set("x-deprecated-reason", reason)
		}
	}

	req := ob.m.messages[mth.input]
	if req == nil {
		return fmt.Errorf("request message %s is not in the image", mth.input)
	}

	// Path parameters: a segment names a field of the request message
	// from its top level, as a dotted path (§5.2, issue #217). An
	// unmatched segment is a generation error — emitting a parameter
	// with no schema would be a silent guess.
	//
	// bound is keyed by the *first* component, because that is the field
	// the remaining-field binding below iterates: a nested binding takes
	// its whole top-level container out of the query string or body
	// rather than splitting it, since this renderer flattens no message
	// anywhere else either (issue #218).
	bound := map[string]bool{}
	var params []any
	for _, name := range templateParams(use.path) {
		owner, fi := resolveFieldPath(ob.m, req, name)
		if fi == nil {
			return fmt.Errorf("path template {%s} names no field of %s", name, mth.input)
		}
		bound[strings.SplitN(name, ".", 2)[0]] = true
		ps, err := ob.paramSchema(owner, fi)
		if err != nil {
			return err
		}
		params = append(params, newOmap().
			set("name", name).
			set("in", "path").
			set("required", true).
			set("schema", ps))
	}

	if bodyless(use.method) {
		for _, fi := range req.fields {
			if bound[fi.desc.GetName()] {
				continue
			}
			ps, err := ob.paramSchema(req, fi)
			if err != nil {
				return err
			}
			p := newOmap().
				set("name", fi.desc.GetName()).
				set("in", "query")
			if findAnn(fi.anns, annRequired).valid() {
				p.set("required", true)
			}
			if d := description(fi.anns); d != "" {
				p.set("description", d)
			}
			p.set("schema", ps)
			params = append(params, p)
		}
	}
	if len(params) > 0 {
		op.set("parameters", params)
	}

	if !bodyless(use.method) {
		body, err := ob.requestBody(req, bound)
		if err != nil {
			return err
		}
		op.set("requestBody", body)
	}

	if len(use.security) > 0 {
		var sec []any
		for _, name := range use.security {
			if !ob.schemes[name] {
				return fmt.Errorf("@http security scheme %q has no definition in the generator config", name)
			}
			ob.usedSchemes[name] = true
			sec = append(sec, newOmap().set(name, []any{}))
		}
		op.set("security", sec)
	}

	responses, err := ob.responses(req, mth)
	if err != nil {
		return err
	}
	op.set("responses", responses)

	// Keyed by the OpenAPI spelling, so two bindings that differ only in
	// a sub-path constraint collide here rather than rendering as two
	// path items OpenAPI cannot tell apart.
	key := openAPIPath(use.path)
	methods, ok := ob.paths[key]
	if !ok {
		methods = make(map[string]*omap)
		ob.paths[key] = methods
	}
	lower := strings.ToLower(use.method)
	if _, dup := methods[lower]; dup {
		return fmt.Errorf("duplicate operation %s %s", use.method, key)
	}
	methods[lower] = op
	return nil
}

// paramSchema renders a request field as a parameter schema. Fields the
// binding rules place in the path or query are value-shaped; a
// message-typed field here has no canonical flat encoding and errors.
func (ob *operationsBuilder) paramSchema(req *messageInfo, fi *fieldInfo) (*omap, error) {
	if fi.desc.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE &&
		!ob.sb.isMapField(fi.desc) {
		fqn := strings.TrimPrefix(fi.desc.GetTypeName(), ".")
		if _, wrapper := wrapperScalar(fqn); !wrapper {
			if _, wkt := wktSchema(fqn); !wkt {
				return nil, fmt.Errorf("field %s is message-typed and cannot bind to the path or query string", fi.desc.GetName())
			}
		}
	}
	s, err := ob.sb.propertySchema(req, fi, ob.sb.messageSensitive(req.fqn))
	if err != nil {
		return nil, err
	}
	// The parameter object carries the description; keeping a copy
	// inside the schema would just duplicate it.
	s.unset("description")
	return s, nil
}

// requestBody renders the §5.2 body binding: the whole request message
// when no template segment bound a field, otherwise an inline object of
// the remaining fields.
func (ob *operationsBuilder) requestBody(req *messageInfo, bound map[string]bool) (*omap, error) {
	var schema *omap
	if len(bound) == 0 {
		if err := ob.sb.component(req.fqn); err != nil {
			return nil, err
		}
		schema = ref(req.fqn)
	} else {
		schema = newOmap().set("type", "object")
		props := newOmap()
		var required []any
		msgSensitive := ob.sb.messageSensitive(req.fqn)
		for _, fi := range req.fields {
			name := fi.desc.GetName()
			if bound[name] {
				continue
			}
			ps, err := ob.sb.propertySchema(req, fi, msgSensitive)
			if err != nil {
				return nil, err
			}
			props.set(name, ps)
			if findAnn(fi.anns, annRequired).valid() {
				required = append(required, name)
			}
		}
		if props.len() > 0 {
			schema.set("properties", props)
		}
		if len(required) > 0 {
			schema.set("required", required)
		}
	}
	content := newOmap().set("application/json", newOmap().set("schema", schema))
	return newOmap().set("required", true).set("content", content), nil
}

// responses derives the response map: 200 from the return type, default
// from @error_code + the §7 report model (§#080, GH #177).
func (ob *operationsBuilder) responses(req *messageInfo, mth *methodInfo) (*omap, error) {
	out := newOmap()

	ok := newOmap()
	if mth.output == "google.protobuf.Empty" {
		ok.set("description", "OK")
	} else {
		outMsg := ob.m.messages[mth.output]
		if outMsg == nil {
			return nil, fmt.Errorf("response message %s is not in the image", mth.output)
		}
		d := description(outMsg.anns)
		if d == "" {
			d = "OK"
		}
		ok.set("description", d)
		if err := ob.sb.component(mth.output); err != nil {
			return nil, err
		}
		ok.set("content", newOmap().set("application/json", newOmap().set("schema", ref(mth.output))))
	}
	out.set("200", ok)

	def := newOmap().set("description", "Validation failure report (RFC-001 §7)")
	def.set("content", newOmap().set("application/json",
		newOmap().set("schema", ref(reportFQN))))
	if codes := ob.errorCodes(req); len(codes) > 0 {
		vals := make([]any, len(codes))
		for i, c := range codes {
			vals[i] = c
		}
		def.set("x-error-codes", vals)
	}
	out.set("default", def)
	ob.needsReport = true
	return out, nil
}

// errorCodes collects the stable violation codes reachable from the
// request message's rules: explicit `code` overrides on @validate plus
// the [error_code] option of every declared function the rules call.
func (ob *operationsBuilder) errorCodes(req *messageInfo) []string {
	seen := map[string]bool{}
	visited := map[string]bool{}
	ob.collectCodes(req.fqn, seen, visited)
	codes := make([]string, 0, len(seen))
	for c := range seen {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

func (ob *operationsBuilder) collectCodes(fqn string, seen, visited map[string]bool) {
	if visited[fqn] {
		return
	}
	visited[fqn] = true
	if a := ob.m.aliases[fqn]; a != nil {
		ob.codesFromEntries(a.anns, seen)
		ob.collectCodes(a.base, seen, visited)
		return
	}
	mi := ob.m.messages[fqn]
	if mi == nil {
		return
	}
	ob.codesFromEntries(mi.anns, seen)
	for _, fi := range mi.fields {
		ob.codesFromEntries(fi.anns, seen)
		if findAnn(fi.anns, annRequired).valid() {
			seen["protowire.required"] = true
		}
		for _, alias := range ob.m.chainOf(mi.fqn, fi.desc.GetName()) {
			ob.collectCodes(alias, seen, visited)
		}
		if fi.desc.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			ob.collectCodes(strings.TrimPrefix(fi.desc.GetTypeName(), "."), seen, visited)
		}
	}
}

func (ob *operationsBuilder) codesFromEntries(entries []dmsg, seen map[string]bool) {
	for _, v := range allAnns(entries, annValidate) {
		if code := argStr(v, "code", 1); code != "" {
			seen[code] = true
		}
		rule := arg(v, "rule", 0)
		if rule.valid() && rule.which("value") == "expression" {
			for _, call := range rule.sub("expression").msgs("calls") {
				if fn := ob.m.functions[call.str("fqn")]; fn != nil && fn.errorCode != "" {
					seen[fn.errorCode] = true
				}
			}
		}
	}
}

// emitPaths renders the accumulated paths in canonical order.
func (ob *operationsBuilder) emitPaths() *omap {
	paths := newOmap()
	keys := make([]string, 0, len(ob.paths))
	for k := range ob.paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	methodOrder := []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}
	for _, k := range keys {
		item := newOmap()
		for _, mm := range methodOrder {
			if op, ok := ob.paths[k][mm]; ok {
				item.set(mm, op)
			}
		}
		paths.set(k, item)
	}
	return paths
}

// resolveFieldPath walks a dotted field path from a request message and
// returns the message that declares the leaf together with the leaf
// itself, or (nil, nil) when any component names nothing or the path
// descends through a non-message. The compiler resolves the same shape
// (§5.2, issue #217); disagreeing here is what made a schema compile,
// bind, and then fail to document.
func resolveFieldPath(m *model, mi *messageInfo, path string) (*messageInfo, *fieldInfo) {
	components := strings.Split(path, ".")
	owner := mi
	for i, component := range components {
		if owner == nil {
			return nil, nil
		}
		fi := fieldByName(owner, component)
		if fi == nil {
			return nil, nil
		}
		if i == len(components)-1 {
			return owner, fi
		}
		if fi.desc.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			return nil, nil // Descending through a scalar names nothing.
		}
		owner = m.messages[strings.TrimPrefix(fi.desc.GetTypeName(), ".")]
	}
	return nil, nil
}

func fieldByName(mi *messageInfo, name string) *fieldInfo {
	for _, fi := range mi.fields {
		if fi.desc.GetName() == name {
			return fi
		}
	}
	return nil
}
