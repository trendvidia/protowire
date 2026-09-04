// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// The compiled descriptor sets under testdata/ that ports load at runtime,
// and the annotation extension numbers each must carry.
//
// STABILITY.md promise 3 allocates 1314–1363 and retires the 50000–59999
// squat. Nothing regenerates a .binpb when the annotations.proto it imports
// changes: testdata/sbe-bench.binpb kept 50100/50101/50200/50300 after every
// reader had moved to 1319–1322, and every port's bench-sbe failed on it
// with "missing (sbe.schema_id) option" (issue #244). This test is what
// notices next time. A descriptor set not listed here fails too, so a new
// fixture is classified when it is added rather than when it goes stale.
var descriptorFixtures = map[string][]protowire.Number{
	"bench-test.binpb":              nil,
	"sbe-bench.binpb":               {1319, 1320, 1321, 1322},
	"annotations/settings.binpb":    {1314, 1315},
	"adversarial/adversarial.binpb": {1319, 1320, 1321},
}

// testdata/adversarial/pb/ holds hand-crafted malformed payloads, not
// descriptor sets; they are what the decoders are attacked with.
const notDescriptorSets = "adversarial/pb/"

const (
	retiredLo, retiredHi = 50000, 59999
	blockLo, blockHi     = 1314, 1363
)

func TestDescriptorFixturesCarryRegisteredNumbers(t *testing.T) {
	seen := map[string]bool{}
	err := filepath.WalkDir(testdataRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".binpb" {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, testdataRoot+string(filepath.Separator)))
		if strings.HasPrefix(rel, notDescriptorSets) {
			return nil
		}
		seen[rel] = true
		want, listed := descriptorFixtures[rel]
		if !listed {
			t.Errorf("%s: descriptor set not listed in descriptorFixtures; add it with the numbers it must carry", rel)
			return nil
		}
		t.Run(rel, func(t *testing.T) { checkFixture(t, path, want) })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel := range descriptorFixtures {
		if !seen[rel] {
			t.Errorf("%s: listed in descriptorFixtures but not on disk", rel)
		}
	}
}

const testdataRoot = "../../testdata"

func checkFixture(t *testing.T, path string, want []protowire.Number) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &fds); err != nil {
		t.Fatalf("not a FileDescriptorSet: %v", err)
	}
	if len(fds.File) == 0 {
		t.Fatal("FileDescriptorSet has no files")
	}
	got := optionNumbers(&fds)
	for n := range got {
		if n >= retiredLo && n <= retiredHi {
			t.Errorf("carries retired extension number %d; recompile the fixture against proto/ (STABILITY.md promise 3)", n)
		}
	}
	for _, n := range want {
		if !got[n] {
			t.Errorf("missing extension number %d on an Options message", n)
		}
		if n < blockLo || n > blockHi {
			t.Errorf("test table wants %d, outside the registered block %d–%d", n, blockLo, blockHi)
		}
	}
}

// optionNumbers collects every extension field number set on any Options
// message in the set: file, message, field, oneof, enum, enum value,
// service and method options, walking nested messages. Both resolved
// extensions and raw unknown fields count, because which of the two a
// number lands in depends on whether the extension's Go type was linked
// into this binary.
func optionNumbers(fds *descriptorpb.FileDescriptorSet) map[protowire.Number]bool {
	out := map[protowire.Number]bool{}
	var walkMsg func(m *descriptorpb.DescriptorProto)
	walkMsg = func(m *descriptorpb.DescriptorProto) {
		if m.Options != nil {
			collect(m.Options.ProtoReflect(), out)
		}
		for _, f := range m.Field {
			if f.Options != nil {
				collect(f.Options.ProtoReflect(), out)
			}
		}
		for _, o := range m.OneofDecl {
			if o.Options != nil {
				collect(o.Options.ProtoReflect(), out)
			}
		}
		for _, e := range m.EnumType {
			walkEnum(e, out)
		}
		for _, n := range m.NestedType {
			walkMsg(n)
		}
	}
	for _, f := range fds.File {
		if f.Options != nil {
			collect(f.Options.ProtoReflect(), out)
		}
		for _, m := range f.MessageType {
			walkMsg(m)
		}
		for _, e := range f.EnumType {
			walkEnum(e, out)
		}
		for _, x := range f.Extension {
			if x.Options != nil {
				collect(x.Options.ProtoReflect(), out)
			}
		}
		for _, s := range f.Service {
			if s.Options != nil {
				collect(s.Options.ProtoReflect(), out)
			}
			for _, m := range s.Method {
				if m.Options != nil {
					collect(m.Options.ProtoReflect(), out)
				}
			}
		}
	}
	return out
}

func walkEnum(e *descriptorpb.EnumDescriptorProto, out map[protowire.Number]bool) {
	if e.Options != nil {
		collect(e.Options.ProtoReflect(), out)
	}
	for _, v := range e.Value {
		if v.Options != nil {
			collect(v.Options.ProtoReflect(), out)
		}
	}
}

func collect(m protoreflect.Message, out map[protowire.Number]bool) {
	m.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if fd.IsExtension() {
			out[fd.Number()] = true
		}
		return true
	})
	b := m.GetUnknown()
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return
		}
		b = b[n:]
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return
		}
		b = b[n:]
		out[num] = true
	}
}
