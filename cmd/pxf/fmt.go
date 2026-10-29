// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// pxf fmt — format PXF documents and v1.2 (RFC-001) .proto sources (#425).
//
// buf format cannot parse `@`, so a project that adopts @validate lost its
// .proto formatter and its CI check with it. The .proto layout is the
// trendvidia/protocompile printer's Legacy preset: buf format's layout,
// which protolsp formats with too (protolsp#9), so the editor and CI agree.

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"

	"github.com/trendvidia/protocompile/ast"
	"github.com/trendvidia/protocompile/ast/printer"
	"github.com/trendvidia/protocompile/parser"
	"github.com/trendvidia/protocompile/report"
	"github.com/trendvidia/protocompile/source"
	"github.com/trendvidia/protocompile/token"
	"github.com/trendvidia/protocompile/token/keyword"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

func fmtCmd() *cobra.Command {
	var write, check bool
	cmd := &cobra.Command{
		Use:   "fmt [flags] <file-or-dir>...",
		Short: "Format PXF documents and v1.2 .proto schemas",
		Long: "Reformat PXF documents and .proto schema sources losslessly:\n" +
			"comments and structure are preserved.\n\n" +
			"A .pxf document: when a schema can be bound (via --message, or the\n" +
			"document's @type directive), keyed repeated fields are canonicalized\n" +
			"per draft §3.13: eligible collections are rewritten to the keyed\n" +
			"block form, entry names are unquoted where identifier-safe, and\n" +
			"redundant key-field assignments are dropped. Formatting itself needs\n" +
			"no schema; only the keyed canonicalization does, and a document that\n" +
			"cannot be typed is still formatted.\n\n" +
			"A .proto source, v1.2 syntax (annotations, type, function)\n" +
			"included: laid out as `buf format` lays it out. A file that does not\n" +
			"parse is reported and left alone. So is a file whose formatted\n" +
			"output would lose a comment, an annotation use, or any name, string\n" +
			"or number: the formatter refuses rather than delete source.\n\n" +
			"A directory argument formats every .pxf and .proto beneath it.\n" +
			"Without -w or --check the result goes to stdout, one file at a time.\n" +
			"--check writes nothing, prints a diff for each file that is not\n" +
			"formatted, and exits non-zero if there is one (the CI entry point).",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if write && check {
				return fmt.Errorf("-w and --check are exclusive")
			}
			files, err := collectFmtInputs(args)
			if err != nil {
				return err
			}
			if len(files) > 1 && !write && !check {
				return fmt.Errorf("%d files to format: pass -w to rewrite them or --check to verify them", len(files))
			}
			var unformatted int
			for _, path := range files {
				src, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				out, err := formatFile(path, src)
				if err != nil {
					return err
				}
				switch {
				case check:
					if !bytes.Equal(src, out) {
						unformatted++
						diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
							A:        difflib.SplitLines(string(src)),
							B:        difflib.SplitLines(string(out)),
							FromFile: path,
							ToFile:   path + " (formatted)",
							Context:  3,
						})
						fmt.Print(diff)
					}
				case write:
					if !bytes.Equal(src, out) {
						info, err := os.Stat(path)
						if err != nil {
							return err
						}
						if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
							return err
						}
					}
				default:
					if _, err := os.Stdout.Write(out); err != nil {
						return err
					}
				}
			}
			if unformatted > 0 {
				return fmt.Errorf("%d of %d file(s) not formatted", unformatted, len(files))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false, "rewrite each file in place")
	cmd.Flags().BoolVar(&check, "check", false, "write nothing; print a diff and exit non-zero if a file is not formatted")
	return cmd
}

// collectFmtInputs expands the arguments: a file is taken as given, and a
// directory contributes every .pxf and .proto beneath it, sorted.
func collectFmtInputs(args []string) ([]string, error) {
	var files []string
	for _, arg := range args {
		arg = strings.TrimSuffix(arg, "...")
		if arg == "" {
			arg = "."
		}
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		var found []string
		err = filepath.WalkDir(arg, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".pxf") || strings.HasSuffix(p, ".proto")) {
				found = append(found, p)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("%s: no .pxf or .proto files found", arg)
		}
		slices.Sort(found)
		files = append(files, found...)
	}
	return files, nil
}

