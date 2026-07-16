# RFC-001 — Protowire Schema Extensions

| Field | Value |
|---|---|
| Status | Draft |
| Target spec version | protowire v1.2.0 (minor, strictly additive) |
| IETF draft | Companion to `draft-trendvidia-protowire-01` (in preparation) |
| Authors | TBD |
| Created | 2026-06-04 |
| Last updated | 2026-07-15 |

## Abstract

This RFC proposes three new top-level declarations to the protowire schema language — `type`, `function`, and `annotation` — together with a general-purpose annotation framework (`@name(args)`) and a structured error model. The additions promote validation from a sidecar concern (today fragmented across `protovalidate`, `protocheck`, and ad-hoc proto options) to a first-class language feature. All additions are strictly additive: every existing v1.x schema parses and validates unchanged. Lowering targets standard `FileDescriptorSet` plus custom options in protowire's reserved extension range (`50400`–`50404`), so downstream tooling (stock `protoc`, `protobuf-go`, every existing port) round-trips the new constructs as opaque options without source-level changes.

## 1. Motivation

The protowire ecosystem already has strong wire-format and serialization stories (PXF, `pb`, SBE) and a coherent presence model (set / null / absent, governed by `(pxf.required)` and `(pxf.default)`). What it lacks is a first-class story for **validation** — the rules that constrain what a "valid" message instance is, beyond mere wire-format conformance.

Today, teams reach for `protovalidate` or `protocheck` for runtime validation. Both work, but both impose costs the spec layer can prevent:

- **Duplication.** The same `Email`, `UUID`, or numeric-range constraints are repeated across dozens of messages in non-trivial schemas.
- **Loss of context in errors.** When a chained constraint fails (`CompanyEmail` failing on its inner `Email` rule), the error has no way to express the chain — users see "must match pattern" without knowing what semantic type they violated.
- **No localization.** Error messages are baked into rule definitions in whichever language the rule author chose.
- **Engine fragmentation.** CEL, Starlark, and Go-native validation engines coexist with no shared declaration surface; teams pick one and stay locked in.
- **Documentation drift.** Constraints used at runtime aren't surfaced in OpenAPI / Swagger / gnostic-style generated docs without a parallel annotation layer.

Public-API platforms — the principal driver for this RFC — feel each of these as recurring incident-class problems. The spec response is to absorb the structured part of validation into the schema language itself and to provide a uniform annotation framework that subsumes today's ad-hoc options.

## 2. Goals

1. Validation rules are **declared once**, named, and reusable.
2. Function-call abstractions ("`is_e164`," "`matches_uuid_v4`") are first-class spec citizens with a defined cross-language contract.
3. A **single annotation framework** (`@name(args)`) carries metadata for validation, documentation, OpenAPI mapping, deprecation, and future concerns — eliminating one-off `[(some_pkg.some_option) = …]` per concern.
4. Errors carry **stable codes**, structured **parameters**, **type-chain provenance**, and integrate with **locale catalogs** for i18n.
5. Cross-port portability: all 10 protowire ports can implement the new constructs without language-specific deviation.

## 3. Non-goals

- **No wire-format changes.** PXF, `pb`, and SBE outputs are byte-identical to v1.1 for any schema not using the new constructs.
- **Not replacing protobuf.** Lowering targets standard `FileDescriptorSet`; stock `protoc` and every existing tool consume the descriptors transparently.
- **Not standardizing engine internals.** CEL/Starlark/Go evaluation semantics remain engine-specific; the spec defines the *contract* between the schema and the engine, not the engine's internals.
- **Not introducing a parallel type system.** `type` declarations are macro-style refinement aliases that lower to the underlying primitive/message/enum, not new wire-level types.

## 4. Design overview

Three new top-level declaration kinds:

| Declaration | Purpose |
|---|---|
| `type Name = Base @validate(...)` | Named refinement alias; reusable constraint bundle |
| `function name(args)` | Signature contract for a validation function; body implemented per-runtime |
| `annotation name(args)` | Declares a new metadata annotation usable via `@name(...)` |

A single annotation framework `@name(args)` carries all metadata uniformly — validation rules, descriptions, examples, deprecation, OpenAPI hints, HTTP routing, future categories. **Hybrid placement**: leading on block declarations (`message`, `service`, `rpc`, `enum`, `oneof`); trailing on single-line declarations (`type`, `field`, `function`).

Existing `[(option) = value]` bracket syntax **coexists** — annotations are first-class sugar with verification benefits, while brackets remain the raw escape hatch for one-off custom options.

All new constructs lower to `FileDescriptorSet` plus extensions in protowire's reserved range. Stock downstream tools see only standard proto.

## 5. Surface syntax

### 5.1 Grammar additions (delta to v1.1)

Three new **contextual keywords** are introduced: `type`, `function`, `annotation`. Each is recognized as a keyword only at the start of a top-level declaration; in every other position — message names, oneof names, field names, enum-value names, service names, rpc names — the parser MUST accept the same word as an identifier. This preserves complete backward compatibility with v1.1 schemas; no source-level incompatibility is introduced.

The character `@` (U+0040) is reserved as a sigil introducing an annotation use site.

