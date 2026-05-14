// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// loadYAML parses YAML 1.2 input into gojq's untyped graph per the
// cmd/pxq/README.md rules:
//
//   - Explicit `!!` tags are authoritative (`!!str 1` stays string,
//     `!!int 01` stays int).
//   - Untagged scalars use YAML 1.2 Core Schema resolution — which
//     yaml.v3 implements by default. That means the historic 1.1
//     coercions (`yes`/`no`/`on`/`off` → bool) are NOT applied;
//     those values stay strings.
//   - Numeric scalars follow the same int-vs-float lexical split as
//     JSON: presence of `.`, `e`, or `E` triggers the float branch.
//
// We walk the yaml.Node tree directly rather than calling
// node.Decode(&interface{}) so we can inspect tags and route numeric
// scalars through numberFromLexical, which preserves PXF's int/float
// distinction the way the JSON adapter does.
func loadYAML(data []byte) (*loadedDoc, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return &loadedDoc{}, nil
	}
	v, err := yamlNode(root.Content[0])
	if err != nil {
		return nil, err
	}
	return &loadedDoc{body: v}, nil
}

func yamlNode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		return yamlScalar(n)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := yamlNode(c)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		// Mapping nodes are flat (k1, v1, k2, v2, ...).
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			v := n.Content[i+1]
			// Map keys must lower to string for gojq.
			key := k.Value
			val, err := yamlNode(v)
			if err != nil {
				return nil, err
			}
			out[key] = val
		}
		return out, nil
	case yaml.AliasNode:
		if n.Alias == nil {
			return nil, fmt.Errorf("unresolved YAML alias")
		}
		return yamlNode(n.Alias)
	default:
		return nil, fmt.Errorf("unsupported YAML node kind %d", n.Kind)
	}
}

// yamlScalar applies the README's tag-vs-lexical rules. Explicit tags
// win; otherwise we honour Core Schema resolution from the !!tag the
// resolver computed and route numbers through numberFromLexical so the
// `1` vs `1.0` distinction is preserved.
func yamlScalar(n *yaml.Node) (any, error) {
	// yaml.v3 fills n.Tag with the resolved tag (`!!str`, `!!int`, etc.)
	// for both explicit and implicit cases. There's no separate "user
	// tagged this" bit on the public API, so we rely on the Core
	// Schema's tagging being correct (it is — see yaml.v3 resolve.go).
	switch n.Tag {
	case "!!null", "":
		// Empty-tag rarely happens for scalars but treat as null for safety.
		if n.Tag == "" && n.Value != "" {
			return n.Value, nil // bare unknown tag → string
		}
		return nil, nil
	case "!!bool":
		switch n.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			// 1.2 Core Schema rejects yes/no/on/off as bool; if we ever
			// see something else under !!bool it's a tagged literal we
			// should honour conservatively.
			return n.Value == "true", nil
		}
	case "!!int":
		return numberFromLexical(n.Value), nil
	case "!!float":
		// Force float even if the value lacks a decimal (`!!float 1`).
		f := numberFromLexical(n.Value)
		switch x := f.(type) {
		case int:
			return float64(x), nil
		case float64:
			return x, nil
		default:
			return x, nil
		}
	case "!!str":
		return n.Value, nil
	default:
		// Custom or domain tags — surface the lexical value as a string.
		return n.Value, nil
	}
}
