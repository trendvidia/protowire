// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// The emission layer: an insertion-ordered map plus YAML and JSON
// serializers over it. This is the only place in the pipeline where
// JSON/YAML exist — the binding format principle (RFC-001 §#080) keeps
// boundary formats at the boundary, and the document is built as plain
// Go values (omap / []any / scalars), not as an OpenAPI object model.
//
// Determinism: omap preserves insertion order and every builder either
// inserts in a canonically sorted order or in authored order, so two
// runs over one image emit identical bytes in both formats.

// omap is an insertion-ordered string-keyed map.
type omap struct {
	keys []string
	vals map[string]any
}

func newOmap() *omap { return &omap{vals: make(map[string]any)} }

// set inserts or replaces a key, preserving first-insertion order.
func (m *omap) set(k string, v any) *omap {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
	return m
}

func (m *omap) get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

func (m *omap) len() int { return len(m.keys) }

// unset removes a key, preserving the order of the rest.
func (m *omap) unset(k string) {
	if _, ok := m.vals[k]; !ok {
		return
	}
	delete(m.vals, k)
	for i, key := range m.keys {
		if key == k {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return
		}
	}
}

// setSorted inserts every key of a plain map in sorted order.
func (m *omap) setSorted(src map[string]any) *omap {
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.set(k, src[k])
	}
	return m
}

// MarshalJSON emits the entries in insertion order.
func (m *omap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// yamlNode converts an emission value into a yaml.Node tree. Only the
// value kinds the builders produce are legal; anything else is a bug in
// this package.
func yamlNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case *omap:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range t.keys {
			kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
			vn, err := yamlNode(t.vals[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, kn, vn)
		}
		return n, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range t {
			en, err := yamlNode(e)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, en)
		}
		return n, nil
	case nil, string, bool, int, int64, uint64, float64:
		n := &yaml.Node{}
		if err := n.Encode(v); err != nil {
			return nil, err
		}
		return n, nil
	default:
		return nil, fmt.Errorf("openapi: unsupported emission value %T", v)
	}
}

// emitYAML renders the document as YAML with two-space indentation.
func emitYAML(doc *omap) ([]byte, error) {
	root, err := yamlNode(doc)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// emitJSON renders the document as two-space-indented JSON with a
// trailing newline, matching the YAML emitter's file discipline.
func emitJSON(doc *omap) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
