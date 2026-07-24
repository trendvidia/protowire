// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package schemaext

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// driverMain implements the RFC-001 §9.3 user side of the generated stubs:
// it embeds UnimplementedFunctions, overrides one function, registers the
// implementation through RegisterFunctions against a capture Engine, and
// invokes each registered Function. Results go to stdout as JSON so the
// test can assert the Violation shapes the spec pins.
const driverMain = `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	v2 "github.com/trendvidia/protocheck/v2"
	"google.golang.org/protobuf/proto"

	basic "pxfdriver.test/gen/basic"
	lib "pxfdriver.test/gen/lib"

	_ "pxfdriver.test/gen/app"
	_ "pxfdriver.test/gen/compose"
	_ "pxfdriver.test/gen/errorspb"
	_ "pxfdriver.test/gen/placement"
	_ "pxfdriver.test/gen/presence"
	_ "pxfdriver.test/gen/pxflegacy"
	_ "pxfdriver.test/gen/schemav1"
)

type capture struct{ fns map[string]v2.Function }

func (c *capture) Register(fqn string, impl v2.Function) error {
	if _, dup := c.fns[fqn]; dup {
		return fmt.Errorf("duplicate registration for %s", fqn)
	}
	c.fns[fqn] = impl
	return nil
}
func (c *capture) RegisterCatalog(string, v2.Catalog) error    { return nil }
func (c *capture) Validate(proto.Message) (*v2.Report, error)  { return nil, fmt.Errorf("capture engine") }

type impl struct{ basic.UnimplementedFunctions }

func (impl) IsEmail(value string) (bool, *v2.Violation) {
	if value == "ok@example.com" {
		return true, nil
	}
	return false, &v2.Violation{Code: "email.invalid", FallbackMessage: "not an email: " + value}
}

type outcome struct {
	OK       bool   ` + "`json:\"ok\"`" + `
	NilViol  bool   ` + "`json:\"nil_violation\"`" + `
	Code     string ` + "`json:\"code\"`" + `
	Fallback string ` + "`json:\"fallback\"`" + `
}

func record(ok bool, viol *v2.Violation) outcome {
	o := outcome{OK: ok, NilViol: viol == nil}
	if viol != nil {
		o.Code = viol.Code
		o.Fallback = viol.FallbackMessage
	}
	return o
}

func run(fn v2.Function, args ...any) outcome {
	ok, viol := fn(args)
	return record(ok, viol)
}

func main() {
	out := map[string]any{}

	out["stub_unimplemented"] = record(basic.UnimplementedFunctions{}.IsEmail("x@y"))

	eng := &capture{fns: map[string]v2.Function{}}
	if err := basic.RegisterFunctions(eng, impl{}); err != nil {
		fmt.Fprintln(os.Stderr, "basic.RegisterFunctions:", err)
		os.Exit(1)
	}
	isEmail, ok := eng.fns["fixtures.basic.is_email"]
	if !ok {
		fmt.Fprintln(os.Stderr, "fixtures.basic.is_email not registered")
		os.Exit(1)
	}
	out["accept"] = run(isEmail, "ok@example.com")
	out["reject"] = run(isEmail, "nope")
	out["arity"] = run(isEmail)
	out["arg_type"] = run(isEmail, 42)

	leng := &capture{fns: map[string]v2.Function{}}
	if err := lib.RegisterFunctions(leng, lib.UnimplementedFunctions{}); err != nil {
		fmt.Fprintln(os.Stderr, "lib.RegisterFunctions:", err)
		os.Exit(1)
	}
	fqns := make([]string, 0, len(leng.fns))
	for fqn := range leng.fns {
		fqns = append(fqns, fqn)
	}
	sort.Strings(fqns)
	out["lib_fqns"] = fqns
	out["lib_unimplemented"] = run(leng.fns["fixtures.lib.matches"], "a", "b")

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`

type outcome struct {
	OK       bool   `json:"ok"`
	NilViol  bool   `json:"nil_violation"`
	Code     string `json:"code"`
	Fallback string `json:"fallback"`
}

