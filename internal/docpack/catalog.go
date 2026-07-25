// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// The widget catalog is the compiler's second data input: appviewer's
// exported registry (trendvidia/appviewer#33). protowire consumes
// appviewer *data*, never appviewer code — nothing here imports the
// runtime, and the catalog arrives as a file.
//
// Three input encodings are accepted, and they are not equals:
//
//	.binpb  protowire.docs.v1.WidgetCatalog, the typed form
//	.pxf    the same message, authored or hand-checked
//	.json   appviewer's `--dump-registry` output — an integration
//	        boundary, converted to the typed form on read
//
// The JSON path is the boundary adapter, and it is the only place in the
// pipeline where JSON exists. When appviewer emits the typed message
// natively, this adapter drops out and no other code changes (the format
// rule of 2026-07-25; precedent #66/Appendix C, adapter-first inbound).

// Catalog is the resolved view of one runtime's widget surface.
type Catalog struct {
	Path   string
	Digest string

	SchemaVersion uint32
	WidgetCount   int

	widgets map[string]*widgetEntry
	common  map[string]string // common prop name → since ("" when unversioned)
}

type widgetEntry struct {
	since  string
	props  map[string]string // prop name → since, falling back to the widget's
	events map[string]string
}

// catalogFormatVersion is the appviewer catalog schema version this
// compiler was written against. A newer catalog is accepted with a
// warning rather than refused: entries this compiler understands still
// resolve, and refusing would couple every documentation build to the
// runtime's release cadence.
const catalogFormatVersion = 3

// loadCatalog reads a widget catalog in any accepted encoding.
func loadCatalog(path string) (*Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	md, err := message(WidgetCatalogMessage)
	if err != nil {
		return nil, err
	}

	var msg dmsg
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".json":
		msg, err = catalogFromJSON(raw, md)
	case ".pxf":
		var m proto.Message
		m, err = pxf.UnmarshalDescriptor(raw, md)
		if err == nil {
			msg = wrap(m.ProtoReflect())
		}
	case ".binpb", ".pb", ".bin":
		m := newMsg(md)
		if err = proto.Unmarshal(raw, m.proto()); err == nil {
			msg = m
		}
	default:
		return nil, fmt.Errorf("%s: unknown registry export format %q (want .binpb, .pxf or .json)", path, ext)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: reading registry export: %w", path, err)
	}

	cat := &Catalog{
		Path:          path,
		Digest:        digestOf(raw),
		SchemaVersion: msg.u32("schema_version"),
		widgets:       map[string]*widgetEntry{},
		common:        map[string]string{},
	}
	for _, p := range msg.msgs("common_props") {
		cat.common[p.str("name")] = p.str("since")
	}
	for _, w := range msg.msgs("widgets") {
		entry := &widgetEntry{
			since:  w.str("since"),
			props:  map[string]string{},
			events: map[string]string{},
		}
		// A widget's own props and the props its structural builder reads
		// on children are both documented as attributes of that widget,
		// so both are anchorable (appviewer#51).
		for _, group := range [][]dmsg{w.msgs("props"), w.msgs("child_props")} {
			for _, p := range group {
				entry.props[p.str("name")] = p.str("since")
			}
		}
		for _, e := range w.msgs("events") {
			entry.events[e.str("name")] = e.str("since")
		}
		cat.widgets[w.str("type")] = entry
	}
	cat.WidgetCount = len(cat.widgets)
	return cat, nil
}

