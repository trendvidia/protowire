// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"path/filepath"
	"testing"

	schemav1 "github.com/trendvidia/protos/protowire/schema/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestRequiredOneofCarrier asserts what fixture 25's header says lowers: a
// leading @required reaches its own oneof's 1327 carrier and no other oneof's
// (§5.2, trendvidia/protocheck#22).
func TestRequiredOneofCarrier(t *testing.T) {
	const name = "25_required_oneof.proto"
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(buildImage(t, filepath.Join(fixtureDir, name)), &set); err != nil {
		t.Fatal(err)
	}
	file := imageFile(t, &set, name)
	if len(file.GetMessageType()) != 1 {
		t.Fatalf("fixture 25 has %d messages, want 1", len(file.GetMessageType()))
	}

	entries := map[string]int{}
	for _, o := range file.GetMessageType()[0].GetOneofDecl() {
		list, _ := proto.GetExtension(o.GetOptions(), schemav1.E_OneofAnnotations).(*schemav1.AnnotationList)
		for _, e := range list.GetEntries() {
			if e.GetName() != "protowire.schema.v1.required" {
				t.Errorf("oneof %s carries %s", o.GetName(), e.GetName())
			}
			entries[o.GetName()]++
		}
	}
	if entries["method"] != 1 || entries["note"] != 0 {
		t.Fatalf("@required entries per oneof = %v, want method:1 and note absent", entries)
	}
}
