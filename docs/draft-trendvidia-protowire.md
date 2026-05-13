---
v: 3
docname: draft-trendvidia-protowire-00
title: The Proto eXpressive Format (PXF) and the protowire Encoding Family
abbrev: protowire
category: info
date: 2026-05-04
ipr: trust200902
stream: IETF
area: Applications
workgroup: Network Working Group
keyword:
 - protobuf
 - serialization
 - text format
 - sbe
author:
 -
   initials: B.
   surname: Franco Jr.
   fullname: B. Franco Jr.
   org: TrendVidia, LLC
   email: contact@trendvidia.com

normative:
  RFC2119:
  RFC3339:
  RFC3629:
  RFC4648:
  RFC5234:
  RFC6838:
  RFC7405:
  RFC8174:
  RFC8259:
  RFC8446:
  RFC9110:
  IEEE754:
    title: IEEE Standard for Floating-Point Arithmetic
    author:
      - org: IEEE
    date: 2019-07
    seriesinfo:
      IEEE: 754-2019
  PROTOBUF:
    title: Protocol Buffers Language Specification (proto3)
    author:
      - org: Google
    target: https://protobuf.dev/reference/protobuf/proto3-spec/
  PROTOBUF-WIRE:
    title: Protocol Buffers Encoding
    author:
      - org: Google
    target: https://protobuf.dev/programming-guides/encoding/
  FIX-SBE:
    title: Simple Binary Encoding, Version 2.0
    author:
      - org: FIX Trading Community
    date: 2020
    target: https://www.fixtrading.org/standards/sbe/

informative:
  I-D.ietf-dispatch-mime-protobuf:
  RFC6648:
  PROTOJSON:
    title: Protocol Buffers JSON Mapping (ProtoJSON)
    author:
      - org: Google
    target: https://protobuf.dev/programming-guides/json/
  GRPC:
    title: gRPC over HTTP/2
    author:
      - org: gRPC Authors
    target: https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md
  DEFUSEDXML:
    title: "defusedxml: Defuses XML bombs and other exploits"
    author:
      - ins: C. Heimes
        name: Christian Heimes
    target: https://pypi.org/project/defusedxml/
  PROTOWIRE-BIGNUM:
    title: PXF arbitrary-precision numeric types
    author:
      - org: TrendVidia LLC
    target: https://github.com/trendvidia/protowire

--- abstract

This document specifies the Proto eXpressive Format (PXF), a UTF-8 text serialization for messages defined by Protocol Buffers schemas, together with three companion encodings: PB (the existing Protocol Buffers binary wire format, with PXF-specific annotations that constrain how it is produced and consumed), SBE (a fixed-layout binary encoding derived from FIX Simple Binary Encoding), and a response envelope. Collectively these are referred to as the protowire family. This document defines the wire surface sufficient for independent interoperable implementations. It does not define library APIs.

--- middle

# Introduction

Protocol Buffers {{PROTOBUF}} is widely deployed for binary serialization of structured data, but its standard text format ("text format", or "prototext") is targeted at debugging and does not address the requirements of human-edited configuration, API integration, or fixed-layout binary streaming. The protowire family covers those requirements while reusing Protocol Buffers schemas as the single source of truth for field identity, types, and numbering.

The family comprises four encodings:

PXF
: A UTF-8 text format with a small, regular grammar; intended for human authoring and machine consumption alike.

PB
: The existing Protocol Buffers binary wire format {{PROTOBUF-WIRE}}. This document does not redefine PB; it specifies the constraints that a protowire implementation applies on top of PB and the annotation extensions it uses.

SBE
: A fixed-layout binary encoding derived from FIX Simple Binary Encoding {{FIX-SBE}}, driven from Protocol Buffers schemas annotated with SBE template metadata.

Envelope
: A versioned response wrapper carrying transport status, application errors, field-level errors, and an opaque payload.

All four are driven from a single set of .proto schemas. PXF and SBE add field-level and message-level annotations expressed as Protocol Buffers extension options, defined in {{annotation-extensions}}.

Status of the underlying Protocol Buffers specification. Protocol Buffers does not currently have a finalized IETF standard. The canonical specification is published by Google at <https://protobuf.dev/> and, in practice, the protoc reference compiler acts as the de facto specification for behavior the published documentation does not pin down. An IETF effort is in progress to register Protocol Buffers media types {{I-D.ietf-dispatch-mime-protobuf}}; that draft registers both "application/protobuf" and "application/x-protobuf" (the latter reflecting historical deployment under the experimental "x-" prefix that predates RFC 6648), but does not redefine the wire format. {{media-type-registrations}} of this document registers a separate "application/protowire-pb" type that signals the additional conformance and annotation requirements specified here.

Relationship to ProtoJSON. Protocol Buffers also defines a JSON mapping commonly called "ProtoJSON" {{PROTOJSON}}, which serializes messages as JSON {{RFC8259}} objects, represents google.protobuf.Timestamp as an RFC 3339 {{RFC3339}} string, and represents google.protobuf.Duration as a decimal-seconds string with an "s" suffix. PXF ({{pxf-text-format}}) is intentionally distinct from ProtoJSON: PXF targets human authoring and review, supports comments, multi-line strings, and bare-identifier enum values, and uses an entry-list document shape rather than JSON's object-and-comma shape. Where the two formats overlap on semantics — most visibly the use of RFC 3339 for timestamps — PXF intentionally adopts the ProtoJSON convention to reduce the number of disjoint time formats a tooling chain has to support. Implementations are not required to provide ProtoJSON; this document does not specify it.

This document treats both {{PROTOBUF}} (the language and feature spec) and {{PROTOBUF-WIRE}} (the binary wire format) as normative references in their protobuf.dev form, and inherits whatever stability properties those documents and the protoc implementation provide. A future revision of this document SHOULD migrate to an IETF Protocol Buffers reference if and when one is published as an RFC.

## Scope

This document defines:

* the lexical and syntactic structure of PXF text ({{pxf-text-format}});

* the constraints that protowire applies to PB binary encoding ({{pb-binary-encoding}});

* the SBE wire framing used by protowire ({{sbe-binary-encoding}});

* the schema-level annotation extensions ({{annotation-extensions}});

* the response envelope ({{response-envelope}});

* the conformance requirements that any decoder operating on untrusted input MUST satisfy ({{decoder-conformance}}).

This document does not define library APIs, programming-language bindings, code generation strategies, or performance characteristics.

## Terminology

{::boilerplate bcp14-tagged}

The following terms are used throughout this document:

schema
: A Protocol Buffers FileDescriptorSet {{PROTOBUF}}, together with any annotation extensions defined in {{annotation-extensions}}.

port
: An independent implementation of the protowire encodings, typically targeting a single programming language.

document
: A complete PXF input: a sequence of bytes that satisfies the production "document" in {{abnf-grammar}}.

value
: A PXF construct that denotes a single scalar, list, or nested block; see {{abnf-grammar}}.

entry
: A key together with an assignment, map, or block tail; see {{entries-and-keys}}.

well-known type
: One of the Protocol Buffers types listed in Section 6 of {{PROTOBUF}} (Timestamp, Duration, the \*Value wrappers, etc.) plus the protowire-defined arbitrary-precision numeric types ({{pxf-annotations}}).

port-trusted bytes
: Bytes whose origin is the calling application; not under attacker control.

attacker-controlled bytes
: Bytes whose origin is, or may be, under the control of a party with a goal adverse to the host of the decoding process.

# The protowire Family

