// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package openapi

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Typed read-only accessors over dynamic messages, mirroring the
// docpack convention: this repository ships no generated Go for its own
// schemas, so carriers, packs and the generator config are all read
// through protoreflect. Field names are compile-time constants from the
// .proto; an unknown one is a bug in this package and panics.

// dmsg wraps a dynamic message with by-name field access. The zero
// value is the "absent message" and answers every getter with a zero
// value, so callers traverse optional submessages without nil checks.
type dmsg struct{ m protoreflect.Message }

func wrap(m protoreflect.Message) dmsg { return dmsg{m: m} }

func (d dmsg) valid() bool { return d.m != nil }

func (d dmsg) fd(name string) protoreflect.FieldDescriptor {
	f := d.m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		panic(fmt.Sprintf("openapi: %s has no field %q", d.m.Descriptor().FullName(), name))
	}
	return f
}

func (d dmsg) str(name string) string {
	if !d.valid() {
		return ""
	}
	return d.m.Get(d.fd(name)).String()
}

func (d dmsg) i64(name string) int64 {
	if !d.valid() {
		return 0
	}
	return d.m.Get(d.fd(name)).Int()
}

func (d dmsg) f64(name string) float64 {
	if !d.valid() {
		return 0
	}
	return d.m.Get(d.fd(name)).Float()
}

func (d dmsg) boolean(name string) bool {
	if !d.valid() {
		return false
	}
	return d.m.Get(d.fd(name)).Bool()
}

func (d dmsg) bytes(name string) []byte {
	if !d.valid() {
		return nil
	}
	return d.m.Get(d.fd(name)).Bytes()
}

// enumName returns an enum field's value name, for taxonomy lookups.
func (d dmsg) enumName(name string) string {
	if !d.valid() {
		return ""
	}
	fd := d.fd(name)
	v := fd.Enum().Values().ByNumber(d.m.Get(fd).Enum())
	if v == nil {
		return fmt.Sprintf("%d", d.m.Get(fd).Enum())
	}
	return string(v.Name())
}

// sub returns a singular message field; unset yields the zero dmsg.
func (d dmsg) sub(name string) dmsg {
	if !d.valid() || !d.m.Has(d.fd(name)) {
		return dmsg{}
	}
	return dmsg{m: d.m.Get(d.fd(name)).Message()}
}

// msgs returns a repeated message field.
func (d dmsg) msgs(name string) []dmsg {
	if !d.valid() {
		return nil
	}
	list := d.m.Get(d.fd(name)).List()
	out := make([]dmsg, list.Len())
	for i := range out {
		out[i] = dmsg{m: list.Get(i).Message()}
	}
	return out
}

// which returns the set field's name in the named oneof, or "".
func (d dmsg) which(oneof string) string {
	if !d.valid() {
		return ""
	}
	od := d.m.Descriptor().Oneofs().ByName(protoreflect.Name(oneof))
	if od == nil {
		panic(fmt.Sprintf("openapi: %s has no oneof %q", d.m.Descriptor().FullName(), oneof))
	}
	fd := d.m.WhichOneof(od)
	if fd == nil {
		return ""
	}
	return string(fd.Name())
}