func formatFile(path string, src []byte) ([]byte, error) {
	if strings.HasSuffix(path, ".proto") {
		return formatProto(path, src)
	}
	return formatPXF(src)
}

func formatPXF(data []byte) ([]byte, error) {
	doc, err := pxf.Parse(data)
	if err != nil {
		return nil, err
	}
	// Keyed canonicalization needs a schema; plain formatting does
	// not. Prefer --message, then the document's own @type. If no
	// name is available we format without canonicalizing; if a name
	// is available but fails to resolve, that is a real error the
	// user asked for and we surface it.
	name := msgName
	if name == "" {
		name = doc.TypeURL
	}
	if name != "" {
		// A type the document's own @proto defines is the
		// document's (draft § The "@proto" Directive, #371).
		sch, err := documentSchema(doc)
		if err != nil {
			return nil, err
		}
		desc, err := documentMessage(sch, name)
		if err != nil {
			return nil, err
		}
		pxf.CanonicalizeKeyed(doc, desc)
	}
	return pxf.FormatDocument(doc), nil
}

// protoFormat is buf format's layout (the printer's Legacy preset).
var protoFormat = printer.Options{Format: true, Formatting: printer.Legacy()}

func formatProto(path string, src []byte) ([]byte, error) {
	file, err := parseProto(path, src)
	if err != nil {
		return nil, err
	}
	out, err := printer.PrintFile(protoFormat, file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// The printer has dropped source without an error before
	// (trendvidia/protocompile#258: every annotation use; #273: some
	// comments). Compare what the two versions say, not how they are laid
	// out, and refuse to hand back a file that says less.
	formatted, err := parseProto(path, []byte(out))
	if err != nil {
		return nil, fmt.Errorf("%s: the formatted output does not parse, so it is not written: %w", path, err)
	}
	if lost := lostTokens(file, formatted); lost != "" {
		return nil, fmt.Errorf("%s: formatting would lose %s, so the file is left as it is", path, lost)
	}
	return []byte(out), nil
}

// parseProto parses a .proto source and fails on an error diagnostic,
// rendering every diagnostic to stderr.
func parseProto(path string, src []byte) (*ast.File, error) {
	rep := &report.Report{}
	file, _ := parser.Parse(path, source.NewFile(path, string(src)), rep)
	var errCount int
	if len(rep.Diagnostics) > 0 {
		renderer := report.Renderer{Colorize: stderrIsTerminal()}
		var err error
		if errCount, _, err = renderer.Render(rep, os.Stderr); err != nil {
			return nil, err
		}
	}
	if errCount > 0 || file == nil {
		return nil, fmt.Errorf("%s: %d parse error(s); not formatted", path, errCount)
	}
	return file, nil
}

// lostTokens names what before says that after does not: a comment (its
// text compared with whitespace removed, since the layout may re-indent a
// block comment), an annotation's `@`, or an identifier, string or number.
// Keywords and punctuation are left out, because the layout may add or
// drop a separator. It returns "" when nothing is lost.
func lostTokens(before, after *ast.File) string {
	count := func(f *ast.File) map[string]int {
		n := map[string]int{}
		for tok := range f.Stream().All() {
			switch tok.Kind() {
			case token.Comment:
				n["comment "+strings.Join(strings.Fields(tok.Text()), "")]++
			case token.Ident, token.String, token.Number:
				n[tok.Kind().String()+" "+tok.Text()]++
			case token.Keyword:
				if tok.Keyword() == keyword.At {
					n["annotation use @"]++
				}
			}
		}
		return n
	}
	b, a := count(before), count(after)
	var lost []string
	for k, v := range b {
		if a[k] < v {
			lost = append(lost, fmt.Sprintf("%s (%d of %d)", k, v-a[k], v))
		}
	}
	if len(lost) == 0 {
		return ""
	}
	slices.Sort(lost)
	return strings.Join(lost, ", ")
}
