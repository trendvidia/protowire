// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// runPxfCaptured drives a fresh command tree with output captured, for
// the assertions about what a failed run prints (#188).
func runPxfCaptured(t *testing.T, args ...string) (out, errOut string, err error) {
	t.Helper()
	t.Setenv("PROTOWIRE_CONFIG", "")
	var stdout, stderr bytes.Buffer
	root := newRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err = root.Execute()
	return stdout.String(), stderr.String(), err
}

// TestRuntimeErrorPrintsNoUsage pins #188: a command that parsed fine and
// failed at runtime reports the error alone. The usage block is help for
// an invocation cobra could not parse; appended to a runtime failure it
// buries the diagnostic and pollutes anything consuming stderr.
func TestRuntimeErrorPrintsNoUsage(t *testing.T) {
	out, errOut, err := runPxfCaptured(t, "decode", "no-such-file.pb")
	if err == nil {
		t.Fatal("expected a runtime error")
	}
	combined := out + errOut
	if strings.Contains(combined, "Usage:") {
		t.Errorf("runtime error printed the usage block:\n%s", combined)
	}
	if !strings.Contains(errOut, "Error:") {
		t.Errorf("runtime error was not printed at all; stderr:\n%s", errOut)
	}
}

// TestQueryRuntimeErrorIsPrinted covers the command that previously set
// SilenceErrors with nothing else printing the error, so runtime
// failures exited 1 in silence.
func TestQueryRuntimeErrorIsPrinted(t *testing.T) {
	_, errOut, err := runPxfCaptured(t, "query", ".", "no-such-file.pxf")
	if err == nil {
		t.Fatal("expected a runtime error")
	}
	if !strings.Contains(errOut, "Error:") {
		t.Errorf("query runtime error was not printed; stderr:\n%s", errOut)
	}
	if strings.Contains(errOut, "Usage:") {
		t.Errorf("query runtime error printed the usage block:\n%s", errOut)
	}
}

// TestParseErrorsKeepUsage pins the other half of the contract: errors
// usage output can actually help with — an unknown flag, a wrong
// argument count — still print it.
func TestParseErrorsKeepUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown flag":   {"decode", "--no-such-flag"},
		"missing args":   {"decode"},
		"too many args":  {"decode", "a.pb", "b.pb"},
		"docs arg count": {"docs", "build"},
	} {
		t.Run(name, func(t *testing.T) {
			out, errOut, err := runPxfCaptured(t, args...)
			if err == nil {
				t.Fatal("expected a parse error")
			}
			if combined := out + errOut; !strings.Contains(combined, "Usage:") {
				t.Errorf("parse error printed no usage block:\n%s", combined)
			}
		})
	}
}
