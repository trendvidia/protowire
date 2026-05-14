// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
//
// pxq is a jq-style query tool whose core operates on PXF documents.
// See cmd/pxq/README.md for the full design.
//
// This file implements Stage A of the rollout: the spine end-to-end
// for the loose-mode PXF→PXF round-trip. JSON/YAML/CSV adapters,
// strict mode, the @pxf.* extension namespace, and the @proto(...)
// constructor land in follow-up PRs.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	formatFlag       string
	protoFiles       []string
	registryServer   string
	registryNS       string
	registrySchemaID string
	messageFlag      string
	strictFlag       bool
	looseFlag        bool
)

func main() {
	root := &cobra.Command{
		Use:   "pxq <query> <file>",
		Short: "jq-style query tool for PXF, CSV, JSON, and YAML",
		Long: "pxq runs jq-style queries against PXF documents. CSV, JSON, and " +
			"YAML inputs are transparently adapted to PXF before the query runs; " +
			"output is always PXF. See cmd/pxq/README.md for the full design.",
		// Two args (query, file) routes to runQuery; subcommands (e.g.
		// `pxq infer-schema`) are matched by name before falling through.
		Args:          cobra.ExactArgs(2),
		RunE:          run,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := root.PersistentFlags()
	f.StringVar(&formatFlag, "format", "",
		"override input format detection (pxf|json|yaml|csv); default is "+
			"inferred from the file extension, with stdin (\"-\") defaulting to pxf")
	f.StringSliceVarP(&protoFiles, "proto", "p", nil,
		".proto file(s) to compile; messages compile into the schema "+
			"resolver used by pxf_proto(...) and pxf_directive('dataset')")
	f.StringVarP(&registryServer, "server", "s", os.Getenv("PROTOREGISTRY_SERVER"),
		"protoregistry gRPC address; together with --namespace and --schema "+
			"fetches a descriptor bundle the schema resolver consumes")
	f.StringVarP(&registryNS, "namespace", "n", os.Getenv("PROTOREGISTRY_NAMESPACE"),
		"protoregistry namespace")
	f.StringVar(&registrySchemaID, "schema", "",
		"protoregistry schema name (within the namespace)")
	f.StringVarP(&messageFlag, "message", "m", "",
		"fully-qualified message name to bind the document root to; "+
			"required for --strict, optional otherwise (the document's "+
			"@type directive supplies it when present)")
	f.BoolVar(&strictFlag, "strict", false,
		"force strict mode (compile-time field-name validation); errors "+
			"if no root type is bound. Default: implicit — strict when a "+
			"root type is in scope, loose otherwise")
	f.BoolVar(&looseFlag, "loose", false,
		"force loose mode (skip strict-mode validation) even when a "+
			"root type is in scope; runtime errors degrade to null per jq")

	root.AddCommand(inferSchemaCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pxq:", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	query, path := args[0], args[1]

	data, err := readInput(path)
	if err != nil {
		return err
	}

	format, err := detectFormat(path, formatFlag)
	if err != nil {
		return err
	}

	doc, err := loadByFormat(format, data)
	if err != nil {
		return fmt.Errorf("parse %s as %s: %w", path, format, err)
	}

	// Anonymous @proto binding is a PXF-grammar rule; non-PXF
	// adapters (CSV/JSON/YAML) synthesize their datasets and have no
	// @proto directives to bind, so the pass is a no-op-with-error
	// trap on them.
	if format == "pxf" {
		if err := resolveAnonymousProtos(doc); err != nil {
			return err
		}
	}

	sch, err := loadSchema(protoFiles, doc.protos, registryRef{
		Server:    registryServer,
		Namespace: registryNS,
		Schema:    registrySchemaID,
	})
	if err != nil {
		return err
	}

	mode, err := pickMode()
	if err != nil {
		return err
	}
	rootType := resolveRootType(messageFlag, doc, sch)
	strict, err := effectiveMode(mode, rootType != nil)
	if err != nil {
		return err
	}

	results, err := runQuery(query, doc, sch, strictOpts{enabled: strict, root: rootType})
	if err != nil {
		return err
	}

	for _, r := range results {
		if err := emitPXF(os.Stdout, r); err != nil {
			return fmt.Errorf("emit: %w", err)
		}
	}
	return nil
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// detectFormat picks the input adapter to use. Explicit --format wins;
// otherwise infer from the file extension. Stdin (`-`) without --format
// defaults to PXF.
func detectFormat(path, override string) (string, error) {
	if override != "" {
		switch override {
		case "pxf", "json", "yaml", "csv":
			return override, nil
		case "yml":
			return "yaml", nil
		default:
			return "", fmt.Errorf("unknown --format %q (want pxf|json|yaml|csv)", override)
		}
	}
	if path == "-" {
		return "pxf", nil
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	switch strings.ToLower(ext) {
	case "pxf":
		return "pxf", nil
	case "json":
		return "json", nil
	case "yaml", "yml":
		return "yaml", nil
	case "csv":
		return "csv", nil
	default:
		return "", fmt.Errorf("cannot infer format from extension %q; pass --format pxf|json|yaml|csv", ext)
	}
}

// pickMode reconciles --strict / --loose into a single modeFlag.
// Mutually exclusive: passing both is a clear user error.
func pickMode() (modeFlag, error) {
	if strictFlag && looseFlag {
		return modeAuto, fmt.Errorf("--strict and --loose are mutually exclusive")
	}
	switch {
	case strictFlag:
		return modeStrict, nil
	case looseFlag:
		return modeLoose, nil
	default:
		return modeAuto, nil
	}
}

func loadByFormat(format string, data []byte) (*loadedDoc, error) {
	switch format {
	case "pxf":
		return loadPXF(data)
	case "json":
		return loadJSON(data)
	case "yaml":
		return loadYAML(data)
	case "csv":
		return loadCSV(data)
	default:
		return nil, fmt.Errorf("internal: unsupported format %q", format)
	}
}
