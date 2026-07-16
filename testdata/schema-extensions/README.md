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
├── 08_engine_config.textproto               — golden EngineConfig (§9.4 project config)
├── 09_wkt_refinements.proto                 — WKT-based type aliases (§6.2 binding rules)
├── 10_literal_args.proto                    — enum-ref, message-literal + list-literal args (§5.1/§8.1)
└── 11_literal_carrier_golden.textproto      — golden lowered AnnotationList, all Literal kinds
```

Each fixture is the input; a sibling `.expected.txt` (added during M2)
captures the expected lowered FileDescriptorSet in `protoc --decode_raw`
form, plus the expected source-map content. The cross-port harness
diffs every port's output against these expectations.

## Per-fixture coverage

| Fixture | Constructs exercised |
|---|---|
| `01_basic.proto` | `type`, `function`, `annotation` (one each), simple `@validate` on a field |
| `02_composition.proto` | `type` chain (3 levels), AND composition semantics |
| `03_message_and_field_annotations.proto` | Hybrid placement; stacked annotations on a field |
| `04_required_and_default.proto` | `@required`, `@default`, coexistence with `[(pxf.required) = true]` / `[(pxf.default) = "..."]` |
| `05_error_overrides.proto` | `code` and `message` args on `@validate`; `[error_code = "..."]` on `function` |
| `06_cross_file_*.proto` | Import + cross-file resolution of types and functions |
| `07_report_golden.textproto` | `Report` / `EnrichedViolation` runtime wire shape (RFC-001 §7); all three `RuleKind`s, typed `Value` params, absent `actual_value` |
| `08_engine_config.textproto` | `EngineConfig` project configuration (RFC-001 §9.4); every field, discovery/precedence rules in prose |
| `09_wkt_refinements.proto` | `type` aliases on `Timestamp`/`Duration` (engine-native binding) and `Any` (`type_url` refinement, no auto-unpack) per §6.2 |
| `10_literal_args.proto` | Enum-value reference and homogeneous list literal as annotation arguments on `any`-typed params (§8.1) |
| `11_literal_carrier_golden.textproto` | Lowered `AnnotationList` with all three `Literal` kinds: resolved `EnumLiteral`, `Any` message literal, `ListLiteral` of `LiteralValue`s (§8.1, issue #64) |

Unlike the schema-text fixtures, the `.textproto` fixtures are message
goldens, not v1.2 schema sources:

- `07_report_golden.textproto` — text-format `protowire.schema.v1.Report`
  ([`proto/schema/v1/report.proto`](../../proto/schema/v1/report.proto)).
  Target for M4 engine work (issues #040–#043): a conformant engine
  validating the §5.3 worked-example instance emits a semantically equal
  report.
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

Verify they parse with stock protoc:

```
protoc -I <root> --encode=protowire.schema.v1.Report \
  protowire/schema/v1/report.proto < 07_report_golden.textproto > /dev/null
protoc -I <root> --encode=protowire.schema.config.v1.EngineConfig \
  protowire/schema/config/v1/config.proto < 08_engine_config.textproto > /dev/null
protoc -I <root> --encode=protowire.schema.v1.AnnotationList \
  protowire/schema/v1/descriptor.proto < 11_literal_carrier_golden.textproto > /dev/null
```

## Adding new fixtures

Each schema-text fixture MUST:

- be self-contained or explicitly note its imports;
- exercise one specific construct or interaction prominently;
- be valid `protowire v1.2` schema text per IETF draft `-01`;
- include a header comment naming the construct it exercises and the
  expected behavior (in prose).

After adding a fixture, also add the corresponding `.expected.txt` and
update the table above.

## Status

Initial fixtures committed at M0 as illustration. Full
`.expected.txt` materialization happens at M2 once issue #034
(descriptor lowering) lands and the canonical output shape is stable.
