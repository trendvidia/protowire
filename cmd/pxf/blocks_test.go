// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.
package main

import (
	"testing"

	"github.com/trendvidia/protowire-go/encoding/pxf"
)

// blocksDir is the cross-port block-entry separator corpus
// (testdata/blocks/, issue #312, draft -01 § Blocks and Map Tails: block
// entries are separated by whitespace and nothing else), run here
// through Parse, a decoder bound to blocks.v1.Doc and `pxf validate`.
const blocksDir = "../../testdata/blocks"

// blocksVerdicts is what each fixture asserts: the error substring all
// three must give, "" for acceptance, and for an accept fixture the
// address city and the child ids it binds.
var blocksVerdicts = map[string]struct {
	reject, city string
	ids          []string
}{
	"err-comma-in-block.pxf":        {reject: `got , (",")`},
	"err-semicolon-in-block.pxf":    {reject: "unexpected character ';'"},
	"err-comma-in-map.pxf":          {reject: `got , (",")`},
	"err-comma-in-list-element.pxf": {reject: `got , (",")`},
	"space-in-block.pxf":            {city: "Berlin"},
	"newline-in-block.pxf":          {city: "Berlin"},
	"comma-between-elements.pxf":    {ids: []string{"greeting", "title"}},
}

func TestBlocks_Corpus(t *testing.T) {
	type verdict = struct {
		reject, city string
		ids          []string
	}
	runCorpus(t, blocksDir, []string{"blocks.proto"}, blocksVerdicts, func(t *testing.T, f fixture[verdict]) {
		md := mustFind(t, f.reg, "blocks.v1.Doc")
		_, perr := pxf.Parse(f.data)
		msg, derr := pxf.UnmarshalDescriptor(f.data, md)
		verr := f.validate(t, "blocks.v1.Doc")
		for what, err := range map[string]error{"Parse": perr, "decode": derr, "validate": verr} {
			if !errMatches(err, f.want.reject) {
				t.Errorf("%s: want %q, got %v", what, f.want.reject, err)
			}
		}
		if f.want.reject != "" || derr != nil {
			return
		}
		r := msg.ProtoReflect()
		fields := md.Fields()
		addr := r.Get(fields.ByName("address")).Message()
		if got := addr.Get(addr.Descriptor().Fields().ByName("city")).String(); got != f.want.city {
			t.Errorf("address.city = %q, want %q", got, f.want.city)
		}
		if f.want.city != "" {
			if got := addr.Get(addr.Descriptor().Fields().ByName("zip")).String(); got != "10115" {
				t.Errorf("address.zip = %q, want 10115", got)
			}
		}
		list := r.Get(fields.ByName("children")).List()
		if list.Len() != len(f.want.ids) {
			t.Fatalf("children: %d elements, want %d", list.Len(), len(f.want.ids))
		}
		for i, id := range f.want.ids {
			c := list.Get(i).Message()
			if got := c.Get(c.Descriptor().Fields().ByName("id")).String(); got != id {
				t.Errorf("children[%d].id = %q, want %q", i, got, id)
			}
		}
	})
}
