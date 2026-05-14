// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TrendVidia, LLC.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bufbuild/protocompile"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/trendvidia/protowire-go/encoding/pxf"
	"github.com/trendvidia/protowire-go/encoding/sbe"

	"github.com/trendvidia/protowire/internal/pxfschema"
	"github.com/trendvidia/protowire/internal/schemaresolve"
)

var (
	protoFiles []string
	msgName    string
	server     string
	namespace  string
	schemaName string
)

func main() {
	root := &cobra.Command{
		Use:   "protowire",
		Short: "PXF encoding tool — human-friendly protobuf text format",
	}

	pf := root.PersistentFlags()
	pf.StringSliceVarP(&protoFiles, "proto", "p", nil, "proto file(s) to compile")
	pf.StringVarP(&msgName, "message", "m", "", "fully qualified message name")
	pf.StringVarP(&server, "server", "s", os.Getenv("PROTOREGISTRY_SERVER"), "protoregistry gRPC address")
	pf.StringVarP(&namespace, "namespace", "n", os.Getenv("PROTOREGISTRY_NAMESPACE"), "protoregistry namespace")
	pf.StringVar(&schemaName, "schema", "", "protoregistry schema name")

	root.AddCommand(
		encodeCmd(), decodeCmd(), validateCmd(), fmtCmd(), lintCmd(),
		sbe2protoCmd(), proto2sbeCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// resolveDescriptor resolves the message descriptor for the configured
// schema source (--proto and/or --server) and narrows to --message.
// Returns an error when neither source is supplied or --message is
// missing.
func resolveDescriptor() (protoreflect.MessageDescriptor, error) {
	if msgName == "" {
		return nil, fmt.Errorf("--message is required")
	}
	if len(protoFiles) == 0 && server == "" {
		return nil, fmt.Errorf("specify --proto or --server to provide a schema")
	}
	reg, err := schemaresolve.Resolve(
		schemaresolve.CompileOptions{UserFiles: protoFiles},
		registryRef(),
	)
	if err != nil {
		return nil, err
	}
	md := reg.Find(msgName)
	if md == nil {
		return nil, fmt.Errorf("message %q not found in resolved schema", msgName)
	}
	return md, nil
}

// registryRef builds a schemaresolve.RegistryRef from the package
// flag vars. Lets the top-level helpers keep their original CLI shape
// while delegating to the shared resolver.
func registryRef() schemaresolve.RegistryRef {
	return schemaresolve.RegistryRef{
		Server:    server,
		Namespace: namespace,
		Schema:    schemaName,
	}
}

func encodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "encode <file.pxf>",
		Short: "Encode PXF to protobuf binary (stdout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			desc, err := resolveDescriptor()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			msg, err := pxf.UnmarshalDescriptor(data, desc)
			if err != nil {
				return err
			}
			out, err := proto.Marshal(msg)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(out)
			return err
		},
	}
}

func decodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decode <file.pb>",
		Short: "Decode protobuf binary to PXF (stdout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			desc, err := resolveDescriptor()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			msg := dynamicpb.NewMessage(desc)
			if err := proto.Unmarshal(data, msg); err != nil {
				return err
			}
			out, err := pxf.Marshal(msg)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(out)
			return err
		},
	}
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.pxf>",
		Short: "Validate PXF against schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			desc, err := resolveDescriptor()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			if _, err := pxf.UnmarshalDescriptor(data, desc); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "valid")
			return nil
		},
	}
}

func fmtCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fmt <file.pxf>",
		Short: "Format PXF file (stdout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			desc, err := resolveDescriptor()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			msg, err := pxf.UnmarshalDescriptor(data, desc)
			if err != nil {
				return err
			}
			out, err := pxf.MarshalOptions{TypeURL: msgName}.Marshal(msg)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(out)
			return err
		},
	}
}

func sbe2protoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sbe2proto <schema.xml>",
		Short: "Convert SBE XML schema to .proto (stdout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			out, err := sbe.XMLToProto(data)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(out)
			return err
		},
	}
}

// resolveFiles returns every file descriptor available from the
// configured schema source (--proto or --server). Used by
// schema-level commands such as `lint`, which operate on entire
// files rather than a single message.
//
// The lint surface specifically wants file-level descriptors (so
// nested-message-and-enum traversal stays inside ValidateReflect),
// not a flat message registry. We run the same compile+fetch pipeline
// as resolveDescriptor but expose the FileDescriptor list instead.
func resolveFiles() ([]protoreflect.FileDescriptor, error) {
	if len(protoFiles) == 0 && server == "" {
		return nil, fmt.Errorf("specify --proto or --server to provide a schema")
	}
	ref := registryRef()
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	var out []protoreflect.FileDescriptor
	if len(protoFiles) > 0 {
		comp := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{}),
		}
		result, err := comp.Compile(context.Background(), protoFiles...)
		if err != nil {
			return nil, fmt.Errorf("compile: %w", err)
		}
		for _, f := range result {
			out = append(out, f)
		}
	}
	if ref.Active() {
		fds, err := schemaresolve.FetchRegistry(ref)
		if err != nil {
			return nil, err
		}
		// lint walks file-level descriptors directly, so hydrate via
		// protodesc here rather than going through Registry (which is
		// a flat message map by design).
		files, err := protodesc.NewFiles(fds)
		if err != nil {
			return nil, fmt.Errorf("protoregistry bundle: %w", err)
		}
		files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
			out = append(out, fd)
			return true
		})
	}
	return out, nil
}

func lintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "Check schema(s) for PXF reserved-name violations (draft §3.13)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			files, err := resolveFiles()
			if err != nil {
				return err
			}
			var all []pxfschema.Violation
			for _, fd := range files {
				all = append(all, pxfschema.ValidateReflect(fd)...)
			}
			if len(all) == 0 {
				fmt.Fprintln(os.Stderr, "ok")
				return nil
			}
			for _, v := range all {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return fmt.Errorf("%d reserved-name violation(s)", len(all))
		},
	}
}

func proto2sbeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "proto2sbe",
		Short: "Convert .proto with SBE annotations to SBE XML (stdout)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(protoFiles) == 0 {
				return fmt.Errorf("--proto is required")
			}
			comp := protocompile.Compiler{
				Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{}),
			}
			result, err := comp.Compile(context.Background(), protoFiles...)
			if err != nil {
				return fmt.Errorf("compile: %w", err)
			}

			want := make(map[string]bool)
			for _, p := range protoFiles {
				want[p] = true
			}
			var files []protoreflect.FileDescriptor
			for _, f := range result {
				if want[f.Path()] {
					files = append(files, f)
				}
			}

			out, err := sbe.ProtoToXML(files...)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(out)
			return err
		},
	}
}
