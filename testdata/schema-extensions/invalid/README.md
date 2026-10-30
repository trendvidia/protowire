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
| `http_repeated_template.proto` | `http-repeated-template` | A `{name}` segment expands to a single path value, so it MUST NOT bind a repeated or map field; `google.api.HttpRule` rejects such a binding outright (RFC-001 §5.2, issue #213). |
| `http_relative_path.proto` | `http-relative-path` | An `@http` path is absolute — it lowers to a `HttpRule` pattern field matched against a request path that starts at `/` (RFC-001 §5.2, issue #213). |
| `http_unbalanced_template.proto` | `http-unbalanced-template` | Every `{` in an `@http` path opens a template segment and MUST be closed by a `}`; an unclosed brace names a variable that can be neither bound nor checked (RFC-001 §5.2, issue #213). |
| `http_empty_method.proto` | `http-empty-method` | `method` selects the `HttpRule` pattern field the route binds under, so it MUST NOT be empty — an empty verb selects no pattern and binds the method to nothing (RFC-001 §5.2, issue #213). |
| `unresolved_expression_call.proto` | `unresolved-expression-call` | A call inside an expression argument must resolve to a visible `function` declaration or to a §5.4 builtin; any other name is a compile error at the call's span (RFC-001 §5.1, §5.4, §8.1, issue #282). Until 2026-09-07 such a name was presumed an engine builtin and compiled clean (protocompile#202). |
| `expression_syntax.proto` | `expression-syntax` | An expression argument must parse under the §5.4 grammar; a fragment that balances its delimiters (so it passes §5.1 capture) but is not an expression is a compile error at the fragment's span (RFC-001 §5.1, §5.4, issue #282). |
| `non_boolean_rule.proto` | `non-boolean-rule` | A rule is a boolean: an expression argument whose result is visibly something else — here a quoted string, which is a §5.4 string literal — is a compile error at its span (RFC-001 §5.1 rule 3, §5.4, issue #437). Until then it compiled and failed every message it was evaluated against. |
| `expression_default.proto` | `expression-default` | A default on an `expression`-typed parameter is checked as an argument bound to that parameter would be, with names resolved from the annotation declaration's scope; an unresolved name in it is a compile error at the name (RFC-001 §5.1 "Expression defaults", issue #437). Until then a default was read as a literal and compiled clean. |
| `expression_name.proto` | `expression-name` | A name in an expression that is not followed by `(` is an enum value reference and must resolve to an enum value; an unresolved name, or one that resolves to something else (here a message), is a compile error at the name (RFC-001 §5.4, issue #427). |
| `required_oneof_member.proto` | `required-oneof-member` | `@required` is not valid on a oneof member: setting one member clears the others, so it would refuse every other arm (RFC-001 §5.2, issue #450). |
| `default_two_oneof_members.proto` | `default-two-oneof-members` | At most one member of a oneof may carry `@default` (RFC-001 §5.2, issue #450). |
| `default_required_oneof.proto` | `default-required-oneof` | A member of a `@required` oneof MUST NOT carry `@default` (RFC-001 §5.2, trendvidia/protocheck#22). |
| `default_repeated.proto` | `default-repeated` | One literal cannot denote a repeated field (RFC-001 §5.2, issue #450). |
| `default_map.proto` | `default-map` | One literal cannot denote a map field (RFC-001 §5.2, issue #450). |
| `default_message_field.proto` | `default-message-field` | `@default` is valid only on message types a PXF literal can denote (RFC-001 §5.2, issue #450). |
| `default_not_field.proto` | `default-not-field` | `@default` is valid only on fields; here it sits on a message (RFC-001 §5.2, issue #450). |
| `default_on_oneof.proto` | `default-on-oneof` | `@default` is valid only on fields; a oneof is not one (RFC-001 §5.2, issue #450). |

Runtime error states (missing implementation, unsatisfiable `@default`,
locale-catalog miss) are **valid** schemas whose behavior is pinned by
report goldens — see fixtures 17–19 in the parent directory.
