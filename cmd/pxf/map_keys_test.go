// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pb"
)

// mapKeysDir is the cross-port map-key corpus (testdata/map-keys/,
// issues #284 and #306): three documents that MUST bind to the keys true
// and false in three spellings, thirteen under invalid/ that MUST NOT
// bind, each rejected with an error naming the key, and three fmt-* pairs
// pinning the formatter's key spelling. Driven here through the CLI
// end-to-end: the bool-key documents needed protowire-go v1.6.0 — the
// keyword production and the spelling fix (protowire-go#93, #109), see
// #286 — and the fmt pairs v1.7.0, the reference formatter's fix
// (protowire-go#123).
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

// labelsPB mirrors mapkeys.v1.Labels for reading `encode`'s output back:
// one field, map<string, string> by_label = 1.
type labelsPB struct {
	ByLabel map[string]string `protowire:"1"`
}

func TestMapKeys_ValidateAndEncodeBind(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(mapKeysDir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	// The fmt-* canonicalization pairs (#306) live in the same directory
	// and have their own test, TestMapKeys_FmtPairsAreFixedPoints; here
	// only the bool-key MUST-bind documents are driven.
	files = slices.DeleteFunc(files, func(p string) bool {
		return strings.HasPrefix(filepath.Base(p), "fmt-")
	})
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

// TestMapKeys_FmtPairsAreFixedPoints drives the three fmt canonicalization
// pairs (#306; draft -01 § Entries and Keys, "Canonical spelling of map
// keys") through the CLI on protowire-go v1.7.0, the release carrying the
// reference formatter's fix (protowire-go#123): each input formats to
// exactly its .expected.pxf, the expected file is a fmt fixed point, and
// both documents of each pair bind through validate and encode to the
// keys the README lists. fmt-keyword-keys (string-keyed): "true",
// "false", "null" and "123" stay quoted, the quoted identifier-safe
// "plain" canonicalizes to bare, bare stays bare. fmt-bare-keys
// (bool-keyed): a bare true and a bare 0 stay bare — a formatter does not
// add quotes the author did not write. fmt-dotted-keys (string-keyed;
// #313): the identifier production admits '.', so "a.b" canonicalizes to
// bare and c.d stays bare, while ".e" and "1.5" stay quoted.
func TestMapKeys_FmtPairsAreFixedPoints(t *testing.T) {
	cases := []struct {
		pair    string
		message string
		// keys checks the map `encode` wrote for the document.
		keys func(t *testing.T, out string)
	}{
		{"fmt-keyword-keys", "mapkeys.v1.Labels", func(t *testing.T, out string) {
			t.Helper()
			var got labelsPB
			if err := pb.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("reading encode's bytes back: %v", err)
			}
			want := []string{"true", "false", "null", "123", "plain", "bare"}
			if len(got.ByLabel) != len(want) {
				t.Fatalf("want exactly the six string keys, got %v", got.ByLabel)
			}
			for _, k := range want {
				if _, ok := got.ByLabel[k]; !ok {
					t.Fatalf("string key %q is absent: %v", k, got.ByLabel)
				}
			}
		}},
		{"fmt-dotted-keys", "mapkeys.v1.Labels", func(t *testing.T, out string) {
			t.Helper()
			var got labelsPB
			if err := pb.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("reading encode's bytes back: %v", err)
			}
			want := []string{"a.b", "c.d", ".e", "1.5"}
			if len(got.ByLabel) != len(want) {
				t.Fatalf("want exactly the four string keys, got %v", got.ByLabel)
			}
			for _, k := range want {
				if _, ok := got.ByLabel[k]; !ok {
					t.Fatalf("string key %q is absent: %v", k, got.ByLabel)
				}
			}
		}},
		{"fmt-bare-keys", "mapkeys.v1.Flags", func(t *testing.T, out string) {
			t.Helper()
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
		}},
	}
	for _, c := range cases {
		t.Run(c.pair, func(t *testing.T) {
			input := filepath.Join(mapKeysDir, c.pair+".pxf")
			expected := filepath.Join(mapKeysDir, c.pair+".expected.pxf")
			want, err := os.ReadFile(expected)
			if err != nil {
				t.Fatal(err)
			}

			got, err := runCLI(t, "fmt", "-p", mapKeysProto(), "-m", c.message, input)
			if err != nil {
				t.Fatalf("fmt %s: %v", c.pair, err)
			}
			if got != string(want) {
				t.Errorf("fmt %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", c.pair, got, want)
			}

			// The expected file is a fmt fixed point.
			fp, err := runCLI(t, "fmt", "-p", mapKeysProto(), "-m", c.message, expected)
			if err != nil {
				t.Fatalf("fmt %s (fixed point): %v", c.pair, err)
			}
			if fp != string(want) {
				t.Errorf("fmt %s not a fixed point:\n--- got ---\n%s\n--- want ---\n%s", c.pair, fp, want)
			}

			// Both documents of the pair bind to the same keys.
			for _, path := range []string{input, expected} {
				if _, err := runCLI(t, "validate", "-p", mapKeysProto(), "-m", c.message, path); err != nil {
					t.Fatalf("validate %s: %v", filepath.Base(path), err)
				}
				out, err := runCLI(t, "encode", "-p", mapKeysProto(), "-m", c.message, path)
				if err != nil {
					t.Fatalf("encode %s: %v", filepath.Base(path), err)
				}
				c.keys(t, out)
			}
		})
	}
}
