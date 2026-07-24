# Schema Extensions Test Fixtures

Conformance fixtures for the Protowire Schema Extensions (v1.2.0) specified
in [`docs/RFC-001-schema-extensions.md`](../../docs/RFC-001-schema-extensions.md)
and [IETF draft `-01`](../../docs/draft-trendvidia-protowire-01.md).

These fixtures serve three purposes:

1. **Cross-port conformance.** Every port adopting the v1.2 spec runs the
   same fixtures and asserts identical parse, lowering, and validation
   behavior. Cross-port equivalence is checked in CI via the existing
   harness pattern (see `scripts/cross_envelope_check.sh` for the
   analogous PXF/SBE conformance run).
2. **Documentation by example.** New users browse the fixtures to learn
   the syntax in concrete form. Each file is short, self-contained, and
   illustrates one or two specific constructs.
3. **Implementation targets.** Issues #030–#035 in
   [`docs/RFC-001-issues.md`](../../docs/RFC-001-issues.md) reference
   these fixtures as the round-trip targets for parser, lowering, and
   source-map work in `protocompile`.

## Layout

```
testdata/schema-extensions/
├── README.md                                — this file
├── 01_basic.proto                           — minimal declarations of each new kind
├── 02_composition.proto                     — chained type aliases
├── 03_message_and_field_annotations.proto   — leading vs trailing placement
├── 04_required_and_default.proto            — @required / @default + bracket coexistence
├── 05_error_overrides.proto                 — @validate with code + message overrides
├── 06_cross_file_lib.proto                  — library imported by 06_cross_file_main.proto
├── 06_cross_file_main.proto                 — uses types/functions from 06_cross_file_lib
├── 07_report_golden.textproto               — golden validation Report (§7 wire shape)
├── 07_report_golden/                        — executable §5.3 worked-example schema + instance (issue #135)
│   ├── instance.textproto                   — the invalid User instance the golden Report was computed from
│   ├── myco/users/user.proto                — the §5.3 User message (v1.2 grammar)
│   └── myco/commons/…                       — types.proto (Email/CompanyEmail/PhoneNumber) + validator.proto (declared functions)
├── 08_engine_config.textproto               — golden EngineConfig (§9.4 project config)
├── 09_wkt_refinements.proto                 — WKT-based type aliases (§6.2 binding rules)
├── 10_literal_args.proto                    — enum-ref, message-literal + list-literal args (§5.1/§8.1)
├── 11_literal_carrier_golden.textproto      — golden lowered AnnotationList, all Literal kinds
├── 12_expression_args.proto                 — engine-expression args: capture edges + call extraction (§5.1/§8.1)
├── 13_declaration_shapes.proto              — every remaining declaration shape (issue #68)
├── 14_refinement_kinds.proto                — enum/wrapper/message refinement + field-level stacking (§6.3)
├── 15_collections_golden.textproto          — golden Report: per-element repeated/map incl. keys + for_key (§6.4)
├── 15_collections_golden/                   — schema + instance the golden was computed from
├── 16_sensitive_golden.textproto            — golden Report: @sensitive redaction at all three sites (§6.7)
├── 16_sensitive_golden/                     — schema + instance
├── 17_missing_impl_golden.textproto         — golden Report: protowire.function.unimplemented (§9.2/§7)
├── 17_missing_impl_golden/                  — schema + instance
├── 18_default_unsatisfiable_golden.textproto — golden Report: RULE_KIND_DEFAULT (§6.4)
├── 18_default_unsatisfiable_golden/         — schema + instance
├── 19_catalog_miss.textproto                — golden Report for the i18n catalog-miss fixture (§7)
├── 19_catalog_miss/                         — schema + instance + messages_de.txt rendering golden
└── invalid/                                 — MUST-NOT-COMPILE fixtures + manifest (see invalid/README.md)
```

The lowered-descriptor expectations for the schema-text fixtures are
exercised by the reference round-trip harness (see *Round trip* below)
rather than per-fixture `.expected.txt` files: the harness compiles the
corpus with the reference `protocompile` pipeline and asserts on the
carrier contents, and the `.textproto` goldens pin the runtime surfaces
(reports, config, lowered literals).

## Per-fixture coverage

