// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const fixtureDir = "../../testdata/schema-extensions"

// checkKnownGaps maps invalid fixtures the current pinned fork does not
// yet reject to the upstream issue tracking the gap. Entries here are
// asserted to still pass --check so a fork upgrade that closes the gap
// flips the test and forces the entry's removal.
var checkKnownGaps = map[string]string{
	// Empty since protocompile v0.33.0 closed protocompile#211 (the
	// §5.4 expression parser); the two fixtures it covered are rejected
	// like every other. The map stays so the next spec-ahead-of-
	// toolchain gap has somewhere to go.
}

// runPxf drives a fresh command tree exactly like the shell would. The
// PROTOWIRE_CONFIG tier is neutralized (empty means unset to the
// loader) so a developer's environment can't leak into the precedence
// assertions.
func runPxf(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv("PROTOWIRE_CONFIG", "")
	root := newRootCmd()
	root.SetArgs(args)
	return root.Execute()
}

// positiveFixtures returns the positive conformance corpus: every
// top-level .proto in testdata/schema-extensions (the invalid/ tree is
// compiled one file at a time by the --check tests, never with the
// positive corpus, per the corpus README).
func positiveFixtures(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(fixtureDir, "*.proto"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("globbing positive fixtures: %v (found %d)", err, len(matches))
	}
	sort.Strings(matches)
	return matches
}

func buildImage(t *testing.T, extraArgs ...string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "image.binpb")
	args := append([]string{"build", "-o", out}, extraArgs...)
	if err := runPxf(t, args...); err != nil {
		t.Fatalf("pxf %s: %v", strings.Join(args, " "), err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestBuildPositiveCorpus is the #164 acceptance path: the positive
// fixtures compile to an image that is byte-stable across runs and is
// stock protobuf — loadable by the vanilla descriptorpb/protodesc
// machinery with every fixture present.
func TestBuildPositiveCorpus(t *testing.T) {
	fixtures := positiveFixtures(t)
	first := buildImage(t, fixtures...)
	second := buildImage(t, fixtures...)
	if !bytes.Equal(first, second) {
		t.Fatalf("image is not byte-stable across runs (%d vs %d bytes)", len(first), len(second))
	}

	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(first, fds); err != nil {
		t.Fatalf("image is not a FileDescriptorSet: %v", err)
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("stock protodesc rejects the lowered image: %v", err)
	}
	for _, f := range fixtures {
		importPath := filepath.Base(f)
		if _, err := files.FindFileByPath(importPath); err != nil {
			t.Errorf("lowered image is missing %s: %v", importPath, err)
		}
	}
	// The canonical annotation library resolves from the bundled embed,
	// with no -p flag and no on-disk copy under the fixture root.
	if _, err := files.FindFileByPath("protowire/schema/v1/annotations.proto"); err != nil {
		t.Errorf("bundled annotation library missing from image: %v", err)
	}
}

// TestBuildDirectoryRoot compiles the corpus via a directory root
// (copied so invalid/ stays out) and checks root-relative import paths.
func TestBuildDirectoryRoot(t *testing.T) {
	rootDir := t.TempDir()
	for _, f := range positiveFixtures(t) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rootDir, filepath.Base(f)), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fromDir := buildImage(t, rootDir)
	fromDots := buildImage(t, rootDir+"/...")
	if !bytes.Equal(fromDir, fromDots) {
		t.Fatal("dir and dir/... arguments produced different images")
	}
	fromFiles := buildImage(t, positiveFixtures(t)...)
	if !bytes.Equal(fromDir, fromFiles) {
		t.Fatal("directory-root and file-args invocations produced different images")
	}
}

// TestCheckInvalidFixtures compiles each invalid fixture one at a time:
// --check must exit non-zero for every error class the corpus manifest
// pins, modulo the tracked toolchain gaps in checkKnownGaps.
func TestCheckInvalidFixtures(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(fixtureDir, "invalid", "*.proto"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("globbing invalid fixtures: %v (found %d)", err, len(matches))
	}
	for _, f := range matches {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			err := runPxf(t, "build", "--check", f)
			if gap, known := checkKnownGaps[name]; known {
				if err != nil {
					t.Fatalf("fixture now rejected — toolchain gap %s is closed; remove it from checkKnownGaps", gap)
				}
				t.Skipf("known toolchain gap: %s", gap)
			}
			if err == nil {
				t.Fatalf("--check accepted invalid fixture %s", name)
			}
		})
	}
}

// TestCheckWritesNoOutput pins --check as diagnostics-only.
func TestCheckWritesNoOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "image.binpb")
	if err := runPxf(t, "build", "--check", "-o", out, filepath.Join(fixtureDir, "01_basic.proto")); err != nil {
		t.Fatalf("--check over valid fixture: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("--check wrote an output file (stat err: %v)", err)
	}
}

