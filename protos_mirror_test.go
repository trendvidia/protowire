// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package protowire_test

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	"github.com/trendvidia/protos"

	"github.com/trendvidia/protowire"
)

// The shared schemas' canonical home is github.com/trendvidia/protos
// (trendvidia/protos#1). proto/ keeps exact copies, so the paths that
// ports, protocompile's specdrift check and cross_annotation_check read
// stay where they were. This test fails if a copy differs from the
// protos version go.mod pins: an edit goes to protos first, and arrives
// here by bumping protos and copying its files.
func TestBundledSchemasMirrorProtos(t *testing.T) {
	var n int
	err := fs.WalkDir(protos.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		want, err := fs.ReadFile(protos.Files, p)
		if err != nil {
			return err
		}
		// The canonical annotation library is imported as protowire/schema/v1/...
		// and lives at proto/schema/v1/... here; the rest map verbatim.
		local := "proto/" + strings.TrimPrefix(p, "protowire/")
		got, err := fs.ReadFile(protowire.BundledProto, local)
		if err != nil {
			t.Errorf("%s: no bundled copy at %s: %v", p, local, err)
			return nil
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from protos' %s; copy it from the protos module", local, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 {
		t.Errorf("protos.Files holds %d schemas, want 8", n)
	}
}
