// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// pxf docs build — compile authored documentation topics to a doc pack
// (#170).
//
// The doc pack is the documentation analog of the lowered image `pxf
// build` produces: one typed interchange artifact that every downstream
// surface reads and none of them re-derives.
//
//	pxf docs build -o docs.binpb --image image.binpb --registry registry.json ./topics/...
//
// Conventions are `pxf build`'s: positional roots, `-o` with "-" for
// stdout, `--check` for CI, deterministic output. Consumers are
// appviewer's bundle publish and runtime help (trendvidia/appviewer#364),
// the goed authoring flow (trendvidia/goed#321), and the boundary
// renderers — `pxf openapi` (#173) and the static-HTML export (#171).

package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/trendvidia/protowire/docpack"
)

func docsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Documentation tooling — the typed doc model and its compiler",
		Long: "docs groups the documentation-platform subcommands. The typed\n" +
			"documentation model lives in proto/docs/v1; `docs build` compiles\n" +
			"authored topics into a doc pack. See docs/DOC-PACK.md.",
	}
	cmd.AddCommand(docsBuildCmd(), docsDigestCmd())
	return cmd
}

func docsDigestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "digest <topic-root-or-file>...",
		Short: "Print the content digest of each topic",
		Long: "digest prints the canonical content digest of every topic in the\n" +
			"inputs, one per line, as \"<digest>\\t<key>\\t<locale>\\t<file>\".\n\n" +
			"The digest covers a topic's title, summary and body — the content a\n" +
			"revisor reads — and is what review.approved_digest and\n" +
			"translation.source_digest record. Approving a topic or filing a\n" +
			"translation means writing the digest printed here into the source,\n" +
			"so the authoring flow never has to reimplement the canonical\n" +
			"encoding.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			digests, diagnostics, err := docpack.Digests(args)
			if err != nil {
				return err
			}
			var errCount int
			for _, diag := range diagnostics {
				fmt.Fprintln(os.Stderr, diag)
				if diag.Severity == docpack.SeverityError {
					errCount++
				}
			}
			for _, d := range digests {
				fmt.Printf("%s\t%s\t%s\t%s\n", d.Digest, d.Key, d.Locale, d.File)
			}
			if errCount > 0 {
				return fmt.Errorf("%d documentation error(s)", errCount)
			}
			return nil
		},
	}
}

func docsBuildCmd() *cobra.Command {
	var (
		output       string
		check        bool
		release      bool
		image        string
		registry     string
		sourceLocale string
		staleFatal   bool
	)
	cmd := &cobra.Command{
		Use:   "build [flags] <topic-root-or-file>...",
		Short: "Compile documentation topics to a doc pack",
		Long: "build compiles PXF topic sources (@type protowire.docs.v1.TopicFile)\n" +
			"into a protowire.docs.v1.DocPack: compiled topics, resolved anchors,\n" +
			"redirects and an embedded full-text search index, in one byte-stable\n" +
			"artifact.\n\n" +
			"Each directory argument contributes every .pxf beneath it; a file\n" +
			"argument contributes itself. Topic identity is (key, locale), never\n" +
			"the path, so files may be organised however authors prefer.\n\n" +
			"Anchors resolve against data inputs only: --image supplies the\n" +
			"lowered schema image for schema and descriptor-path anchors,\n" +
			"--registry supplies the appviewer registry export for widget\n" +
			"anchors. Either may be omitted when the corpus uses no anchors of\n" +
			"that kind. A dangling anchor is a compile error; a moved target is a\n" +
			"redirect entry in the model. Descriptor paths are re-derived on\n" +
			"every build and stamped with the image digest — never trusted from\n" +
			"the source.\n\n" +
			"--release applies release policy: topics that are not\n" +
			"REVIEW_STATE_APPROVED are refused, as are approvals invalidated by a\n" +
			"later edit and topics that never chose an audience tier. The revisor\n" +
			"gate is compiler policy, so no authoring tool can skip it.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := docpack.Compile(docpack.Options{
				Inputs:                 args,
				ImagePath:              image,
				CatalogPath:            registry,
				SourceLocale:           sourceLocale,
				Release:                release,
				StaleTranslationsFatal: staleFatal,
				ToolVersion:            toolVersion(),
			})
			if err != nil {
				return err
			}
			for _, diag := range result.Diagnostics {
				fmt.Fprintln(os.Stderr, diag)
			}
			if result.Errors > 0 {
				return fmt.Errorf("%d documentation error(s)", result.Errors)
			}
			if check {
				fmt.Fprintln(os.Stderr, "ok")
				return nil
			}
			// Deterministic marshaling keeps the pack byte-stable across
			// runs — the acceptance bar for caching it, diffing it in CI
			// and shipping it as bundle data.
			raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(result.Pack)
			if err != nil {
				return err
			}
			if output == "" || output == "-" {
				_, err = os.Stdout.Write(raw)
				return err
			}
			return os.WriteFile(output, raw, 0o644)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&output, "output", "o", "-", "output path for the doc pack (\"-\" = stdout)")
	f.BoolVar(&check, "check", false, "compile and report diagnostics only; write no pack (CI entry point)")
	f.BoolVar(&release, "release", false, "apply release policy: refuse unreviewed topics and unset audience tiers")
	f.StringVar(&image, "image", "", "lowered FileDescriptorSet image for schema and descriptor-path anchors")
	f.StringVar(&registry, "registry", "", "appviewer registry export (.binpb, .pxf or .json) for widget anchors")
	f.StringVar(&sourceLocale, "source-locale", "en", "BCP 47 locale topics are authored in")
	f.BoolVar(&staleFatal, "stale-translations-fatal", false, "treat stale translations as errors rather than warnings")
	return cmd
}

// toolVersion reports the version recorded in the pack's provenance. A
// build from source has no module version, so "devel" stands in — the
// field says what produced the pack, and "devel" is the honest answer
// for a working tree.
func toolVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "devel"
	}
	return info.Main.Version
}
