// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package protowire exposes the repository's canonical schemas
// (pxf/*, sbe/*, envelope/v1/*) as an embed.FS so the in-tree CLIs
// can resolve them without forcing every consumer to pass -p flags.
//
// This file lives at the module root because //go:embed patterns can
// only descend, not ascend with "..", so the package containing the
// directive must be a parent (or same-directory) of every embedded
// file. The repo root is the only directory that satisfies that for
// the existing proto/ layout.
package protowire

import "embed"

// BundledProto is the read-only file system carrying every canonical
// .proto in the proto/ tree. Used by cmd/pxf to seed its descriptor
// registry without -p, and available to any future tool that wants
// the same baseline.
//
//go:embed proto
var BundledProto embed.FS

// ConformanceFixtures is the read-only file system carrying the canonical
// RFC-001 schema-extensions conformance corpus (testdata/schema-extensions,
// fixtures plus goldens). Exported so downstream harnesses — the protocheck
// end-to-end round trip (#144) and, eventually, every port's conformance
// suite — consume the fixtures at a pinned module version instead of
// carrying drifting copies.
//
//go:embed testdata/schema-extensions
var ConformanceFixtures embed.FS