// resolveWidget checks a widget anchor's canonical id against the
// catalog and returns the target's since-version. The error names what
// exists, because "no such prop" is only useful next to the props that
// do exist.
func (c *Catalog) resolveWidget(id string) (since string, err error) {
	typ, memberKind, member := widgetMember(id)
	entry, ok := c.widgets[typ]
	if !ok {
		return "", fmt.Errorf("widget %q is not in the registry export (%s)", typ, c.Path)
	}
	switch memberKind {
	case "":
		return entry.since, nil
	case "prop":
		if s, ok := entry.props[member]; ok {
			return firstNonEmpty(s, entry.since), nil
		}
		if s, ok := c.common[member]; ok {
			return firstNonEmpty(s, entry.since), nil
		}
		return "", fmt.Errorf("widget %s has no property %q (has: %s)", typ, member, joinKeys(entry.props, c.common))
	case "event":
		if s, ok := entry.events[member]; ok {
			return firstNonEmpty(s, entry.since), nil
		}
		return "", fmt.Errorf("widget %s has no event %q (has: %s)", typ, member, joinKeys(entry.events))
	}
	return "", fmt.Errorf("widget anchor %q has an unknown member kind %q", id, memberKind)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func joinKeys(maps ...map[string]string) string {
	var keys []string
	for _, m := range maps {
		for k := range m {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return "none"
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// ── JSON boundary adapter ─────────────────────────────────────────────────

// jsonCatalog mirrors appviewer's exported Catalog. Only the facts anchor
// resolution needs are modeled; fields appviewer adds for its own
// purposes are ignored rather than rejected, so a newer runtime's export
// still builds documentation.
type jsonCatalog struct {
	SchemaVersion  uint32           `json:"schema_version"`
	RuntimeVersion string           `json:"runtime_version"`
	Widgets        []jsonWidget     `json:"widgets"`
	CommonProps    []jsonProp       `json:"common_props"`
	ActionFuncs    []jsonActionFunc `json:"action_funcs"`
}

type jsonWidget struct {
	Type       string      `json:"type"`
	Since      string      `json:"since"`
	Doc        string      `json:"doc"`
	Container  bool        `json:"container"`
	Structural bool        `json:"structural"`
	Bindable   bool        `json:"bindable"`
	Props      []jsonProp  `json:"props"`
	Events     []jsonEvent `json:"events"`
	ChildProps []jsonProp  `json:"child_props"`
}

type jsonProp struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Enum  []string `json:"enum"`
	Doc   string   `json:"doc"`
	Since string   `json:"since"`
}

type jsonEvent struct {
	Name  string         `json:"name"`
	Doc   string         `json:"doc"`
	Since string         `json:"since"`
	Args  []jsonEventArg `json:"args"`
}

type jsonEventArg struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Doc  string `json:"doc"`
}

type jsonActionFunc struct {
	Name  string `json:"name"`
	Since string `json:"since"`
}

// propKinds maps appviewer's open-vocabulary kind strings onto the typed
// enum. An unrecognised kind becomes PROP_KIND_UNSPECIFIED: a catalog
// from a newer runtime must still resolve the anchors it shares with
// this one.
var propKinds = map[string]string{
	"string": "PROP_KIND_STRING",
	"int":    "PROP_KIND_INT",
	"float":  "PROP_KIND_FLOAT",
	"bool":   "PROP_KIND_BOOL",
	"color":  "PROP_KIND_COLOR",
	"enum":   "PROP_KIND_ENUM",
	"list":   "PROP_KIND_LIST",
	"state":  "PROP_KIND_STATE",
	"any":    "PROP_KIND_ANY",
}

// catalogFromJSON converts appviewer's export into the typed model.
//
// Two shapes are accepted: the full `--dump-registry` object, and a bare
// widgets array (appviewer's own `registry.Export()` golden, which is
// convenient to point at directly). Everything past this function sees
// only WidgetCatalog.
func catalogFromJSON(raw []byte, md protoreflect.MessageDescriptor) (dmsg, error) {
	var jc jsonCatalog
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	switch {
	case bytes.HasPrefix(trimmed, []byte("[")):
		if err := json.Unmarshal(raw, &jc.Widgets); err != nil {
			return dmsg{}, err
		}
		jc.SchemaVersion = catalogFormatVersion
	default:
		if err := json.Unmarshal(raw, &jc); err != nil {
			return dmsg{}, err
		}
	}

	cat := newMsg(md)
	cat.setU32("schema_version", jc.SchemaVersion)
	if jc.RuntimeVersion != "" {
		cat.setStr("runtime_version", jc.RuntimeVersion)
	}
	// Sorted on the way in: the pack is byte-stable, and a producer that
	// reorders its export must not change our output.
	sort.Slice(jc.Widgets, func(i, j int) bool { return jc.Widgets[i].Type < jc.Widgets[j].Type })
	for _, w := range jc.Widgets {
		out := cat.appendMsg("widgets")
		out.setStr("type", w.Type)
		out.setStr("since", w.Since)
		out.setStr("doc", w.Doc)
		out.setBool("container", w.Container)
		out.setBool("structural", w.Structural)
		out.setBool("bindable", w.Bindable)
		addProps(out, "props", w.Props)
		addProps(out, "child_props", w.ChildProps)
		for _, e := range sortedEvents(w.Events) {
			ev := out.appendMsg("events")
			ev.setStr("name", e.Name)
			ev.setStr("doc", e.Doc)
			ev.setStr("since", e.Since)
			for _, a := range e.Args {
				arg := ev.appendMsg("args")
				arg.setStr("name", a.Name)
				arg.setEnum("kind", propKind(a.Kind))
				arg.setStr("doc", a.Doc)
			}
		}
	}
	addProps(cat, "common_props", jc.CommonProps)
	sort.Slice(jc.ActionFuncs, func(i, j int) bool { return jc.ActionFuncs[i].Name < jc.ActionFuncs[j].Name })
	for _, f := range jc.ActionFuncs {
		fn := cat.appendMsg("action_funcs")
		fn.setStr("name", f.Name)
		fn.setStr("since", f.Since)
	}
	return cat, nil
}

func addProps(parent dmsg, field string, props []jsonProp) {
	sorted := append([]jsonProp(nil), props...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, p := range sorted {
		out := parent.appendMsg(field)
		out.setStr("name", p.Name)
		out.setEnum("kind", propKind(p.Kind))
		out.setStr("doc", p.Doc)
		out.setStr("since", p.Since)
		for _, v := range p.Enum {
			out.appendStr("enum_values", v)
		}
	}
}

func sortedEvents(events []jsonEvent) []jsonEvent {
	out := append([]jsonEvent(nil), events...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func propKind(name string) string {
	if k, ok := propKinds[name]; ok {
		return k
	}
	return "PROP_KIND_UNSPECIFIED"
}
