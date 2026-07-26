// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/trendvidia/protowire/internal/schemaresolve"
)

// The canonical error-response schema (§#080/GH #177): every operation's
// default response is the §7 report model, rendered from the bundled
// schema/v1/report.proto through the same message mapper as user
// schemas — one mapper, no hand-written mirror to drift.

const reportFQN = "protowire.schema.v1.Report"

var (
	reportOnce  sync.Once
	reportModel *model
	reportErr   error
)

// reportComponents returns the transitive component closure of Report,
// keyed by FQN.
func reportComponents() (map[string]*omap, error) {
	reportOnce.Do(func() {
		files, err := schemaresolve.CompileBundledFiles("schema/v1/report.proto")
		if err != nil {
			reportErr = err
			return
		}
		fds := &descriptorpb.FileDescriptorSet{}
		seen := map[string]bool{}
		var addFile func(fd protoreflect.FileDescriptor)
		addFile = func(fd protoreflect.FileDescriptor) {
			if seen[fd.Path()] {
				return
			}
			seen[fd.Path()] = true
			imports := fd.Imports()
			for i := 0; i < imports.Len(); i++ {
				addFile(imports.Get(i).FileDescriptor)
			}
			fds.File = append(fds.File, protodesc.ToFileDescriptorProto(fd))
		}
		for _, fd := range files {
			addFile(fd)
		}
		reportModel, reportErr = buildModel(fds)
	})
	if reportErr != nil {
		return nil, reportErr
	}
	b := newSchemaBuilder(reportModel, nil)
	if err := b.component(reportFQN); err != nil {
		return nil, fmt.Errorf("rendering §7 report schema: %w", err)
	}
	out := make(map[string]*omap, len(b.out))
	for k, v := range b.out {
		out[k] = v
	}
	return out, nil
}
