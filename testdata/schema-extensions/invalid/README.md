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
| `map_field_in_literal.proto` | `map-field-in-literal` | Map fields are not supported in v1.2 message literals (RFC-001 §5.1 literal rule 3, deferred). |
| `container_alias.proto` | `container-alias` | `repeated` / `map<,>` are forbidden in `typeRef`; container-shaped alias targets are deferred (RFC-001 §5.1, §6.3). |
| `unbalanced_expression.proto` | `unbalanced-capture` | Expression-argument capture requires `()`/`[]`/`{}` to balance at the argument boundary (RFC-001 §5.1, capture step). |
| `undeclared_annotation.proto` | `undeclared-annotation` | An annotation use site must resolve to a visible `annotation` declaration. |

Runtime error states (missing implementation, unsatisfiable `@default`,
locale-catalog miss) are **valid** schemas whose behavior is pinned by
report goldens — see fixtures 17–19 in the parent directory.