| Fixture | Constructs exercised |
|---|---|
| `01_basic.proto` | `type`, `function`, `annotation` (one each), simple `@validate` on a field |
| `02_composition.proto` | `type` chain (3 levels), AND composition semantics |
| `03_message_and_field_annotations.proto` | Hybrid placement; stacked annotations on a field |
| `04_required_and_default.proto` | `@required`, `@default`, coexistence with `[(pxf.required) = true]` / `[(pxf.default) = "..."]` |
| `05_error_overrides.proto` | `code` and `message` args on `@validate`; `[error_code = "..."]` on `function` |
| `06_cross_file_*.proto` | Import + cross-file resolution of types and functions |
| `07_report_golden.textproto` | `Report` / `EnrichedViolation` runtime wire shape (RFC-001 §7); all three `RuleKind`s, params provenance (inline rules ⇒ empty `params`), absent `actual_value` |
| `07_report_golden/` | Executable form of the §5.3 worked example (issue #135): the schema (`myco/users/user.proto` + `myco/commons` imports) and the invalid instance the golden was computed from — engines validate `instance.textproto` and diff against the golden |
| `08_engine_config.textproto` | `EngineConfig` project configuration (RFC-001 §9.4); every field, discovery/precedence rules in prose |
| `09_wkt_refinements.proto` | `type` aliases on `Timestamp`/`Duration` (engine-native binding) and `Any` (`type_url` refinement, no auto-unpack) per §6.2 |
| `10_literal_args.proto` | Enum-value reference and homogeneous list literal as annotation arguments on `any`-typed params (§8.1) |
| `11_literal_carrier_golden.textproto` | Lowered `AnnotationList` with all three `Literal` kinds: resolved `EnumLiteral`, `Any` message literal, `ListLiteral` of `LiteralValue`s (§8.1, issue #64) |
| `12_expression_args.proto` | Engine-expression arguments (§5.1 capture-then-classify, issue #91): balanced-delimiter capture with opaque string literals, named args after an expression arg, declared-function call extraction into `Expression.calls` vs. undiagnosed engine builtins (§8.1) |
| `13_declaration_shapes.proto` | Every remaining declaration shape (issue #68): paren-less + empty-paren annotation declarations; params of every §5.1 type (incl. enum-typed, message-typed, bytes, expression) with defaults and negative literals; zero-param and multi-param functions; a function with both an option list and trailing annotations; a bare type alias; use sites with no parens, empty parens, all-defaulted args, positional-then-named args, and message literals with and without the optional leading type name |
| `14_refinement_kinds.proto` | Refinement over every value-shaped base kind (§6.3): enum, wrapper (with a chained derived alias), and message bases; a field-level rule stacked atop the chain; an alias consumed as a repeated element type |
| `15_collections_golden{,.textproto}/` | Per-element validation on repeated and map fields (§6.4, issues #141/#153): element rules, map KEY rules with `for_key = true` on the same subscripted path as the entry's value violation, and a field-level collection rule |
| `16_sensitive_golden{,.textproto}/` | `@sensitive` report redaction (§6.7): type-alias, field, and transitive message-level sensitivity — `actual_value` unset + `value_redacted = true` — with a non-sensitive control field |
| `17_missing_impl_golden{,.textproto}/` | The missing-implementation error state (§9.2): lenient engine + unregistered declared function ⇒ reserved `protowire.function.unimplemented` with its spec-pinned fallback template; strict engines fail startup instead |
| `18_default_unsatisfiable_golden{,.textproto}/` | The unsatisfiable-rule error state (§6.4, issue #133): a `@default` failing the field's own rules ⇒ `RULE_KIND_DEFAULT` with the substituted default as `actual_value` |
| `19_catalog_miss{,.textproto}/` | The locale-catalog-miss error state (§7): function-authored params feeding catalog interpolation on a hit, `fallback_message` verbatim on a miss; rendering golden `messages_de.txt`, pinned stub + catalog in the schema header |
| `invalid/` | Eight MUST-NOT-COMPILE fixtures — arity mismatch ("invalid signature"), positional-after-named, heterogeneous list, unknown literal field, map field in literal, container alias, unbalanced capture, undeclared annotation — with the error-class manifest in `invalid/README.md` |

Unlike the schema-text fixtures, the `.textproto` fixtures are message
goldens, not v1.2 schema sources:

- `07_report_golden.textproto` — text-format `protowire.schema.v1.Report`
  ([`proto/schema/v1/report.proto`](../../proto/schema/v1/report.proto)).
  Target for M4 engine work (issues #040–#043): a conformant engine
  validating the §5.3 worked-example instance emits a semantically equal
  report. The worked example is executable: compile
  `07_report_golden/myco/users/user.proto` (import roots:
  `07_report_golden/` for the `myco/…` imports, plus a root mapping
  `protowire/` to the repo's [`proto/`](../../proto) for
  `annotations.proto`), validate `07_report_golden/instance.textproto`,
  and diff the emitted Report against the golden (violation order per
  §6.4; `wall_time_nanos` and `engine` excluded).

  **Pinned conformance stubs.** Function bodies are engine-runtime
  concerns (§6.5), so cross-port determinism requires pinning the
  implementations a conformance run registers for the worked example's
  declared functions. Both are always-pass stubs — the golden exercises
  declaration, registration, and enrichment plumbing, not stub logic:

  | Function | Registered stub |
  |---|---|
  | `myco.users.same_domain(User)` | always `(true, nil)` — the message-level rule passes, contributing no violation |
  | `myco.commons.valid_phone(string)` | always `(true, nil)` — never invoked for the golden instance (`phone` is absent), but runtime-init verification requires every declared function to be registered |
- `08_engine_config.textproto` — text-format
  `protowire.schema.config.v1.EngineConfig`
  ([`proto/schema/config/v1/config.proto`](../../proto/schema/config/v1/config.proto)),
  i.e. the content of a project's `protowire.config.textproto`. Target
  for config-loader implementations (§9.4 discovery/precedence).
- `11_literal_carrier_golden.textproto` — text-format
  `protowire.schema.v1.AnnotationList`: the lowered carrier for
  non-scalar annotation arguments, covering all three `Literal` kinds.
  Target for the protocompile lowering pass (#034) and every port's
  carrier reader.

The report goldens added by the corpus expansion (issue #68) follow the
07 pattern — a schema + instance directory beside a top-level golden —
with two differences, stated in each golden's header: `engine` and
`wall_time_nanos` are omitted entirely (they are excluded from
cross-port equality), and any pinned conformance stubs or catalogs live
in the schema's header comment. `19_catalog_miss/` additionally pins the
format-time rendering contract (§7): `messages_de.txt` lists the
localized message text per violation — catalog hit interpolates the
pinned template from `cause.params`, catalog miss falls back to
`fallback_message` verbatim.

Verify the goldens parse with stock protoc:

```
for g in 07_report_golden 15_collections_golden 16_sensitive_golden \
         17_missing_impl_golden 18_default_unsatisfiable_golden \
         19_catalog_miss; do
  protoc -I <root> --encode=protowire.schema.v1.Report \
    protowire/schema/v1/report.proto < $g.textproto > /dev/null
done
protoc -I <root> --encode=protowire.schema.config.v1.EngineConfig \
  protowire/schema/config/v1/config.proto < 08_engine_config.textproto > /dev/null
protoc -I <root> --encode=protowire.schema.v1.AnnotationList \
  protowire/schema/v1/descriptor.proto < 11_literal_carrier_golden.textproto > /dev/null
```

## Round trip

The corpus's descriptor-level contract (issue #68) is a three-stage
round trip:

1. **`protocompile`** (reference v1.2 toolchain) parses and lowers the
   positive corpus to a `FileDescriptorSet` carrying the `50400`–`50404`
   extensions.
2. **Stock `protoc`** — with `google/protobuf/descriptor.proto`,
   `protowire/schema/v1/descriptor.proto`, and `pxf/annotations.proto`
   in scope — decodes that set to text format and **re-marshals** it.
3. The re-marshaled bytes MUST equal the input byte-for-byte (§8.5:
   carriers are well-formed proto; stock tooling round-trips them
   transparently — as typed extensions when the carrier schema is
   imported, as unknown fields when it is not).

The executable harness lives in `protocheck`'s `roundtrip/` package
(trendvidia/protowire#144), which consumes this corpus through the
module's `ConformanceFixtures` embed at a pinned version, drives the
real protocompile pipeline and the forked `protoc-gen-go` §9.3 stub
generator over it, and compiles + invokes the generated stubs. It
cannot live here: protowire is public and the reference toolchain
modules are not. Ports replicate the same three stages with their own
toolchain plus a stock `protoc`.

Compile-error conformance for `invalid/` is the complement: each file
there MUST fail stage 1 — see `invalid/README.md` for the per-file
error classes.

## Adding new fixtures

Each schema-text fixture MUST:

- be self-contained or explicitly note its imports;
- exercise one specific construct or interaction prominently;
- be valid `protowire v1.2` schema text per IETF draft `-01` (or live
  in `invalid/` with a manifest row naming its error class);
- include a header comment naming the construct it exercises and the
  expected behavior (in prose).

After adding a fixture, update the table above; for runtime-behavior
fixtures add the schema + instance directory and the top-level report
golden; keep the protocheck round-trip harness's fixture list in sync.

## Status

Initial fixtures committed at M0 as illustration; corpus expanded to
comprehensive coverage per issue #68 (declaration shapes, all
refinement kinds, collection/key validation, `@sensitive` redaction,
the four runtime/compile error states, and the `invalid/` suite).
Cross-port adoption (M9+) gates on this suite passing in each port.
