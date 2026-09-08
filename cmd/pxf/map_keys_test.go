// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pb"
)

// mapKeysDir is the cross-port bool map-key corpus (testdata/map-keys/,
// issue #284): three documents that MUST bind to the keys true and false
// in three spellings, and thirteen under invalid/ that MUST NOT bind,
// each rejected with an error naming the key. Driven here through the
// CLI end-to-end, which needed protowire-go v1.6.0 — the keyword
// production and the spelling fix (protowire-go#93, #109) — see #286.
const mapKeysDir = "../../testdata/map-keys"

func mapKeysProto() string { return filepath.Join(mapKeysDir, "bool-keys.proto") }

// mapKeysEntryKey is the key as the document spells it — the token before
// the ':' of a `key: "value"` entry, quotes included when the spelling is
// quoted — so a rejection can be checked to name it.
var mapKeysEntryKey = regexp.MustCompile(`(\S+):\s+"`)

// flagsPB mirrors mapkeys.v1.Flags for reading `encode`'s output back:
// one field, map<bool, string> by_flag = 1.
type flagsPB struct {
	ByFlag map[bool]string `protowire:"1"`
}

func TestMapKeys_ValidateAndEncodeBind(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(mapKeysDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("the README lists three MUST-bind documents, found %d", len(files))
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := runCLI(t, "validate", "-p", mapKeysProto(), "-m", "mapkeys.v1.Flags", path); err != nil {
				t.Fatalf("validate: %v", err)
			}
			out, err := runCLI(t, "encode", "-p", mapKeysProto(), "-m", "mapkeys.v1.Flags", path)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			var got flagsPB
			if err := pb.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("reading encode's bytes back: %v", err)
			}
			if len(got.ByFlag) != 2 {
				t.Fatalf("want exactly the keys true and false, got %v", got.ByFlag)
			}
			for _, k := range []bool{true, false} {
				if _, ok := got.ByFlag[k]; !ok {
					t.Fatalf("key %v is absent: %v", k, got.ByFlag)
				}
			}
		})
	}
}

func TestMapKeys_ValidateRejectsNamingTheKey(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(mapKeysDir, "invalid", "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 13 {
		t.Fatalf("the README lists thirteen MUST-NOT-bind documents, found %d", len(files))
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m := mapKeysEntryKey.FindSubmatch(data)
			if m == nil {
				t.Fatal("fixture has no `key: \"value\"` entry")
			}
			key := string(m[1])
			_, err = runCLI(t, "validate", "-p", mapKeysProto(), "-m", "mapkeys.v1.Flags", path)
			if err == nil {
				t.Fatalf("validate accepted %s, which must not bind", key)
			}
			if want := `invalid bool map key ` + key + ` for field "by_flag"`; !strings.Contains(err.Error(), want) {
				t.Fatalf("the rejection must name the key and the field:\n want …%s…\n got  %v", want, err)
			}
		})
	}
}