A protowire implementation accepts and emits four wire forms for any message defined in a schema: PXF text, PB binary, SBE binary (when the message carries SBE annotations), and Envelope binary (which is itself a PB message).

## Common Schema Layer

For any schema S, the four encodings represent the same logical value space. Implementations MUST produce wire output that round-trips through every encoding for which the schema is well defined; concretely, for a value v of type T defined in S:

* decode_PB(encode_PB(v))    == v
* decode_PXF(encode_PXF(v))  == v
* decode_SBE(encode_SBE(v))  == v   if T carries SBE annotations
* encode_PB(decode_PXF(t))   == encode_PB(v)
                                if t = encode_PXF(v)

Equality is defined per {{PROTOBUF}} Section "Message equality" (field-by-field, with proto3 default semantics applied).

## Wire Equivalence

Two ports are wire-equivalent for schema S if, for every value v of every message type in S, both ports produce byte-identical PB and Envelope output and parse each other's PXF and SBE output to equal values. Wire equivalence is the contract this document specifies; it is not an API-stability or performance contract.

# PXF Text Format {#pxf-text-format}

PXF is a UTF-8 text format. A PXF document is a sequence of entries denoting a single message value, optionally preceded by a type directive that identifies the schema type the document represents.

## Character Set

A PXF document is a sequence of bytes that, when interpreted as UTF-8 {{RFC3629}}, yields a sequence of Unicode scalar values.

* A document MUST be valid UTF-8.

* Decoders MUST reject documents containing invalid UTF-8 byte sequences with an error.

* Decoders MUST reject documents containing Unicode surrogate code points (U+D800 through U+DFFF) or code points above U+10FFFF, regardless of how the surrogate or out-of-range value is expressed (raw bytes that round-trip as such are already excluded by valid-UTF-8 conformance; \\uHHHH and \\UHHHHHHHH escapes producing those code points are excluded by {{string-literals}}).

* A leading UTF-8 byte order mark (U+FEFF, encoded as %xEF.BB.BF) MAY be present and MUST be ignored. Subsequent occurrences of U+FEFF are interpreted as ordinary characters and have no special meaning.

## Whitespace and Comments

The following Unicode scalar values are whitespace: U+0009 (HT), U+000A (LF), U+000D (CR), and U+0020 (SPACE). Whitespace separates tokens but is otherwise insignificant.

PXF supports two comment forms:

* Line comments begin with "#" or "//" and extend to (but do not include) the next U+000A.

* Block comments begin with "/\*" and end at the next occurrence of "\*/". Block comments do not nest.

Comments MAY appear anywhere whitespace MAY appear. A comment is treated as a single whitespace character for the purposes of tokenization.

## ABNF Grammar {#abnf-grammar}

The PXF surface grammar is given in ABNF {{RFC5234}} {{RFC7405}}. The grammar describes a token stream; whitespace and comments per {{whitespace-and-comments}} MAY appear between any two adjacent tokens and are not shown.

~~~ abnf
document        = *directive *field-entry

directive       = type-directive
                / dataset-directive
                / proto-directive
                / named-directive
type-directive  = %s"@type" 1*WSP identifier
named-directive = "@" directive-name
                  *( 1*WSP identifier )
                  [ block-tail ]
directive-name  = identifier ; excluding the spec-reserved names
                             ; "type", "dataset", "proto",
                             ; "entry", "table", "datasource",
                             ; "view", "procedure", "function",
                             ; "permissions", and the value
                             ; keywords "null", "true", "false"
                             ; ({{reserved-directive-names}}).

dataset-directive
                = %s"@dataset" [ 1*WSP identifier ]
                  1*WSP "(" column-list ")"
                  *( 1*WSP row )
column-list     = identifier *( "," identifier )
row             = "(" row-cell *( "," row-cell ) ")"
row-cell        = [ row-value ]
row-value       = string / number / bool / null / bytes
                / timestamp / duration / identifier
                ; cells exclude list and block-value
                ; ({{the-dataset-directive}})

proto-directive = %s"@proto" proto-body
proto-body      = proto-anon-body
                / proto-named-body
                / proto-source-body
                / proto-descr-body
proto-anon-body = 1*WSP "{" proto-message-body "}"
proto-named-body
                = 1*WSP identifier
                  1*WSP "{" proto-message-body "}"
proto-source-body
                = 1*WSP triple-string
proto-descr-body
                = 1*WSP bytes
proto-message-body
                = <protobuf message-body production
                   from the Protocol Buffers language>
                ; ({{the-proto-directive}})

entry           = field-entry / map-entry
field-entry     = identifier ( assignment-tail / block-tail )
map-entry       = map-key map-tail

assignment-tail = "=" value
map-tail        = ":" value
block-tail      = "{" *entry "}"

map-key         = identifier / string / integer

value           = string
                / number
                / bool
                / null
                / bytes
                / timestamp
                / duration
                / identifier
                / list
                / block-value

list            = "[" [ value *( [","] value ) ] "]"
block-value     = "{" *entry "}"

number          = float / integer
integer         = [ "-" ] 1*DIGIT
float           = [ "-" ] 1*DIGIT
                  ( "." *DIGIT [ exponent ] / exponent )
exponent        = ( "e" / "E" ) [ "+" / "-" ] 1*DIGIT

bool            = %s"true" / %s"false"
null            = %s"null"

identifier      = ident-start *ident-part
ident-start     = ALPHA / "_"
ident-part      = ALPHA / DIGIT / "_" / "."

string          = triple-string / simple-string

simple-string   = DQUOTE *( string-char / escape-seq ) DQUOTE
string-char     = %x20-21 / %x23-5B / %x5D-7F
                / utf8-non-ascii          ; LF, ", \ excluded

triple-string   = 3DQUOTE triple-content 3DQUOTE
triple-content  = *( %x00-21 / %x23-7F / utf8-non-ascii )
                  ; any UTF-8 sequence not containing 3DQUOTE

escape-seq      = "\" ( simple-escape
                      / hex-escape
                      / octal-escape
                      / unicode-4-escape
                      / unicode-8-escape )

simple-escape   = DQUOTE / "\" / "'" / "?"
                / %x61 / %x62 / %x66 / %x6E
                / %x72 / %x74 / %x76         ; a b f n r t v
hex-escape      = "x" 2HEXDIG
octal-escape    = oct-lead 2OCT-DIGIT       ; value <= 0xFF
unicode-4-escape = "u" 4HEXDIG
unicode-8-escape = "U" 8HEXDIG

bytes           = %x62 DQUOTE *base64-char DQUOTE   ; 'b' "..."
base64-char     = ALPHA / DIGIT / "+" / "/" / "="
timestamp       = date-time     ; per RFC 3339, Section 5.6

duration        = 1*duration-segment
duration-segment = 1*DIGIT [ "." 1*DIGIT ] time-unit
time-unit       = "ns" / "us" / micro-us / "ms"
                / "s" / "m" / "h"
micro-us        = %xC2.B5 %x73    ; UTF-8 of "µs"

OCT-DIGIT       = %x30-37
oct-lead        = %x30-33         ; ensures \NNN <= 0xFF
3DQUOTE         = DQUOTE DQUOTE DQUOTE
utf8-non-ascii  = <any UTF-8-encoded Unicode scalar value
                  in U+0080..U+10FFFF, excluding surrogates>
~~~

ABNF core productions ALPHA, DIGIT, HEXDIG, DQUOTE, and WSP are imported from {{RFC5234}} Appendix B.1. The case-sensitive string-prefix notation %s is from {{RFC7405}}; PXF identifiers, keywords ("true", "false", "null"), and the "@type" directive are case-sensitive.

