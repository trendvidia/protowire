// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"strings"
	"testing"
)

// indocBody is a body typed by its own source-form @proto, with a type
// error in it: user is a string field.
const indocBody = `@proto """
syntax = "proto3";
package l.v1;
import "pxf/secret.proto";
message L { string user = 1; pxf.Secret pw = 2; }
"""
@type l.v1.L
user = 1
`

// A body whose @type names a type its own @proto defines is validated
// against that type, whatever -m says, and -m is optional (#370). Before,
// -m could not see the @proto type, and a different -m type reported
// "unknown field", so the document's real error was never reported.
func TestValidate_BodyResolvesInDocumentProto(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no -m", nil},
		{"-m names the @proto type", []string{"-m", "l.v1.L"}},
		{"-m names another type", []string{"-p", testProto(), "-m", "test.v1.AllTypes"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDoc(t, indocBody, c.args...)
			if err == nil {
				t.Fatal("validate accepted user = 1 on a string field")
			}
			if !strings.Contains(err.Error(), `expected string for field "user"`) {
				t.Fatalf("want the type error on user, got: %v", err)
			}
		})
	}

	ok := strings.TrimSuffix(indocBody, "user = 1\n") + "user = \"ann\"\npw = \"hunter2\"\n"
	if err := validateDoc(t, ok); err != nil {
		t.Fatalf("a well-typed body was rejected: %v", err)
	}
}

// Without an in-document definition, @type stands in for an absent -m;
// with neither, -m is still required.
func TestValidate_TypeDirectiveStandsInForMessage(t *testing.T) {
	doc := "@type test.v1.AllTypes\nstring_field = \"x\"\n"
	if err := validateDoc(t, doc, "-p", testProto()); err != nil {
		t.Fatalf("@type without -m: %v", err)
	}
	if err := validateDoc(t, "string_field = \"x\"\n", "-p", testProto()); err == nil ||
		!strings.Contains(err.Error(), "--message is required") {
		t.Fatalf("no @type and no -m: want --message is required, got %v", err)
	}
}

// A body consumes no anonymous @proto, so one in a body document is
// unbound and rejected (draft §3.4.5).
func TestValidate_BodyRejectsUnboundAnonymousProto(t *testing.T) {
	doc := "@proto { string user = 1; }\n@type l.v1.L\nuser = \"ann\"\n"
	err := validateDoc(t, doc)
	if err == nil || !strings.Contains(err.Error(), "anonymous @proto") {
		t.Fatalf("want the unbound anonymous @proto rejected, got %v", err)
	}
}