Two identifiers mentioned in earlier drafts are **not** reserved in protobuf namespace:
- `expression` — a parameter-type designator usable inside `annotation X(arg: expression)` declarations; everywhere else it is a regular identifier.
- `this` — bound only inside engine-language bodies of `@validate(...)` and similar; protocompile captures those bodies opaquely, so `this` is not lexed specially.

**Why contextual.** Real-world Google APIs (Cloud DLP and others) and many production schemas use `type` as a `oneof` or field name (`oneof type { ... }`). Hard-reserving these words would break all such schemas. Contextual recognition uses the parser's lookahead to distinguish `type Email = ...` at file scope (keyword) from `oneof type { ... }` after `oneof` (identifier). The pattern is well-precedented — Java 9's `module`, `requires`, `exports`, `opens`, `to`, `with` are contextual keywords for exactly this reason.

```ebnf
topLevelDecl
    ::= /* existing: import, package, option, message, enum, service, extend */
      | typeDecl
      | functionDecl
      | annotationDecl

typeDecl
    ::= "type" Ident "=" typeRef annotationList? ";"
typeRef
    ::= qualifiedIdent                (* primitive | enum | wrapper | message | another `type` *)

functionDecl
    ::= "function" Ident "(" paramList? ")" optionList? annotationList? ";"
paramList   ::= param ("," param)*
param       ::= Ident ":" paramType
paramType   ::= qualifiedIdent

annotationDecl
    ::= "annotation" Ident "(" annotParamList? ")" ";"
annotParam      ::= Ident ":" annotParamType ("=" defaultValue)?
annotParamType  ::= "expression" | "string" | "int32" | "int64" | "float" | "double"
                  | "bool" | "bytes" | "any" | qualifiedIdent

annotation
    ::= "@" qualifiedIdent ("(" annotArgList? ")")?
annotArgList  ::= annotArg ("," annotArg)*
annotArg      ::= (Ident "=")? annotArgValue        (* positional or named *)
annotArgValue ::= literal | qualifiedIdent | engineExpression
annotationList ::= annotation+
```

Placement is by production: trailing on `typeDecl`, `field`, `functionDecl`, `enumValue`; leading on `messageDecl`, `enumDecl`, `serviceDecl`, `rpcDecl`, `oneofDecl`.

v1.2 explicitly forbids `repeated`/`map<,>` in `typeRef` (collection refinement is deferred — see §13).

### 5.2 Framework annotation library

Shipped at `protowire/proto/schema/v1/annotations.proto` (importable by any schema):

```proto
syntax = "proto3";
package protowire.schema.v1;

annotation validate(rule: expression, code: string = "", message: string = "");
annotation required;
annotation default(value: any);
annotation description(text: string);
annotation example(value: any);
annotation error_code(code: string);
annotation deprecated(reason: string = "");
annotation sensitive;
annotation http(method: string, path: string);
```

The existing PXF annotations `(pxf.required)` and `(pxf.default)` retain their bracket forms and extension numbers (`50000`, `50001`) for backward compatibility. `@required` and `@default(value)` are the canonical annotation form going forward; lowering preserves the legacy options where consumers depend on them.

### 5.3 Worked example

```proto
syntax = "proto3";
package myco.users;

import "myco/commons/types.proto";
import "myco/commons/validator.proto";
import "protowire/schema/v1/annotations.proto";

function same_domain(msg: User) [error_code = "user.domain_mismatch"];

@description("a user account on the platform")
@validate(same_domain(this))
message User {
  CompanyEmail email = 1
    @description("primary email")
    @example("alice@acme.com");

  PhoneNumber phone = 2 @required;

  string country = 3
    @default("US")
    @validate(this in ["US", "CA", "GB"], code = "user.invalid_country");
}

@description("user management operations")
service Users {
  @http(method = "GET", path = "/users/{user_id}")
  @description("retrieve a user by ID")
  rpc Get(GetUserRequest) returns (User);
}
```

## 6. Semantics

### 6.1 Presence model — aligned with PXF

This RFC inherits protowire's three-state presence model verbatim:

| State | Validation behavior |
|---|---|
| **Set** | Validation runs on the value. |
| **Null** | Validation skipped; null is explicit "no value." The field already opted into nullability via wrapper / `optional`. |
| **Absent** | Validation skipped; if `@required`, absence itself is the error (prior layer). If `@default(value)`, the default substitutes and validation runs on the default. |

This eliminates the proto3 zero-value ambiguity in the validation layer: validation runs only on values the producer meant to set. `@required` is the separate "must be present" lever, orthogonal from "if present, must match."

### 6.2 Wrapper and well-known type handling

Five normative rules define what `this` binds to inside a refinement rule,
by the base type's kind. None of them change descriptor lowering — a type
alias always records its literal `base_type_fqn` — they pin what the
already-lowered alias *means* at evaluation time.

1. **Wrappers** (`google.protobuf.StringValue`, `Int64Value`, etc.):
   `this` binds to the **unwrapped** scalar value when the wrapper is set.
   The rule does not execute when null. This matches PXF's wrapper sugar
   (`nullable_name = "hello"`).

2. **`google.protobuf.Timestamp` / `google.protobuf.Duration`**: `this`
   binds to the **engine-native temporal value** — parallel to wrapper
   unwrap, so rules read naturally (`type Future = google.protobuf.Timestamp
   @validate(this > now());`). Engines MUST support the comparison
   operators (`<`, `<=`, `==`, `>=`, `>`) between temporal values of the
   same kind. Temporal literals and helpers (`now()`, duration
   construction) are engine-stdlib concerns, not spec syntax — expressions
   are opaque engine source (§5.1). As with wrappers, the rule does not
   execute when the field is unset (§6.1). CEL's native `Timestamp`/
   `Duration` mapping already satisfies this rule unmodified.

