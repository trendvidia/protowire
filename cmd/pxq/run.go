// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
)

// stderr is the destination for loose-mode runtime-error hints. Tests
// override it via `errSink = io.Discard` to keep test output clean;
// production goes to os.Stderr.
var errSink io.Writer = os.Stderr

func stderr() io.Writer { return errSink }

// docBody returns the gojq-ready body graph from a loadedDoc, falling
// back to an empty map when the document is body-less (the pxf_* funcs
// still reach directives via the captured env). Always returns a
// map[string]any so jq path expressions like `.x` don't hit a nil
// receiver.
func docBody(doc *loadedDoc) any {
	if doc == nil || doc.body == nil {
		return map[string]any{}
	}
	return doc.body
}

// runQuery compiles `query`, registers the pxf_* extension functions
// against `doc` and `sch`, and runs against the document body —
// returning every result the iterator produces.
//
// Loose mode (Stage A): runtime errors degrade to nil (jq's
// errors-as-null model), matching the README. Compile-time errors in
// the query itself are returned to the caller.
func runQuery(query string, doc *loadedDoc, sch *schema) ([]any, error) {
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	env := &funcEnv{doc: doc, sch: sch}
	code, err := gojq.Compile(q, registerFuncs(env)...)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	body := docBody(doc)
	iter := code.Run(body)
	var out []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		// In loose mode, runtime errors degrade to null per the README
		// (errors-as-null, jq-compatible). Surface the error string on
		// stderr at most once per kind so a typo isn't completely silent
		// — Stage A keeps that helper minimal; Stage C will swap it for
		// the per-file hint the README describes.
		if e, isErr := v.(error); isErr {
			fmt.Fprintf(stderr(), "pxq: %s (loose mode → null)\n", e)
			out = append(out, nil)
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
