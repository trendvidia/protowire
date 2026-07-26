// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"
	"sort"
	"strings"
)

// Severity distinguishes what fails a build from what merely deserves
// saying. Errors always fail; warnings fail only when the caller opts
// into a stricter policy (release builds, --stale-translations=error).
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// Loc is where a diagnostic happened: the topic source file, and the
// topic within it when the problem is topic-scoped. There are no line
// numbers — PXF parsing gives them for syntax errors, but by the time
// the compiler is validating a typed model the useful coordinate is the
// topic key, which is also what the authoring layer navigates by.
type Loc struct {
	File  string
	Topic string
}

// Diagnostic is one compiler finding.
type Diagnostic struct {
	Severity Severity
	Loc      Loc
	Message  string
}

func (d Diagnostic) String() string {
	var sb strings.Builder
	if d.Loc.File != "" {
		sb.WriteString(d.Loc.File)
		sb.WriteString(": ")
	}
	sb.WriteString(d.Severity.String())
	sb.WriteString(": ")
	if d.Loc.Topic != "" {
		sb.WriteString(d.Loc.Topic)
		sb.WriteString(": ")
	}
	sb.WriteString(d.Message)
	return sb.String()
}

// diags collects findings across the passes.
type diags struct {
	list []Diagnostic
}

func (d *diags) errorf(loc Loc, format string, args ...any) {
	d.list = append(d.list, Diagnostic{SeverityError, loc, fmt.Sprintf(format, args...)})
}

func (d *diags) warnf(loc Loc, format string, args ...any) {
	d.list = append(d.list, Diagnostic{SeverityWarning, loc, fmt.Sprintf(format, args...)})
}

func (d *diags) errors() int {
	n := 0
	for _, x := range d.list {
		if x.Severity == SeverityError {
			n++
		}
	}
	return n
}

// sorted returns the diagnostics in a stable order: errors before
// warnings, then by file, topic and message. Two runs over the same
// inputs must produce the same report, for the same reason the pack is
// byte-stable — CI diffs it.
func (d *diags) sorted() []Diagnostic {
	out := append([]Diagnostic(nil), d.list...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.Loc.File != b.Loc.File {
			return a.Loc.File < b.Loc.File
		}
		if a.Loc.Topic != b.Loc.Topic {
			return a.Loc.Topic < b.Loc.Topic
		}
		return a.Message < b.Message
	})
	return out
}
