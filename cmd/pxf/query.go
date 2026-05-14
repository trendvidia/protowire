// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// query-subcommand-specific flag vars. The shared flags
// (-p / -m / -s / -n / --schema) live on the root as PersistentFlags
// and are already declared in main.go; this file owns only what's
// genuinely query-specific.
var (
	formatFlag string
	strictFlag bool
	looseFlag  bool
)

// queryCmd registers `pxf query <query> <file>`. The previous
// formerly shipped as the standalone `pxq` binary; it now lives as a
// subcommand of `pxf` so the toolchain ships as one executable.
func queryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query <query> <file>",
		Short: "Run a jq-style query against PXF, CSV, JSON, or YAML input",
		Long: "Embeds itchyny/gojq with a pxf_* extension namespace that exposes\n" +
			"PXF directives and schema-bound row binding. Output is always PXF;\n" +
			"see cmd/pxf/QUERY.md for the full design and the function\n" +
			"reference.",
		Args:          cobra.ExactArgs(2),
		RunE:          runQueryCmd,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := cmd.Flags()
	f.StringVar(&formatFlag, "format", "",
		"override input format detection (pxf|json|yaml|csv); default is "+
			"inferred from the file extension, with stdin (\"-\") defaulting to pxf")
	f.BoolVar(&strictFlag, "strict", false,
		"force strict mode (compile-time field-name validation); errors "+
			"if no root type is bound. Default: implicit — strict when a "+
			"root type is in scope, loose otherwise")
	f.BoolVar(&looseFlag, "loose", false,
		"force loose mode (skip strict-mode validation) even when a "+
			"root type is in scope; runtime errors degrade to null per jq")
	return cmd
}

// runQueryCmd is the cobra RunE for `pxf query`. Mirrors the
// previous pxq `run` function — the only changes are flag-var names
// (msgName/server/namespace/schemaName instead of the prior pxq-local
// aliases) and the schemaresolve.RegistryRef shape that now lives in
// the shared internal package.
func runQueryCmd(_ *cobra.Command, args []string) error {
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

	sch, err := loadSchema(protoFiles, doc.protos, schemaresolve.RegistryRef{
		Server:    server,
		Namespace: namespace,
		Schema:    schemaName,
	})
	if err != nil {
		return err
	}

	mode, err := pickMode()
	if err != nil {
		return err
	}
	rootType := resolveRootType(msgName, doc, sch)
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

// loadByFormat dispatches the parsed input through the format-
// specific adapter and returns the unified loadedDoc shape the
// query / emit layers consume.
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