// TestFunctionLibraryPrecedence covers the §9.4 slice `build` honors:
// a discovered protowire.config.textproto feeds function_libraries into
// the image, an explicit --config beats discovery, and the per-setting
// --function-library flag beats both.
func TestFunctionLibraryPrecedence(t *testing.T) {
	rootDir := t.TempDir()
	copyFixture := func(name string) {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rootDir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copyFixture("01_basic.proto")
	copyFixture("06_cross_file_lib.proto")

	writeConfig := func(path, lib string) {
		if err := os.WriteFile(path, []byte("function_libraries: \""+lib+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	has := func(image []byte, importPath string) bool {
		fds := &descriptorpb.FileDescriptorSet{}
		if err := proto.Unmarshal(image, fds); err != nil {
			t.Fatal(err)
		}
		for _, f := range fds.GetFile() {
			if f.GetName() == importPath {
				return true
			}
		}
		return false
	}
	target := filepath.Join(rootDir, "01_basic.proto")

	// Baseline: no config anywhere on the walk from rootDir pulls the
	// lib in.
	if has(buildImage(t, target), "06_cross_file_lib.proto") {
		t.Fatal("function library present without any configuration")
	}

	// Discovered file: config sits in the schema root itself.
	discovered := filepath.Join(rootDir, "protowire.config.textproto")
	writeConfig(discovered, "06_cross_file_lib.proto")
	if !has(buildImage(t, target), "06_cross_file_lib.proto") {
		t.Fatal("discovered config's function_libraries not compiled into the image")
	}

	// Explicit --config: points at a config naming no libraries, beating
	// the discovered file that names one.
	explicit := filepath.Join(t.TempDir(), "protowire.config.textproto")
	writeConfig(explicit, "01_basic.proto")
	if has(buildImage(t, "--config", explicit, target), "06_cross_file_lib.proto") {
		t.Fatal("--config did not take precedence over the discovered file")
	}

	// Per-setting flag: beats the explicit config file.
	image := buildImage(t, "--config", explicit, "--function-library", "06_cross_file_lib.proto", target)
	if !has(image, "06_cross_file_lib.proto") {
		t.Fatal("--function-library flag did not take precedence over --config")
	}
}

// TestBuildImportPathCollision pins the guard against two arguments
// claiming one import path for different files.
func TestBuildImportPathCollision(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	for _, d := range []string{dirA, dirB} {
		src := "syntax = \"proto3\"; package clash;"
		if err := os.WriteFile(filepath.Join(d, "clash.proto"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := runPxf(t, "build", "-o", filepath.Join(t.TempDir(), "out.binpb"), dirA, dirB)
	if err == nil || !strings.Contains(err.Error(), "claimed by both") {
		t.Fatalf("expected import-path collision error, got %v", err)
	}
}

// TestBuildNullMask: the image `pxf build` lowers gives a user message
// that can hold a null the generated `_null` FieldMask, so code that
// buf or protoc generates from it has the field (#377). The canonical
// schemas it pulls in from the bundle get none.
func TestBuildNullMask(t *testing.T) {
	dir := t.TempDir()
	schema := `syntax = "proto3";
package nm.v1;
import "google/protobuf/wrappers.proto";
import "pxf/secret.proto";
message M {
  google.protobuf.Int32Value w = 1;
  pxf.Secret s = 2;
}
`
	if err := os.WriteFile(filepath.Join(dir, "nm.proto"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(buildImage(t, filepath.Join(dir, "nm.proto")), fds); err != nil {
		t.Fatal(err)
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("stock protodesc rejects the lowered image: %v", err)
	}
	find := func(name string) protoreflect.MessageDescriptor {
		d, err := files.FindDescriptorByName(protoreflect.FullName(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return d.(protoreflect.MessageDescriptor)
	}
	fd := find("nm.v1.M").Fields().ByName("_null")
	if fd == nil {
		t.Fatal("nm.v1.M has no generated _null field in the image")
	}
	if fd.Number() != 536870911 || fd.Message() == nil || fd.Message().FullName() != "google.protobuf.FieldMask" {
		t.Errorf("_null is number %d, type %v; want 536870911, google.protobuf.FieldMask", fd.Number(), fd.Message())
	}
	if f := find("pxf.Secret").Fields().ByNumber(536870911); f != nil {
		t.Errorf("canonical pxf.Secret got a generated %s field", f.Name())
	}
}

// TestBuildNullMaskSkipsRequired: a proto2 required field cannot hold a
// null (#379), so a message whose only such field is required gets no
// mask, while an optional field in the same file still earns one. Needs
// protocompile v0.35.1 (protocompile#257).
func TestBuildNullMaskSkipsRequired(t *testing.T) {
	dir := t.TempDir()
	schema := `syntax = "proto2";
package nm.v1;
message Strict { required int32 r = 1; }
message Loose { optional int32 o = 1; }
`
	if err := os.WriteFile(filepath.Join(dir, "nm2.proto"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(buildImage(t, filepath.Join(dir, "nm2.proto")), fds); err != nil {
		t.Fatal(err)
	}
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		t.Fatalf("stock protodesc rejects the lowered image: %v", err)
	}
	for name, want := range map[string]bool{"nm.v1.Strict": false, "nm.v1.Loose": true} {
		d, err := files.FindDescriptorByName(protoreflect.FullName(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := d.(protoreflect.MessageDescriptor).Fields().ByName("_null") != nil; got != want {
			t.Errorf("%s has a _null mask: %v, want %v", name, got, want)
		}
	}
}
