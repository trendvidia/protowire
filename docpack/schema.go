// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// BundledDocsSchemas are the canonical documentation schemas, as import
// paths into the repository's embedded proto/ tree. They are compiled on
// demand rather than added to schemaresolve.CompileBundledAll: only the
// docs pipeline needs them, and every other `pxf` subcommand would pay
// the compile cost for messages it never resolves.
var BundledDocsSchemas = []string{
	"docs/v1/topic.proto",
	"docs/v1/pack.proto",
	"docs/v1/registry.proto",
}

// Fully-qualified names of the messages the compiler binds to. Bound by
// name against the bundled schemas — the same resolution path `pxf`
// already uses for every other typed document.
const (
	TopicFileMessage     = "protowire.docs.v1.TopicFile"
	DocPackMessage       = "protowire.docs.v1.DocPack"
	WidgetCatalogMessage = "protowire.docs.v1.WidgetCatalog"
)

var (
	schemaOnce sync.Once
	schemaReg  *schemaresolve.Registry
	schemaErr  error
)

// Schema returns the descriptor registry for the bundled documentation
// schemas. Compiled once per process: the sources are embedded and
// immutable, so a second compile could only produce the same answer more
// slowly.
func Schema() (*schemaresolve.Registry, error) {
	schemaOnce.Do(func() {
		reg := schemaresolve.NewRegistry()
		if err := schemaresolve.CompileSources(reg, schemaresolve.CompileOptions{
			BundledFiles: BundledDocsSchemas,
		}); err != nil {
			schemaErr = fmt.Errorf("compiling bundled docs schemas: %w", err)
			return
		}
		schemaReg = reg
	})
	return schemaReg, schemaErr
}

// message resolves one bundled docs message by fully-qualified name.
func message(name string) (protoreflect.MessageDescriptor, error) {
	reg, err := Schema()
	if err != nil {
		return nil, err
	}
	md := reg.Find(name)
	if md == nil {
		return nil, fmt.Errorf("bundled docs schema does not define %s", name)
	}
	return md, nil
}
