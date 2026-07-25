// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// pxf build — compile v1.2 (RFC-001) schema sources to a lowered
// FileDescriptorSet image (#164).
//
// This is the only v1.2-aware step of the two-stage workflow:
//
//	pxf build -o image.binpb ./schemas/...
//	buf generate image.binpb
//
// Everything downstream (stock buf/protoc, the forked protoc-gen-go,
// protoc-gen-pxf-*) consumes the image; stock tooling never sees v1.2
// source. The compiler is the trendvidia/protocompile fork's library
// pipeline (incremental.Run + queries.FDS) — deliberately distinct from
// the upstream bufbuild/protocompile the other subcommands use for
// stock parsing.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/trendvidia/protocompile/engineconfig"
	"github.com/trendvidia/protocompile/fdp"
	"github.com/trendvidia/protocompile/incremental"
	"github.com/trendvidia/protocompile/incremental/queries"
	"github.com/trendvidia/protocompile/ir"
	"github.com/trendvidia/protocompile/report"
	"github.com/trendvidia/protocompile/source"

	"github.com/trendvidia/protowire"
)

func buildCmd() *cobra.Command {
	var (
		output    string
		check     bool
		configTxt string
		funcLibs  []string
	)
	cmd := &cobra.Command{
		Use:   "build [flags] <schema-root-or-file>...",
		Short: "Compile v1.2 schemas to a lowered FileDescriptorSet image",
		Long: "build compiles Protowire v1.2 (RFC-001) schema sources with the\n" +
			"reference compiler and writes a lowered FileDescriptorSet whose\n" +
			"schema-extension semantics ride in the 50400-50404 carrier options.\n" +
			"The image is stock protobuf: feed it to `buf generate` or\n" +
			"`protoc --descriptor_set_in` — no v1.2-aware buf/protoc needed.\n\n" +
			"Each directory argument is an import root; every .proto beneath it\n" +
			"is compiled with its root-relative path as import path (a trailing\n" +
			"/... is accepted and equivalent). A file argument is compiled under\n" +
			"its base name with its parent directory as import root.\n\n" +
			"Engine configuration (RFC-001 §9.4) is discovered by upward walk\n" +
			"from the first argument's directory; precedence is per-setting\n" +
			"flags > --config > PROTOWIRE_CONFIG > discovered file > defaults.\n" +
			"Configured function_libraries are compiled into the image.\n\n" +
			"Well-known types and the canonical annotation libraries\n" +
			"(protowire/schema/v1/annotations.proto, pxf/annotations.proto) are\n" +
			"resolved from the bundled schemas; no -p flags needed.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var flags engineconfig.Overrides
			if cmd.Flags().Changed("function-library") {
				flags.FunctionLibraries = funcLibs
			}
			return runBuild(cmd, args, buildOptions{
				output:    output,
				check:     check,
				configTxt: configTxt,
				flags:     flags,
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&output, "output", "o", "-", "output path for the descriptor-set image (\"-\" = stdout)")
	f.BoolVar(&check, "check", false, "compile and report diagnostics only; write no image (CI entry point)")
	f.StringVar(&configTxt, "config", "", "explicit protowire.config.textproto path (skips PROTOWIRE_CONFIG and discovery)")
	f.StringSliceVar(&funcLibs, "function-library", nil, "function library import path(s); overrides the configured function_libraries")
	return cmd
}

// buildOptions carries the resolved flag surface of one `pxf build`
// invocation.
type buildOptions struct {
	output    string
	check     bool
	configTxt string
	flags     engineconfig.Overrides
}

func runBuild(cmd *cobra.Command, args []string, opts buildOptions) error {
	roots, paths, err := collectBuildInputs(args)
	if err != nil {
		return err
	}

	cfg, err := engineconfig.Load(engineconfig.Options{
		Flags:      opts.flags,
		Dir:        configDiscoveryDir(args[0]),
		ConfigPath: opts.configTxt,
	})
	if err != nil {
		return err
	}
	// Function libraries are schema files whose declarations downstream
	// consumers (§9.3 stub codegen, engine startup) read from the image,
	// so they compile as workspace members, resolved like any import.
	for _, lib := range cfg.FunctionLibraries {
		if !slices.Contains(paths, lib) {
			paths = append(paths, lib)
		}
	}

	chain := source.Openers{&bundledOpener}
	for _, root := range roots {
		chain = append(chain, &source.FS{FS: os.DirFS(root)})
	}
	chain = append(chain, source.WKTs())

	results, rep, err := incremental.Run(cmd.Context(), incremental.New(), queries.FDS{
		Opener:    &chain,
		Session:   new(ir.Session),
		Workspace: source.NewWorkspace(paths...),
		Options:   fdp.Options{},
	})
	if err != nil {
		return fmt.Errorf("compiling schemas: %w", err)
	}

	var errCount int
	if rep != nil && len(rep.Diagnostics) > 0 {
		renderer := report.Renderer{Colorize: stderrIsTerminal()}
		if errCount, _, err = renderer.Render(rep, os.Stderr); err != nil {
			return err
		}
	}
	if errCount > 0 {
		return fmt.Errorf("%d compile error(s)", errCount)
	}
	if opts.check {
		fmt.Fprintln(os.Stderr, "ok")
		return nil
	}

	if len(results) != 1 || results[0].Value == nil {
		return fmt.Errorf("compiling schemas: expected 1 non-nil FileDescriptorSet result, got %d", len(results))
	}
	// Deterministic marshaling keeps the image byte-stable across runs —
	// the acceptance bar for caching it as a build artifact.
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(results[0].Value)
	if err != nil {
		return err
	}
	if opts.output == "" || opts.output == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(opts.output, raw, 0o644)
}

// collectBuildInputs turns the positional arguments into import roots
// and the workspace's import paths. A directory (with or without a
// trailing /...) contributes every .proto beneath it under its
// root-relative path; a file contributes its base name with its parent
// directory as root. Two arguments claiming one import path for
// different files is an error — the opener chain would silently serve
// whichever root comes first.
func collectBuildInputs(args []string) (roots, paths []string, err error) {
	seenRoot := map[string]bool{}
	fileFor := map[string]string{} // import path → absolute source path
	addPath := func(importPath, abs string) error {
		if prev, ok := fileFor[importPath]; ok {
			if prev != abs {
				return fmt.Errorf("import path %q is claimed by both %s and %s", importPath, prev, abs)
			}
			return nil
		}
		fileFor[importPath] = abs
		paths = append(paths, importPath)
		return nil
	}
	addRoot := func(dir string) {
		if !seenRoot[dir] {
			seenRoot[dir] = true
			roots = append(roots, dir)
		}
	}

	for _, arg := range args {
		arg = strings.TrimSuffix(arg, "...")
		arg = strings.TrimSuffix(arg, string(filepath.Separator))
		if arg == "" {
			arg = "."
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			if !strings.HasSuffix(abs, ".proto") {
				return nil, nil, fmt.Errorf("%s: not a .proto file or directory", arg)
			}
			addRoot(filepath.Dir(abs))
			if err := addPath(filepath.Base(abs), abs); err != nil {
				return nil, nil, err
			}
			continue
		}
		addRoot(abs)
		var found bool
		walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".proto") {
				return err
			}
			rel, err := filepath.Rel(abs, p)
			if err != nil {
				return err
			}
			found = true
			return addPath(filepath.ToSlash(rel), p)
		})
		if walkErr != nil {
			return nil, nil, walkErr
		}
		if !found {
			return nil, nil, fmt.Errorf("%s: no .proto files found", arg)
		}
	}
	sort.Strings(paths)
	return roots, paths, nil
}