The grammar is LL(1) modulo the lexical disambiguation rules in {{numeric-literals}} and {{timestamps-and-durations}}:

* An input that begins with four DIGIT followed by "-" is tokenized as a timestamp, not as a negative integer or a bare identifier-prefix.

* An input matching 1\*DIGIT \[ "." 1\*DIGIT \] time-unit, where the time-unit is one of the literal strings in the grammar above, is tokenized as a duration. An identifier whose initial characters happen to match a duration prefix (for example "5seconds") is tokenized as an identifier because identifier productions extend through ALPHA / "\_".

* Numeric literals take precedence over identifiers: a leading DIGIT or "-" forces the numeric branch.

## Documents and Directives

A PXF document represents a single message value. Before the body, the document MAY carry zero or more directives. A directive begins with "@" followed by a name. Four directive shapes are given semantics by this document: "@type" names the body's schema-typed message ({{the-type-directive}}); "@dataset" carries row-oriented bulk data ({{the-dataset-directive}}); "@proto" carries an embedded protobuf schema ({{the-proto-directive}}); and "@entry" bundles heterogeneous typed sub-messages ({{the-entry-directive}}) via the named-directive shape ({{named-directives}}). Directive names beyond the reserved set ({{reserved-directive-names}}) are application-extensible.

### The "@type" Directive {#the-type-directive}

The "@type" directive, of the form

~~~
@type identifier
~~~

names the fully-qualified message type. Decoders MAY require the type directive when the calling application has not pre-bound a target type; decoders MUST ignore the directive when a target type has been pre-bound and the directive matches it, and MUST reject when it does not match.

At most one "@type" directive MAY appear in a document.

### Named Directives {#named-directives}

A named directive has the form

~~~
@<name> *( <prefix-id> ) [ "{" inner "}" ]
~~~

where \<name\> is an identifier other than the spec-reserved names listed in {{reserved-directive-names}}; each \<prefix-id\> is an identifier (possibly dotted); and inner is a sequence of entries syntactically identical to the body of a message block.

The prefix-identifier list is positional; the count and meaning of the identifiers are per-directive-registration. The conventional shape used in the wild is a single prefix identifier naming the inner block's message type (e.g. chameleon's `@header chameleon.v1.LayerHeader { ... }` preamble, which carries per-file sanity-check fields the resolver enforces against its chain spec). {{the-entry-directive}} defines a second shape ("@entry") that uses two prefix identifiers.

Named directives carry side-channel metadata that the consumer's runtime interprets — never the body's schema layer. This document defines no \<name\> beyond those registered in subsections of {{documents-and-directives}}; specific names are reserved by other specifications or by applications.

Conformance:

* A decoder that does not recognize a directive name MAY skip the directive after parsing its prefix identifiers and block (if present) for syntactic well-formedness, or MAY reject the document. Implementations SHOULD document which policy they apply.

* A decoder MUST NOT attempt to interpret the inner block against the body's schema; the inner block is a self-contained sub-document and is decoded against its own type (if any) by the consumer.

* The inner block is parsed by the same entry grammar as a message body ({{abnf-grammar}}): the same brace-matching, string, and comment rules apply. Braces inside string literals or comments do not close the block.

* The number and interpretation of prefix identifiers are specified by each directive's registration. A decoder that recognizes the directive name MUST enforce the registered cardinality. A decoder that does not recognize the name MAY accept any cardinality (the grammar imposes no upper bound).

* Multiple named directives MAY appear; their relative order is preserved by decoders that expose a directive list to callers.

A document with no entries denotes the message-typed default value (all fields unset).

### The "@entry" Directive {#the-entry-directive}

The "@entry" directive is the canonical PXF shape for bundling heterogeneous, typed sub-messages alongside (or in place of) a document body. Its form is

~~~
@entry [ <name> ] [ <type> ] [ "{" inner "}" ]
~~~

where \<name\> is a caller-provided label identifier, \<type\> is a dotted identifier naming the inner block's message type, and inner is a sequence of entries decoded against \<type\>. Each prefix identifier is optional; permitted shapes are:

~~~
@entry { ... }                       ; anonymous, typeless
@entry name { ... }                  ; labeled, typeless
@entry name some.pkg.Type { ... }    ; labeled and typed
@entry some.pkg.Type { ... }         ; typed only (no label)
~~~

The fourth shape is disambiguated from the second by the presence of a "." in the single prefix identifier: a dotted identifier is interpreted as a type; an undotted identifier is interpreted as a label. Applications that need an undotted type identifier MUST use the third shape with an empty-string label or supply the label explicitly.

"@entry" is consumer-interpreted; this document defines no meaning for the label beyond its preservation in directive order. Typical uses include manifest documents, mixed-type exports, and any case where a document needs to carry several typed values that are not naturally a single repeated field.

Conformance:

* A decoder that recognizes "@entry" MUST accept zero, one, or two prefix identifiers and MUST reject three or more.

* A decoder MUST preserve "@entry" occurrences in document order when exposing them to callers. Duplicate labels MAY appear; this document does not impose a uniqueness rule.

* An "@entry" with no body block ("{}") denotes the default value of its declared \<type\>, or an empty anonymous sub-document if \<type\> is absent.

* "@entry" inherits the schema constraint of {{schema-constraints}}: \<name\> MUST NOT be case-sensitively equal to "null", "true", or "false"; \<type\> MUST refer to a non-conformant-name-free message type.

### The "@dataset" Directive {#the-dataset-directive}

The "@dataset" directive is the canonical PXF shape for representing many instances of a single message type in a single document — the protowire-native replacement for CSV. Its form is

~~~
@dataset <type> ( <col1> [ , <col2> ... ] )
( <val1> [ , <val2> ... ] )
( <val1> [ , <val2> ... ] )
...
~~~

where \<type\> is a dotted identifier naming the row message type; \<col1\>, \<col2\>, ... is the column list (at least one column); and each subsequent parenthesized tuple is a row whose values bind positionally to the columns.

Anonymous binding. \<type\> MAY be omitted when an anonymous "@proto" directive ({{the-proto-directive}}) precedes the "@dataset" in document order. The anonymous schema is consumed as the row message type and the header carries only the column list:

~~~
@dataset ( <col1> [ , <col2> ... ] )
( <val1> [ , <val2> ... ] )
...
~~~

The anonymous "@proto" binding is one-shot per {{the-proto-directive}}: a second untyped "@dataset" in the same document requires a second preceding anonymous "@proto".

Cell value grammar (v1). A cell value is drawn from the value production of {{abnf-grammar}} minus list and block-value: string, number, bool, null, bytes, timestamp, duration, or identifier. List literals ("\[ ... \]") and block values ("{ ... }") are NOT permitted in cells. This restriction keeps rows tokenizable with simple top-level comma splitting and reserves nested-cell semantics for a future revision.

Three-state cells. Each row cell denotes one of three states, carried into the bound message via the same semantics as the PXF annotations of {{pxf-annotations}}:

* An empty cell (no value between two commas, or leading / trailing inside a row, e.g. "(1, , 3)") denotes an absent field. When the bound field carries (pxf.default), the default literal is applied; when it carries (pxf.required), the document MUST be rejected.

* A bare "null" literal denotes a present-but-null field. This satisfies (pxf.required) and suppresses (pxf.default), with the singular-field nullability rules of {{booleans-null-and-identifier-values}} applied against \<type\>'s declared field.

* Any other value literal denotes a present field with that value.

