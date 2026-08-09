# RFC-001 — Protowire Schema Extensions

| Field | Value |
|---|---|
| Status | **Ratified** (2026-07-16, issue #56) |
| Target spec version | protowire v1.2.0 (minor, strictly additive) |
| IETF draft | Companion to `draft-trendvidia-protowire-01` (in preparation) |
| Authors | TrendVidia |
| Created | 2026-06-04 |
| Last updated | 2026-07-16 |

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

engineExpression
    ::= exprSourceText    (* opaque balanced-token capture; see "Engine expressions" below *)

literal        ::= scalarLit | listLiteral | messageLiteral
scalarLit      ::= strLit | intLit | floatLit | boolLit   (* proto lexical forms *)
literalValue   ::= literal | qualifiedIdent               (* qualifiedIdent = enum-value reference *)

listLiteral    ::= "[" (literalValue ("," literalValue)*)? "]"

messageLiteral ::= qualifiedIdent? "{" (fieldInit ("," fieldInit)*)? "}"
fieldInit      ::= Ident ":" literalValue
```

Placement is by production: trailing on `typeDecl`, `field`, `functionDecl`, `enumValue`; leading on `messageDecl`, `enumDecl`, `serviceDecl`, `rpcDecl`, `oneofDecl`.

**Literal values.** `@example(myco.commons.Money{currency: "USD", units: 5})`
is the canonical message-literal spelling. Normative rules:

1. **Type name presence follows the §8.1 typing rule.** When the
   annotation param — or, recursively, the message field being
   initialized — is `any` / `google.protobuf.Any`, the explicit leading
   type name is REQUIRED. When a concrete message type is declared, the
   name is OPTIONAL; if present it MUST resolve to exactly the declared
   type. The type is never inferred from the value's shape. The name
   resolves like any type reference (a bare in-scope name binds).
2. **One way to write field initializers**: `fieldName: value` (the
   field's proto name), comma-separated, no trailing comma, each field
   at most once; an unknown field name is a compile error. Textformat
   freedoms are deliberately NOT adopted: no colon-less nested blocks,
   no semicolon/newline separators, no repeated-field-by-repetition,
   no `[type.url]` bracket form.
3. **Repeated fields** take `listLiteral` values (`tags: ["a", "b"]`).
   **Map fields are not supported** in v1.2 message literals (compile
   error; deferred).
4. **Enum-typed fields and list elements** take a `qualifiedIdent`
   (bare or qualified value name), linker-resolved into an
   `EnumLiteral` (§8.1) — same as at the argument level. Lists are
   homogeneous; nesting is legal; `[]` and `{}` are legal; expressions
   never appear inside literals.
5. `strLit` serves `bytes`-typed params (proto's bytes default-value
   convention); `intLit`/`floatLit` take an optional leading `-`.
6. Parse note: at `annotArgValue`, a `qualifiedIdent` followed by `{`
   is a `messageLiteral`; a bare `qualifiedIdent` is an enum-value
   reference — one-token lookahead. Arguments bound to
   `expression`-typed params keep the opaque balanced-text capture
   (see *Engine expressions* below).

**Engine expressions.** v1.2 validation is **annotation-only**: engine
expressions appear exclusively as annotation arguments bound to
`expression`-typed parameters (`@validate(this in ["US", "CA"])`). There
is no standalone expression construct at any other position in the
grammar. Because an argument's parameter binding is not known until the
linker resolves the annotation declaration, argument parsing is
**capture-then-classify**:

1. **Capture.** The compiler captures every annotation argument as raw
   source text. An argument extends to the first `,` or `)` at zero
   delimiter depth — `()`, `[]`, and `{}` pairs must balance, and string
   literals (proto lexical forms) are opaque to the scan, so delimiters
   and commas inside them do not count. The scan is character-level: it
   tracks only string-literal boundaries and delimiter depth, so engine
   operators that are not protobuf tokens (`||`, `&&`, `<=`, `!`) pass
   through unexamined. An unbalanced delimiter or an unterminated
   string literal is a compile error reported at the argument. The
   capture must be non-empty.
2. **Named-argument recognition.** An argument that begins
   `Ident "="` — where the token after `=` is not itself `=` — is a
   named argument; the name and `=` are consumed before capture.
   Expression fragments are unaffected: `code == "x"` begins with a
   `==` and stays a positional capture.
3. **Classify.** At link time, an argument bound to an
   `expression`-typed parameter keeps the captured source **verbatim**
   (leading/trailing whitespace trimmed; quotes are NOT stripped — a
   quoted string is a string-literal expression and evaluates as such
   in the engine, which is almost certainly a type error there). Every
   other argument MUST re-parse under `literal | qualifiedIdent`; any
   other shape is a compile error.

The fragment's inner syntax belongs to the engine (§9): the compiler
never interprets it beyond the tokenization used for function-call
extraction (§8.1).

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
annotation sensitive(class: string = "");
annotation http(
  method: string,
  path: string,
  summary: string = "",
  operation_id: string = "",
  tags: any = [],
  security: any = []
);
```

**`@http` and the operation surface.** `method` and `path` are the routing skeleton; the remaining parameters carry the operation metadata a REST-surface generator needs (issue #173). `path` may contain `{…}` template segments, each binding a field of the request message named **from its top level as a dotted path** (`{order_id}`, `{shelf.id}`); remaining request fields bind to the query string for bodyless methods and to the request body otherwise. `summary` falls back to the first sentence of `@description`; `operation_id` is derived as `<Service>_<Method>` when empty; `tags` and `security` take list literals of strings (§8.1 `Literal.list`), and the security-*scheme definitions* they name are generator configuration rather than schema content — the same §9.4 argument that keeps engine configuration out of file options and that rejected `@encrypted` (§6.7).

**The routing skeleton lowers twice.** `method` and `path` MUST lower both to the §8.1 annotation carrier *and* to the standard `google.api.http` option on `MethodOptions` (field `72295728`, `google.api.HttpRule`). The two are complements, not alternatives: the carrier keeps the whole operation surface — `summary`, `operation_id`, `tags`, `security` — which `HttpRule` has no place for, while the standard extension carries the skeleton that every off-the-shelf REST binder reads (connect vanguard, grpc-gateway, Envoy's `grpc_json_transcoder`, buf's OpenAPI plugins). Emitting only the carrier is the failure this rule exists to prevent, and it is a silent one: the binder finds no rules, binds no routes, reports nothing, and every REST URL 404s as though the endpoint were unimplemented (issue #213).

The transform is mechanical, because `@http`'s model is already 1:1 with `HttpRule`:

| `@http` | `HttpRule` |
|---|---|
| `method` ∈ {`GET`,`PUT`,`POST`,`DELETE`,`PATCH`}, case-insensitive | the same-named pattern field |
| any other `method` | `custom = CustomHttpPattern{kind: <verb, upper-cased>, path}` |
| `path` | the pattern field's value, verbatim including `{name}` segments |
| bodyless method (`GET`/`HEAD`/`DELETE`/`OPTIONS`) | no `body` — unbound fields bind to the query string |
| any other method | `body: "*"` — every field the path template did not bind |
| several `@http` use sites on one method | first is the rule; the rest are `additional_bindings`, in source order |

**Several bindings on one method.** A method MAY carry several `@http` use sites — the shape that expresses a versioned or aliased route for one RPC. Each is a binding, and a REST-surface renderer emits **one operation per binding**: describing only the first would document less than the image binds, which is the dual-lowering failure above (issue #213) pointed the other way (issue #215). Bindings after the first MUST name their own `operation_id`. The derived `<Service>_<Method>` spelling is document-unique *by construction* only while a method has one binding; an index-derived alternative (`…_2`) would rename a generated client's method whenever two annotation lines are reordered, so the name is authored instead. A renderer MUST also reject two operations claiming one `operation_id`, which repetition makes reachable and which nothing checked while the construction argument held. Both obligations are the **renderer's**, not the compiler's: `operation_id` is documentation metadata, and a port that renders no REST surface parses it and interprets nothing. Conformance fixture: `testdata/schema-extensions/22_http_additional_bindings.proto`.

`selector` and `response_body` are never written: the rule is attached to its method, and responses are derived (below). A method that already carries an author-written `(google.api.http)` keeps it unchanged — the compiler MUST NOT add a second, competing rule; because the annotation's own path still reaches the §8.1 carrier, the two spellings can then describe different routes, and a compiler SHOULD warn at the use site (the reference compiler does). A compiler MAY offer an opt-out (the reference CLI spells it `pxf build --google-api-http=false`); emission is the default.

The emitted option adds **no import** to the lowered file. Like the §8.1 carriers, it rides in the options message's unknown-field bytes, so the descriptor set stays self-contained and resolvable by stock `protodesc`; consumers that link `google.api` resolve it through their own type registry, exactly as they do for a `protoc`-produced descriptor.

Because the skeleton is lowered rather than merely carried, it is also **checked**: a `{name}` segment that binds no field of the request message is a compile error, as are a template segment binding a repeated or map field, a non-absolute path, unbalanced template braces, and an empty `method`. Each of those would otherwise produce a rule that no binder can serve, and each has its own conformance fixture: `invalid/http_unbound_template.proto`, `http_repeated_template.proto`, `http_relative_path.proto`, `http_unbalanced_template.proto`, `http_empty_method.proto`.

**Template grammar** (issue #217). A segment's variable is a **dotted field path from the top level** of the request message: each component names a field of the message the previous one resolved to, no component may descend through a scalar, and no component may be `repeated` — a repeated field has no single value a path segment could carry. A variable MAY additionally be constrained by the `HttpRule` sub-path form `{name=shelves/*}`; the constraint belongs to the routing skeleton and reaches `google.api.http` verbatim.

A REST-surface renderer normalises only what its own format cannot express: OpenAPI has no `{name=pattern}` template form, so the **path key** drops the constraint and keeps the variable (`/named/{name}`), while a dotted variable carries through unchanged because OpenAPI parameter names admit dots (`/shelves/{shelf.id}`). A nested binding removes **only the leaf it names** from the remaining-field binding: the leaf's siblings still bind, under their dotted names, exactly where an `HttpRule` consumer places them (`shelf.display_name` in the query string beside a path-bound `{shelf.id}`; in the body, the container is carried minus that leaf). Descending into a container exposes that container's own message-typed members to the binding, and one with nothing bound below it is refused like any other message-typed field — a renderer flattens where a bound leaf forces it to, not everywhere. A binding that descends *through* a self-referential type has no finite flattening and MUST be refused rather than truncated; the request message is itself on that descent, so the rule does not begin one level down (issue #218). Flattening also dissolves the reference a container's own `@sensitive` marker would have sat on, so the §6.7 minima MUST travel with it: a leaf lifted out of a sensitive container is a sensitive declaration wherever it lands — path parameter, query parameter, or inlined body property — and an inlined container carries its own markers on the object that replaces the reference. Conformance fixture: `testdata/schema-extensions/23_http_template_paths.proto`.

**What the grammar above still leaves unsettled.** The dotted-path and sub-path *shapes* now mean the same thing at both ends of the toolchain, but three edges of that agreement are open, and a port MUST NOT read any of them as settled spec. Each is stated where it is, rather than resolved by whichever end was written last:

- *A message-typed leaf.* The grammar constrains interior components only, so `{mid.inner}` — and equally a bare top-level `{mid}` — compiles and lowers when `inner` is message-typed, and the reference renderer then refuses it: a path parameter has no flat encoding for a message. Either the compiler rejects a non-value-shaped leaf (a sixth source-level narrowing, on top of the five above) or the renderer acquires a spelling for one. The compile-and-bind-then-refuse shape is unchanged from what issue #217 describes; only its reach shrank.
- *Two bindings differing only in a constraint.* `{name=shelves/*}` and `{name=shelves/*/books/*}` are distinct routes to a binder — the canonical shape for a nested resource — but normalise to the same OpenAPI path key, so the reference renderer reports a duplicate operation and refuses the pair. Rendering both would need the pattern expanded into distinct keys (`/v1/shelves/{shelf}` and `/v1/shelves/{shelf}/books/{book}`), which is a template-rewriting scheme the renderer has nowhere else and which has to decide what the expanded variables are named.
- *The constraint reaches no reader.* Dropping the pattern from the path key drops it from the document entirely: the rendered parameter is an unconstrained string, and OpenAPI's default `style: simple` percent-encodes the `/` that a `shelves/*` value must contain. A JSON Schema `pattern` on the parameter would carry the constraint faithfully (`*` is one segment, `**` is zero or more), but it does not fix the encoding, so a document that claims a multi-segment path parameter is describing something OpenAPI cannot express either way.

Operation *metadata* carries **no validation semantics**. A port that renders no REST surface parses `summary`, `operation_id`, `tags` and `security` like any other annotation arguments and interprets nothing; conformance requires carrying them through the §8.1 carrier, not acting on them. Responses are derived rather than authored — the success response from the method's return type, error responses from `@error_code` plus the §7 report model — so `@http` has no `responses` parameter (rationale: `docs/RFC-001-issues.md` §#080).

Every parameter beyond `method` and `path` is defaulted, so the v1.2.0 two-argument form keeps its meaning and no existing schema changes shape. Conformance fixture: `testdata/schema-extensions/21_http_operation.proto`.

The existing PXF annotations `(pxf.required)` and `(pxf.default)` retain their bracket forms and extension numbers (`50000`, `50001`): bracket-written options remain valid v1.2 input and lower identically to v1.1. `@required` and `@default(value)` are the canonical annotation form going forward, and they lower **exclusively** to the schema-extension carrier (§8.1) — the annotation forms never emit the legacy options (§8.5). A consumer that reads only `(pxf.required)`/`(pxf.default)` observes bracket-written options and nothing else; enforcing the annotation forms requires a carrier-aware (v1.2) consumer.

`@default(value)` inherits the placement constraint the draft states for `(pxf.default)` (draft `-01` §annotation-extensions, "Default Placement"): the annotation carries one literal, so it is valid only on singular scalar fields, enum fields, and the message types a PXF literal can denote — never on a `repeated` field, a `map<K,V>` field, a group, or any other message type. The constraint is on the annotation, not on the surface that writes it, so the compiler rejects a misplaced `@default(value)` at the use site with the same force a PXF binding tool rejects a misplaced bracket option. Note this is the one place `@default` and `@validate` diverge in their treatment of collections: §6.4's per-element rule makes `@validate` meaningful on a `repeated` or `map<K,V>` field, whereas `@default` has no per-element reading — a single literal cannot denote a collection, and the annotation grammar has no element or key separator to give it one.

Both `@default(value)` and `@required` also inherit the oneof-member constraints (draft `-01` §annotation-extensions, "Oneof Members"): `@default(value)` on a oneof member applies only when no member of that oneof is present, at most one member of a oneof may carry it, and `@required` is not valid on a oneof member at all. The presence model of §6.1 is defined per field, and inside a oneof that reading does not hold — setting any member clears the rest, so a member is "absent" whenever a sibling was chosen. `@required`'s only coherent reading there is a property of the oneof, for which this revision defines no annotation scope.

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
    @validate(this in ["US", "CA", "GB"], code = "user.invalid_country", message = "country not supported");
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

This eliminates the proto3 zero-value ambiguity in the validation layer: validation runs only on values the producer meant to set. `@required` is the separate "must be present" lever, orthogonal from "if present, must match." Absence of a `@required` field reports `code: "protowire.required"` with the spec-pinned fallback message (§7).

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

`repeated` and `map<K,V>` validate per-element (using the element's type rules) plus any field-level `@validate` against the collection as a whole. For `map<K,V>`, per-element covers **both dimensions**: entry values validate against the value type's rules, and entry keys against the key type's rules (carried on the synthetic entry's `key` field by the §8 lowering). Key violations set `EnrichedViolation.for_key` (§7), with the path's map subscript addressing the entry as usual.

**Default substitution.** When an absent field carries `@default(value)`,
the default substitutes and the field's rules run against the substituted
value (§6.1). Every violation those rules produce MUST carry
`rule_kind: RULE_KIND_DEFAULT` — superseding the `VALIDATE` or
`TYPE_REFINEMENT` kind the same rule would carry for a producer-set value
— and `actual_value` MUST be the substituted default. The distinct kind
marks the failure as a **schema-authoring error**: the declared default
itself fails the field's rules, so no producer input can trigger it and
no producer change can fix it. Rule origin stays recoverable from
`cause.code` and `type_chain`. Tooling MAY additionally reject such
defaults at compile time (the §5.2 annotation library documents the best
practice); a schema that passes that check never yields
`RULE_KIND_DEFAULT` at runtime.

**Recursion depth.** Nested-message validation is depth-limited. The root
instance is at depth 0; entering any message-typed value (a nested field,
a repeated element, a map value) increments the depth by 1 — scalars and
scalar-collection elements do not. The limit is
`EngineConfig.max_recursion_depth` (§9.4); `0` means the **normative
default of 64**. When a value at depth greater than the limit would need
validating, the engine does **not** descend: it records one synthetic
violation for the subtree — `code: "protowire.depth_exceeded"`, the
spec-pinned fallback message (§7), `path` at
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
descriptor surface exists.

**Classification parameter.** `@sensitive` takes one optional
parameter, `class: string = ""` (issue #111). The vocabulary is open
and org-defined (`"credentials"`, `"pii"`, `"payment"`, …):
sensitivity taxonomies are organizational policy, not cross-port
interop, and protection-layer consumers (e.g. the chameleon editor's
key management) map class names to key domains in their own
runtime-layer configuration. The spec pins only the mechanics:

1. **Reserved prefix.** Class names beginning with `protowire.` are
   reserved for future spec-defined classes, mirroring the §7
   violation-code reservation; compilers MUST reject them.
2. **One class per field.** `class` is a single string, never a list:
   consumers that route protection by class (field → class → key
   domain) need that resolution to be deterministic. Orgs needing
   intersections define composite classes.
3. **Effective class.** Where sensitivity arrives from several sites
   (field, type-alias chain, message), a field's *effective class* is
   the class of the **nearest `@sensitive` that specifies one**: the
   field's own annotation first, then the alias chain most-derived
   first, then the message-level marker. A bare `@sensitive` (or an
   explicit `class = ""`) never erases an outer class — it reasserts
   sensitivity without reclassifying.
4. **Minima are class-invariant.** Every consumer minimum above
   applies identically to every class, including `""` (sensitive but
   unclassified). Classes add routing granularity in consumers that
   opt in; they never weaken the redaction floor.

**No protection metadata in the schema.** A schema-level key-reference
annotation (`@encrypted(key_ref)`) was considered and rejected (issue
#112). Key references, algorithms, and rotation state are deployment
topology: they vary per environment and per tenant while the data's
meaning is unchanged, and annotations lower into `FileDescriptorSet`
artifacts (§8.1) that are committed, embedded, and shipped across
organizational boundaries — the same reasoning that keeps engine
configuration out of file options (§9.4). The sanctioned contract is
split: the schema declares *what* is sensitive and *which class* it
belongs to; the protection layer (PXF / chameleon) maps class → key
domain in its own configuration, so key rotation and per-environment
key topology never touch the schema.

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
  bool for_key = 8;                       // rule violated by the map key, not the entry's value
  SourceRef source_ref = 9;               // §8.3.1 rule join key, when a source map resolved the rule
}

message SourceRef {
  string file = 1;                        // SourceMap.file of the resolving map
  string descriptor_path = 2;             // canonical §8.3.1 descriptor path
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

`RULE_KIND_DEFAULT` marks violations whose rule was evaluated against a
`@default`-substituted value (§6.4): a schema-authoring error — the
declared default fails the field's own rules — rather than an instance
error.

A subscripted map segment addresses the entry's **value**. When a rule is
evaluated against the map key itself, the engine MUST set
`for_key = true` on the enriched violation: a key violation and a value
violation on the same entry otherwise serialize to identical paths, and
neither `RuleKind` nor `code` is required to disambiguate them. The
alternative — appending a pseudo-segment for the entry's synthetic
`key = 1` field — is rejected: the preceding subscripted segment already
addresses the value, so `labels[k].key` would collide with a genuine
`key` field on a message-typed map value.

`source` and `source_ref` carry complementary provenance: `source` is a
resolved position for display; `source_ref` is the rule's **identity** —
the §8.3.1 join key `(SourceMap.file, descriptor_path)`, rendered by the
single shared formatter, never hand-assembled. Engines MUST populate
`source_ref` whenever the violated rule was resolved through an embedded
source map (extension 50404) and MUST leave it unset otherwise.
Consumers correlating a wire Report with lowered rules — an editor
overlay mapping runtime violations onto source ranges, a registry
joining reports to stored descriptors — join on `source_ref` rather than
fuzzy-matching `source` positions. Both fields are deterministic from
the descriptor, so they participate in cross-port report equality
(goal 5) with no carve-out.

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

**Catalog sources.** A locale catalog is data, not schema —
translator-maintained strings loaded at engine init, never compiled
into descriptors. The source format for a catalog referenced from the
§9.4 `catalog_libraries` is a **text-format
`protowire.schema.catalog.v1.Catalog` message** (normative schema
`protowire/proto/schema/catalog/v1/catalog.proto` — stock proto3 with
the same artifact status as the §9.4 engine config: consumed by
validator binaries and tooling, never embedded in descriptors,
allocates no extension numbers):

```proto
message Catalog {
  string locale = 1;                // BCP 47 tag; the RegisterCatalog key (§9.1)
  map<string, string> entries = 2;  // violation code → message template
}
```

```textproto
# myco/i18n/de.textproto — text-format protowire.schema.catalog.v1.Catalog
locale: "de"
entries { key: "string.min_len"     value: "mindestens {min_len} Zeichen erforderlich" }
entries { key: "users.email.format" value: "keine gültige Adresse (Muster {pattern})" }
```

Normative rules:

1. **One locale per file.** `locale` is a non-empty BCP 47 tag and is
   the `RegisterCatalog` key (§9.1). Multiple `catalog_libraries`
   entries MAY declare the same locale (per-domain catalogs); loaders
   merge them into the single per-locale catalog before registration.
   The same code in two files for one locale is a load error — never a
   silent override. (Within one file, text format already rejects
   duplicate map keys.)
2. **Template interpolation.** `{name}` placeholders interpolate the
   violation's `params`; a placeholder with no matching param — and any
   brace that does not form a placeholder — passes through verbatim, so
   a missing translation surfaces visibly rather than silently
   vanishing. No escape syntax is defined. A catalog miss falls back to
   `fallback_message`, as above.
3. **Path resolution.** `catalog_libraries` values are filesystem paths
   resolved relative to the directory of the config file that declares
   them (absolute paths are taken as-is). They are **not** proto import
   paths — contrast `function_libraries`, whose declarations compile
   into the image (§9.4). Build tools do not compile or embed catalogs;
   validator binaries and editors load them when the engine is
   configured.
4. **Locale negotiation** — matching a consumer's requested locale
   (for example an LSP client's `InitializeParams.locale`) against
   registered catalogs — is consumer policy; the spec pins only
   registration keyed by the file's `locale`.

Plural, gender, and ICU-MessageFormat-style template forms are
deferred (§13).

`@validate(...)` accepts optional `code` and `message` to override defaults at use sites.

**Params provenance.** `cause.params` is populated from exactly two
sources: the `Violation` returned by a declared function (§6.5) — the
function implementation authors its params — and spec-defined synthetic
violations (`protowire.depth_exceeded` carries `{limit}`, §6.4). For
violations produced by **inline expression rules**, engines MUST leave
`params` empty: expressions are opaque engine source (§5.1), and no
mapping from operator shapes to parameter names could be normative
across engine languages. A rule that needs structured params for
catalog interpolation declares a `function`. (`@validate`'s use-site
`code`/`message` overrides are author-supplied and unaffected.) With
provenance pinned, `params` participates in cross-port report equality
(goal 5) with no carve-out.

Violation codes beginning with **`protowire.`** are reserved for
spec-defined violations; user rules and function implementations MUST NOT
mint codes in that namespace. This revision defines four:
`protowire.required` (a `@required` field is absent, §6.1),
`protowire.depth_exceeded` (recursion depth limit reached, §6.4),
`protowire.function.invalid_argument` (a generated registration
adapter's arity or argument-type guard failed before the user's
function implementation was invoked, §9.3), and
`protowire.function.unimplemented` (a declared function was invoked
with no registered implementation: the §9.2 lenient placeholders and
the §9.3 `UnimplementedFunctions` stubs). The `protowire.function.*`
pair is minted by spec-mandated codegen and engine machinery, never
by user code — a function implementation that wants to reject an
argument returns its own code. Runtimes MUST
expose the reserved codes as typed constants in their host language,
and spec-mandated codegen MUST reference those constants rather than
string literals.

**Reserved-code fallback messages.** A spec-defined violation has no
schema author, so cross-port report equality (goal 5) on its
`fallback_message` requires the string to come from the spec itself —
the same argument that makes engine-synthesized messages for inline
rules non-normative. Each reserved code therefore carries a
spec-pinned `fallback_message`:

| Code | `fallback_message` |
|---|---|
| `protowire.required` | `field is required` |
| `protowire.depth_exceeded` | `recursion depth limit exceeded` |
| `protowire.function.unimplemented` | `<function>: not implemented` |
| `protowire.function.invalid_argument` | `<function>: expected <n> argument(s)` (arity guard) or `<function>: argument <i> is not <type>` (argument-type guard, §9.3) |

The first two are static — the effective limit already travels in
`params.limit` (§6.4); where a param carries the datum, the fallback
string stays static. In the templates, `<function>` is the declared
function's fully-qualified name, `<n>` and `<i>` are decimal integers
(`<i>` zero-based), and `<type>` is the parameter type **as declared
in the schema** (§6.5 signature types, e.g. `string`, `int64`, a
message FQN) — never a host-language type name, which would break
cross-port equality. Runtimes MUST expose the static strings (and
template helpers for the `protowire.function.*` pair) alongside the
code constants, and spec-mandated codegen MUST reference them.

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
spelling of message and list literals is defined by the `literal`
production in §5.1.

**Expression lowering.** `Expression.source` is the §5.1 capture,
verbatim. `Expression.calls` is populated by the compiler — extraction
is REQUIRED, not best-effort: the captured fragment is scanned for
qualified identifiers and string literals under the proto lexical
rules (characters that cannot begin or extend a proto token — engine
operators like `||` or `<=` — act solely as separators), and every
maximal `qualifiedIdent "("` occurrence is a call site whose arity is
its count of top-level commas plus one (zero for empty parentheses). Call sites whose name resolves to a
visible `function` declaration (same file or imported) are recorded in
`calls`, and the linker verifies the arity against the declaration —
a mismatch is a compile error with the call's source span. Names that
do not resolve are presumed engine builtins (`now()`, `this.size()` —
`this.`-prefixed paths can never resolve to a declaration): they are
not recorded and not diagnosed; missing-implementation handling for
them stays with the engine's init-time verification (§9.2). Consumers
(engine init walks, source-map `callAnchor`s in §8.3.1) may therefore
rely on `calls` being complete with respect to declared functions.

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

**Legacy PXF options — no dual-emission.** The annotation forms
`@required` and `@default(value)` lower only to the `50400` carrier;
the compiler MUST NOT synthesize `(pxf.required)` (`50000`) or
`(pxf.default)` (`50001`) from them. The two surfaces are disjoint:
brackets lower to the legacy options exactly as written, annotations
lower to the carrier, and neither is back-filled from the other. The
consequence is deliberate: a v1.1 consumer of the legacy options (for
example, a PXF runtime that enforces `(pxf.required)` at decode time)
does not see `@required`/`@default` — migrating a schema from brackets
to annotations transfers enforcement to carrier-aware consumers, so
runtimes upgrade **before** schemas migrate. Both forms of the same
semantic MAY coexist on one field during migration; compilers MAY warn
when the two carry conflicting values, but MUST NOT reconcile them.

**`SourceCodeInfo` is authorial — no synthesized comments.** The
lowering pass MUST NOT inject annotation-derived text (for example
`@description`) into `SourceCodeInfo` leading or trailing comments,
nor otherwise synthesize `SourceCodeInfo` entries: `SourceCodeInfo`
carries exactly what the user wrote. Annotation-derived documentation
is surfaced by annotation-aware codegen reading the `50400` carrier
(the §9.3 plugins and per-port equivalents), which is the canonical
— and only — layer for documentation emission. The consequence is
deliberate: stock plugins and documentation generators that render
only `SourceCodeInfo` comments do not surface `@description`; a port
that wants annotation-derived documentation in generated code
requires an annotation-aware plugin, not a compiler that rewrites
the source record.

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

Functions referenced in the descriptor must be registered with the engine at startup. The engine walks the descriptor on init and verifies each FQN is present in its registry. Missing-impl default behavior is **lenient**: the engine starts with `Unimplemented` placeholders that fail at first call with `protowire.function.unimplemented` (§7). A `strict_validation=true` engine option turns missing impls into startup failures.

### 9.3 Codegen contract

Per-language codegen plugins emit, for each function declaration:

1. An interface (`Functions`) with one method per declared function;
2. A default struct (`UnimplementedFunctions`) returning `(false, <the pinned protowire.function.unimplemented violation, §7>)` for every method;
3. A registration helper (`RegisterFunctions(engine, impl)`) binding methods to FQNs.

Users implement the interface (typically by embedding `UnimplementedFunctions` and overriding what they use) and call the helper at startup.

This mirrors the gRPC server-stub pattern. The helper adapts each
typed method to the untyped §9.1 `Function` signature: generated
guards check arity and argument types before invoking the
implementation, emitting `protowire.function.invalid_argument` (§7,
via the runtime's typed constant) on mismatch, and `Register` errors
are propagated. Reference Go shape:

```go
type Functions interface {
    IsE164(value string) (bool, *Violation)
    Matches(value, pattern string) (bool, *Violation)
}

type UnimplementedFunctions struct{}
func (UnimplementedFunctions) IsE164(string) (bool, *Violation) {
    return false, &Violation{Code: CodeFunctionUnimplemented,
        FallbackMessage: MsgFunctionUnimplemented("myco.commons.is_e164")} // "myco.commons.is_e164: not implemented"
}

func RegisterFunctions(eng Engine, impl Functions) error {
    if err := eng.Register("myco.commons.is_e164", func(args []any) (bool, *Violation) {
        if len(args) != 1 {
            return false, &Violation{Code: CodeFunctionInvalidArgument,
                FallbackMessage: MsgFunctionArity("myco.commons.is_e164", 1)} // "myco.commons.is_e164: expected 1 argument(s)"
        }
        a0, ok := args[0].(string)
        if !ok {
            return false, &Violation{Code: CodeFunctionInvalidArgument,
                FallbackMessage: MsgFunctionArgType("myco.commons.is_e164", 0, "string")} // "myco.commons.is_e164: argument 0 is not string"
        }
        return impl.IsE164(a0)
    }); err != nil {
        return err
    }
    // matches: same adapter shape
    return nil
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
  repeated string catalog_libraries = 3;   // paths to locale catalog files (§7),
                                           //   text-format catalog.v1.Catalog,
                                           //   resolved relative to this config file
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
| 8 | ~~Migration story for existing `protovalidate`-using projects~~ **Resolved 2026-07-24** (issue #66): adapter-first migration, see Appendix C; no compiler compat mode, no in-place rewriter | tooling |
| 9 | Performance budget + benchmark suite | per-port |
| 10 | Conformance test fixtures in `protowire/testdata/schema-extensions/` | spec |
| 11 | Upstream `buf/protocompile` compatibility (this codebase is a fork) | protocompile |
| 12 | Stream-level validation invariants (aggregate rules across a stream's messages, ordering constraints) — deferred from §6.6, needs its own design pass like container-shaped aliases (#1) | spec |
| 13 | ~~Sensitivity-class taxonomy (`@sensitive(class: ...)`)~~ **Resolved 2026-07-25** (issue #111): additive `class: string = ""` parameter — open org-defined vocabulary, `protowire.` prefix reserved, single class, effective-class rule; see §6.7. The consumer that triggered the deferral's "until needed" clause is the chameleon editor's key management | spec |
| 14 | ~~Schema-level encryption / key-reference annotation (`@encrypted(key_ref)`)~~ **Rejected 2026-07-25** (issue #112): protection metadata never enters the schema — key refs are deployment topology and would leak through descriptor artifacts (§8.1, same reasoning as §9.4); the class → key-domain mapping lives in the protection layer's configuration, see §6.7 | spec / chameleon |
| 15 | Plural / gender / ICU-MessageFormat template forms in locale catalogs (`catalog.v1.Catalog` templates are plain `{param}` interpolation for now, §7) — revisit when a consumer needs plural rules | spec |

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
| `[(pxf.required) = true]` | `@required` | Both forms valid; disjoint lowering (§8.5) — the annotation form is carrier-only, so legacy-option consumers see only the bracket |
| `[(pxf.default) = "viewer"]` | `@default("viewer")` | Same; migrate consumers to the carrier before migrating schemas |
| `[(buf.validate.field).cel = "..."]` | `@validate(<expression>)` | Conceptual equivalent; migration path in Appendix C — a `--compat` compiler mode was rejected (#66) |
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

## Appendix C — Migrating from protovalidate

Projects using `buf.validate` options migrate in three phases; each
phase is independently shippable and per-file incremental. (Resolved
2026-07-24, issue #66. The `--compat` compiler flag contemplated at
ratification was rejected — `buf.validate` options are ordinary custom
options in stock proto3, which protowire tooling already parses and
round-trips opaquely per §8.5, so there is no parse-level
incompatibility to bridge; compatibility during transition is a
runtime concern and the Phase 0 adapter is that answer. An in-place
rewriter (`pxf migrate-validate`) is not built: Phase 0 removes
migration urgency, and the Phase 1 mapping below is mechanical enough
that a rewriter can be added later without spec change if demand
appears.)

**Phase 0 — adapter, zero schema change.** Keep `buf.validate` schemas
as-is and validate at the protowire seam via
`github.com/trendvidia/protocheck/protovalidate`
(`pxf.UnmarshalOptions{Validator: v}`). Violation rule IDs are
namespaced `buf.validate.*`.

**Phase 1 — per-file rewrite.** Under the default `cel` engine (§9.4),
expressions carry over verbatim:

| protovalidate form | protowire v1.2 form | Notes |
|---|---|---|
| `(buf.validate.field).cel = {id, message, expression}` | `@validate(<expression>, code = <id>, message = <message>)` | Same `this` binding: field value, wrapper unwrap, native temporals (§6.2). `id` must not use the reserved `protowire.` prefix (§7). |
| `(buf.validate.message).cel` | leading `@validate` on the message | `this` binds to the message in both systems. |
| `(buf.validate.field).required = true` | `@required` | **Semantic delta:** protovalidate rejects zero values on implicit-presence scalars; protowire `@required` checks *presence* only (§6.1) and null counts as present. Fields relying on zero-rejection need an explicit rule (e.g. `@validate(this != "")`) or explicit presence. |
| Standard rules (`string.min_len`, `int32.gt`, …) | `@validate` with the equivalent stdlib expression; shared shapes become `type` aliases (`type NonEmptyString = string @validate(this.size() >= 1)`) | Type aliases (§6.3) are the idiomatic replacement for rule sets repeated across fields. |
| `ignore` / zero-value knobs | none needed | protowire never evaluates rules on unset fields (§6.1); the knob's job disappears. |

**Phase 2 — retire.** Drop `buf/validate/validate.proto` imports and
the adapter dependency; the protocheck engine is the single validator.

**Coexistence (normative).** protowire engines MUST NOT interpret
`buf.validate` options; they are foreign custom options preserved
opaquely (§8.5). During transition both validators MAY run at the same
seam; a field carrying both forms is validated by both, and reports
remain distinguishable by rule-ID namespace (`buf.validate.*` never
collides with `protowire.*` or schema-authored codes). Migrate
file-at-a-time rather than rule-at-a-time to avoid double-maintenance.
