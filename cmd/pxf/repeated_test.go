// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pb"
)

// repeatedDir holds the repeated-field spelling fixtures (issue #337):
// a repeated field is written with a list value, may be bound more than
// once, and the lists concatenate. A scalar or a block bound to a
// repeated field is rejected.
const repeatedDir = "../../testdata/repeated"

func repeatedProto() string { return filepath.Join("../../testdata", "test.proto") }

type repeatedNestedPB struct {
	Name  string `protowire:"1"`
	Value int32  `protowire:"2"`
}

// repeatedAllTypesPB mirrors the two repeated fields of test.v1.AllTypes.
type repeatedAllTypesPB struct {
	RepeatedString []string           `protowire:"12"`
	RepeatedNested []repeatedNestedPB `protowire:"13"`
}

func TestRepeated_ListBindingsConcatenate(t *testing.T) {
	path := filepath.Join(repeatedDir, "concat-lists.pxf")
	if _, err := runCLI(t, "validate", "-p", repeatedProto(), "-m", "test.v1.AllTypes", path); err != nil {
		t.Fatalf("validate: %v", err)
	}
	out, err := runCLI(t, "encode", "-p", repeatedProto(), "-m", "test.v1.AllTypes", path)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got repeatedAllTypesPB
	if err := pb.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("reading encode's bytes back: %v", err)
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(got.RepeatedString, want) {
		t.Errorf("repeated_string = %q, want %q", got.RepeatedString, want)
	}
	var names []string
	for _, n := range got.RepeatedNested {
		names = append(names, n.Name)
	}
	if want := []string{"n1", "n2"}; !slices.Equal(names, want) {
		t.Errorf("repeated_nested names = %q, want %q", names, want)
	}
}

func TestRepeated_ScalarAndBlockBindingsReject(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repeatedDir, "err-*.pxf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("the README lists three reject fixtures, found %d", len(files))
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := runCLI(t, "validate", "-p", repeatedProto(), "-m", "test.v1.AllTypes", path)
			if err == nil {
				t.Fatal("validate accepted a repeated field bound to a scalar or a block")
			}
			if !strings.Contains(err.Error(), "repeated field") {
				t.Fatalf("the rejection should name the repeated-field rule, got: %v", err)
			}
		})
	}
}
