// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// fixture is one file of a cross-port corpus, handed to a driver's check.
type fixture[V any] struct {
	name string // file name, the verdict table's key
	path string
	data []byte
	want V
	reg  *schemaresolve.Registry
	// protoArgs are the -p flags that hand the corpus schemas to the CLI.
	protoArgs []string
}

// validate runs `pxf validate` on the fixture against msg.
func (f fixture[V]) validate(t *testing.T, msg string) error {
	t.Helper()
	_, err := runCLI(t, append(append(append([]string{"validate"}, f.protoArgs...), "-m", msg), f.path)...)
	return err
}

// runCorpus drives one cross-port fixture corpus (#380): every *.pxf in
// dir, compiled schemas in scope, each fixture a subtest checked by
// check. The verdict table must list exactly the fixtures on disk, so a
// fixture added without a verdict, or a verdict left behind, fails.
func runCorpus[V any](t *testing.T, dir string, schemas []string, verdicts map[string]V, check func(t *testing.T, f fixture[V])) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(verdicts) {
		t.Errorf("found %d fixtures, the verdict table lists %d", len(files), len(verdicts))
	}
	paths := make([]string, len(schemas))
	var protoArgs []string
	for i, s := range schemas {
		paths[i] = filepath.Join(dir, s)
		protoArgs = append(protoArgs, "-p", paths[i])
	}
	reg, err := schemaresolve.Resolve(schemaresolve.CompileOptions{
		UserFiles:    paths,
		BundledFiles: schemaresolve.CompileBundledAll,
	}, schemaresolve.RegistryRef{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			want, ok := verdicts[name]
			if !ok {
				t.Fatal("fixture has no verdict")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			check(t, fixture[V]{name: name, path: path, data: data, want: want, reg: reg, protoArgs: protoArgs})
		})
	}
}

// mustFind returns the message name names in reg, failing the test when
// it is not registered.
func mustFind(t *testing.T, reg *schemaresolve.Registry, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := reg.Find(name)
	if md == nil {
		t.Fatalf("%s not found", name)
	}
	return md
}