// configDiscoveryDir maps the first schema argument to the directory
// engine-config discovery starts its upward walk from (§9.4: the
// working directory or an explicit schema root — the schema root here).
func configDiscoveryDir(arg string) string {
	arg = strings.TrimSuffix(arg, "...")
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		return arg
	}
	return filepath.Dir(arg)
}

// bundledCanonicalOpener serves the repository's canonical schemas from
// the embedded proto/ tree, so `pxf build` users never need to vendor
// protowire/schema/v1/annotations.proto or the pxf/sbe/envelope
// libraries. Import paths map to the embed two ways: the canonical
// annotation library is imported as protowire/schema/v1/... but lives
// at proto/schema/v1/..., while the legacy libraries (pxf/..., sbe/...,
// envelope/v1/...) live at proto/<import path> verbatim.
//
// The single package-level instance exists because protocompile openers
// must be comparable through a stable pointer.
var bundledOpener = bundledCanonicalOpener{}

type bundledCanonicalOpener struct{}

func (*bundledCanonicalOpener) Open(p string) (*source.File, error) {
	p = path.Clean(p)
	candidates := []string{"proto/" + p}
	if rest, ok := strings.CutPrefix(p, "protowire/"); ok {
		candidates = append(candidates, "proto/"+rest)
	}
	for _, c := range candidates {
		if data, err := protowire.BundledProto.ReadFile(c); err == nil {
			return source.NewFile(p, string(data)), nil
		}
	}
	return nil, fs.ErrNotExist
}

func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
