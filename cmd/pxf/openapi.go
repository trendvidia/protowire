// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/trendvidia/protowire/internal/openapi"
)

// openapiCmd renders the OpenAPI boundary (RFC-001 §#080, issue #173):
// lowered image + optional doc pack in, openapi.yaml/json out. Third
// member of the family: build (schemas → image), docs build (topics →
// pack), openapi (image + pack → boundary formats).
func openapiCmd() *cobra.Command {
	var (
		output   string
		format   string
		check    bool
		pack     string
		cfgPath  string
		audience string
	)
	cmd := &cobra.Command{
		Use:   "openapi [flags] <image.binpb>",
		Short: "render an OpenAPI document from a lowered schema image",
		Long: `Render the OpenAPI boundary from a pxf build image and an optional
pxf docs build pack.

The schema half maps messages, enums and v1.2 type aliases to
components/schemas (keyed by FQN); common @validate shapes become
native keywords (pattern, minLength/maxLength, enum, minimum/maximum)
and everything else is carried through under x-validation. Methods
carrying @http become operations; responses are derived — 200 from the
return type, default from the RFC-001 §7 report model.

Document-level metadata (info, servers, security-scheme definitions),
audience tiers (FQN globs) and protoregistry coordinates for x-since
come from ` + openapi.ConfigFileName + `, discovered upward from the
image or named with --config. --audience emits only elements at or
below the tier; an element whose closure reaches a more restricted
element fails generation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format == "" {
				format = "yaml"
				if strings.HasSuffix(output, ".json") {
					format = "json"
				}
			}
			result, err := openapi.Generate(openapi.Options{
				ImagePath:  args[0],
				PackPath:   pack,
				ConfigPath: cfgPath,
				Audience:   audience,
				Format:     format,
			})
			if err != nil {
				return err
			}
			if check {
				fmt.Fprintln(cmd.ErrOrStderr(), "ok")
				return nil
			}
			if output == "" || output == "-" {
				_, err = cmd.OutOrStdout().Write(result.Document)
				return err
			}
			return os.WriteFile(output, result.Document, 0o644)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&output, "output", "o", "-", `output path ("-" = stdout)`)
	f.StringVar(&format, "format", "", `"yaml" or "json" (default yaml, or json when -o ends in .json)`)
	f.BoolVar(&check, "check", false, "render and report diagnostics only; write no document (CI entry point)")
	f.StringVar(&pack, "pack", "", "compiled doc pack from `pxf docs build` (optional)")
	f.StringVar(&cfgPath, "config", "", "explicit "+openapi.ConfigFileName+" path (skips discovery)")
	f.StringVar(&audience, "audience", "", "emission tier: public, community, partner, enterprise or internal (default public)")
	return cmd
}
