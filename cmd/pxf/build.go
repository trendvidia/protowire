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
	pbwire "google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/trendvidia/protocompile/engineconfig"
	"github.com/trendvidia/protocompile/fdp"
	"github.com/trendvidia/protocompile/incremental"
	"github.com/trendvidia/protocompile/incremental/queries"
	"github.com/trendvidia/protocompile/ir"
	"github.com/trendvidia/protocompile/report"
	"github.com/trendvidia/protocompile/source"

	"github.com/trendvidia/protowire"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

func buildCmd() *cobra.Command {
	var (
		output        string
		check         bool
		configTxt     string
		funcLibs      []string
		googleAPIHTTP bool
		nullMask      bool
		sourceInfo    bool
	)
	cmd := &cobra.Command{
		Use:   "build [flags] <schema-root-or-file>...",
		Short: "Compile v1.2 schemas to a lowered FileDescriptorSet image",
		Long: "build compiles Protowire v1.2 (RFC-001) schema sources with the\n" +
			"reference compiler and writes a lowered FileDescriptorSet whose\n" +
			"schema-extension semantics ride in the 1327-1331 carrier options.\n" +
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
			"resolved from the bundled schemas; no -p flags needed.\n\n" +
			"@http lowers twice (RFC-001 §5.2): the annotation carrier keeps the\n" +
			"whole operation surface, and the routing skeleton is written again as\n" +
			"the standard google.api.http option, so connect vanguard, grpc-gateway,\n" +
			"Envoy and buf's OpenAPI plugins bind the routes off a stock image.\n" +
			"Pass --google-api-http=false to emit the carrier alone.\n\n" +
			"Every message with a field that can hold a null gets the generated\n" +
			"_null FieldMask (RFC-001 §8.6); --null-mask=false adds none, so a\n" +
			"project that does not carry PXF nulls over binary keeps protoc's\n" +
			"messages. The image carries SourceCodeInfo, as `buf build`'s does,\n" +
			"so generated code keeps its doc comments; --source-info=false\n" +
			"leaves it out.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var flags engineconfig.Overrides
			if cmd.Flags().Changed("function-library") {
				flags.FunctionLibraries = funcLibs
			}
			return runBuild(cmd, args, buildOptions{
				output:        output,
				check:         check,
				configTxt:     configTxt,
				flags:         flags,
				googleAPIHTTP: googleAPIHTTP,
				nullMask:      nullMask,
				sourceInfo:    sourceInfo,
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&output, "output", "o", "-", "output path for the descriptor-set image (\"-\" = stdout)")
	f.BoolVar(&check, "check", false, "compile and report diagnostics only; write no image (CI entry point)")
	f.StringVar(&configTxt, "config", "", "explicit protowire.config.textproto path (skips PROTOWIRE_CONFIG and discovery)")
	f.StringSliceVar(&funcLibs, "function-library", nil, "function library import path(s); overrides the configured function_libraries")
	f.BoolVar(&googleAPIHTTP, "google-api-http", true, "also lower @http's routing skeleton to the standard google.api.http option")
	f.BoolVar(&nullMask, "null-mask", true, "add the generated _null FieldMask to messages with a field that can hold a null (RFC-001 §8.6)")
	f.BoolVar(&sourceInfo, "source-info", true, "include SourceCodeInfo (comments and source positions) in the image")
	return cmd
}

// buildOptions carries the resolved flag surface of one `pxf build`
// invocation.
type buildOptions struct {
	output    string
	check     bool
	configTxt string
	flags     engineconfig.Overrides
	// googleAPIHTTP mirrors --google-api-http: whether @http's routing
	// skeleton is also written as the standard google.api.http option
	// (RFC-001 §5.2, issue #213). Default true.
	googleAPIHTTP bool
	// nullMask mirrors --null-mask: whether user messages get the
	// generated `_null` FieldMask (RFC-001 §8.6, #377). Default true; off
	// for a project whose generated code must match protoc's (#424).
	nullMask bool
	// sourceInfo mirrors --source-info: whether the image carries
	// SourceCodeInfo, without which every generated doc comment is lost
	// (#424). Default true, as for `buf build`.
	sourceInfo bool
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

	// Stated in both directions rather than only suppressing: the
	// default lives in fdp (the zero Options emits), and a flip there
	// would otherwise silently turn --google-api-http=true into a no-op
	// (RFC-001 §5.2, issue #213). EmitGoogleAPIHTTP(true) is the zero
	// value, so Options stays comparable for the incremental query key.
	var fdpOpts fdp.Options
	fdpOpts.Apply(fdp.EmitGoogleAPIHTTP(opts.googleAPIHTTP), fdp.IncludeSourceCodeInfo(opts.sourceInfo))

	// A nil selector generates no mask anywhere.
	var nullMask func(string) bool
	if opts.nullMask {
		nullMask = schemaresolve.NullMask
	}

	results, rep, err := incremental.Run(cmd.Context(), incremental.New(), queries.FDS{
		Opener:    &chain,
		Session:   &ir.Session{NullMask: nullMask},
		Workspace: source.NewWorkspace(paths...),
		Options:   fdpOpts,
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
	image := proto.Clone(results[0].Value).(*descriptorpb.FileDescriptorSet)
	markImports(image, paths)

	// Deterministic marshaling keeps the image byte-stable across runs —
	// the acceptance bar for caching it as a build artifact.
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(image)
	if err != nil {
		return err
	}
	if opts.output == "" || opts.output == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(opts.output, raw, 0o644)
}

// bufImageFileExtension is the field buf's image format adds to every file
// (buf.alpha.image.v1.ImageFile, field buf_extension). buf generate skips a
// file whose is_import (field 1) is set unless given --include-imports, so
// without it buf generated every dependency of a pxf build image, a
// second copy of field_mask.proto included (#430).
const bufImageFileExtension = 8042

// markImports writes buf's image marker on every file of fds: is_import
// for a file the arguments did not name, and is_syntax_unspecified (field
// 3) as buf writes it. Stock protoc and plugins ignore the field.
func markImports(fds *descriptorpb.FileDescriptorSet, targets []string) {
	for _, f := range fds.File {
		ext := pbwire.AppendTag(nil, 1, pbwire.VarintType)
		ext = pbwire.AppendVarint(ext, pbwire.EncodeBool(!slices.Contains(targets, f.GetName())))
		ext = pbwire.AppendTag(ext, 3, pbwire.VarintType)
		ext = pbwire.AppendVarint(ext, pbwire.EncodeBool(syntaxUnspecified(f)))

		raw := f.ProtoReflect().GetUnknown()
		raw = pbwire.AppendTag(raw, bufImageFileExtension, pbwire.BytesType)
		raw = pbwire.AppendBytes(raw, ext)
		f.ProtoReflect().SetUnknown(raw)
	}
}

// syntaxUnspecified reports whether f's source has no syntax statement.
// protocompile records that in buf's SourceCodeInfo extension (field
// 536000000, is_syntax_unspecified = 2), so it is known only when the
// image carries source info; without it this reports false. It reads the
// marshaled bytes: whether the extension decodes as a known field or an
// unknown one depends on whether buf's Go package is linked, which
// protocompile is removing (trendvidia/protocompile#266).
func syntaxUnspecified(f *descriptorpb.FileDescriptorProto) bool {
	const sourceCodeInfoExtension, isSyntaxUnspecified = 536000000, 2
	if f.GetSourceCodeInfo() == nil {
		return false
	}
	b, err := proto.Marshal(f.GetSourceCodeInfo())
	if err != nil {
		return false
	}
	for len(b) > 0 {
		num, typ, n := pbwire.ConsumeTag(b)
		if n < 0 {
			return false
		}
		b = b[n:]
		if num == sourceCodeInfoExtension && typ == pbwire.BytesType {
			ext, n := pbwire.ConsumeBytes(b)
			if n < 0 {
				return false
			}
			for len(ext) > 0 {
				num, typ, m := pbwire.ConsumeTag(ext)
				if m < 0 {
					return false
				}
				ext = ext[m:]
				if num == isSyntaxUnspecified && typ == pbwire.VarintType {
					v, _ := pbwire.ConsumeVarint(ext)
					return v != 0
				}
				if m = pbwire.ConsumeFieldValue(num, typ, ext); m < 0 {
					return false
				}
				ext = ext[m:]
			}
			b = b[n:]
			continue
		}
		if n = pbwire.ConsumeFieldValue(num, typ, b); n < 0 {
			return false
		}
		b = b[n:]
	}
	return false
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
