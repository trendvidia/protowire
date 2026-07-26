// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// Topic sources are PXF documents typed as protowire.docs.v1.TopicFile.
// PXF is protowire's own authoring format — comments, triple-quoted
// strings for prose, and a typed binding to the schema — so the doc
// model needs no bespoke file format, and the same parser that reads a
// schema fixture reads a topic.

// source is one authored topic file.
type source struct {
	Abs    string // absolute path on disk
	Rel    string // path relative to its input root, slash-separated
	Digest string // lowercase hex SHA-256 of the file bytes
	File   dmsg   // parsed TopicFile
}

// collectSources turns the positional arguments into topic sources. A
// directory (with or without a trailing /...) contributes every .pxf
// beneath it under its root-relative path; a file contributes its base
// name. Two arguments claiming one relative path for different files is
// an error — the pack records source paths, and a collision would make
// provenance ambiguous. Mirrors `pxf build`'s input handling so the two
// subcommands take arguments the same way.
func collectSources(args []string) ([]string, map[string]string, error) {
	fileFor := map[string]string{} // relative path → absolute path
	var order []string

	add := func(rel, abs string) error {
		if prev, ok := fileFor[rel]; ok {
			if prev != abs {
				return fmt.Errorf("topic path %q is claimed by both %s and %s", rel, prev, abs)
			}
			return nil
		}
		fileFor[rel] = abs
		order = append(order, rel)
		return nil
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
			if !strings.HasSuffix(abs, ".pxf") {
				return nil, nil, fmt.Errorf("%s: not a .pxf file or directory", arg)
			}
			if err := add(filepath.Base(abs), abs); err != nil {
				return nil, nil, err
			}
			continue
		}
		var found bool
		walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".pxf") {
				return err
			}
			rel, err := filepath.Rel(abs, p)
			if err != nil {
				return err
			}
			found = true
			return add(filepath.ToSlash(rel), p)
		})
		if walkErr != nil {
			return nil, nil, walkErr
		}
		if !found {
			return nil, nil, fmt.Errorf("%s: no .pxf topic files found", arg)
		}
	}
	sort.Strings(order)
	return order, fileFor, nil
}

// loadSources reads and parses every collected topic file. Parse failures
// are diagnostics rather than hard errors: one unparseable file should
// not hide the problems in the rest of the corpus.
//
// The overlay substitutes in-memory contents by collected relative path
// (Options.Overlay); a key matching no collected file joins the build as
// an overlay-only source, so an editor buffer that has never been saved
// still compiles with the rest of its root.
func loadSources(args []string, overlay map[string][]byte, d *diags) ([]*source, error) {
	md, err := message(TopicFileMessage)
	if err != nil {
		return nil, err
	}
	order, fileFor, err := collectSources(args)
	if err != nil {
		return nil, err
	}
	for rel := range overlay {
		if _, ok := fileFor[rel]; !ok {
			fileFor[rel] = ""
			order = append(order, rel)
		}
	}
	sort.Strings(order)

	out := make([]*source, 0, len(order))
	for _, rel := range order {
		abs := fileFor[rel]
		data, ok := overlay[rel]
		if !ok {
			if data, err = os.ReadFile(abs); err != nil {
				return nil, err
			}
		}
		sum := sha256.Sum256(data)

		// The @type directive is optional in PXF, but when a topic file
		// declares one it must be the topic-file message: a document
		// claiming to be something else was almost certainly pointed at
		// the wrong compiler.
		if doc, err := pxf.Parse(data); err != nil {
			d.errorf(Loc{File: rel}, "parsing PXF: %v", err)
			continue
		} else if doc.TypeURL != "" && doc.TypeURL != TopicFileMessage {
			d.errorf(Loc{File: rel}, "@type is %s; topic files must be %s", doc.TypeURL, TopicFileMessage)
			continue
		}

		msg, err := pxf.UnmarshalDescriptor(data, md)
		if err != nil {
			d.errorf(Loc{File: rel}, "binding to %s: %v", TopicFileMessage, err)
			continue
		}
		out = append(out, &source{
			Abs:    abs,
			Rel:    rel,
			Digest: hex.EncodeToString(sum[:]),
			File:   wrap(msg),
		})
	}
	return out, nil
}

// digestOf is the shared file-digest helper for provenance records.
func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
