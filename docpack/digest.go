// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The content digest is the hinge of both gates in the model: a review
// approves a specific body, and a translation is made from a specific
// body. Both record a digest, and the compiler compares.
//
// Canonical form: a Topic message carrying only `title`, `summary` and
// `body`, marshaled deterministically. Metadata, review state and
// translation provenance are deliberately excluded — retagging a topic
// or re-recording who approved it does not change what a revisor read,
// and invalidating approvals for those edits would train authors to
// treat the gate as noise.

// contentFields are the fields the digest covers. Changing this list
// changes every recorded digest, so it is a format break: bump
// PackProvenance.format_version alongside it.
var contentFields = []string{"title", "summary", "body"}

// contentDigest returns the lowercase hex SHA-256 of a topic's canonical
// content encoding.
func contentDigest(topic dmsg) (string, error) {
	if !topic.valid() {
		return "", fmt.Errorf("cannot digest an absent topic")
	}
	md := topic.m.Descriptor()
	canon := dynamicpb.NewMessage(md)
	for _, name := range contentFields {
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			return "", fmt.Errorf("%s has no field %q", md.FullName(), name)
		}
		if topic.m.Has(fd) {
			canon.Set(fd, topic.m.Get(fd))
		}
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(canon)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ── Heading ids ───────────────────────────────────────────────────────────

var nonSlugRunes = regexp.MustCompile(`[^a-z0-9]+`)

// slugify derives a heading's fragment id from its text: lowercase,
// non-alphanumerics collapsed to single hyphens, ends trimmed. Authors
// may set an explicit id when the derived one is unstable or ugly; the
// derived form exists so that the common case needs no ceremony and
// still produces a deep link.
func slugify(s string) string {
	s = nonSlugRunes.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}
