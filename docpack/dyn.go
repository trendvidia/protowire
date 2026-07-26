// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package docpack

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Typed accessors over dynamic messages.
//
// The documentation schemas live in proto/docs/v1 and have no generated
// Go bindings — this repository ships none for any of its canonical
// schemas, because the schemas are the artifact and every port generates
// its own. The compiler therefore works the way the rest of `pxf` does:
// descriptors resolved from the bundled embed, values read and written
// through protoreflect.
//
// The wrapper below exists so that the passes read like field access
// rather than like reflection. Field names are compile-time constants
// mirroring the .proto; a name that does not exist is a bug in this
// package, not bad input, so lookup panics rather than quietly yielding
// a zero value that would make a validation pass silently pass.

// dmsg wraps a dynamic message with by-name field access. The zero value
// is the "absent message" and answers every getter with a zero value, so
// callers can traverse optional submessages without nil checks at every
// hop.
type dmsg struct{ m protoreflect.Message }

func wrap(m protoreflect.Message) dmsg { return dmsg{m: m} }

// newMsg allocates an empty dynamic message of the given type.
func newMsg(md protoreflect.MessageDescriptor) dmsg {
	return dmsg{m: dynamicpb.NewMessage(md)}
}

// valid reports whether the wrapper holds a message at all.
func (d dmsg) valid() bool { return d.m != nil }

// proto returns the underlying message for marshaling.
func (d dmsg) proto() protoreflect.ProtoMessage { return d.m.Interface() }

func (d dmsg) fd(name string) protoreflect.FieldDescriptor {
	f := d.m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if f == nil {
		panic(fmt.Sprintf("docpack: %s has no field %q", d.m.Descriptor().FullName(), name))
	}
	return f
}

func (d dmsg) str(name string) string {
	if !d.valid() {
		return ""
	}
	return d.m.Get(d.fd(name)).String()
}

func (d dmsg) u32(name string) uint32 {
	if !d.valid() {
		return 0
	}
	return uint32(d.m.Get(d.fd(name)).Uint())
}

// enumNum returns the numeric value of an enum field.
func (d dmsg) enumNum(name string) protoreflect.EnumNumber {
	if !d.valid() {
		return 0
	}
	return d.m.Get(d.fd(name)).Enum()
}

// enumName returns an enum field's value name, for diagnostics.
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

// sub returns a singular message field. An unset field yields the zero
// dmsg, which reads as absent rather than panicking.
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

// strs returns a repeated string field.
func (d dmsg) strs(name string) []string {
	if !d.valid() {
		return nil
	}
	list := d.m.Get(d.fd(name)).List()
	out := make([]string, list.Len())
	for i := range out {
		out[i] = list.Get(i).String()
	}
	return out
}

// which returns the name of the field set in the named oneof, or "" when
// the oneof is empty.
func (d dmsg) which(oneof string) string {
	if !d.valid() {
		return ""
	}
	od := d.m.Descriptor().Oneofs().ByName(protoreflect.Name(oneof))
	if od == nil {
		panic(fmt.Sprintf("docpack: %s has no oneof %q", d.m.Descriptor().FullName(), oneof))
	}
	fd := d.m.WhichOneof(od)
	if fd == nil {
		return ""
	}
	return string(fd.Name())
}

// ── Construction ──────────────────────────────────────────────────────────

func (d dmsg) setStr(name, v string) {
	d.m.Set(d.fd(name), protoreflect.ValueOfString(v))
}

func (d dmsg) setU32(name string, v uint32) {
	d.m.Set(d.fd(name), protoreflect.ValueOfUint32(v))
}

func (d dmsg) setBool(name string, v bool) {
	d.m.Set(d.fd(name), protoreflect.ValueOfBool(v))
}

// setEnum sets an enum field by value name. The name is a constant from
// the .proto, so an unknown one is a bug here and panics.
func (d dmsg) setEnum(name, value string) {
	fd := d.fd(name)
	ev := fd.Enum().Values().ByName(protoreflect.Name(value))
	if ev == nil {
		panic(fmt.Sprintf("docpack: %s has no value %q", fd.Enum().FullName(), value))
	}
	d.m.Set(fd, protoreflect.ValueOfEnum(ev.Number()))
}

// setMsg sets a singular message field to an existing message.
func (d dmsg) setMsg(name string, v dmsg) {
	d.m.Set(d.fd(name), protoreflect.ValueOfMessage(v.m))
}

// newSub allocates, sets and returns a singular message field.
func (d dmsg) newSub(name string) dmsg {
	fd := d.fd(name)
	sub := dynamicpb.NewMessage(fd.Message())
	d.m.Set(fd, protoreflect.ValueOfMessage(sub))
	return dmsg{m: sub}
}

// appendMsg appends and returns a new element of a repeated message field.
func (d dmsg) appendMsg(name string) dmsg {
	fd := d.fd(name)
	list := d.m.Mutable(fd).List()
	elem := list.NewElement()
	list.Append(elem)
	return dmsg{m: elem.Message()}
}

func (d dmsg) appendStr(name, v string) {
	d.m.Mutable(d.fd(name)).List().Append(protoreflect.ValueOfString(v))
}

func (d dmsg) appendU32(name string, v uint32) {
	d.m.Mutable(d.fd(name)).List().Append(protoreflect.ValueOfUint32(v))
}