3. **`google.protobuf.Any`** does **not** unwrap. `this` binds to the
   structured value with `type_url` and `value` accessible;
   `this.type_url == "..."` string refinement is the canonical pattern.
   Engines MUST NOT auto-unpack the payload: unpacking requires resolving
   the payload type against a descriptor pool at evaluation time — exactly
   the value-scanning inference protowire forbids, and a silent behavior
   change as pools grow. A rule that needs payload access declares a
   `function` taking the `Any` and unpacks explicitly in its
   implementation.

4. **All other message types** — including the remaining WKTs (`Struct`,
   `FieldMask`, …): `this` binds to the structured message; field access
   follows the engine's proto integration. No further special cases.

5. **Run-stable `now()`**: any engine-provided current-time builtin MUST
   return the same instant for every evaluation within a single validation
   run (one `Report`, §7). Otherwise a `@validate(this > now())` rule
   evaluated in collect-all mode could pass and fail within the same
   report for equal values, and function memoization (§6.5) would be
   unsound.

### 6.3 Type refinement and composition

Types are macros: at every use site, the type's refinement rule(s) expand into the field's annotation list. Composition is **pure AND**: each derived type adds its rule to the chain; no override semantics. Base sets the data type; derived only narrows.

```proto
type Email        = string @validate(matches(this, "^[^@]+@[^@]+$"));
type CompanyEmail = Email  @validate(ends_with(this, "@acme.com"));
```

A field declared `CompanyEmail email = 1;` carries both rules in evaluation order: `Email`'s rule first, then `CompanyEmail`'s. Type-chain provenance (`string → Email → CompanyEmail`) is preserved in the source map for error attribution.

Refinement scope in v1.2 is limited to **value-shaped** type kinds: primitives, enums, wrappers, and messages. **Container-shaped** kinds (`repeated`, `map<K,V>`) are deferred to a future minor revision (see §13). Containers still hold typed elements (`repeated Email`) — only the alias *target* is restricted.

### 6.4 Validation execution

Per-field validation runs in source-order through the type chain (base → derived → field-level annotations). Default behavior is **collect-all**: the validator gathers every violation in a message instance and reports them together. An engine-level option enables **fail-fast** for callers preferring early termination.

`oneof` validates only the active variant.

