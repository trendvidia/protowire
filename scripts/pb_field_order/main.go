// Copyright 2026 TrendVidia LLC
// SPDX-License-Identifier: MIT

// Command pb_field_order rewrites a protobuf message with its fields in
// field-number order, at every level, so the write gate compares bytes the
// way draft -01 § PB Binary Encoding compares them (#568). An emitter SHOULD
// write field-number order, and protobuf-go, for one, writes a oneof after the
// other fields.
//
// Only the order of field records changes. Each record keeps its own bytes:
// varints, packed and unpacked repeated fields, and map entries are left as
// written. Records of one field number keep their relative order, so a
// repeated field's elements do not move. A length-delimited field whose type
// is a message (a map entry included) is reordered inside too; its length
// does not change, because reordering keeps the byte count.
//
//	pb_field_order --descriptor text.binpb --schema text.v1.Shapes < in > out
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func main() {
	descPath := flag.String("descriptor", "", "FileDescriptorSet with imports (protoc --include_imports -o)")
	schema := flag.String("schema", "", "fully qualified message name")
	flag.Parse()
	if err := run(*descPath, *schema, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pb_field_order:", err)
		os.Exit(1)
	}
}

func run(descPath, schema string, in io.Reader, out io.Writer) error {
	if descPath == "" || schema == "" {
		return errors.New("--descriptor and --schema are required")
	}
	md, err := loadMessage(descPath, schema)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	sorted, err := FieldOrder(data, md)
	if err != nil {
		return err
	}
	_, err = out.Write(sorted)
	return err
}

func loadMessage(descPath, schema string) (protoreflect.MessageDescriptor, error) {
	raw, err := os.ReadFile(descPath)
	if err != nil {
		return nil, err
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("%s: %w", descPath, err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", descPath, err)
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(schema))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", schema, err)
	}
	md, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a message", schema)
	}
	return md, nil
}

type record struct {
	num   protowire.Number
	bytes []byte
}

// FieldOrder returns data, a message of type md, with its field records in
// field-number order at every level.
func FieldOrder(data []byte, md protoreflect.MessageDescriptor) ([]byte, error) {
	var recs []record
	for len(data) > 0 {
		num, typ, tagLen := protowire.ConsumeTag(data)
		if tagLen < 0 {
			return nil, protowire.ParseError(tagLen)
		}
		valLen := protowire.ConsumeFieldValue(num, typ, data[tagLen:])
		if valLen < 0 {
			return nil, protowire.ParseError(valLen)
		}
		rec := data[:tagLen+valLen]
		if fd := md.Fields().ByNumber(num); fd != nil && typ == protowire.BytesType && fd.Message() != nil {
			payload, n := protowire.ConsumeBytes(data[tagLen:])
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			inner, err := FieldOrder(payload, fd.Message())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fd.FullName(), err)
			}
			// Same length, so the record's length prefix still holds.
			rebuilt := append([]byte{}, data[:tagLen+valLen-len(payload)]...)
			rec = append(rebuilt, inner...)
		}
		recs = append(recs, record{num: num, bytes: rec})
		data = data[tagLen+valLen:]
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].num < recs[j].num })
	var out []byte
	for _, r := range recs {
		out = append(out, r.bytes...)
	}
	return out, nil
}
