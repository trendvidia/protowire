---
v: 3
docname: draft-trendvidia-protowire-01
title: The Proto eXpressive Format (PXF) and the protowire Encoding Family
abbrev: protowire
category: info
date: 2026-06-05
ipr: trust200902
stream: IETF
area: Applications
workgroup: Network Working Group
keyword:
 - protobuf
 - serialization
 - text format
 - sbe
 - validation
 - schema
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
  PROTOWIRE-RFC-001:
    title: "RFC-001 — Protowire Schema Extensions"
    author:
      - org: TrendVidia LLC
    target: https://github.com/trendvidia/protowire/blob/main/docs/RFC-001-schema-extensions.md
  CEL:
    title: "Common Expression Language"
    author:
      - org: Google
    target: https://github.com/google/cel-spec
  STARLARK:
    title: "Starlark Language"
    author:
      - org: Bazel Authors
    target: https://github.com/bazelbuild/starlark
---

# Editor's Note (non-normative)

This revision of `draft-trendvidia-protowire` updates the document to
include the v1.2 **Protowire Schema Extensions** — three new top-level
declarations (`type`, `function`, `annotation`), a unified
`@annotation(...)` use-site syntax, and a structured validation error
model. The additions are strictly additive: every schema valid under
`-00` (corresponding to protowire v1.0–v1.1) remains valid under `-01`.

The substantive content of this revision is concentrated in a new
{{schema-language-extensions}} section and additions to
{{annotation-extensions}} and {{abnf-grammar}}. Other sections are
materially unchanged from `-00` and are reproduced verbatim in the
published draft; in this working-copy version they appear as placeholders
of the form "[Unchanged from -00; see prior draft]" for editorial review
efficiency.

A non-IETF design rationale companion is published at
{{PROTOWIRE-RFC-001}}.

--- abstract

This document specifies the Proto eXpressive Format (PXF), a UTF-8 text
serialization for messages defined by Protocol Buffers schemas, together
with three companion encodings: PB (the existing Protocol Buffers binary
wire format, with PXF-specific annotations that constrain how it is
produced and consumed), SBE (a fixed-layout binary encoding derived from
FIX Simple Binary Encoding), and a response envelope. Collectively these
are referred to as the protowire family. This document defines the wire
surface sufficient for independent interoperable implementations. It does
not define library APIs.

This revision (`-01`) introduces a set of strictly-additive schema
language extensions — `type`, `function`, and `annotation` declarations,
a unified annotation use-site syntax, and a structured validation error
model — that promote message validation to a first-class schema concern.
The additions do not change the PB, SBE, or envelope wire formats.

--- middle

# Introduction

[Unchanged from -00; see prior draft. The text below is reproduced
verbatim from `-00` in the published version of this revision.]

## Scope

This document defines:

* the lexical and syntactic structure of PXF text ({{pxf-text-format}});

* the constraints that protowire applies to PB binary encoding
  ({{pb-binary-encoding}});

* the SBE wire framing used by protowire ({{sbe-binary-encoding}});

* the schema-level annotation extensions ({{annotation-extensions}});

* the schema language extensions for validation, refinement aliases,
  and metadata annotations ({{schema-language-extensions}}); *(new in
  -01)*

* the response envelope ({{response-envelope}});

* the conformance requirements that any decoder operating on untrusted
  input MUST satisfy ({{decoder-conformance}}).

This document does not define library APIs, programming-language
bindings, code generation strategies, or performance characteristics.

## Terminology

[Unchanged from -00 except for the additions below.]

The following additional terms are introduced in `-01`:

type alias
: A named refinement of a primitive, enum, wrapper, or message type,
  declared with the `type` keyword. See {{type-declarations}}.

function declaration
: A signature for a named validation predicate, declared with the
  `function` keyword. Bodies are implemented per-runtime by the
  consuming validator. See {{function-declarations}}.

annotation declaration
: A named metadata attachment declared with the `annotation` keyword,
  usable at use sites via the `@name(args)` syntax. See
  {{annotation-declarations}}.

