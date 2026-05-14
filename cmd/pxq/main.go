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
	"os"

	"github.com/spf13/cobra"
)

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

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pxq:", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	query, path := args[0], args[1]

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	input, err := loadPXF(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
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
