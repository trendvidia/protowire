// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire/docpack"
)

// Doc-pack integration (§#080 Gap 2): topics anchored to schema
// elements contribute their audience tier — documentation and API
// surface cannot disagree — and their summary fills an element's
// missing description.

// packTopic is the slice of a CompiledTopic this renderer consumes.
type packTopic struct {
	key      string
	audience string // enum value name
	summary  string
	// anchors are the resolved schema-anchor ids (element FQNs).
	anchors []string
}

// loadPack reads a compiled DocPack and extracts the schema-anchored
// topics. The pack is read, never re-derived (DOC-PACK.md).
func loadPack(path string) ([]packTopic, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reg, err := docpack.Schema()
	if err != nil {
		return nil, err
	}
	md := reg.Find(docpack.DocPackMessage)
	if md == nil {
		return nil, fmt.Errorf("bundled docs schemas are missing %s", docpack.DocPackMessage)
	}
	msg := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(raw, msg); err != nil {
		return nil, fmt.Errorf("%s is not a compiled doc pack: %w", path, err)
	}
	pack := wrap(msg)

	var topics []packTopic
	for _, ct := range pack.msgs("topics") {
		topic := ct.sub("topic")
		t := packTopic{
			key:      topic.str("key"),
			audience: topic.sub("meta").enumName("audience"),
			summary:  topic.str("summary"),
		}
		for _, ra := range ct.msgs("anchors") {
			if ra.sub("authored").which("target") == "schema" {
				t.anchors = append(t.anchors, ra.str("resolved_id"))
			}
		}
		if len(t.anchors) > 0 {
			topics = append(topics, t)
		}
	}
	return topics, nil
}
