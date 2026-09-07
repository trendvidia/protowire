// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"os"
	"path/filepath"
	"testing"

	schemav1 "github.com/trendvidia/protocompile/gen/protowire/schema/v1"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// carrierOf returns the AnnotationList an Options message carries at
// 1327, or nil. It re-encodes the options and walks the bytes rather
// than asking for a typed extension, so it reads the carrier the same
// way a port without the generated package would — and is indifferent
// to whether the test binary happens to have registered it.
func carrierOf(t *testing.T, opts proto.Message) *schemav1.AnnotationList {
	t.Helper()
	if opts == nil || !opts.ProtoReflect().IsValid() {
		return nil
	}
	b, err := proto.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			t.Fatal("corrupt options tag")
		}
		b = b[n:]
		vn := protowire.ConsumeFieldValue(num, typ, b)
		if vn < 0 {
			t.Fatal("corrupt options field")
		}
		if num == 1327 && typ == protowire.BytesType {
			v, _ := protowire.ConsumeBytes(b)
			payload = append(payload, v...)
		}
		b = b[vn:]
	}
	if payload == nil {
		return nil
	}
	var al schemav1.AnnotationList
	if err := proto.Unmarshal(payload, &al); err != nil {
		t.Fatal(err)
	}
	return &al
}

// TestLiteralCarrierGolden compares what the reference lowers from
// 10_literal_args.proto with 11_literal_carrier_golden.textproto, entry
// by entry in fixture order (each message's own entries, then its
// fields'). Until now the golden was only syntax-checked (README, the
// protoc --encode line) and the corpus would have passed under every
// historical answer to which member a numeric literal takes (#262);
// this is the assertion of the lowered MEMBER that §8.1 "Choosing the
// member" needs.
func TestLiteralCarrierGolden(t *testing.T) {
	raw := buildImage(t, filepath.Join(fixtureDir, "10_literal_args.proto"))
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	got := &schemav1.AnnotationList{}
	found := false
	for _, f := range set.File {
		if filepath.Base(f.GetName()) != "10_literal_args.proto" {
			continue
		}
		found = true
		for _, m := range f.MessageType {
			if al := carrierOf(t, m.Options); al != nil {
				got.Entries = append(got.Entries, al.Entries...)
			}
			for _, fd := range m.Field {
				if al := carrierOf(t, fd.Options); al != nil {
					got.Entries = append(got.Entries, al.Entries...)
				}
			}
		}
	}
	if !found {
		t.Fatal("10_literal_args.proto not in the built image")
	}

	goldenSrc, err := os.ReadFile(filepath.Join(fixtureDir, "11_literal_carrier_golden.textproto"))
	if err != nil {
		t.Fatal(err)
	}
	want := &schemav1.AnnotationList{}
	if err := prototext.Unmarshal(goldenSrc, want); err != nil {
		t.Fatalf("golden does not parse: %v", err)
	}

	if len(got.Entries) != len(want.Entries) {
		t.Fatalf("reference lowered %d entries, golden has %d\ngot:\n%s", len(got.Entries), len(want.Entries), prototext.Format(got))
	}
	for i := range want.Entries {
		if !proto.Equal(want.Entries[i], got.Entries[i]) {
			t.Errorf("entry %d differs\n got: %s\nwant: %s", i,
				prototext.MarshalOptions{}.Format(got.Entries[i]),
				prototext.MarshalOptions{}.Format(want.Entries[i]))
		}
	}
}
