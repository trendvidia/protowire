// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

// Package schemaext holds the RFC-001 end-to-end round-trip test (issue
// #144): the testdata/schema-extensions fixtures are parsed and lowered by
// the reference protocompile toolchain, the resulting descriptors are fed
// to the forked protoc-gen-go, and the generated §9.3 function stubs are
// compiled against protocheck and invoked. This is the only test that can
// catch drift between the lowering pass's actual output and the
// generator's assumptions — the FunctionDecl.options map keys, the FQNs
// written into Annotation.name, and positional-vs-named annotation args.
package schemaext

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/trendvidia/protocompile/fdp"
	"github.com/trendvidia/protocompile/incremental"
	"github.com/trendvidia/protocompile/incremental/queries"
	"github.com/trendvidia/protocompile/ir"
	"github.com/trendvidia/protocompile/report"
	"github.com/trendvidia/protocompile/source"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

const (
	annotationsPath = "protowire/schema/v1/annotations.proto"
	pxfLegacyPath   = "pxf/annotations.proto"

	// driverModule is the module path of the throwaway Go module the
	// generated code is compiled into.
	driverModule = "pxfdriver.test"
)

// fixtureFiles are compiled under their bare names: that is the import
// path 06_cross_file_main.proto uses to reach 06_cross_file_lib.proto.
var fixtureFiles = []string{
	"01_basic.proto",
	"02_composition.proto",
	"03_message_and_field_annotations.proto",
	"04_required_and_default.proto",
	"05_error_overrides.proto",
	"06_cross_file_lib.proto",
	"06_cross_file_main.proto",
}

// goPackages maps every file protoc-gen-go generates for to its Go import
// path inside the driver module (the plugin's M parameter).
var goPackages = map[string]string{
	"01_basic.proto":                          driverModule + "/gen/basic",
	"02_composition.proto":                    driverModule + "/gen/compose",
	"03_message_and_field_annotations.proto":  driverModule + "/gen/placement",
	"04_required_and_default.proto":           driverModule + "/gen/presence",
	"05_error_overrides.proto":                driverModule + "/gen/errorspb",
	"06_cross_file_lib.proto":                 driverModule + "/gen/lib",
	"06_cross_file_main.proto":                driverModule + "/gen/app",
	annotationsPath:                           driverModule + "/gen/schemav1",
	pxfLegacyPath:                             driverModule + "/gen/pxflegacy",
}

// harness carries the shared round-trip state: the lowered descriptor set
// and the output of the forked protoc-gen-go over it.
type harness struct {
	repoRoot string
	// moduleDir is this nested test module's root — go commands (plugin
	// build, module resolution for the driver) run here so they see this
	// module's private-dep requires and protobuf replace, not the public
	// parent module's graph.
	moduleDir string
	// fds is the lowered FileDescriptorSet, marshal→unmarshal
	// round-tripped so the carrier options are readable through the
	// generated protowire.schema.v1 extension types (the fdp package
	// emits user options as unknown-field bytes).
	fds    *descriptorpb.FileDescriptorSet
	byPath map[string]*descriptorpb.FileDescriptorProto
	// generated maps a proto path from goPackages to the Go source
	// protoc-gen-go produced for it.
	generated map[string]string
}

var (
	harnessOnce sync.Once
	harnessVal  *harness
	harnessErr  error
)

// getHarness compiles the fixtures and runs the generator exactly once per
// test binary.
func getHarness(t *testing.T) *harness {
	t.Helper()
	harnessOnce.Do(func() { harnessVal, harnessErr = buildHarness() })
	if harnessErr != nil {
		t.Fatalf("building round-trip harness: %v", harnessErr)
	}
	return harnessVal
}

func buildHarness() (*harness, error) {
	moduleDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return nil, err
	}
	h := &harness{repoRoot: repoRoot, moduleDir: moduleDir}
	if err := h.compile(); err != nil {
		return nil, err
	}
	if err := h.generate(); err != nil {
		return nil, err
	}
	return h, nil
}