violation
: A structured value emitted by a function declaration when validation
  fails, carrying a code, structured parameters, and a fallback
  message. See {{error-model}}.

engine
: A runtime component that executes the expressions and function calls
  appearing in a schema's validation annotations. The engine is
  selected per project; this document specifies the contract between
  the schema and the engine, not the engine's internals. See
  {{cross-runtime}}.

# The protowire Family

[Unchanged from -00; see prior draft.]

# PXF Text Format {#pxf-text-format}

[Unchanged from -00; see prior draft. The presence-semantics subsection
on "set / null / absent" remains authoritative and is the normative
basis for {{validation-execution}}.]

# PB Binary Encoding {#pb-binary-encoding}

[Unchanged from -00; see prior draft.]

# SBE Binary Encoding {#sbe-binary-encoding}

[Unchanged from -00; see prior draft.]

# Annotation Extensions {#annotation-extensions}

[Content from -00 is unchanged. The following are added by -01.]

## Schema Extension Annotations {#schema-extension-annotations}

A new family of annotations is defined in package `protowire.schema.v1`
to support the schema language extensions specified in
{{schema-language-extensions}}. These annotations are declared, not
extended onto `FieldOptions` individually — they participate in the
unified annotation framework defined in {{annotation-declarations}} and
{{annotation-use-sites}}, and they lower to the carrier extension
described in {{lowering}}.

The framework-defined annotations are:

| Name | Parameters | Attaches to |
|---|---|---|
| `validate` | `rule: expression, code: string = "", message: string = ""` | any value-bearing declaration |
| `required` | (none) | field, oneof |
| `default` | `value: any` | field |
| `description` | `text: string` | any declaration |
| `example` | `value: any` | type, field |
| `error_code` | `code: string` | function, validate |
| `deprecated` | `reason: string = ""` | any declaration |
| `http` | `method: string, path: string` | rpc |

The bracket-form annotations `(pxf.required)` and `(pxf.default)` defined
in `-00` retain their assigned extension field numbers (`50000` and
`50001` respectively) and their semantics. `@required` and `@default`
defined here are the canonical surface forms going forward; the bracket
forms remain valid input that decoders MUST accept indefinitely.

# Schema Language Extensions {#schema-language-extensions}

This section is new in `-01`.

## Overview {#schema-extensions-overview}

The schema language defined in {{PROTOBUF}} provides messages, fields,
oneofs, enums, services, and methods. This document adds three further
top-level declarations to the surface a protowire-conformant schema MAY
contain:

`type`
: A named refinement alias bundling a base type with zero or more
  validation rules. See {{type-declarations}}.

`function`
: A signature for a named validation predicate whose body is provided
  by the consuming runtime. See {{function-declarations}}.

`annotation`
: A declaration of a metadata attachment usable via the `@name(args)`
  use-site syntax. See {{annotation-declarations}}.

Each declaration is package-scoped, importable across files, and lowers
into the standard FileDescriptorSet representation defined in
{{PROTOBUF}}. The wire format is unaffected: the lowered descriptor uses
custom options carried in the protowire-reserved extension number
range, transparent to consumers that do not import the carrier schemas.

## Reserved Keywords {#reserved-keywords}

The following identifiers are introduced as **contextual keywords** by
`-01`:

* `type`
* `function`
* `annotation`

Each MUST be recognized as a keyword only at the start of a top-level
declaration (file scope). In every other position — message names,
oneof names, field names, enum-value names, service names, rpc names —
the parser MUST accept the same word as an identifier. Implementations
SHOULD use the parser's grammar production to distinguish the two
contexts; lookahead at the file-scope production is sufficient.

The contextual treatment preserves complete backward compatibility:
every schema valid under `-00` (protowire v1.0 or v1.1) that uses
`type`, `function`, or `annotation` as a message, oneof, field, or
enum-value name remains valid under `-01` without modification. No
source-level incompatibility is introduced.

The character `@` (U+0040) is reserved as a sigil introducing an
annotation use site. Outside string literals and comments, an
occurrence of `@` MUST be followed by an identifier and MAY introduce
an annotation use site as defined in {{annotation-use-sites}}.

