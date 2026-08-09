# Invalid fixtures — compile-error conformance

Every `.proto` in this directory is **deliberately invalid v1.2 schema
text**: a conformant v1.2 compiler MUST reject it with at least one
error diagnostic. Diagnostic *message text* is host-toolchain-specific
and never part of the conformance surface; what each port asserts is
(a) compilation fails, and (b) the primary diagnostic is attributable
to the error class below (by source span or by the port's own error
taxonomy).

Cross-port harnesses MUST compile these files one at a time (each file
is self-contained apart from the annotation library import) and MUST
NOT include this directory when compiling the positive corpus.

| Fixture | Error class | Violated requirement |
|---|---|---|
| `arity_mismatch.proto` | `arity-mismatch` | A call to a declared function inside an expression argument must match the declaration's arity; the linker verifies extraction-required calls (RFC-001 §8.1). This is the "invalid signature" error state of issue #68. |
| `positional_after_named.proto` | `positional-after-named` | Use-site arguments: named may follow positional; positional MUST NOT follow named (IETF draft `-01`, *Annotation Use Sites*). |
| `heterogeneous_list.proto` | `heterogeneous-list` | List literals are homogeneous — all elements share one kind (RFC-001 §8.1). |
| `unknown_literal_field.proto` | `unknown-literal-field` | A message-literal field initializer must name a field of the literal's type (RFC-001 §5.1 literal rule 2). |
| `untyped_list_element.proto` | `untyped-list-element` | A message-literal list element under an `any`-typed parameter must carry an explicit type name — no declared type is implied for it to inherit (RFC-001 §5.1 literal rule 1, issue #176). |
| `map_field_in_literal.proto` | `map-field-in-literal` | Map fields are not supported in v1.2 message literals (RFC-001 §5.1 literal rule 3, deferred). |
| `container_alias.proto` | `container-alias` | `repeated` / `map<,>` are forbidden in `typeRef`; container-shaped alias targets are deferred (RFC-001 §5.1, §6.3). |
| `cyclic_alias.proto` | `cyclic-alias` | Type-alias composition is a chain over a value-shaped base kind; a cyclic alias never reaches a base and has no data type. Diagnosed at the declaration site even when no field references the alias (RFC-001 §6.3, issue #181). |
| `unbalanced_expression.proto` | `unbalanced-capture` | Expression-argument capture requires `()`/`[]`/`{}` to balance at the argument boundary (RFC-001 §5.1, capture step). |
| `undeclared_annotation.proto` | `undeclared-annotation` | An annotation use site must resolve to a visible `annotation` declaration. |
| `reserved_sensitive_class.proto` | `reserved-sensitive-class` | `@sensitive` class names beginning with `protowire.` are reserved for future spec-defined classes; compilers MUST reject them (RFC-001 §6.7 classification rule 1, issue #111). |
| `http_unbound_template.proto` | `http-unbound-template` | Every `{name}` segment of an `@http` path binds a field of the request message. Since the skeleton lowers to a standard `google.api.http` rule, a segment that binds nothing is an unservable route, and compilers MUST reject it rather than emit it (RFC-001 §5.2, issue #213). This fixture pins only the binds-nothing class: whether a segment may name a *nested* field as a dotted path is unsettled between the reference implementations (issue #217), so no fixture asserts either answer. |

Runtime error states (missing implementation, unsatisfiable `@default`,
locale-catalog miss) are **valid** schemas whose behavior is pinned by
report goldens — see fixtures 17–19 in the parent directory.