// compile parses the fixtures with the v1.2 parser and lowers them to a
// FileDescriptorSet through the real protocompile pipeline — the same
// carriers (ext 1327–1331) every downstream consumer reads.
func (h *harness) compile() error {
	files := make(map[string]*source.File)
	add := func(importPath, diskPath string) error {
		b, err := os.ReadFile(filepath.Join(h.repoRoot, filepath.FromSlash(diskPath)))
		if err != nil {
			return err
		}
		files[importPath] = source.NewFile(importPath, string(b))
		return nil
	}
	for _, f := range fixtureFiles {
		if err := add(f, "testdata/schema-extensions/"+f); err != nil {
			return err
		}
	}
	if err := add(annotationsPath, "proto/schema/v1/annotations.proto"); err != nil {
		return err
	}
	if err := add(pxfLegacyPath, "proto/pxf/annotations.proto"); err != nil {
		return err
	}

	openers := &source.Openers{source.NewMap(files), source.WKTs()}
	results, rep, err := incremental.Run(context.Background(), incremental.New(), queries.FDS{
		Opener:    openers,
		Session:   new(ir.Session),
		Workspace: source.NewWorkspace(fixtureFiles...),
		Options:   fdp.Options{},
	})
	if err != nil {
		return fmt.Errorf("compiling fixtures: %w", err)
	}
	if rep != nil {
		for _, d := range rep.Diagnostics {
			if d.Level() <= report.Error {
				return fmt.Errorf("compile diagnostic: %s", d.Message())
			}
		}
	}
	if len(results) != 1 || results[0].Value == nil {
		return fmt.Errorf("compiling fixtures: expected 1 non-nil FDS result, got %d", len(results))
	}

	// Round-trip so user-written options move from unknown-field bytes
	// into resolvable extension fields (fdp.DescriptorProto contract).
	raw, err := proto.Marshal(results[0].Value)
	if err != nil {
		return err
	}
	h.fds = &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, h.fds); err != nil {
		return err
	}
	h.byPath = make(map[string]*descriptorpb.FileDescriptorProto)
	for _, f := range h.fds.GetFile() {
		h.byPath[f.GetName()] = f
	}
	for _, f := range fixtureFiles {
		if h.byPath[f] == nil {
			return fmt.Errorf("lowered FileDescriptorSet is missing %s (have %v)", f, fileNames(h.fds))
		}
	}
	return nil
}

// generate builds the forked protoc-gen-go from this module's dependency
// graph (the google.golang.org/protobuf replace) and drives it over the
// lowered descriptors via the standard plugin protocol.
func (h *harness) generate() error {
	binDir, err := os.MkdirTemp("", "schemaext-plugin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(binDir)
	bin := filepath.Join(binDir, "protoc-gen-go")

	build := exec.Command("go", "build", "-o", bin, "google.golang.org/protobuf/cmd/protoc-gen-go")
	build.Dir = h.moduleDir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("building protoc-gen-go: %v\n%s", err, out)
	}

	var mappings []string
	for protoPath, goPkg := range goPackages {
		mappings = append(mappings, "M"+protoPath+"="+goPkg)
	}
	sort.Strings(mappings)
	toGenerate := make([]string, 0, len(goPackages))
	for protoPath := range goPackages {
		toGenerate = append(toGenerate, protoPath)
	}
	sort.Strings(toGenerate)

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: toGenerate,
		Parameter:      proto.String(strings.Join(mappings, ",")),
		ProtoFile:      h.fds.GetFile(),
	}
	raw, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	gen := exec.Command(bin)
	gen.Stdin = bytes.NewReader(raw)
	var stdout, stderr bytes.Buffer
	gen.Stdout = &stdout
	gen.Stderr = &stderr
	if err := gen.Run(); err != nil {
		return fmt.Errorf("running protoc-gen-go: %v\n%s", err, stderr.String())
	}
	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(stdout.Bytes(), resp); err != nil {
		return fmt.Errorf("decoding CodeGeneratorResponse: %w", err)
	}
	if resp.GetError() != "" {
		return fmt.Errorf("protoc-gen-go: %s", resp.GetError())
	}

	// Response file names are derived from the Go import path:
	// pxfdriver.test/gen/basic/01_basic.pb.go. Key the content back to the
	// proto path it was generated from.
	byGenName := make(map[string]string)
	for _, f := range resp.GetFile() {
		byGenName[f.GetName()] = f.GetContent()
	}
	h.generated = make(map[string]string)
	for protoPath, goPkg := range goPackages {
		base := strings.TrimSuffix(path.Base(protoPath), ".proto") + ".pb.go"
		name := goPkg + "/" + base
		content, ok := byGenName[name]
		if !ok {
			return fmt.Errorf("generator produced no file %s (have %v)", name, keysOf(byGenName))
		}
		h.generated[protoPath] = content
	}
	return nil
}

func fileNames(fds *descriptorpb.FileDescriptorSet) []string {
	var names []string
	for _, f := range fds.GetFile() {
		names = append(names, f.GetName())
	}
	return names
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