The word `expression` is a parameter-type designator usable only
inside `annotation X(arg: expression)` declarations
({{annotation-declarations}}); elsewhere it MUST be treated as a
regular identifier.

The word `this` is bound only inside engine-language bodies of
`@validate(...)` and similar annotations. Protobuf parsers capture
those bodies opaquely (per {{annotation-use-sites}}) and MUST NOT lex
`this` as a keyword in protobuf source.

## Type Declarations {#type-declarations}

A type declaration introduces a named refinement of a base type:

~~~
type-decl     = "type" ws name ws "=" ws type-ref ws [ annotation-list ] ";"
type-ref      = qualified-ident
qualified-ident = name *( "." name )
~~~

`type-ref` MUST resolve to one of: a Protocol Buffers primitive type
(`string`, `bytes`, `int32`, `int64`, `uint32`, `uint64`, `sint32`,
`sint64`, `fixed32`, `fixed64`, `sfixed32`, `sfixed64`, `float`,
`double`, `bool`); an enum type; a message type; a well-known wrapper
type (`google.protobuf.StringValue`, `google.protobuf.Int32Value`,
etc.); or another type declaration.

This revision (`-01`) prohibits `repeated` and `map<K,V>` as `type-ref`.
A future revision MAY relax this prohibition.

The optional `annotation-list` attaches refinement rules and other
metadata to the type. By convention, at least one `@validate(...)`
annotation appears, expressing the constraint the type adds beyond its
base type's constraints.

Type declarations MUST be uniquely named within their package. Forward
references within a file are permitted; cyclic references MUST be
rejected by the parser.

### Composition Semantics

When a type declaration `D` names another type declaration `B` as its
base, `D`'s constraints AND with `B`'s constraints at every use site.
This is the only composition operator; there is no override,
intersection, or removal mechanism. Composition is transitive: a chain
`A → B → C` accumulates constraints from `A`, then `B`, then `C` in
that order at any field declared as type `C`.

Composition MUST NOT change the underlying data type. A derived type's
base MUST resolve to the same primitive, enum, wrapper, or message kind
as its ancestor's; deriving an `int32` type from a `string`-based type
declaration is a parse error.

### Use at Field Sites

A field whose declared type names a type declaration MUST be processed
as follows by validators:

* The field's underlying wire type is that of the type declaration's
  resolved base type, unchanged.

* For each composition step in the type's chain, the corresponding
  refinement rules are appended to the field's validation rule list
  in base-to-derived order.

* Subsequent rules MAY be added at the field site via field-level
  annotations.

The complete rule list for a field is evaluated as specified in
{{validation-execution}}.

## Function Declarations {#function-declarations}

A function declaration specifies the signature of a named validation
predicate:

~~~
function-decl = "function" ws name ws "(" [ param-list ] ")"
                [ ws option-list ] [ ws annotation-list ] ";"
param-list    = param *( "," ws param )
param         = name ws ":" ws type-ref
option-list   = "[" option *( "," ws option ) "]"
~~~

Function declarations carry no body. The body MUST be provided by the
consuming runtime through a registration mechanism specified in
{{function-registration}}. The signature alone is the contract between
the schema and the runtime.

### Return Contract

Every function declared per `-01` returns a pair of values: a boolean
result and an optional structured violation. The contract is:

* On success, the function returns the equivalent of `(true, null)`.

* On failure, the function returns `(false, V)` where `V` is a
  Violation as defined in {{error-model}}.

The specific syntax for expressing this pair in any given runtime
language is per-runtime; the contract is what crosses the
schema/runtime boundary.

### Parameter Types

The parameter `type-ref` MUST resolve to a primitive type, an enum
type, a message type, or a type declaration. This revision does not
permit `repeated`, `map<K,V>`, or `expression` as parameter types.

### Function References at Validation Sites

Function calls appear inside `expression`-typed annotation arguments
(see {{annotation-use-sites}}). The parser SHALL extract every function
reference (fully-qualified call name and arity) from each expression
body and emit it into the descriptor's `Expression.calls` field for
runtime verification. A function reference whose name does not resolve
to a function declaration in scope MUST be reported as a parse error.

