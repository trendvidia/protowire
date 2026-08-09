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

// Loc is where a diagnostic happened: the topic source file, the topic
// within it when the problem is topic-scoped, and — when the compiler
// still holds the position — the source coordinates of the entry the
// check complained about.
//
// File + topic key is the primary address (it is what the authoring
// layer navigates by); Line and Column exist so an editor placing
// squiggles does not have to re-parse the sources it just handed the
// compiler (#187, trendvidia/goed#321). They are 1-based; zero means
// unknown. The baseline for every topic-scoped diagnostic is the
// position of the topic's `key` entry; checks that know the offending
// entry — a review field, a translation digest, a topic-level anchor —
// point at that entry instead.
type Loc struct {
	File   string
	Topic  string
	Line   int
	Column int
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
		if d.Loc.Line > 0 {
			fmt.Fprintf(&sb, ":%d", d.Loc.Line)
			if d.Loc.Column > 0 {
				fmt.Fprintf(&sb, ":%d", d.Loc.Column)
			}
		}
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
		if a.Loc.Line != b.Loc.Line {
			return a.Loc.Line < b.Loc.Line
		}
		if a.Loc.Column != b.Loc.Column {
			return a.Loc.Column < b.Loc.Column
		}
		return a.Message < b.Message
	})
	return out
}