`repeated` and `map<K,V>` validate per-element (using the element's type rules) plus any field-level `@validate` against the collection as a whole.

**Recursion depth.** Nested-message validation is depth-limited. The root
instance is at depth 0; entering any message-typed value (a nested field,
a repeated element, a map value) increments the depth by 1 — scalars and
scalar-collection elements do not. The limit is
`EngineConfig.max_recursion_depth` (§9.4); `0` means the **normative
default of 64**. When a value at depth greater than the limit would need
validating, the engine does **not** descend: it records one synthetic
violation for the subtree — `code: "protowire.depth_exceeded"`, `path` at
the field where descent stopped, `params: {limit: <the effective limit>}`,
`rule_kind: RULE_KIND_VALIDATE` — sets `Report.truncated = true` (§7), and
continues with siblings in collect-all mode. The instance therefore fails
**closed**: an unvalidatable subtree is never silently accepted, and one
pathological subtree does not hide the rest of the report (as a hard error
would). The depth definition, the default, and the at-limit behavior are
normative — identical deep instances MUST yield equivalent reports across
ports — while the enforcement mechanism (call stack vs. explicit counter)
is implementation-defined. Engines MAY offer a per-call override in their
SPI; the config file is the canonical project-level setting. The limit
also bounds worst-case stack use on attacker-controlled deep messages —
relevant for the public-API driving use case.

### 6.5 Function contract

```
function name(args) → (bool, *Violation)
```

`(true, nil)` on success; `(false, &Violation{...})` on failure. The `Violation` is structured (§7) and carries a stable code + parameters + a fallback message. Functions are pure (no I/O, no global state) — engines may memoize calls.

Functions are declarations only; bodies are implemented in the engine runtime and registered by fully-qualified name at engine init. No engine tag on the declaration: the spec specifies the contract; runtime registration provides the implementation.

### 6.6 Streaming and RPC validation contract

Six rules define how validation applies at RPC boundaries. The normative
contract is transport-agnostic; the gRPC mapping in rule 6 is the
reference.

1. **The unit of validation is one message.** Each message is validated
   independently as it crosses the RPC boundary, exactly as §6.4 defines
   for a single instance — one message, one (potential) `Report` (§7).
   Stream-level invariants (aggregate rules across messages, ordering
   constraints) are out of scope for v1.2 and deferred (§13).

2. **Placement.** The receiver MUST validate a message before delivering
   it to application code. The sender MAY additionally validate before
   transmission — recommended where the producer is untrusted or in
   strict deployments. Server-side validation stays authoritative (§10).

3. **Mid-stream failure terminates the stream** with an error carrying
   the structured `Report`. Skip-and-continue silently drops data and
   breaks ordering assumptions; deliver-anyway is validation turned off.
   There is deliberately no configuration knob for lenient stream
   behavior: an application wanting custom handling opts out of automatic
   validation and calls `Validate()` itself.

4. **No rollback.** Messages delivered before the failing one stay
   delivered; the stream terminates at the first invalid message. Any
   transactional semantics across a stream are application concerns.

5. **Direction asymmetry.**
   - A **request-direction** message found invalid at the server: the
     client sent bad data → terminate with `INVALID_ARGUMENT`, `Report`
     attached.
   - A **response-direction** message found invalid at the sender
     (server pre-send check): the server produced bad data → `INTERNAL`,
     `Report` attached. The caller is never blamed for the callee's
     output.
   - A **response-direction** message found invalid at the client
     (receiver check): surfaced as a local client-library error carrying
     the `Report`; the client does not "status" the server.
   - Unary RPCs are the degenerate case: validate the request before the
     handler, the response before send. Bidirectional streams apply the
     rules per direction independently.

6. **Transport mapping is layered.** For gRPC, the status code follows
   rule 5 and the `Report` is embedded in `google.rpc.Status.details` as
   an `Any` (`type.googleapis.com/protowire.schema.v1.Report`). Other RPC
   frameworks map through their adapter, carrying the same `Report`.

### 6.7 Sensitivity classification (`@sensitive`)

`@sensitive` classifies the value carried by a declaration as sensitive
material — credentials, tokens, personal data. It attaches to fields, to
`type` aliases (macro-expanding to every consuming field, like refinement
rules, §6.3), and to messages (every field of the message is sensitive; a
field whose type is a sensitive message is itself sensitive).

**Classification, not protection.** `@sensitive` does not alter wire
encoding, storage, programmatic access, or validation semantics.
Encryption-at-rest, key management, and access control remain
runtime-layer concerns (PXF / chameleon), which MAY consume the
classification to select fields for protection. The schema declares
*what* is sensitive, never *how* it is protected.

Normative consumer minima:

1. **Rendering surfaces.** Any surface that renders field values for
   human or log consumption — generated `String()`/debug formatters,
   structured-logging integrations, IDE hover, exporter and query-tool
   default output — MUST replace a sensitive field's value with the
   fixed placeholder `[REDACTED]`.
2. **Validation reports.** Engines MUST NOT populate
   `EnrichedViolation.actual_value` for a violation on a sensitive
   field; they set `value_redacted = true` instead, keeping redaction
   distinguishable from genuine absence (§6.1). Function implementations
   SHOULD NOT copy the offending value into `Violation.params`; engines
   cannot enforce this mechanically.
3. **Documentation emit.** Generated documentation (OpenAPI, JSON
   Schema, doc comments) MUST NOT include values or examples for
   sensitive fields; `@example` on a sensitive declaration is a
   compile-time warning.

The annotation lowers through the standard `50400` `AnnotationList`
carrier like every other annotation; no dedicated extension number or
descriptor surface exists. A classification parameter (e.g. a
secret / PII / confidential taxonomy) and a schema-level key-reference
annotation are deferred (§13); adding optional parameters with defaults
to a canonical annotation is an additive, minor-version change.

## 7. Error model

The normative wire shapes for the error model live in
`protowire/proto/schema/v1/report.proto` (stock proto3, parseable by any
v1.x port; a runtime artifact emitted by engines, never by the compiler —
it allocates no extension numbers). The definitions below are excerpted
from that file.

A `Violation` is the engine-independent failure value returned by
validation functions (§6.5):

```proto
message Violation {
  string code = 1;                        // stable, machine-readable
  map<string, Value> params = 2;          // {value, pattern, min, max, ...}
  string fallback_message = 3;            // engine-author default, used on catalog miss
}
```

`params` values — and every other value slot in the report — use the typed
`protowire.schema.v1.Value` message: an explicit oneof over string / int64 /
uint64 / double / bool / bytes / enum name / `Any`-wrapped message / list /
null. `google.protobuf.Value` is deliberately **not** used: it folds int64
into double, cannot carry bytes, and erases the set/null/absent
distinction. A `Value` field left unset means *absent*; `null_value` means
the producer explicitly set null — carrying protowire's three-state
presence model (§6.1) through to reports.

The engine enriches each function-returned `Violation` with context the
function cannot know:

```proto
message EnrichedViolation {
  Violation cause = 1;
  FieldPath path = 2;                     // structured path into the message
  repeated string type_chain = 3;         // ["string", "Email", "CompanyEmail"], base first
  Value actual_value = 4;                 // unset = field absent — or redacted
  SourceLocation source = 5;              // from the embedded source map (50404)
  RuleKind rule_kind = 6;                 // RULE_KIND_{VALIDATE,REQUIRED,DEFAULT,TYPE_REFINEMENT}
  bool value_redacted = 7;                // @sensitive field: value withheld (§6.7)
}
```

`FieldPath` is structured — a sequence of segments carrying `field_name`,
`field_number`, and an optional typed subscript (`index` for repeated
elements; `string_key` / `int_key` / `uint_key` / `bool_key` for map keys)
— never a dotted string. Dotted renderings are derived for display and
never parsed back; map keys are typed, never coerced through strings.
(`RuleKind` values carry the `RULE_KIND_` prefix because proto enum values
share package scope and `EntryKind.TYPE_REFINEMENT` in `descriptor.proto`
already claims the bare name.)

A complete validation run produces a `Report` — the shape all 10 ports
emit equivalently:

```proto
message Report {
  string message_type = 1;                    // FQN of the root message validated
  repeated EnrichedViolation violations = 2;  // empty + truncated == false ⇒ valid
  ExecutionMode mode = 3;                     // COLLECT_ALL (default, §6.4) | FAIL_FAST
  bool truncated = 4;                         // violations not exhaustive (fail-fast stop or engine limit)
  EngineInfo engine = 5;                      // {name, version}, e.g. "protocheck-go/cel"
  uint64 wall_time_nanos = 6;                 // 0 = not measured
}
```

Localized messages are produced at format time from `code` + `params` through a registered catalog (one per locale, registered with the engine alongside function impls). Catalog miss falls back to `fallback_message`. Programmatic clients consume `code` + `params` directly; human consumers receive the localized rendering.

`@validate(...)` accepts optional `code` and `message` to override defaults at use sites.

Violation codes beginning with **`protowire.`** are reserved for
spec-defined violations; user rules and function implementations MUST NOT
mint codes in that namespace. This revision defines two:
`protowire.required` (a `@required` field is absent, §6.1) and
`protowire.depth_exceeded` (recursion depth limit reached, §6.4).

## 8. Descriptor lowering

### 8.1 Universal annotation carrier

```proto
// protowire/proto/schema/v1/descriptor.proto
syntax = "proto3";
package protowire.schema.v1;

import "google/protobuf/descriptor.proto";

message AnnotationList { repeated Annotation entries = 1; }

message Annotation {
  string name = 1;                                   // FQN
  repeated AnnotationArg args = 2;
  SourceLocation location = 3;
}

message AnnotationArg {
  string name = 1;                                   // empty for positional
  oneof value {
    string string_value = 10;
    int64 int_value = 11;
    double double_value = 12;
    bool bool_value = 13;
    bytes bytes_value = 14;
    Literal literal = 15;
    Expression expression = 20;
  }
}

message Literal {
  oneof kind {
    EnumLiteral enum_value       = 1;                // linker-resolved enum value reference
    google.protobuf.Any message  = 2;                // typed message literal, serialized at lowering
    ListLiteral list             = 3;                // [elem, elem, ...]
  }
}

message EnumLiteral {
  string enum_type = 1;                              // FQN, e.g. "myco.orders.OrderStatus"
  string value_name = 2;                             // "CANCELLED"
  int32 number = 3;                                  // resolved numeric value
}

message ListLiteral { repeated LiteralValue elements = 1; }

message LiteralValue {
  oneof kind {
    string string_value  = 10;
    int64  int_value     = 11;
    double double_value  = 12;
    bool   bool_value    = 13;
    bytes  bytes_value   = 14;
    Literal literal      = 15;                       // enum value, message, or nested list
  }
}

message Expression {
  string source = 1;                                 // raw engine source
  repeated FunctionRef calls = 2;                    // extracted at compile
  SourceLocation location = 3;
}

extend google.protobuf.FileOptions      { AnnotationList file_annotations       = 50400; }
extend google.protobuf.MessageOptions   { AnnotationList message_annotations    = 50400; }
extend google.protobuf.FieldOptions     { AnnotationList field_annotations      = 50400; }
extend google.protobuf.EnumOptions      { AnnotationList enum_annotations       = 50400; }
extend google.protobuf.EnumValueOptions { AnnotationList enum_value_annotations = 50400; }
extend google.protobuf.ServiceOptions   { AnnotationList service_annotations    = 50400; }
extend google.protobuf.MethodOptions    { AnnotationList method_annotations     = 50400; }
extend google.protobuf.OneofOptions     { AnnotationList oneof_annotations      = 50400; }
```

The annotation carrier shares wire number `50400` across all eight Options
messages, but each `extend` field is named per kind (`file_annotations`,
`message_annotations`, …) so every extension has a unique fully-qualified
name within the `protowire.schema.v1` package.

Three rules govern `Literal` lowering. **Enum references are lowered
resolved**: the linker records the enum type FQN, value name, and number
in `EnumLiteral` — consumers never re-resolve a bare name against a
descriptor pool. **List literals are homogeneous**: all elements of one
`ListLiteral` share the same kind (and, for enum elements, the same
`enum_type`); the compiler rejects heterogeneous lists; elements carry no
name and can never be expressions (`this in [...]` inside a `@validate`
rule is one opaque `Expression`, not a list literal). **Message literals
are explicitly typed**: the type comes from the annotation param's
declared type, or from an explicit type name at the use site when the
param is `any` — never inferred from the value's shape; the lowered form
is a `google.protobuf.Any` serialized at compile time and unpacked against
the `FileDescriptorSet` the consumer already holds. The source-level
spelling of message literals is pinned by the IETF draft (#003).

### 8.2 File-scope declaration carriers

```proto
extend google.protobuf.FileOptions {
  FileFunctions       functions        = 50401;
  FileAnnotationDecls annotation_decls = 50402;
  FileTypeDecls       type_decls       = 50403;
}
```

`FileFunctions`, `FileAnnotationDecls`, and `FileTypeDecls` carry the corresponding declarations with their parameters, options, and source locations. Type aliases are preserved in the descriptor (not only macro-expanded at use sites) so tooling — IDE go-to-definition, OpenAPI generators that produce named `components/schemas/Email` — can resolve them as named entities.

### 8.3 Embedded source map

```proto
extend google.protobuf.FileOptions { SourceMap source_map = 50404; }
```

The `SourceMap` carries entries mapping descriptor positions back to source-file locations and capturing the type-refinement chain that produced each rule. Embedded (not sidecar) — one artifact, no sync-drift between descriptor and map.

#### 8.3.1 `descriptor_path` grammar (normative)

Every `SourceEntry.descriptor_path` is produced and parsed by this grammar.
The delimiters `[`, `]`, `#`, and `/` are not legal in proto identifiers, so
paths never require escaping.

```ebnf
descriptorPath   = elementPath , [ annotationAnchor , [ callAnchor ] ] ;
elementPath      = [ fqn ] ;                (* canonical FullName, no leading dot *)
annotationAnchor = "[" , fqn , "#" , ordinal , "]" ;
callAnchor       = "/arg#" , index , "/call#" , index ;
fqn              = ident , { "." , ident } ;
ordinal          = decimal ;                (* zero-based, no leading zeros *)
index            = decimal ;
```

- `elementPath` is the carrier element's canonical fully-qualified name, as
  `protoreflect.FullName` renders it. Enum values use their parent-scoped name
  (`pkg.OK`, not `pkg.Status.OK`). For file-level annotations it is the file's
  package name — empty for packageless files, so the path begins with `[`.
- `annotationAnchor` selects one `Annotation` in the carrier's
  `AnnotationList`: the annotation's fully-qualified `name` plus a zero-based
  ordinal counting only same-named annotations on that carrier, in list order
  (including rules macro-expanded from type aliases).
  `myco.User.email[protowire.schema.v1.validate#1]` is the second `@validate`
  on the field.
- `callAnchor` (kind `FUNCTION_CALL` only) descends into the anchored
  annotation: `arg#i` indexes `Annotation.args`; `call#j` indexes that
  argument's `Expression.calls`.

Shape by kind: `TYPE_REFINEMENT` entries use a bare `elementPath` (at most one
per field or extension); `ANNOTATION_USE`, `FIELD_VALIDATE`, and
`MESSAGE_VALIDATE` use `elementPath annotationAnchor`; `FUNCTION_CALL` appends
the `callAnchor`.

A `descriptor_path` is unique within its enclosing `SourceMap`; cross-file
indexes key by `(SourceMap.file, descriptor_path)`, since package names are
shared across files. Producers and consumers use one shared formatter/parser
(protocompile `fdp/descriptor_path.go`, exported); consumers never hand-split
the string.

### 8.4 Extension number allocation

| Number | Carrier | Targets |
|---|---|---|
| `50400` | `AnnotationList` (`file_annotations`, `message_annotations`, …) | all 8 Options messages |
| `50401` | `FileFunctions functions` | FileOptions |
| `50402` | `FileAnnotationDecls annotation_decls` | FileOptions |
| `50403` | `FileTypeDecls type_decls` | FileOptions |
| `50404` | `SourceMap source_map` | FileOptions |

Range `50400`–`50499` is allocated in this revision for future schema-extension carriers, within protowire's documented `50000`–`59999` family range (per `STABILITY.md`). The `50100`–`50101` numbers are skipped because SBE already claims them on `FileOptions` (`sbe.schema_id`, `sbe.version`), and an extension number may be used only once per extended message.

### 8.5 Backward compatibility with stock tooling

The carrier extensions are well-formed proto. Stock `protoc`, `protobuf-go`, and every existing protowire port treat them as opaque options when `protowire/proto/schema/v1/descriptor.proto` is not imported — preserving them as `UnknownFields`, round-tripping byte-identically. When imported, the extensions decode as typed values for tools that want structured access.

`protocompile`'s existing option-interpretation pipeline (see `options/options.go:14`) handles arbitrary extension numbers without modification. The lowering pass produces uninterpreted options that the existing interpreter populates into the carrier extensions; no new descriptor pathway is required.

## 9. Engine integration

### 9.1 Engine SPI

Per-port engine SPI carries the same logical contract:

```go
// reference Go interface; per-port equivalents follow
type Engine interface {
    Register(fqn string, impl Function) error
    RegisterCatalog(locale string, catalog Catalog) error
    Validate(msg proto.Message) (*Report, error)
}

type Function func(args []any) (bool, *Violation)
```

A project selects one engine at validator-binary build time (CEL, Starlark, Go, etc.). Mix-and-match engines per project is out of scope for v1.2 — adding it later is a strictly additive change to the engine-config schema, not the language.

### 9.2 Function registration model

Functions referenced in the descriptor must be registered with the engine at startup. The engine walks the descriptor on init and verifies each FQN is present in its registry. Missing-impl default behavior is **lenient**: the engine starts with `Unimplemented` placeholders that fail at first call with a clear error. A `strict_validation=true` engine option turns missing impls into startup failures.

### 9.3 Codegen contract

Per-language codegen plugins emit, for each function declaration:

1. An interface (`Functions`) with one method per declared function;
2. A default struct (`UnimplementedFunctions`) returning `(false, "not implemented")` for every method;
3. A registration helper (`RegisterFunctions(engine, impl)`) binding methods to FQNs.

Users implement the interface (typically by embedding `UnimplementedFunctions` and overriding what they use) and call the helper at startup.

This mirrors the gRPC server-stub pattern. Reference Go shape:

```go
type Functions interface {
    IsE164(value string) (bool, *Violation)
    Matches(value, pattern string) (bool, *Violation)
}

type UnimplementedFunctions struct{}
func (UnimplementedFunctions) IsE164(string) (bool, *Violation) {
    return false, &Violation{Code: "unimplemented", FallbackMessage: "is_e164: not implemented"}
}

func RegisterFunctions(eng Engine, impl Functions) {
    eng.Register("myco.commons.is_e164", impl.IsE164)
    eng.Register("myco.commons.matches", impl.Matches)
}
```

### 9.4 Engine configuration

The normative schema lives in `protowire/proto/schema/config/v1/config.proto`
(stock proto3; a build-time artifact consumed by validator binaries and
tooling, never embedded in descriptors — it allocates no extension numbers).

A project's engine selection and engine-level knobs live in a single
**`protowire.config.textproto`** file at the project root: a text-format
`protowire.schema.config.v1.EngineConfig` message. Text-format proto keeps
the no-JSON/YAML principle while avoiding the alternative of a schema-less
`.proto` file carrying configuration as file options — which would burn
carrier extension numbers and leak build configuration into
`FileDescriptorSet` artifacts.

```proto
message EngineConfig {
  string engine = 1;                       // registered identifier: "cel", "starlark", "go";
                                           //   unknown name = startup error, never a fallback
  repeated string function_libraries = 2;  // proto import paths of function-declaration files (§9.2, §9.3)
  repeated string catalog_libraries = 3;   // locale catalog sources (§7)
  bool strict_validation = 4;              // missing impls fail startup instead of first call (§9.2)
  protowire.schema.v1.ExecutionMode default_mode = 5;  // UNSPECIFIED ⇒ COLLECT_ALL (§6.4)
  uint32 max_recursion_depth = 6;          // 0 ⇒ normative default 64 (§6.4)
}
```

**Discovery.** Tools walk upward from the working directory (or an
explicitly given schema root) to the filesystem root; the *nearest*
`protowire.config.textproto` wins. There is no merging or cascading
between nested configs — nearest wins, full stop (the same model as
`go.mod`; merge semantics would reintroduce implicit inheritance).

**Precedence** (highest first):

1. Per-setting CLI flags (`--engine`, `--strict-validation`, …) override
   individual fields of the loaded config;
2. `--config <path>` selects the file explicitly, skipping discovery;
3. the `PROTOWIRE_CONFIG` environment variable — a pointer to a file
   only, never inline settings (there are no per-setting env vars);
4. the discovered `protowire.config.textproto`;
5. built-in defaults: `engine: "cel"`, lenient registration (§9.2),
   collect-all (§6.4).

## 10. Cross-language story

Server-side validation is the default and authoritative use case. Java, TypeScript, Python, etc. codegen produces typed messages and skips engine-specific validation by default — the server (a single chosen engine runtime) enforces.

For teams wanting **client-side mirror validation**, a `--strict-portability` codegen mode rejects functions that cannot be expressed identically across runtimes. Practically: rules using only inline engine-standard-library expressions are portable — including comparisons on unwrapped temporal values (§6.2 rule 2); rules referencing custom `function` declarations require each consuming runtime to register an equivalent implementation.

Multi-runtime function implementations (a Java impl alongside the Go impl for `is_e164`) are operationally expensive and out of scope for v1.2. v2.x may revisit if demand justifies.

## 11. Compatibility

- **Wire format:** Unchanged. PXF, `pb`, SBE outputs are byte-identical for v1.1 schemas.
- **Existing schemas:** Every valid v1.1 schema is a valid v1.2 schema. Brackets `[(pxf.required) = true]` continue to work and lower identically.
- **Existing tooling:** Stock `protoc`, `protobuf-go`, and every protowire port preserve the new carrier extensions as opaque options when the framework `.proto` is not imported.
- **Per-port adoption:** Each port adopts the v1.2 spec on its own schedule. Schemas using only v1.1 constructs work in any v1.1+ port. Schemas using v1.2 constructs work in v1.2+ ports only; in v1.1 ports they produce parser errors at the new keywords.
- **Versioning policy:** v1.2 is strictly additive — no existing keyword changes meaning, no extension number is reused, no grammar production is narrowed. `STABILITY.md` is updated to document the v1.2 surface.

## 12. Phasing

| M | Goal | Components |
|---|---|---|
| **M0** | Spec freeze | RFC ratified; IETF draft `-01` published; STABILITY.md updated |
| **M1** | Parser + IR (Go reference) | Extended grammar in `protocompile`; IR carries new decls (no lowering yet) |
| **M2** | Lowering + carrier | `@annot` → carrier extensions; descriptor round-trips through stock `protoc` |
| **M3** | Source map | Embedded `SourceMap`; `protolsp` consumes |
| **M4** | Engine SPI + Go runtime | Validator binary (`protocheck`) registers + executes; runtime-init verification |
| **M5** | Go codegen plugin | `Functions` / `UnimplementedFunctions`; one real end-to-end project validates |
| **M6** | i18n catalogs | Locale-driven message formatting |
| **M7** | `protolsp` + `pxfed` integration | IDE diagnostics; descriptor consumption in `pxfed` |
| **M8** | OpenAPI generator | Separate tool consuming descriptors; mappings from `@validate` shapes to OpenAPI keywords |
| **M9+** | Other ports | Java, TypeScript, Python, C++, Rust, … each adopts spec independently |

Each milestone is a vertical slice with a demoable result, not a layered "all of X then all of Y."

## 13. Open questions

Items deferred for separate resolution. Each becomes a tracked issue.

| # | Topic | Owner |
|---|---|---|
| 1 | Container-shaped type aliases (`type Tags = repeated string @validate(...)`) — v2 minor target | spec |
| 2 | ~~Engine-config file format (`engine: cel`, function-library imports)~~ **Resolved 2026-07-15** (issue #60): `protowire.config.textproto` + `proto/schema/config/v1/config.proto`, see §9.4 | spec |
| 3 | ~~Well-known types semantics (`Timestamp`, `Duration`, `Any`)~~ **Resolved 2026-07-15** (issue #61): temporal WKTs bind engine-native, `Any` never unwraps, run-stable `now()`, see §6.2 | spec |
| 4 | ~~Recursive message validation depth limits~~ **Resolved 2026-07-15** (issue #62): normative default 64, `EngineConfig.max_recursion_depth`, fail-closed `protowire.depth_exceeded` violation, see §6.4 | spec / engine |
| 5 | ~~Streaming RPC validation contract~~ **Resolved 2026-07-15** (issue #63): per-message validation, fail-closed stream termination, direction-asymmetric status mapping, see §6.6 | spec |
| 6 | ~~`Literal` shape detail in `AnnotationArg` (enum names, message literals, lists)~~ **Resolved 2026-07-15** (issue #64): resolved `EnumLiteral`, `Any` message literals, homogeneous `ListLiteral` of `LiteralValue`, see §8.1 | spec |
| 7 | ~~Validation report wire shape (`Report` carrying `EnrichedViolation`s)~~ **Resolved 2026-07-15** (issue #65): pinned in `proto/schema/v1/report.proto`, see §7 | spec |
| 8 | Migration story for existing `protovalidate`-using projects | tooling |
| 9 | Performance budget + benchmark suite | per-port |
| 10 | Conformance test fixtures in `protowire/testdata/schema-extensions/` | spec |
| 11 | Upstream `buf/protocompile` compatibility (this codebase is a fork) | protocompile |
| 12 | Stream-level validation invariants (aggregate rules across a stream's messages, ordering constraints) — deferred from §6.6, needs its own design pass like container-shaped aliases (#1) | spec |
| 13 | Sensitivity-class taxonomy (`@sensitive(class: ...)` — secret / PII / confidential) — deferred from §6.7 until a consumer needs to distinguish classes; additive parameter | spec |
| 14 | Schema-level encryption / key-reference annotation (e.g. `@encrypted(key_ref)`) and chameleon interplay — deferred from §6.7; today the schema stays orthogonal to key management | spec / chameleon |

## 14. References

- `protowire/docs/draft-trendvidia-protowire-00.txt` — current IETF draft
- `protowire/docs/draft-trendvidia-protowire-01.{md,xml,txt}` — in preparation; formal spec text for this RFC
- `protowire/STABILITY.md` — compatibility policy
- `protowire/CHANGELOG.md` — release log
- `protowire/proto/pxf/annotations.proto` — existing `(pxf.required)`, `(pxf.default)`
- `protowire/proto/schema/v1/annotations.proto` — new framework annotation library (to be added)
- `protowire/proto/schema/v1/descriptor.proto` — new lowering schemas (to be added)
- `protowire/proto/schema/v1/report.proto` — validation report wire shapes (§7)
- `protowire/proto/schema/config/v1/config.proto` — project-level engine configuration (§9.4)
- Buf's `protovalidate` — prior art for proto-native validation
- gnostic — prior art for OpenAPI-via-proto-annotations

## Appendix A — Mapping from PXF annotations to schema-extension annotations

| PXF (v1.1) | Schema extension (v1.2 canonical) | Notes |
|---|---|---|
| `[(pxf.required) = true]` | `@required` | Both forms valid; bracket retained for backward compat |
| `[(pxf.default) = "viewer"]` | `@default("viewer")` | Same; bracket retained |
| `[(buf.validate.field).cel = "..."]` | `@validate(<expression>)` | Conceptual equivalent; protovalidate-using projects migrate or use `--compat` mode (TBD) |
| n/a | `@description("...")` | Was prose comments; now structured |
| n/a | `@example(value)` | New; doubles as test fixture |
| n/a | `@error_code("...")` | New; structured error attribution |

## Appendix B — Per-port implementation status (initial)

| Port | Status |
|---|---|
| `protocompile` (Go) | Reference parser; M1–M2 candidate for first implementation |
| `protocheck` | Engine candidate; M4 candidate for runtime + Go SPI |
| `protolsp` | M3 / M7 — source-map consumer, IDE integration |
| `pxfed` | M7 — codegen consumption |
| `protowire-go` | M5 — runtime wiring through `protocheck` |
| `protowire-java` | M9+ — per-port adoption schedule TBD |
| `protowire-typescript` | M9+ |
| `protowire-python` / `cpp` / `rust` / `csharp` / `kotlin` / `swift` / `dart` | M9+ |