## Annotation Declarations {#annotation-declarations}

An annotation declaration specifies a named use site for metadata:

~~~
annotation-decl = "annotation" ws name ws "(" [ annot-param-list ] ")" ";"
annot-param-list = annot-param *( "," ws annot-param )
annot-param     = name ws ":" ws annot-param-type
                  [ ws "=" ws default-value ]
annot-param-type = "expression" / "string" / "int32" / "int64" /
                   "float" / "double" / "bool" / "bytes" / "any" /
                   qualified-ident
~~~

The `annot-param-type` value `expression` indicates that the
corresponding argument at use sites is parsed as engine-language source
rather than as a typed Protocol Buffers literal; the parser captures it
as opaque text for the engine to compile.

The `any` parameter type accepts any literal value that the consuming
target permits; specific compatibility is enforced at the lowering
layer. Message literals bound to `any` parameters require the
explicit type-name form (see {{literal-values}}).

Annotation declarations are themselves declared in schema; the
framework-defined annotations listed in
{{schema-extension-annotations}} reflect the canonical set distributed
with this specification, but user-declared annotations participate in
the same machinery.

## Annotation Use Sites {#annotation-use-sites}

A use site is introduced by the `@` sigil:

~~~
annotation     = "@" qualified-ident [ "(" [ annot-arg-list ] ")" ]
annot-arg-list = annot-arg *( "," ws annot-arg )
annot-arg      = [ name ws "=" ws ] annot-arg-value
annot-arg-value = literal / qualified-ident / expression-body
annotation-list = 1*annotation
~~~

Use-site arguments MAY be positional or named. Named arguments MAY
follow positional ones; positional MUST NOT follow named.

### Literal Values {#literal-values}

Non-scalar argument values take one of three literal forms (see
{{abnf-grammar}}): a list literal (`["US", "CA", "GB"]`), a message
literal (`myco.commons.Money{currency: "USD", units: 5}`), or an
enum-value reference (a qualified identifier, resolved at link time).

A message literal's type comes from the annotation parameter's declared
type; when the parameter — or, recursively, the message field being
initialized — is typed `any` (or `google.protobuf.Any`), the explicit
leading type name is REQUIRED. When a concrete message type is
declared, the type name is OPTIONAL; if present it MUST resolve to
exactly the declared type. The type is never inferred from the value's
shape.

Field initializers use the field's declared name with a mandatory
colon, separated by commas without a trailing comma; each field appears
at most once, and an unknown field name is an error. The text-format
liberties (colon-less nested blocks, semicolon separators,
repeated-field repetition, bracketed type URLs) are not part of this
grammar. Repeated fields take list-literal values; map fields are not
supported in this revision. Lists are homogeneous, MAY nest, and MAY be
empty; expression bodies never appear inside literals.

### Placement

Annotations attach to the immediately-adjacent declaration. Placement
is hybrid:

* On block declarations (`message`, `service`, `rpc`, `enum`, `oneof`),
  annotations MUST be **leading** — appearing on the lines preceding
  the keyword.

* On non-block declarations (`type`, `function`, `field`, `enum-value`),
  annotations MUST be **trailing** — appearing on the same or following
  lines after the declaration's terminating component, before the
  terminating semicolon.

Schemas MAY stack multiple annotations on a single target; they evaluate
in source order.

### Coexistence with Bracket Options

The pre-existing `[(qualified-ident) = value]` field-option syntax
defined in {{PROTOBUF}} and used by `(pxf.required)` / `(pxf.default)`
in `-00` is preserved unchanged. Annotations and bracket options MAY
coexist on the same field. The two forms are not interchangeable in
all cases — bracket options MUST resolve to a declared `extend
google.protobuf.*Options` extension; annotations MUST resolve to a
declared `annotation` — but they lower to the same descriptor surface
where their semantics overlap (notably `@required` lowers to set both
the annotation carrier entry AND `(pxf.required) = true`).

## Presence Semantics {#validation-execution}