// TestRegisterFunctionsAndInvoke completes the round trip: the generated
// stubs are compiled into a throwaway module against protocheck, then
// RegisterFunctions and every registered function are invoked and the
// returned Violations checked against the spec-pinned §7 shapes.
func TestRegisterFunctionsAndInvoke(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a generated module; skipped with -short")
	}
	h := getHarness(t)

	dir := t.TempDir()
	for protoPath, goPkg := range goPackages {
		rel := strings.TrimPrefix(goPkg, driverModule+"/")
		pkgDir := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		base := strings.TrimSuffix(filepath.Base(protoPath), ".proto") + ".pb.go"
		if err := os.WriteFile(filepath.Join(pkgDir, base), []byte(h.generated[protoPath]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(driverMain), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(driverGoMod(t, h)), 0o644); err != nil {
		t.Fatal(err)
	}

	goRun := func(args ...string) string {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		// The driver module stands alone: keep the repo's workspace (if
		// any) from leaking in; module resolution is pinned by the
		// replace directives written into its go.mod.
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	goRun("mod", "tidy")
	stdout := goRun("run", ".")

	var results map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("decoding driver output: %v\n%s", err, stdout)
	}
	get := func(key string) outcome {
		t.Helper()
		raw, ok := results[key]
		if !ok {
			t.Fatalf("driver output missing %q:\n%s", key, stdout)
		}
		var o outcome
		if err := json.Unmarshal(raw, &o); err != nil {
			t.Fatalf("decoding %q: %v", key, err)
		}
		return o
	}
	assertViolation := func(key, wantCode, wantFallback string) {
		t.Helper()
		o := get(key)
		if o.OK || o.NilViol {
			t.Errorf("%s: ok=%v violation nil=%v, want a failure with a Violation", key, o.OK, o.NilViol)
			return
		}
		if o.Code != wantCode {
			t.Errorf("%s: Code = %q, want %q", key, o.Code, wantCode)
		}
		if o.Fallback != wantFallback {
			t.Errorf("%s: FallbackMessage = %q, want %q", key, o.Fallback, wantFallback)
		}
	}

	if o := get("accept"); !o.OK || !o.NilViol {
		t.Errorf("accept: got %+v, want (true, nil)", o)
	}

	// A user implementation's own Violation passes through untouched.
	assertViolation("reject", "email.invalid", "not an email: nope")

	// The reserved §7 shapes, exactly as the spec pins them.
	assertViolation("stub_unimplemented",
		"protowire.function.unimplemented", "fixtures.basic.is_email: not implemented")
	assertViolation("arity",
		"protowire.function.invalid_argument", "fixtures.basic.is_email: expected 1 argument(s)")
	assertViolation("arg_type",
		"protowire.function.invalid_argument", "fixtures.basic.is_email: argument 0 is not string")
	assertViolation("lib_unimplemented",
		"protowire.function.unimplemented", "fixtures.lib.matches: not implemented")

	var libFQNs []string
	if err := json.Unmarshal(results["lib_fqns"], &libFQNs); err != nil {
		t.Fatalf("decoding lib_fqns: %v", err)
	}
	if want := []string{"fixtures.lib.is_e164", "fixtures.lib.matches"}; !equalStrings(libFQNs, want) {
		t.Errorf("lib registered FQNs = %v, want %v", libFQNs, want)
	}
}

// driverGoMod pins the driver module to the same protobuf and protocheck
// builds this test ran against, wherever the surrounding build resolved
// them (workspace checkout or module cache).
func driverGoMod(t *testing.T, h *harness) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("module " + driverModule + "\n\ngo 1.26\n\n")
	for _, mod := range []string{"google.golang.org/protobuf", "github.com/trendvidia/protocheck/v2"} {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", mod)
		cmd.Dir = h.repoRoot
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list -m %s: %v", mod, err)
		}
		dir := strings.TrimSpace(string(out))
		if dir == "" {
			t.Fatalf("go list -m %s: empty module dir", mod)
		}
		fmt.Fprintf(&b, "replace %s => %s\n", mod, dir)
	}
	return b.String()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