Row arity. Each row MUST have the same number of cells as the header column list. Decoders MUST reject a row whose arity differs from the column count. Trailing-empty shorthand (a row with fewer cells implying absent tail columns) is not permitted in v1.

Column entries (v1). Each column entry MUST be an unqualified field name declared on \<type\>. Dotted paths (e.g. "addr.city") are NOT permitted in v1 and are reserved for a future revision. Decoders MUST reject a column entry containing a ".".

Standalone. A document containing a "@dataset" directive MUST NOT also contain a "@type" directive and MUST NOT contain top-level field entries ({{entries-and-keys}}). The "@dataset" header is itself the document's type declaration; the rows are the document's payload. A document MAY contain multiple "@dataset" directives (whether of the same or different row types); their relative order MUST be preserved by decoders that expose a dataset list to callers.

Consumer contract. "@dataset" is consumer-interpreted in the same side-channel manner as named directives ({{named-directives}}): the rows are exposed to callers through a parser API distinct from the body's schema layer. This document does not mandate a single canonical "decode-as-repeated-\<type\>" semantics; an application that wants that one-liner constructs it on top of the rows API. The schema layer of \<type\> MUST NOT receive the rows as message body entries.

Streaming consumption. Because "@dataset" is the canonical PXF replacement for CSV, implementations are expected to handle datasets whose row sequence does not fit in memory. Tools MAY expose a streaming row-iteration API alongside (or in place of) the materializing one. A streaming API MUST yield rows in source order, MUST enforce the per-row arity rule and the v1 cell-grammar rule on each row as it is consumed (not deferred to end-of-input), and SHOULD bound its working-set memory by the size of the largest single row plus a small bookkeeping overhead — not by the size of the row sequence. Streaming and materializing APIs that coexist in the same implementation MUST produce byte-identical row sequences for the same input.

Conformance:

* A decoder MUST reject a document where a "@dataset" directive coexists with a "@type" directive or with any top-level field-entry.

* A decoder MUST reject a row whose arity differs from the column count.

* A decoder MUST reject a column entry containing a ".".

* A decoder MUST reject a cell value that is a list literal or a block value.

* A decoder MUST resolve identifier-shaped cell values against the corresponding column field on \<type\>: an enum-typed column accepts an enum value name ({{booleans-null-and-identifier-values}}); a string-typed column accepts only quoted strings, not bare identifiers.

* "@dataset" inherits the schema constraint of {{schema-constraints}}: \<type\> MUST refer to a non-conformant-name-free message type, and each column entry MUST name a field whose name is itself non-conformant-name-free.

### The "@proto" Directive {#the-proto-directive}

The "@proto" directive carries an embedded protobuf schema, making the PXF document self-describing. Its form is

~~~
@proto <body>
~~~

where \<body\> is one of four shapes distinguished lexically:

~~~
@proto { <message-body> }                  ; anonymous
@proto <dotted-name> { <message-body> }    ; named
@proto """<proto-source>"""                ; source-form file
@proto b"<base64-FileDescriptorSet>"       ; descriptor form
~~~

Anonymous form. The body is a Protocol Buffers message-body production (a sequence of field declarations and nested types) with no surrounding `message Name { ... }` wrapper and no "package" statement. The anonymous message is consumed as the typed binding of the next directive in document order that requires one and does not carry an explicit type name (e.g. "@dataset" per {{the-dataset-directive}}). The binding is one-shot.

Named form. The dotted identifier names the message type: the dotted prefix is the package and the final component is the message name. The body is a Protocol Buffers message-body production, identical to the anonymous form. A document with multiple named "@proto" directives binds them into a single document-scoped schema; conflicting type definitions (two "@proto" directives sharing a fully-qualified name with diverging bodies) MUST be rejected.

Source form. The triple-quoted-string body is a complete Protocol Buffers source file with optional "syntax", "package", "import", and one or more "message" / "enum" declarations.

Descriptor form. The bytes-literal body is a base64-encoded google.protobuf.FileDescriptorSet (the pre-compiled form).

Imports. Source-form "@proto" directives that contain "import" statements MUST resolve those imports from one of:

* the bundled Protocol Buffers well-known types (e.g. "google/protobuf/timestamp.proto", "google/protobuf/duration.proto");

* the canonical PXF and SBE annotation schemas ("pxf/annotations.proto", "sbe/annotations.proto");

* the canonical envelope ("envelope/v1/envelope.proto");

* another "@proto" directive in the same document.

Imports from the filesystem or a remote schema registry are NOT permitted from inside a source-form "@proto" body. An import that does not resolve to one of the four sources above MUST be rejected.