Validation operates over the three-state presence model already
specified for PXF in `-00` ({{pxf-text-format}}, "Field presence: set,
null, absent"). This subsection restates the model from the validation
layer's perspective:

| State of the field | Action by validator |
|---|---|
| Set | Each rule in the field's rule list is evaluated against the value. |
| Null | The field's rule list is NOT evaluated. |
| Absent, no `@required`, no `@default` | The field's rule list is NOT evaluated. |
| Absent, `@required` present | A Violation with code `required` is emitted in a layer prior to rule evaluation. |
| Absent, `@default(v)` present | The value `v` is substituted; rules are evaluated against `v`. |

For fields whose declared type is a well-known wrapper type
(`google.protobuf.StringValue` etc.), the binding of the identifier
`this` inside any rule is the **unwrapped** scalar value when the
wrapper is set, and the rule is NOT evaluated when the wrapper is
null.

For fields whose declared type is a message, `this` binds to the
message instance, and rules MAY reference its fields by name.

For fields inside a `oneof` group, rules attached to inactive variants
MUST NOT be evaluated. Only the active variant's rules apply.

For `repeated` fields whose element type is a type declaration, the
type's rule list is evaluated **per element**. Field-level rules
attached via the field's own `annotation-list` are evaluated against
the **collection**.

For `map<K,V>` fields, value rules apply per value; key rules apply per
key when the key's declared type is a type declaration.

### Evaluation Order and Result Aggregation

For a given field, rules evaluate in source-list order: base-type
refinements first (in the order their declarations appear in the type
chain), then field-level annotations.

By default, the validator SHALL evaluate every rule on every relevant
field of the message instance and aggregate all resulting violations
into a single Report (see {{error-model}}). An engine-level option MAY
provide a fail-fast mode that terminates at the first violation; the
collect-all mode is the conformance-mandated default.

## Error Model {#error-model}

A Violation is a structured value with the following abstract shape:

| Field | Type | Description |
|---|---|---|
| `code` | string | A stable, machine-readable identifier for the failure mode. |
| `params` | map of string to any | Structured parameters relevant to the failure. |
| `fallback_message` | string | A human-readable message used when no locale catalog entry matches `code`. |

Functions emit Violations on failure. The engine MUST further enrich
each emitted Violation with at least the following context:

| Field | Type | Description |
|---|---|---|
| `path` | string | Dotted path into the message instance. |
| `type_chain` | list of string | Type-alias chain from base to derived, when the Violation was produced by a `TYPE_REFINEMENT` rule. |
| `actual_value` | any | The value the rule was evaluated against. |
| `source` | source-location | The position in the originating `.proto` source. |
| `rule_kind` | enum | One of `VALIDATE`, `REQUIRED`, `DEFAULT`, `TYPE_REFINEMENT`. |

The on-wire shape of the enriched Violation, the aggregate Report, and
their serialization across PXF, PB, SBE, and the envelope are
described in {{lowering}} and remain TBD in this revision pending
resolution of the open issue tracked at {{PROTOWIRE-RFC-001}}.

### Localization

When rendering a Violation to a human audience, implementations SHOULD
substitute `params` into a locale-specific template selected by the
Violation's `code`. The catalog of (locale, code, template) tuples is
registered with the validator at initialization time. Catalogs are
defined in schema using framework-provided annotations whose shape is
described in {{PROTOWIRE-RFC-001}}.

On catalog miss, implementations MUST fall back to `fallback_message`.
The fallback is always present and is the engine author's chosen
default.

## Lowering to FileDescriptorSet {#lowering}

Implementations MUST lower the `-01` constructs to the standard
FileDescriptorSet representation specified in {{PROTOBUF}}, augmented
with custom option extensions carried in protowire-reserved field
numbers. The schemas for these extensions are specified in
`protowire/proto/schema/v1/descriptor.proto` and summarized below.

### Extension Number Allocation

The number range 50400 through 50499 is reserved by `-01` for
schema-extension carriers. Allocated in this revision:

| Number | Carrier | Target Options Messages |
|---|---|---|
| 50400 | `AnnotationList` (named per kind: `file_annotations`, `message_annotations`, `field_annotations`, `enum_annotations`, `enum_value_annotations`, `service_annotations`, `method_annotations`, `oneof_annotations`) | FileOptions, MessageOptions, FieldOptions, EnumOptions, EnumValueOptions, ServiceOptions, MethodOptions, OneofOptions |
| 50401 | `FileFunctions functions` | FileOptions |
| 50402 | `FileAnnotationDecls annotation_decls` | FileOptions |
| 50403 | `FileTypeDecls type_decls` | FileOptions |
| 50404 | `SourceMap source_map` | FileOptions |

The annotation carrier shares wire number 50400 across all eight target
messages; each `extend` field carries a per-kind name so that the
fully-qualified extension names remain unique within the
`protowire.schema.v1` package. Numbers 50100 and 50101 are intentionally
not used by this allocation because the SBE annotations
(`sbe.schema_id`, `sbe.version`) already occupy them on FileOptions, and
an extension number may be used only once per extended message.

Numbers 50405 through 50499 are reserved for future schema-extension
carriers and MUST NOT be allocated by user schemas. Renumbering any of
the above allocations is a wire break per the rules defined in
`STABILITY.md` of {{PROTOWIRE-RFC-001}}'s source repository.

### Backward Compatibility with Stock Protocol Buffers Tooling

The carrier extensions are well-formed Protocol Buffers extensions per
{{PROTOBUF}}. Implementations not importing
`protowire/proto/schema/v1/descriptor.proto` MUST preserve the carrier
options as opaque unknown-field bytes across decode/re-encode cycles,
in accordance with the standard Protocol Buffers extension semantics.

Implementations that DO import the descriptor file decode the carriers
as typed messages.

## Cross-Runtime Considerations {#cross-runtime}

A schema using function declarations does not, by itself, specify which
runtime engine evaluates them. The engine is selected per project at
validator-binary build time; this revision specifies that for a given
project, at most one engine MAY be active.

Function bodies MUST be registered with the engine at initialization
time, keyed by the function's fully-qualified name. An engine
encountering a referenced function name with no registered body MUST
either:

* refuse to start (strict mode), or

* substitute a registered fallback that returns `(false, V)` with `V`
  bearing code `"unimplemented"` (lenient mode).

The default mode is implementation-defined; the contract this document
specifies is that one of the two behaviors MUST be exhibited.

### Multi-Runtime Schemas

A schema intended to validate identically across multiple consuming
runtimes (for example, server-side in a Go validator and client-side in
a Java validator) MUST limit its function references to those for which
equivalent implementations are registered in every consuming runtime.
This is an operational concern; this document does not provide a
mechanism for enforcing it at schema declaration time. Implementations
MAY provide a portability-check mode that warns or errors on
non-portable references.

# Response Envelope {#response-envelope}

[Unchanged from -00; see prior draft.]

# Decoder Conformance {#decoder-conformance}

[Unchanged from -00 except for the addition below.]

## Validation Engine Conformance {#validation-conformance}

A protowire implementation that ships a validation engine for the
schema language extensions MUST:

* implement the presence semantics specified in
  {{validation-execution}};

* emit Violations whose shape conforms to the structure defined in
  {{error-model}};

* enrich emitted Violations with at minimum the fields listed in
  {{error-model}};

* operate in collect-all mode by default;

* accept registration of function bodies by fully-qualified name;

* either refuse to start or substitute a documented fallback when a
  referenced function has no registered body.

An implementation MAY ship without a validation engine; in that case
the implementation processes the schema's declarations for purposes
unrelated to validation (e.g., code generation) and MUST NOT silently
fail validation rules.

# Media Type Registrations {#media-type-registrations}

[Unchanged from -00; see prior draft.]

--- back

# ABNF Grammar {#abnf-grammar}

[The ABNF productions from -00 are reproduced verbatim in the published
version. The additions below extend the file-body and declaration-body
productions and define the new constructs.]

## Additions in -01

~~~
file-body       =/ type-decl / function-decl / annotation-decl

message-body    =/ annotation
oneof-body      =/ annotation
field           =/ field-base [ ws annotation-list ]
enum-value      =/ enum-value-base [ ws annotation-list ]

type-decl       = "type" ws name ws "=" ws type-ref
                  [ ws annotation-list ] ws ";"
type-ref        = qualified-ident

function-decl   = "function" ws name ws "(" [ param-list ] ws ")"
                  [ ws option-list ] [ ws annotation-list ] ws ";"
param-list      = param *( ws "," ws param )
param           = name ws ":" ws type-ref
option-list     = "[" ws option *( ws "," ws option ) ws "]"

annotation-decl = "annotation" ws name ws "(" [ annot-param-list ]
                  ws ")" ws ";"
annot-param-list = annot-param *( ws "," ws annot-param )
annot-param     = name ws ":" ws annot-param-type
                  [ ws "=" ws default-value ]
annot-param-type = %s"expression" / %s"string" / %s"int32" /
                   %s"int64" / %s"float" / %s"double" / %s"bool" /
                   %s"bytes" / %s"any" / qualified-ident

annotation      = "@" qualified-ident [ "(" [ annot-arg-list ] ")" ]
annot-arg-list  = annot-arg *( ws "," ws annot-arg )
annot-arg       = [ name ws "=" ws ] annot-arg-value
annot-arg-value = literal / qualified-ident / expression-body
annotation-list = 1*( ws annotation )

literal         = scalar-literal / list-literal / message-literal
scalar-literal  = string-lit / int-lit / float-lit / bool-lit
                  ; lexical productions per -00; string-lit also
                  ; serves bytes-typed params
literal-value   = literal / qualified-ident
                  ; qualified-ident = enum-value reference
list-literal    = "[" ws [ literal-value
                  *( ws "," ws literal-value ) ] ws "]"
message-literal = [ qualified-ident ws ] "{" ws
                  [ field-init *( ws "," ws field-init ) ] ws "}"
field-init      = name ws ":" ws literal-value

expression-body = ; engine-specific subgrammar, captured as balanced
                  ; bracketed text by protowire parsers per the
                  ; rules in {{function-references}}
~~~

`field-base` and `enum-value-base` denote the productions defined in
`-00` for fields and enum values respectively, retained for delta
clarity.

## Contextual Keywords

The contextual keywords introduced in `-01` are:

~~~
contextual-keyword = %s"type" / %s"function" / %s"annotation"
~~~

These are recognized as keywords only at the start of a top-level
declaration (file scope) per {{reserved-keywords}}. In every other
position they MUST be accepted as identifiers, preserving backward
compatibility with the schema constraints defined in `-00`.

# Acknowledgments

The schema language extensions specified in this revision draw on prior
work in Buf's `protovalidate` (proto-native validation rules over CEL),
Google's `gnostic` (OpenAPI-via-proto-annotations), Liquid Haskell and
F* (refinement type theory), and Common Expression Language
{{CEL}} and Starlark {{STARLARK}} (embeddable expression languages
suitable as engine targets). The error model owes its three-state
presence framing to the PXF set/null/absent design specified in `-00`.

# Implementation Status

[Per RFC 7942 — to be populated before publication.]

# Open Issues

This revision tracks open issues whose resolution is required before
publication or whose deferral to a future revision has been formally
agreed:

* **Container-shaped type aliases.** Allowing `repeated` and `map<K,V>`
  as `type-ref` bases. Deferred to a future revision.

* **Engine-config schema.** The project-level configuration that
  selects a runtime engine and lists function-library imports remains
  to be specified.

* **Well-known type semantics.** The binding of `this` and the shape of
  validation rules over `google.protobuf.Timestamp`,
  `google.protobuf.Duration`, and `google.protobuf.Any` requires a
  normative pass.

* **Recursive-message depth limits.** Maximum nesting depth and the
  validator's behavior at the limit.

* **Streaming RPC validation.** Per-message vs. per-stream evaluation
  semantics.

* **Validation report wire shape.** The complete serialization of an
  aggregate Report across PXF, PB, SBE, and the envelope.

These issues are tracked in {{PROTOWIRE-RFC-001}} and the protowire
issue tracker.
