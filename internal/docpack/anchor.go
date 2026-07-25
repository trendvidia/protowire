// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/trendvidia/protocompile/fdp"
)

// Anchor identity and stability classification (issue #170, § Anchor
// stability contract).
//
// Every anchor reduces to a canonical id string. That id is what the
// pack records, what redirects are keyed by, and what a runtime looks up
// — one spelling per target, produced in one place, so two consumers can
// never disagree about whether they are pointing at the same thing.

// Anchor oneof field names, mirroring proto/docs/v1/topic.proto.
const (
	anchorSchema         = "schema"
	anchorDescriptorPath = "descriptor_path"
	anchorWidget         = "widget"
	anchorRoute          = "route"
	anchorTopic          = "topic"
)

// stabilityOf classifies an anchor kind. Schema FQNs, widget IDs, topic
// keys and routes are stable by construction — identity is the name.
// Descriptor paths are derived from a specific image with a specific
// toolchain and are valid only against the provenance recorded beside
// them (trendvidia/protolsp#260 is the proof this distinction is load
// bearing, not theoretical).
func stabilityOf(kind string) string {
	if kind == anchorDescriptorPath {
		return "ANCHOR_STABILITY_DERIVED"
	}
	return "ANCHOR_STABILITY_STABLE"
}

var (
	fqnPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	typePattern   = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	identPattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	topicPattern  = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)
	tagPattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// anchorID returns an anchor's canonical id together with its oneof kind.
// A malformed anchor is reported as an error rather than silently given
// an id that would resolve against nothing.
//
// The descriptor-path form is produced through fdp.DescriptorPath, the
// shared §8.3.1 formatter — never hand-assembled here. Deriving the
// string does not prove the target exists; that check needs the image
// and lives in resolveAnchor.
func anchorID(a dmsg) (kind, id string, err error) {
	kind = a.which("target")
	switch kind {
	case "":
		return "", "", fmt.Errorf("anchor has no target set")

	case anchorSchema:
		fqn := a.sub(anchorSchema).str("fqn")
		if fqn == "" {
			return kind, "", fmt.Errorf("schema anchor has an empty fqn")
		}
		if !fqnPattern.MatchString(fqn) {
			return kind, "", fmt.Errorf("schema anchor fqn %q is not a fully-qualified name", fqn)
		}
		return kind, fqn, nil

	case anchorDescriptorPath:
		dp := a.sub(anchorDescriptorPath)
		element := dp.str("element_fqn")
		if element == "" {
			return kind, "", fmt.Errorf("descriptor-path anchor has an empty element_fqn")
		}
		if !fqnPattern.MatchString(element) {
			return kind, "", fmt.Errorf("descriptor-path anchor element_fqn %q is not a fully-qualified name", element)
		}
		annotation := dp.str("annotation_fqn")
		if annotation != "" && !fqnPattern.MatchString(annotation) {
			return kind, "", fmt.Errorf("descriptor-path anchor annotation_fqn %q is not a fully-qualified name", annotation)
		}
		if annotation == "" && dp.u32("ordinal") != 0 {
			return kind, "", fmt.Errorf("descriptor-path anchor sets ordinal %d without an annotation_fqn", dp.u32("ordinal"))
		}
		return kind, fdp.DescriptorPath{
			Element:    element,
			Annotation: annotation,
			Ordinal:    int(dp.u32("ordinal")),
		}.String(), nil

	case anchorWidget:
		w := a.sub(anchorWidget)
		typ, prop, event := w.str("type"), w.str("prop"), w.str("event")
		if typ == "" {
			return kind, "", fmt.Errorf("widget anchor has an empty type")
		}
		if !typePattern.MatchString(typ) {
			return kind, "", fmt.Errorf("widget anchor type %q is not a PascalCase registry name", typ)
		}
		if prop != "" && event != "" {
			return kind, "", fmt.Errorf("widget anchor %s sets both prop %q and event %q", typ, prop, event)
		}
		switch {
		case prop != "":
			if !identPattern.MatchString(prop) {
				return kind, "", fmt.Errorf("widget anchor prop %q is not an identifier", prop)
			}
			return kind, typ + "#prop:" + prop, nil
		case event != "":
			if !identPattern.MatchString(event) {
				return kind, "", fmt.Errorf("widget anchor event %q is not an identifier", event)
			}
			return kind, typ + "#event:" + event, nil
		}
		return kind, typ, nil

	case anchorRoute:
		p := a.sub(anchorRoute).str("path")
		switch {
		case p == "":
			return kind, "", fmt.Errorf("route anchor has an empty path")
		case !strings.HasPrefix(p, "/"):
			return kind, "", fmt.Errorf("route anchor %q must start with %q", p, "/")
		case strings.ContainsAny(p, " \t\n"):
			return kind, "", fmt.Errorf("route anchor %q contains whitespace", p)
		case strings.Contains(p, "#"):
			return kind, "", fmt.Errorf("route anchor %q contains a fragment", p)
		}
		return kind, p, nil

	case anchorTopic:
		key := a.sub(anchorTopic).str("key")
		if key == "" {
			return kind, "", fmt.Errorf("topic anchor has an empty key")
		}
		if !topicPattern.MatchString(key) {
			return kind, "", fmt.Errorf("topic anchor key %q is not a dotted lowercase key", key)
		}
		return kind, key, nil
	}
	return kind, "", fmt.Errorf("anchor has an unknown target kind %q", kind)
}

// widgetMember splits a widget anchor's canonical id back into its type
// and member. Used by catalog resolution, which checks membership
// against the exported entry.
func widgetMember(id string) (typ, kind, member string) {
	typ, rest, ok := strings.Cut(id, "#")
	if !ok {
		return id, "", ""
	}
	kind, member, _ = strings.Cut(rest, ":")
	return typ, kind, member
}
