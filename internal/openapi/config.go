// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire/docpack"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// ConfigFileName is the discovered generator-config file, a
// protowire.openapi.v1.GeneratorConfig in textproto form, looked up by
// upward walk from the image's directory — the same discovery shape
// protowire.config.textproto uses for the engine config.
const ConfigFileName = "protowire.openapi.textproto"

const configMessage = "protowire.openapi.v1.GeneratorConfig"

var (
	configSchemaOnce sync.Once
	configSchemaReg  *schemaresolve.Registry
	configSchemaErr  error
)

func configSchema() (*schemaresolve.Registry, error) {
	configSchemaOnce.Do(func() {
		reg := schemaresolve.NewRegistry()
		if err := schemaresolve.CompileSources(reg, schemaresolve.CompileOptions{
			BundledFiles: []string{"openapi/v1/config.proto"},
		}); err != nil {
			configSchemaErr = fmt.Errorf("compiling bundled openapi config schema: %w", err)
			return
		}
		configSchemaReg = reg
	})
	return configSchemaReg, configSchemaErr
}

// config is the parsed generator configuration.
type config struct {
	title       string
	version     string
	description string
	servers     []serverDef
	schemes     []schemeDef
	audiences   []audienceRule
	registry    registryCoords
}

type serverDef struct{ url, description string }

type schemeDef struct {
	name, typ, scheme, bearerFormat, in, paramName string
}

type audienceRule struct {
	glob string
	// tier is the docs taxonomy enum value name, normalized
	// ("AUDIENCE_PARTNER").
	tier string
	rank int
}

type registryCoords struct{ server, namespace, schema string }

func (r registryCoords) configured() bool {
	return r.server != "" && r.namespace != "" && r.schema != ""
}

// defaultConfig covers a bare image with no config anywhere.
func defaultConfig() *config {
	return &config{title: "Protowire API", version: "0.0.0"}
}

// discoverConfig walks upward from dir looking for ConfigFileName.
func discoverConfig(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		cand := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// loadConfig parses one GeneratorConfig textproto.
func loadConfig(path string) (*config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reg, err := configSchema()
	if err != nil {
		return nil, err
	}
	md := reg.Find(configMessage)
	if md == nil {
		return nil, fmt.Errorf("bundled openapi config schema is missing %s", configMessage)
	}
	msg := dynamicpb.NewMessage(md)
	if err := prototext.Unmarshal(raw, msg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	d := wrap(msg)

	cfg := defaultConfig()
	info := d.sub("info")
	if t := info.str("title"); t != "" {
		cfg.title = t
	}
	if v := info.str("version"); v != "" {
		cfg.version = v
	}
	cfg.description = info.str("description")

	for _, s := range d.msgs("servers") {
		cfg.servers = append(cfg.servers, serverDef{url: s.str("url"), description: s.str("description")})
	}
	for _, s := range d.msgs("security_schemes") {
		def := schemeDef{
			name:         s.str("name"),
			typ:          s.str("type"),
			scheme:       s.str("scheme"),
			bearerFormat: s.str("bearer_format"),
			in:           s.str("in"),
			paramName:    s.str("param_name"),
		}
		if def.name == "" {
			return nil, fmt.Errorf("%s: security_schemes entry without a name", path)
		}
		cfg.schemes = append(cfg.schemes, def)
	}
	for _, a := range d.msgs("audiences") {
		tier, rank, err := normalizeTier(a.str("tier"))
		if err != nil {
			return nil, fmt.Errorf("%s: audiences %q: %w", path, a.str("fqn_glob"), err)
		}
		if a.str("fqn_glob") == "" {
			return nil, fmt.Errorf("%s: audiences entry without fqn_glob", path)
		}
		cfg.audiences = append(cfg.audiences, audienceRule{glob: a.str("fqn_glob"), tier: tier, rank: rank})
	}
	r := d.sub("registry")
	cfg.registry = registryCoords{server: r.str("server"), namespace: r.str("namespace"), schema: r.str("schema")}
	if (cfg.registry != registryCoords{}) && !cfg.registry.configured() {
		return nil, fmt.Errorf("%s: registry needs server, namespace and schema (partial coordinates configured)", path)
	}
	return cfg, nil
}

// normalizeTier maps a config tier string ("partner") onto the docs
// taxonomy value name and rank.
func normalizeTier(s string) (string, int, error) {
	name := "AUDIENCE_" + strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		name = "AUDIENCE_PUBLIC"
	}
	rank, ok := docpack.AudienceRank(name)
	if !ok || name == "AUDIENCE_UNSPECIFIED" {
		return "", 0, fmt.Errorf("unknown audience tier %q (want public, community, partner, enterprise or internal)", s)
	}
	return name, rank, nil
}
