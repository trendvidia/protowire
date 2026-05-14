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

var formatFlag string

func main() {
	root := &cobra.Command{
		Use:   "pxq <query> <file>",
		Short: "jq-style query tool for PXF, CSV, JSON, and YAML",
		Long: "pxq runs jq-style queries against PXF documents. CSV, JSON, and " +
			"YAML inputs are transparently adapted to PXF before the query runs; " +
			"output is always PXF. See cmd/pxq/README.md for the full design.",
		Args:          cobra.ExactArgs(2),
		RunE:          run,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Flags().StringVar(&formatFlag, "format", "",
		"override input format detection (pxf|json|yaml|csv); default is "+
			"inferred from the file extension, with stdin (\"-\") defaulting to pxf")

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

	input, err := loadByFormat(format, data)
	if err != nil {
		return fmt.Errorf("parse %s as %s: %w", path, format, err)
	}

	results, err := runQuery(query, input)
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

func loadByFormat(format string, data []byte) (any, error) {
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
