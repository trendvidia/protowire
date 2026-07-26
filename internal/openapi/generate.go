// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package openapi renders the OpenAPI boundary from a lowered schema
// image and an optional compiled doc pack (RFC-001 §#080, issue #173).
//
// It is a boundary renderer in the §#080 sense: the internal model is
// the image's protobuf carriers; JSON and YAML exist only in the final
// emission step. Responses are derived, never authored (GH #177);
// audience tiers are generator configuration, filtered at emission and
// never stripped from descriptors; x-since is derived from registry
// history or omitted.
package openapi

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Options configures one generation run.
type Options struct {
	// ImagePath is the lowered FileDescriptorSet from `pxf build`.
	ImagePath string
	// PackPath optionally names a compiled doc pack from `pxf docs build`.
	PackPath string
	// ConfigPath explicitly names the generator config; empty discovers
	// ConfigFileName upward from the image's directory.
	ConfigPath string
	// Audience is the emission tier ("public" … "internal"); empty
	// means public.
	Audience string
	// Format is "yaml" or "json".
	Format string
}

// Result carries the rendered document.
type Result struct {
	Document []byte
}

// Generate runs the renderer.
func Generate(opts Options) (*Result, error) {
	cfg := defaultConfig()
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = discoverConfig(filepath.Dir(opts.ImagePath))
	}
	if configPath != "" {
		var err error
		cfg, err = loadConfig(configPath)
		if err != nil {
			return nil, err
		}
	}

	_, audienceRank, err := normalizeTier(opts.Audience)
	if err != nil {
		return nil, err
	}

	m, err := loadModel(opts.ImagePath)
	if err != nil {
		return nil, err
	}

	ai := newAudienceIndex(cfg.audiences)
	if opts.PackPath != "" {
		topics, err := loadPack(opts.PackPath)
		if err != nil {
			return nil, err
		}
		for _, t := range topics {
			for _, fqn := range t.anchors {
				ai.contribute(fqn, t.key, t.audience)
			}
		}
	}
	if err := ai.checkTransitive(m); err != nil {
		return nil, err
	}
	include := func(fqn string) bool { return ai.rankOf(fqn) <= audienceRank }

	since, err := buildSince(cfg.registry)
	if err != nil {
		return nil, err
	}

	schemes := make(map[string]bool, len(cfg.schemes))
	for _, s := range cfg.schemes {
		schemes[s.name] = true
	}

	sb := newSchemaBuilder(m, include)
	if err := sb.buildAll(); err != nil {
		return nil, err
	}
	ob := newOperationsBuilder(m, sb, include, schemes)
	if err := ob.buildAll(); err != nil {
		return nil, err
	}

	doc := newOmap()
	doc.set("openapi", "3.1.0")
	info := newOmap().set("title", cfg.title).set("version", cfg.version)
	if cfg.description != "" {
		info.set("description", cfg.description)
	}
	doc.set("info", info)
	if len(cfg.servers) > 0 {
		servers := make([]any, 0, len(cfg.servers))
		for _, s := range cfg.servers {
			sv := newOmap().set("url", s.url)
			if s.description != "" {
				sv.set("description", s.description)
			}
			servers = append(servers, sv)
		}
		doc.set("servers", servers)
	}

	paths := ob.emitPaths()
	// Stamp operations with x-since through the path index.
	for _, svc := range m.services {
		for _, mth := range svc.methods {
			use, ok := parseHTTP(findAnn(mth.anns, annHTTP))
			if !ok {
				continue
			}
			if item, found := paths.get(use.path); found {
				if op, found := item.(*omap).get(strings.ToLower(use.method)); found {
					since.stamp(op.(*omap), svc.fqn+"."+mth.name)
				}
			}
		}
	}
	doc.set("paths", paths)

	// Components: user schemas plus, when any operation was emitted,
	// the §7 report closure.
	all := make(map[string]*omap, len(sb.out))
	for k, v := range sb.out {
		if v != nil {
			all[k] = v
		}
	}
	if ob.needsReport {
		reports, err := reportComponents()
		if err != nil {
			return nil, err
		}
		for k, v := range reports {
			if v != nil {
				all[k] = v
			}
		}
	}
	for fqn, s := range all {
		since.stamp(s, fqn)
	}
	schemasOut := newOmap()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		schemasOut.set(k, all[k])
	}
	components := newOmap()
	if schemasOut.len() > 0 {
		components.set("schemas", schemasOut)
	}
	if len(ob.usedSchemes) > 0 {
		out := newOmap()
		for _, s := range cfg.schemes {
			if !ob.usedSchemes[s.name] {
				continue
			}
			def := newOmap().set("type", s.typ)
			if s.scheme != "" {
				def.set("scheme", s.scheme)
			}
			if s.bearerFormat != "" {
				def.set("bearerFormat", s.bearerFormat)
			}
			if s.in != "" {
				def.set("in", s.in)
			}
			if s.paramName != "" {
				def.set("name", s.paramName)
			}
			out.set(s.name, def)
		}
		components.set("securitySchemes", out)
	}
	if components.len() > 0 {
		doc.set("components", components)
	}

	var raw []byte
	switch opts.Format {
	case "", "yaml":
		raw, err = emitYAML(doc)
	case "json":
		raw, err = emitJSON(doc)
	default:
		return nil, fmt.Errorf("unknown format %q (want yaml or json)", opts.Format)
	}
	if err != nil {
		return nil, err
	}
	return &Result{Document: raw}, nil
}