Precedence with external schemas. When a decoder is supplied with an external schema (via a tool's `-p` flag or a pre-bound descriptor) and the document also carries an "@proto" directive, the embedded schema is authoritative. Decoders MAY verify that the external schema agrees with the embedded one by comparing the normalized FileDescriptorSet bytes of each; on disagreement, decoders MUST reject the document.

Conformance:

* A decoder MUST support the descriptor form. Decoders MAY support the source, named, and anonymous forms. A decoder that does not support a body shape encountered in a document MUST report an error indicating which shape was rejected; pre-compilation by a separate tool (e.g. `protowire compile`) is the recommended fallback.

* A decoder MUST reject an anonymous "@proto" directive that is not consumed by a subsequent typed directive in the same document.

* A decoder MUST reject duplicate fully-qualified type definitions across multiple "@proto" directives in the same document when the definitions diverge.

* A decoder MUST reject an "import" inside a source-form "@proto" body that does not resolve to one of the four permitted sources listed above.

* "@proto" inherits the schema constraint of {{schema-constraints}}: the embedded schema MUST NOT declare a message field, oneof, or enum value whose name case-sensitively equals "null", "true", or "false".

### Reserved Directive Names {#reserved-directive-names}

The directive-name production ({{abnf-grammar}}) excludes a set of identifiers reserved by this document. Applications MUST NOT use any of these names for their own named directives ({{named-directives}}). The reserved set is:

* "type" — names the body's message type ({{the-type-directive}}).

* "dataset" — row-oriented bulk data ({{the-dataset-directive}}).

* "proto" — embedded protobuf schema ({{the-proto-directive}}).

* "entry" — bundle/manifest sub-messages ({{the-entry-directive}}).

* "table" — reserved for a future revision describing the storage shape of tabular data (column types, primary keys, indexes, partitioning, constraints). v1 decoders MUST reject "@table" as an unknown reserved directive.

* "datasource", "view", "procedure", "function", "permissions" — reserved for a future revision describing database export/import metadata (data origin, view definitions, stored procedures and functions, access control). v1 decoders MUST reject these as unknown reserved directives.

* "null", "true", "false" — value keywords ({{booleans-null-and-identifier-values}}); excluded to keep the directive-name lexer unambiguous.

The check is case-sensitive: directive names such as "@TABLE", "@Proto", "@DATASOURCE" lex as ordinary application directives ({{named-directives}}) and are accepted, subject to any application's own naming rules.

Future revisions of this document MAY allocate semantics to the future-reserved names ("table", "datasource", "view", "procedure", "function", "permissions") and MAY add new names to the reserved set. Allocating semantics to a name that was already reserved is not a breaking change because the name was already rejected by v1 decoders; adding a new reservation IS a breaking change and requires a major-version bump.

## Entries and Keys {#entries-and-keys}

An entry binds a key to a value. The key is a field name within the surrounding message type, with the following rules:

* An identifier key matches a field by its proto field name (the lowerCamelCase or snake_case name in the schema, as written; both forms are accepted, and emitters SHOULD use whichever the schema declares).

* A string or integer key is permitted only inside a map\<K,V\> literal (i.e. the *map-entry* production of {{abnf-grammar}}). A string key MUST be a UTF-8 string; for map\<K,V\> fields with non-string K, the string is parsed as a literal of K's type. An integer key matches a map\<K,V\> field whose K is one of the protobuf scalar integer types (int32, int64, sint32, sint64, uint32, uint64, fixed32, fixed64, sfixed32, sfixed64, bool encoded as 0/1).

The three entry tails are NOT interchangeable; the grammar in {{abnf-grammar}} splits them across two productions:

* An assignment-tail "=" binds a scalar, list, or block to a field of an enclosing *message* type. It is the right-hand side of *field-entry* and REQUIRES an identifier key (= proto field name). Parsers MUST reject "=" with a non-identifier key (string or integer) at parse time.

* A map-tail ":" binds a value to a key of an enclosing *map* type. It is the right-hand side of *map-entry*. *map-entry* MUST NOT appear at document top level (the document represents a proto message, never a map\<K,V\>); parsers MUST reject ":" at the top level with an error indicating that field assignments use "=".

* A block-tail "{ ... }" with no preceding "=" or ":" is permitted only in message context, where the bound field is message-typed; it is equivalent to "= { ... }". Like assignment-tail, it REQUIRES an identifier key, and parsers MUST reject a block-tail with a non-identifier key. Map values that are themselves messages MUST use the explicit form "key: { ... }"; the bare-block form is not accepted in map context.

Inside a "{ ... }" block the parser cannot statically tell whether the surrounding field is message-typed or map-typed; both *field-entry* and *map-entry* are accepted in that position and the message-vs-map disambiguation is performed by the schema-resolution step that runs after parsing.

Repeated fields MAY be expressed either as a single key with a list-typed value, or as multiple entries with the same key and scalar (or block) values; the two forms denote the same field value, with elements concatenated in document order.

## String Literals {#string-literals}

PXF supports two string forms. Both denote sequences of Unicode scalar values.

Simple strings use double-quote delimiters and recognize escape sequences:

~~~
"Hello, world\n"
~~~

Within a simple-string, U+000A (LF) MUST NOT appear unescaped: line continuations are not supported. Decoders MUST reject a simple-string containing a literal LF.

The defined simple escapes are:

~~~
\"      U+0022 QUOTATION MARK
\\      U+005C REVERSE SOLIDUS
\'      U+0027 APOSTROPHE
\?      U+003F QUESTION MARK
\a      U+0007 BELL
\b      U+0008 BACKSPACE
\f      U+000C FORM FEED
\n      U+000A LINE FEED
\r      U+000D CARRIAGE RETURN
\t      U+0009 CHARACTER TABULATION
\v      U+000B LINE TABULATION
~~~

Numeric escapes:

~~~
\xHH        two hex digits, denotes the byte 0xHH
\NNN        three octal digits, value MUST be <= 0xFF;
            denotes the byte 0xNN
\uHHHH      four hex digits, denotes Unicode scalar U+HHHH
\UHHHHHHHH  eight hex digits, denotes Unicode scalar
            U+HHHHHHHH
~~~

The \\uHHHH and \\UHHHHHHHH forms MUST denote a Unicode scalar value: the code point MUST be in U+0000..U+10FFFF and MUST NOT be a surrogate (U+D800..U+DFFF). Decoders MUST reject otherwise.

The interpretation of \\xHH and octal escapes depends on the target field type:

* When the surrounding string literal is bound to a proto3 string-typed field, the result of escape expansion MUST be valid UTF-8. Decoders MUST reject a string-typed value whose escape-expanded byte sequence is not valid UTF-8 ({{utf-8-enforcement}}).

* When the surrounding string literal is bound to a proto3 bytes-typed field, no UTF-8 constraint applies.

Triple-quoted strings ("""...""") begin with three U+0022 characters and end at the next occurrence of three consecutive U+0022 characters. Inside a triple-quoted string:

* Escape sequences are NOT interpreted; backslashes are literal.

* If the byte immediately following the opening """ is U+000A, that LF MUST be removed by the decoder.

* After leading-LF stripping, if every non-empty line preceding the closing """ shares a common leading-whitespace prefix, that prefix MUST be removed from each such line. The "leading whitespace" is the longest sequence of U+0020 and U+0009 characters at the start of a line; lines that consist exclusively of whitespace do not constrain the prefix.

The output of triple-quote processing MUST be valid UTF-8 when bound to a string-typed field; the same UTF-8 conformance rule in {{utf-8-enforcement}} applies.

## Bytes Literals {#bytes-literals}

A bytes literal is the lowercase letter "b" immediately followed by a double-quoted body containing only base64 characters {{RFC4648}}:

~~~
b"SGVsbG8sIHdvcmxkIQ=="
~~~

Decoders MUST accept both the standard base64 alphabet and the URL-safe alphabet {{RFC4648}} Section 5; padding ("=") is OPTIONAL and MUST be tolerated whether present or absent. Decoders MUST reject input containing characters outside both alphabets and "=". Whitespace is NOT permitted inside the body.

Backslashes inside `b"..."` are NOT interpreted as escape introducers.

## Numeric Literals {#numeric-literals}

Integers are sequences of decimal digits, optionally preceded by "-". Hexadecimal, octal, and binary integer literals are NOT defined in this version.

Floats are decimal-point or exponent forms; see the ABNF in {{abnf-grammar}}. The literal "1." is a valid float (integer part 1, empty fractional part); the literal ".5" is NOT (no integer part). This restriction is intentional: an unprefixed "." is reserved for future qualified-key syntax.

The target field type determines the numeric domain:

* For fixed-width integer fields (int32, int64, uint32, uint64, sint32, sint64, fixed32, fixed64, sfixed32, sfixed64), decoders MUST reject literals whose value falls outside the field's representable range.

* For float and double fields, decoders MUST accept literals in the IEEE 754 {{IEEE754}} representable range; values that round to +Inf or -Inf are rejected unless explicitly written as the identifiers "inf", "+inf", "-inf", "nan".

* For fields of the well-known types pxf.BigInt, pxf.Decimal, and pxf.BigFloat ({{pxf-annotations}}), decoders MUST preserve the literal's exact value, including, for Decimal, the exact scale: the literal "1.00" decodes to a Decimal with unscaled value 100 and scale 2, distinct from "1.0" or "1".

The number of digits in any single numeric literal is bounded; see {{mandatory-limits}}.

## Booleans, Null, and Identifier Values {#booleans-null-and-identifier-values}

The literals "true" and "false" denote the boolean values; "null" denotes the null value. All three are case-sensitive. A "null" literal:

* When bound to a singular message-typed field, MUST clear the field.

* When bound to a singular scalar field, MUST be rejected unless the schema marks the field as nullable via a wrapper type (e.g. google.protobuf.StringValue) or via a future annotation.

* When bound to a list-typed (repeated) field, MUST be rejected: list elements are not nullable.

An identifier appearing as a value denotes an enum value name when the target field is enum-typed. Decoders MUST reject unknown enum names unless the schema declares the field with open enum semantics, in which case the unknown name is preserved in the message's unknown-fields set.

Because "true", "false", and "null" are reserved value keywords, a protobuf schema MUST NOT declare an enum value, message field, or oneof bearing any of those names; see {{schema-constraints}}.

## Timestamps and Durations {#timestamps-and-durations}

A timestamp literal is an RFC 3339 {{RFC3339}} date-time production. The lexer recognizes a timestamp by lookahead: an input matching the regular expression `/^[0-9]{4}-/` at a position where a value is expected MUST be tokenized as a timestamp. This rule disambiguates against the identifier and integer productions.

The choice of RFC 3339 aligns with the ProtoJSON {{PROTOJSON}} serialization of google.protobuf.Timestamp, which uses the same format. A PXF timestamp literal and the ProtoJSON string form of the same Timestamp value are byte-identical except for the surrounding ProtoJSON quote characters. PXF duration literals are NOT byte-compatible with ProtoJSON's Duration form (a decimal-seconds string with an "s" suffix); PXF retains the multi-segment unit form ("1h30m500ms") because it is materially easier to read in human-authored documents.

Decoders MUST accept timestamps with arbitrary fractional-second precision up to nanoseconds. Decoders that target a fixed-precision representation (typically google.protobuf.Timestamp, which has nanosecond resolution) MUST reject literals exceeding that precision rather than silently truncating.

A duration literal is a sequence of one or more segments, each consisting of a numeric magnitude followed by a unit suffix:

~~~
30s          ; 30 seconds
1h30m        ; 1 hour 30 minutes
500ms        ; 500 milliseconds
1.5h         ; 1.5 hours
2µs          ; 2 microseconds (alternative form: 2us)
~~~

The defined units are "ns", "us", "µs", "ms", "s", "m", and "h", denoting nanoseconds, microseconds (twice; "us" and "µs" are semantic equivalents), milliseconds, seconds, minutes, and hours respectively. Day, week, month, and year units are NOT defined and MUST NOT be inferred.

## Lists

A list value is a sequence of values delimited by "\[" and "\]". Elements MAY be separated by "," or by intervening whitespace alone (including newlines), or by both:

~~~
[1, 2, 3]
[1 2 3]
[
  "alpha"
  "beta"
]
~~~

A trailing comma is permitted.

List values bind only to repeated fields, to fields whose Protocol Buffers type is a list-shaped well-known type, and to list elements within other lists.

## Blocks and Map Tails

A block value "{ entry-list }" denotes a nested message when the surrounding context is message-typed, and a map literal when the surrounding context is map-typed. Within the block, keys are interpreted against the nested message's or map's schema. Blocks nest to arbitrary depth subject to the limit in {{mandatory-limits}}.

A nested message field is bound with "=" (or with the bare-block form, which is its abbreviation):

~~~
address = { city = "Berlin", zip = "10115" }
address   { city = "Berlin", zip = "10115" }   ; same value
~~~

A map field is bound with "=" between the field name and the block, but the entries inside the block use ":" because each entry is a key-of-the-map-to-value pair:

~~~
headers = {
  "X-Request-ID":  "abc123"
  "Content-Type":  "application/pxf"
}
~~~

Decoders MUST reject "=" inside a map block and ":" inside a message block ({{entries-and-keys}}).

Within either kind of block, entries MAY be separated by ";" or by newlines or by both; this is consistent with the ABNF in {{abnf-grammar}}, where entries are juxtaposed without an explicit separator and the optional ";" is consumed as whitespace.

## Schema Constraints {#schema-constraints}

The PXF lexer reserves "true", "false", and "null" as value keywords ({{booleans-null-and-identifier-values}}), and "type" as the type-directive name ({{the-type-directive}}). A protobuf schema whose declared names collide with any of these keywords produces fields, oneofs, enum values, or named-directive registrations that are unreachable from PXF surface syntax: the keyword wins in the tokenizer, so an assignment such as "field = null" always resolves to the null-literal branch, and an enum value literally named "null" can never be selected by name. PXF therefore constrains schemas as follows.

A protobuf schema bound for PXF use MUST NOT declare any of the following with a name case-sensitively equal to "null", "true", or "false":

* a message field (any field of any message);

* a oneof (the oneof identifier itself, not its member fields, which are governed by the message-field rule above);

* an enum value.

In addition, a named-directive registration ({{named-directives}}) MUST NOT use a name listed in the reserved-directive-name set of {{reserved-directive-names}}. This is already enforced by the directive-name production of {{abnf-grammar}}.

The check is case-sensitive. Names such as "NULL", "True", or "FALSE" lex as ordinary identifiers and are unaffected. Names that begin with a digit (e.g. "1null") are already forbidden by the protobuf identifier rule and need no PXF-specific restriction.

Conformance:

* Tools that bind a protobuf descriptor for PXF use (encoders, decoders, codegen plugins, schema linters, schema registries) MUST reject a non-conforming schema at bind time. A conforming decoder MUST NOT silently produce bindings in which a declared enum value, field, or oneof is unreachable from PXF surface syntax.

* Implementations SHOULD report the offending element by its fully-qualified protobuf name (e.g. "trades.v1.Side.null") and SHOULD point at this section.

* Schemas that violate this constraint were never round-trippable through PXF. Rejecting them on upgrade surfaces a pre-existing latent bug rather than introducing one; this document therefore defines no migration accommodation.

# PB Binary Encoding {#pb-binary-encoding}

The protowire PB encoding is the Protocol Buffers binary wire format {{PROTOBUF-WIRE}} with no protowire-specific changes to the wire grammar. This document does not redefine PB.

Constraints applied by protowire:

* Annotation extensions defined in {{annotation-extensions}} are encoded as Protocol Buffers field options on the relevant FieldOptions, MessageOptions, and FileOptions messages. Their field numbers are reserved ({{annotation-field-number-range}}).

* Decoders MUST enforce the limits in {{mandatory-limits}} on PB inputs. In particular, the depth limit applies to nested submessages, groups, and map entries; the length-prefix limit MUST be enforced before allocation.

* A protowire emitter SHOULD produce fields in field-number order; decoders MUST accept any field order, per the existing PB rules.

# SBE Binary Encoding {#sbe-binary-encoding}

The protowire SBE encoding is a subset of FIX Simple Binary Encoding {{FIX-SBE}}. An SBE message is generated for any Protocol Buffers message annotated with sbe.template_id; the schema-level identifiers come from sbe.schema_id and sbe.version on the file ({{sbe-annotations}}).

Field-level annotations sbe.length and sbe.encoding control fixed-byte-length string and bytes fields and primitive-type narrowing respectively ({{sbe-annotations}}).

## Header and Block Length

Every SBE message on the wire is preceded by an 8-byte message-header carrying the schema's blockLength, templateId, schemaId, and version, in little-endian byte order. Decoders MUST validate, before reading any field of the body:

1. data.length \>= HEADER_SIZE + wire_block_length

2. wire_block_length \>= template_block_length, where template_block_length is the value the decoder's compiled schema specifies for this template. A wire block strictly smaller than the template block length MUST be rejected. A wire block strictly larger MUST be accepted (forward compatibility for additive schema evolution).

## Repeating Groups

Each repeating group is preceded by a group-header carrying blockLength and numInGroup. Before iterating any group, decoders MUST validate:

1. pos + GROUP_HEADER_SIZE \<= data.length

2. count multiplied by wire_block_length does not overflow 64-bit unsigned arithmetic

3. pos + GROUP_HEADER_SIZE + count \* wire_block_length \<= data.length

4. If count \> 0, then wire_block_length \> 0. A wire_block_length of 0 with a non-zero count MUST be rejected before any per-element allocation.

## Variable-Length Data

Variable-length data ("varData") fields follow the fixed block, each preceded by a length prefix per the schema's varDataEncoding. Decoders MUST validate that pos + LENGTH_PREFIX_SIZE + length \<= data.length before reading the data.

# Annotation Extensions {#annotation-extensions}

protowire defines extensions on Protocol Buffers FileOptions, MessageOptions, and FieldOptions. Field numbers are allocated from the reserved range described in {{annotation-field-number-range}}.

## PXF Annotations {#pxf-annotations}

The PXF annotations apply to FieldOptions:

~~~
extend google.protobuf.FieldOptions {
  bool   required = 50000;
  string default  = 50001;
}
~~~

pxf.required: when true, decoders MUST reject a PXF document in which the annotated field is absent. A field bound to "null" is considered present for the purpose of this check; null-rejection for non-nullable types is governed by {{booleans-null-and-identifier-values}}.

pxf.default: when set, the value is a PXF literal (parsed by the same rules as a value in any entry). Decoders MUST treat an absent annotated field as if the document had supplied the default literal. The default applies only to absent fields, not to fields explicitly set to "null" or to the proto3 zero value.

The schema-level constraints of {{schema-constraints}} apply independently of these annotations: a field, oneof, or enum value whose name collides with a reserved PXF keyword is non-conforming regardless of whether pxf.required or pxf.default is set.

The well-known types pxf.BigInt, pxf.Decimal, and pxf.BigFloat are defined in {{PROTOWIRE-BIGNUM}} (this is the proto/pxf/bignum.proto file in the canonical repository). They provide arbitrary-precision signed integer, exact decimal, and binary floating-point representations respectively, encoded over PB as length-delimited unsigned big-endian magnitudes plus sign and scale fields.

## SBE Annotations {#sbe-annotations}

SBE annotations apply at three scopes:

~~~
extend google.protobuf.FileOptions {
  uint32 schema_id = 50100;
  uint32 version   = 50101;
}

extend google.protobuf.MessageOptions {
  uint32 template_id = 50200;
}

extend google.protobuf.FieldOptions {
  uint32 length   = 50300;
  string encoding = 50301;
}
~~~

sbe.schema_id and sbe.version identify the schema as a whole. sbe.template_id MUST be unique among messages within a schema.

sbe.length specifies a fixed byte length for string- or bytes-typed fields. Values longer than the limit MUST be truncated by emitters; values shorter MUST be padded with U+0000 bytes; decoders MUST trim trailing U+0000 bytes when populating string fields and MUST NOT trim them when populating bytes fields.

sbe.encoding narrows a Protocol Buffers numeric type to a smaller SBE primitive. The defined values are: "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "float", "double". Emitters MUST reject values that fall outside the narrowed type's range; decoders MUST sign-extend or zero-extend per the SBE primitive when populating the wider Protocol Buffers field.

# Response Envelope {#response-envelope}

The Envelope message provides a uniform response carrier across wire formats. Its schema is defined in package envelope.v1:

~~~
message Envelope {
  int32  status          = 1;
  string transport_error = 2;
  bytes  data            = 3;
  AppError error         = 4;
}

message AppError {
  string code               = 1;
  string message            = 2;
  repeated string args      = 3;
  repeated FieldError details = 4;
  map<string,string> metadata = 5;
}

message FieldError {
  string field   = 1;
  string code    = 2;
  string message = 3;
  repeated string args = 4;
}
~~~

Semantics:

* status carries an HTTP {{RFC9110}} or gRPC {{GRPC}} status code.

* transport_error is set when no application-layer response was produced (network error, timeout, connection refused). Implementations MUST NOT set transport_error and error simultaneously.

* data is the success payload, encoded in whichever wire form the surrounding transport selects (PXF, PB, JSON). Decoders MUST treat data as opaque bytes; it is parsed only after the envelope itself is parsed.

* error.code and details\[\*\].code are machine-readable identifiers; clients perform localization by consulting a string table keyed by "error.\<code\>" and "field.\<code\>" and substituting args positionally.

The package path "envelope.v1" is part of the wire contract. Incompatible changes MUST bump the package to "envelope.v2"; "v1" and "v2" MAY coexist indefinitely.

# Decoder Conformance {#decoder-conformance}

This section specifies the requirements that any protowire decoder operating on attacker-controlled bytes MUST satisfy.

The threat model assumes:

* the attacker controls every byte of the input,

* the attacker controls the input length, up to a configured maximum,

* the attacker MAY submit many inputs concurrently from many sources,

* the schema (.proto descriptor, SBE template) and the configured limits are NOT attacker-controlled.

Under these conditions, a conforming decoder MUST terminate in time and memory bounded by the input length and the configured limits only. It MUST NOT crash, abort, or unwind the host process; it MUST NOT allocate beyond the configured budget; it MUST NOT return string-typed fields whose contents are not valid UTF-8.

## Mandatory Limits {#mandatory-limits}

Conforming decoders MUST enforce the following limits. Defaults are recommended; the values are configurable per call by the calling application except where noted.

| Limit | Default | Applies to |
|-------|---------|------------|
| MaxNestingDepth | 100 | PXF block/list nesting; PB submessage / group / map-entry nesting |
| MaxMessageSize | 67108864 (64 MiB) | Total input length to a single decode call |
| MaxNumericLiteralDigits | 4096 | Digit count of any PXF numeric literal |
| MaxBytesLiteralLength | = MaxMessageSize | Decoded byte length of any PXF b"..." literal |
| MaxVarintBytes | 10 (fixed) | Every PB varint read. NOT configurable. |
| MaxRepeatedCount | = MaxMessageSize | Element count of any repeated, map, or SBE group field |

A decoder presented with input that requires exceeding any limit MUST return an error before allocating memory proportional to the violating quantity. It MUST NOT abort, panic, raise an uncatchable exception, or unwind into a state from which the caller cannot recover.

Length arithmetic MUST be performed in 64-bit unsigned arithmetic and checked against the buffer length before any narrowing conversion to a native integer width and before any allocation.

## Recursion

PXF parsers descend recursively into "{...}" blocks and "\[...\]" lists; PB parsers descend into submessages, groups, and map entries. Conforming decoders MUST:

1. maintain a depth counter incremented at every recursive descent;

2. reject input that would cause the counter to exceed MaxNestingDepth;

3. thread the depth counter through inner decoder instances constructed mid-stream. In particular, when a nested submessage is decoded by handing its bytes to a freshly constructed input-stream object, the depth counter MUST be passed in rather than reset to zero.

## UTF-8 Enforcement {#utf-8-enforcement}

Proto3 string fields are sequences of valid UTF-8 {{RFC3629}}. Conforming decoders:

* MUST validate UTF-8 strictly when populating any string-typed field, regardless of the source encoding (PB length-delimited bytes, SBE char-array fields, SBE varData, PXF simple-string or triple-string).

* MUST NOT use UTF-8 decoders that substitute U+FFFD for invalid sequences when populating string-typed fields.

* MUST reject PXF \\xHH and \\NNN (octal) escapes that produce invalid UTF-8 when the surrounding literal is bound to a string-typed field. The same byte sequences are permitted inside `b"..."` (bytes literal) and inside string literals bound to bytes-typed fields.

* MUST reject PXF \\uHHHH and \\UHHHHHHHH escapes that encode a surrogate code point or a code point above U+10FFFF.

## SBE Bounds Checking

The bounds-checking obligations in {{sbe-binary-encoding}} are conformance requirements, restated here for emphasis. Specifically:

* Wire block length less than template block length MUST be rejected ({{header-and-block-length}}).

* Group count multiplied by group block length MUST be 64-bit checked against the buffer length before iteration ({{repeating-groups}}).

* A group with count \> 0 and wire_block_length == 0 MUST be rejected before any per-element allocation ({{repeating-groups}}).

## Map Keys

In implementation languages where dynamic property assignment walks a prototype chain (notably JavaScript / TypeScript), a conforming decoder MUST NOT use a plain object literal as the container for attacker-keyed maps. Such decoders MUST use a prototype-free object (Object.create(null)) or a Map, or MUST explicitly reject the keys "\_\_proto\_\_", "constructor", and "prototype". The same obligation applies in any other implementation language that exhibits prototype-mutation semantics for reserved string keys.

# Media Types

This document defines the following media types.

application/pxf
: PXF text format ({{pxf-text-format}}). The schema-type association is carried either by the document's @type directive or out-of-band (e.g. an HTTP Content-Schema parameter). Charset: UTF-8 (fixed; the format MUST be UTF-8 per {{character-set}}).

application/protowire-pb
: Protocol Buffers binary, with the protowire constraints ({{pb-binary-encoding}}).

application/protowire-sbe
: SBE binary, with the protowire constraints ({{sbe-binary-encoding}}).

application/protowire-envelope
: The Envelope message ({{response-envelope}}), encoded as Protocol Buffers binary. The data field's content type is carried in the envelope-data-type parameter of the media type or, for transports that lack media-type parameters, in a transport-level header.

# IANA Considerations

## Media Type Registrations {#media-type-registrations}

Relationship to "application/protobuf" and "application/x-protobuf". Prior to {{I-D.ietf-dispatch-mime-protobuf}}, no IETF-registered media type existed for Protocol Buffers binary, and deployments converged informally on "application/protobuf" and (less preferably, per {{RFC6648}}) "application/x-protobuf". The dispatch draft registers both. Neither carries protowire's additional decoder-conformance and annotation-extension constraints. This document registers "application/protowire-pb" as a distinct type rather than layering a parameter on "application/protobuf" because (a) the conformance requirements in {{decoder-conformance}} are mandatory for protowire payloads and (b) protowire payloads are tied to a schema that uses the annotation extensions in {{annotation-extensions}}. A recipient that handles "application/protobuf" but not "application/protowire-pb" will, in the absence of those annotations and limits, parse the bytes correctly but will not provide the protowire conformance guarantees. Servers negotiating with a client that advertises only "application/protobuf" SHOULD downgrade to that media type and accept the loss of protowire-specific guarantees rather than refuse the request.

IANA is requested to register the following media types in the "Media Types" registry {{RFC6838}}:

~~~
Type name:        application
Subtype name:     pxf
Required parameters: none
Optional parameters: charset (fixed value: utf-8)
Encoding considerations: 8bit; UTF-8 text.
Security considerations: See Section 11 of this document.
Interoperability considerations: See Section 8.
Published specification: This document.
Applications that use this media type: Configuration
    tooling, API integration, schema-driven editors.
Fragment identifier considerations: none defined.
Author/Change controller: IETF.
Provisional registration: yes.

Type name:        application
Subtype name:     protowire-pb
Required parameters: none
Optional parameters: schema (URI of the FileDescriptorSet)
Encoding considerations: binary.
Security considerations: See Section 11.
Interoperability considerations: See Section 8.
Published specification: This document.
Applications that use this media type: API integration.
Fragment identifier considerations: none defined.
Author/Change controller: IETF.
Provisional registration: yes.

Type name:        application
Subtype name:     protowire-sbe
Required parameters: none
Optional parameters: schema (URI of the SBE schema XML)
Encoding considerations: binary.
Security considerations: See Section 11.
Interoperability considerations: See Section 8.
Published specification: This document.
Applications that use this media type: Low-latency message
    streaming, market-data fan-out.
Fragment identifier considerations: none defined.
Author/Change controller: IETF.
Provisional registration: yes.

Type name:        application
Subtype name:     protowire-envelope
Required parameters: none
Optional parameters: envelope-data-type (media type of the
    "data" field's contents).
Encoding considerations: binary.
Security considerations: See Section 11.
Interoperability considerations: See Section 8.
Published specification: This document.
Applications that use this media type: API integration.
Fragment identifier considerations: none defined.
Author/Change controller: IETF.
Provisional registration: yes.
~~~

## Annotation Field Number Range {#annotation-field-number-range}

This document allocates Protocol Buffers extension field numbers in the range 50000-59999 to the protowire family. Field numbers in this range are reserved for the extensions defined in {{annotation-extensions}} and for future extensions of this document; they MUST NOT be reused for unrelated extensions. The currently assigned numbers are:

~~~
pxf.required          50000
pxf.default           50001
sbe.schema_id         50100
sbe.version           50101
sbe.template_id       50200
sbe.length            50300
sbe.encoding          50301
~~~

Future protowire extensions SHOULD allocate within this range and document the assignment in a successor of this document.

# Security Considerations

The protowire family is designed to be parsed safely on attacker-controlled bytes. {{decoder-conformance}} specifies the conformance requirements that follow from this objective. This section addresses the considerations that do not reduce to a single conformance requirement.

Resource exhaustion. Without the limits in {{mandatory-limits}}, every protowire encoding admits trivial denial-of-service inputs: deeply nested PXF blocks blow native call stacks; large PB length prefixes drive allocator pressure; SBE group counts multiplied by element sizes overflow length arithmetic; long PXF numeric literals drive quadratic big-number parsers. Implementers MUST apply {{mandatory-limits}}.

Length-arithmetic overflow. All offset, length, and count arithmetic on attacker-supplied quantities MUST use 64-bit unsigned operations and MUST be checked against the input length before any narrowing. Implementations in languages whose default integer width on the host platform is 32 bits MUST be particularly careful to use explicit 64-bit types.

Trapping conversions. Several implementation languages provide integer conversions that abort the process on out-of-range input (Swift Int(_:), Rust "as" without checked_\*, Java Math.toIntExact, C++ static_cast with subsequent UB). When converting attacker-supplied lengths or counts to native integer widths, implementations MUST use fallible conversion forms.

UTF-8 substitution. Many platform string APIs silently substitute U+FFFD for invalid UTF-8. When such substitution is applied to a string-typed field, the resulting message violates the proto3 invariant that string fields contain valid UTF-8 and may differ from the producer's intent. Implementations MUST use strict, error-returning UTF-8 decoders on string fields ({{utf-8-enforcement}}).

Schema input. An implementation that accepts a FileDescriptorSet or SBE schema XML at runtime, where the descriptor or XML is, or may be, attacker-controlled, MUST apply the limits of {{mandatory-limits}} to the schema parser as well. XML schema parsers MUST disable DTDs and external entities to mitigate XXE attacks (e.g. defusedxml {{DEFUSEDXML}} in Python; XmlReaderSettings.DtdProcessing = Prohibit in .NET; the feature flags "disallow-doctype-decl", "external-general-entities", and "external-parameter-entities" in Java).

Prototype pollution. Decoders implemented in JavaScript, TypeScript, or any language with prototype-mutation semantics for reserved string keys MUST avoid plain object literals as the storage for attacker-keyed maps; see {{map-keys}}.

Cryptographic transport. This document specifies an encoding family. It does not provide confidentiality, integrity, or origin authentication; transports carrying protowire payloads SHOULD use TLS {{RFC8446}} or an equivalent.

Application-level error metadata. AppError.metadata ({{response-envelope}}) is a free-form string-to-string map. Servers SHOULD NOT place sensitive information (credentials, raw user input) in metadata. Clients SHOULD treat metadata values as untrusted and apply context-appropriate sanitization before display or logging.
