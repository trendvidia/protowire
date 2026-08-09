# Changelog

This is the **spec-level** changelog: grammar bumps, envelope versions,
annotation additions, and other things every port has to mirror. Per-port
release notes live in each port's own changelog.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
loosely; the project follows [SemVer](https://semver.org/) per
[`STABILITY.md`](STABILITY.md).

## [Unreleased]

`@http` becomes what it always claimed to be: sugar over `google.api.http`, not a documentation-only annotation. Driven by the REST surface of trendvidia/voya, which adopts `connectrpc.com/vanguard` in-process per service and could not work at all until this landed. **Descriptor output changes** — that is the release — while PXF, `pb`, SBE, envelope and report outputs are byte-identical to v1.9.0 for every schema, document and topic corpus; doc packs are too, apart from the image SHA-256 they stamp into provenance and descriptor-path anchors. Two distinct things move the descriptor bytes, and only the first is about `@http`: the new `google.api.http` option on annotated methods, and the `50402` source spans of the bundled `protowire/schema/v1/annotations.proto`, whose `annotation http(` declaration shifted when its doc comment grew. That library ships inside **every** image, so every image's bytes change and every downstream image cache invalidates — a schema that uses no `@http` at all still rebuilds to different bytes. See [`STABILITY.md`](STABILITY.md) for what is and is not promised about the foreign extension number this now writes.

### Added

- **`@http` lowers to the standard `google.api.http` extension** (issue [#213](https://github.com/trendvidia/protowire/issues/213)). The routing skeleton was complete and correct in the `50400` carrier and in an encoding no off-the-shelf consumer reads: every REST binder — connect vanguard, grpc-gateway, Envoy's `grpc_json_transcoder`, buf's OpenAPI plugins — looks for `MethodOptions` field `72295728`, found nothing, bound zero routes and said nothing about it, so every REST URL 404'd as though the endpoint were unimplemented. Worse beside the #173 OpenAPI renderer, which described routes that nothing served. RFC-001 §5.2 now lowers `method`/`path` **both** ways: the carrier keeps the enriched surface (`summary`, `operation_id`, `tags`, `security`) that `HttpRule` cannot express, and the standard option carries the skeleton everything downstream acts on. The mapping is mechanical and pinned in §5.2 — five named verbs to their pattern fields and anything else to `CustomHttpPattern`; bodyless verbs (`GET`/`HEAD`/`DELETE`/`OPTIONS`) with no body and everything else `body: "*"`, matching the §5.2 field-binding rule the OpenAPI renderer already implements; repeated `@http` as primary + `additional_bindings`. An author-written `(google.api.http)` is never joined by a competing rule, and the compiler warns at the annotation that consequently does not route the method. The emitted option adds **no import** to the lowered file — it rides in unknown-field bytes exactly as the §8.1 carriers do, so images stay self-contained and stock `protodesc` still loads them. On by default; `pxf build --google-api-http=false` suppresses the standard option and nothing else — it restores the *rule* bytes, not the whole pre-v1.10.0 image, because the annotation library's shifted source spans are independent of the flag.
- **The routing skeleton is checked, not just carried** (issue [#213](https://github.com/trendvidia/protowire/issues/213)). Because it lowers to a rule something will try to serve, a `{name}` segment binding no field of the request message is now a compile error at the annotation's source position — as are a segment binding a repeated field, a relative path, unbalanced template braces and an empty `method`. Each would otherwise reach an image as a route no binder can bind, which is the same silent failure in a smaller box; `pxf openapi` rejected the naming-nothing case already, so the two ends of the toolchain agree on it. They do not yet agree on everything between it and `HttpRule`'s own template grammar — the compiler accepts dotted (`{order.id}`) and sub-path (`{name=shelves/*}`) segments the renderer refuses, tracked in [#217](https://github.com/trendvidia/protowire/issues/217). Five new corpus fixtures pin the five classes — `invalid/http_unbound_template.proto`, `http_repeated_template.proto`, `http_relative_path.proto`, `http_unbalanced_template.proto`, `http_empty_method.proto` — and `21_http_operation.proto` gains the dual-lowering expectation.

  **This narrows accepted schema input at a minor version.** A schema whose `@http` carries any of the five now fails to build where v1.9.0 accepted it, and `--google-api-http=false` does *not* relax the checks: the routing skeleton is validated whether or not the standard option is written, because the carrier's route is what `pxf openapi` publishes either way. The narrowing is deliberate — every rejected form was already a route that could not be served, so the alternative is shipping the same failure silently — but it is a narrowing, not an addition, and [`STABILITY.md`](STABILITY.md) otherwise reserves those for a major bump. Migration is mechanical and the diagnostic names the offending segment: rename the template segment to a field that exists (`{orderId}` → `{order_id}`), make the path absolute, close the brace, or fill in the verb.

- **Every `@http` binding renders as its own operation** (issue [#215](https://github.com/trendvidia/protowire/issues/215)). The renderer read only the first `@http` on a method, so once repetition lowered to `additional_bindings` a document could describe fewer routes than the image binds — #213's failure pointed the other way. `pxf openapi` now emits one operation per use site, and `x-since` stamps every binding rather than the first. Bindings after the first must name their own `operation_id`: the derived `<Service>_<Method>` is document-unique only while a method has one binding, and the index-derived alternative (`…_2`) would rename a generated client's method whenever two annotation lines are reordered — a silent SDK break from an edit that changes no API. The renderer also gained a **document-wide `operationId` uniqueness check**, which is independent of repetition: uniqueness was argued "by construction" in §#080 and never verified, so two methods claiming one id rendered a colliding document in silence. Both checks sit in the renderer, not the compiler — `operation_id` is documentation metadata a port may parse and ignore (§5.2). New conformance fixture `22_http_additional_bindings.proto`; the corpus now states what repetition means for ports, and the `pxf openapi`↔routes parity test can fail on this class.

### Changed

- **`@http`'s documentation stops claiming interchangeability** (issue [#213](https://github.com/trendvidia/protowire/issues/213)). [`proto/schema/v1/annotations.proto`](proto/schema/v1/annotations.proto) described `@http` as pairing "naturally with the `google.api.http` option in schemas that prefer it; either form is accepted by tooling that understands both", and the conformance fixture called binding "a generator concern, not a compiler one". Read together they implied the two forms were interchangeable at the descriptor level, which they were not. Both now describe the lowering that exists.
- **Reference toolchain pinned at protocompile v0.24.0** (was v0.23.0): picks up the `google.api.http` lowering and the §5.2 routing checks ([trendvidia/protocompile#132](https://github.com/trendvidia/protocompile/pull/132)), plus `fdp.EmitGoogleAPIHTTP` behind the new `--google-api-http` flag.

## [1.9.0] – 2026-07-26

A single-accessor follow-up to the v1.6 editor-integration story, driven by goed's schema-anchor hover (trendvidia/goed#325, PR trendvidia/goed#334): the element-kind fact the compiler already indexes becomes readable, so the editor drops its second decode of the image. No wire-format changes — PXF, `pb`, SBE, envelope, report, and doc-pack outputs are byte-identical to v1.8.0 for every schema, document, and topic corpus; the only change is additive Go API on the `docpack` package. See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **Element-kind lookup on the docpack `Image`** (issue [#206](https://github.com/trendvidia/protowire/issues/206)). The image already indexes what kind of element every fully-qualified name names — schema-anchor resolution checks membership against exactly that index — but only unexported code could read the kind. `Image.Kind(fqn)` now exports it, returning an `ElementKind` (`message`, `field`, `oneof`, `enum`, `enum value`, `service`, `method`, `type alias`) plus existence — the same shape as the #185 accessors: the presentational fact a hover renders is read off the index resolution checks, so an editor can never disagree with the compiler. It is also the only truthful source for v1.2 `type` aliases, which live in the `FileTypeDecls` carrier (§8.2), not the descriptor tree; goed drops its interim protodesc re-decode (trendvidia/goed#334) and alias hovers stop degrading to a generic "schema element". Reference-implementation surface only (additive within a minor per `STABILITY.md`); no wire-format change.

## [1.8.0] – 2026-07-26

The doc-platform contract batch, driven by planning the goed authoring layer (trendvidia/goed#321) against the landed platform: the contracts three repos were about to re-derive independently are pinned in one place. The resolved-anchor-id spellings become normative format contract with a conformance golden, the review identity convention is pinned before it calcifies divergently, the anchor surface widens to composition props and transitions, and doc coverage becomes compiler policy under `--release`. No wire-format changes — PXF, `pb`, SBE, envelope, and report outputs are byte-identical to v1.7.0 for every schema and document; doc packs from corpora using no new anchor kind are byte-identical too (`PackProvenance.format_version` unchanged), and the only schema delta is the additive `TransitionAnchor` oneof arm in `protowire.docs.v1` (no extension numbers). See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **Composition-prop and transition anchors** (issue [#199](https://github.com/trendvidia/protowire/issues/199)). The two catalog surfaces the v9 mirror carried "for provenance and future anchor kinds" are now anchorable, resolving the issue as its option 1. Template-composition attributes (`template`, `slot`, `content_slot`; appviewer#209) resolve as widget prop anchors on **any** catalog widget, exactly like common props — they are offered by context, and pick-to-help lands on a typed node carrying `slot` — with `target_since` falling back composition entry → widget entry. Screen transitions (appviewer#276) get a dedicated `TransitionAnchor` kind (`Anchor` oneof field 6) with canonical id `transition:<name>`: transitions are catalog-global and belong to no widget type, so a distinct kind rather than a widget pseudo-member. Stability `STABLE`, redirects through the existing same-kind machinery, `Catalog.ResolveTransition` on the #185 query surface. Coverage denominators no longer special-case either set.
- **Resolved-id grammar as format contract** (issue [#198](https://github.com/trendvidia/protowire/issues/198)). `ResolvedAnchor.resolved_id` is the join key for three independent consumers (the compiler, appviewer's ADR-0012 Store mirror, goed's coverage diff), and its spellings previously existed only as code. [`docs/DOC-PACK.md`](docs/DOC-PACK.md) § Resolved anchor IDs is now normative: one grammar line per anchor kind, the enum-value parent-scoping rule (a value spells `pkg.STATUS_OPEN` / `pkg.Order.KIND_WEB`, never enum-qualified), and the rule that widget member spellings never leak their resolution source. The spellings are covered by the pack's byte-stability promise — changing one is a format break requiring a `format_version` bump. The conformance corpus `testdata/docs/grammar/` authors one anchor per spelling arm; the resolved set is golden in `resolved_ids.golden`, so drift fails this repository's suite rather than a downstream consumer's lookups.
- **Doc-coverage policy in `pxf docs build`** (issue [#200](https://github.com/trendvidia/protowire/issues/200)). "Every public registry widget and exported `@http` endpoint has a documenting topic" is a release property, and like the revisor gate it is now compiler policy rather than per-consumer reimplementation. Opt-in via `--coverage widgets|members` (existing packs cannot start failing on upgrade): warnings while drafting, errors under `--release`, diagnostics in the standard `Loc` shape located at the demanding data input. Denominator: registry widget types plus `@http`-annotated methods off the image source map (new `Image.HTTPMethods()` — the same set `pxf openapi` renders operations for); `members` adds per-widget props/events and the transition vocabulary, while common and composition props are excluded with a documented rationale (they resolve on every widget, so there is no single canonical id to require). Numerator: exact `resolved_id` matches — the same lookup semantics pick-to-help uses, so "covered" means "help lands there" — with `--coverage-approved` raising the bar to documented-and-approved. Audience-aware through `AudienceRank`: an element demands a topic visible at its own tier; inputs carry no element tiers today, so elements read as public, with the rule already written against #173 introducing them.

### Changed

- **Review identity convention pinned** (issue [#197](https://github.com/trendvidia/protowire/issues/197)). `Review.author`/`reviewers`/`revisor` and `Translation.translator` were documented as free-form, but the release gate is a string comparison — two spellings for one human defeat it in both directions (a forge login beside an email is a false pass on self-approval; `Jane Doe <jane@x>` beside `jane@x` counts one reviewer as two). The canonical form is now normative: the **git author email, normalized to lowercase**, which every producer MUST write and compare ([`docs/DOC-PACK.md`](docs/DOC-PACK.md) § Review identity). The compiler warns on values that do not look canonical, at the offending entry's source position; the warning is never escalated by `--release` — the convention protects the gate, it is not itself the gate. goed's interim behavior (write `git config user.email` lowercased) is ratified unchanged.

## [1.7.0] – 2026-07-25

The RFC-001 §7 i18n story becomes implementable end to end: `catalog_libraries` was a dangling pointer — the §9.4 engine config named it, but nothing defined what a referenced file contains — and this release pins the source format, ships the conformance fixture, and pairs with the reference loader in protocompile v0.23.0. No wire-format changes — PXF, `pb`, SBE, envelope, doc-pack, and report outputs are byte-identical to v1.6.0 for every schema and document; the only schema delta is the new `protowire.schema.catalog.v1` package (no extension numbers). See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **Locale catalog source format** (issue [#194](https://github.com/trendvidia/protowire/issues/194)). RFC-001 §7 pins what a file named by `EngineConfig.catalog_libraries` contains: a text-format `protowire.schema.catalog.v1.Catalog` message (new package `proto/schema/catalog/v1/catalog.proto` — same artifact status as the §9.4 engine config: loaded at engine init, never embedded in descriptors, no extension numbers). One locale per file (BCP 47 tag, the `RegisterCatalog` key); entries map violation code → `{param}` template with the §7 interpolation and fallback-on-miss semantics; multiple files per locale merge with duplicate codes a load error; paths resolve relative to the config file (they are not proto import paths — catalogs are runtime data, `pxf build` does not compile them). The `19_catalog_miss` conformance fixture's pinned catalog is now a real source file (`catalog_de.textproto`), and the `08_engine_config` fixture's `catalog_libraries` values are `.textproto` paths. The reference loader landed beside `engineconfig` as the `catalogs` package ([trendvidia/protocompile#131](https://github.com/trendvidia/protocompile/pull/131), v0.23.0), returning plain `(locale → entries)` data that feeds `protocheck.NewMapCatalog` directly; it unblocks trendvidia/protolsp#267. Plural/gender/ICU template forms deferred (§13 #15).

### Changed

- **Reference toolchain pinned at protocompile v0.23.0** (was v0.22.0): picks up the `catalogs` loader and the re-vendored `config.proto`/`catalog.proto` spec files (`checkspecdrift` covers both). No compiler-surface change.

## [1.6.0] – 2026-07-25

Editor-integration follow-up to the v1.5.0 documentation platform, driven by the goed authoring layer (trendvidia/goed#321): the doc-pack compiler becomes an importable Go library with an unsaved-buffer overlay and an anchor-completion query surface, its diagnostics carry source positions, and the `WidgetCatalog` mirror catches up to the live appviewer registry export. No wire-format changes — PXF, `pb`, SBE, envelope, and doc-pack outputs are byte-identical to v1.5.0 for every schema, document, and topic corpus; the only schema delta is additive fields on `protowire.docs.v1.WidgetCatalog` (no extension numbers). See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **`docpack` is an importable library** (issue [#185](https://github.com/trendvidia/protowire/issues/185)). `internal/docpack` → public `docpack/`; `cmd/pxf` is a thin caller. For in-process consumers on a diagnostics debounce: `Options.Overlay` splices unsaved editor buffers into the topic root by root-relative path (topic identity is (key, locale) across the whole root, so single-buffer checks would be unsound; overlay-only keys compile as new sources), `Options.Image`/`Options.Catalog` accept preloaded data inputs via the exported `LoadImage`/`LoadCatalog` (pinned byte-identical to the path-based build), and the anchor-completion query surface — `Image.FQNs`/`Has`/`Paths`/`HasPath`/`AnnotationsOn`, `Catalog.Widgets`/`Props`/`Events`/`CommonProps`, exported `Catalog.ResolveWidget` — exposes exactly the sets resolution checks membership against, keyed by the canonical `resolved_id` spellings.
- **Source positions on doc diagnostics** (issue [#187](https://github.com/trendvidia/protowire/issues/187)). `docpack.Loc` gains optional 1-based `Line`/`Column` (zero = unknown), taken from the PXF AST the loader already parses — no second parse. Baseline is the topic's `key` entry; checks that know their offending entry point at it (review/meta/translation fields, each topic-level anchor's own list element, redirect entries). Topic positions are keyed by (key, locale) identity, never index-paired. `Diagnostic.String()` renders `file:line:col:` when present.

### Changed

- **`protowire.docs.v1.WidgetCatalog` mirrors appviewer catalog schema v9** (issue [#186](https://github.com/trendvidia/protowire/issues/186)). Additive fields only: `WidgetCatalog.composition_props`/`transitions` (new `TransitionSpec` message), `WidgetSpec.icon`/`category`/`variadic_children`, `PropSpec.required`/`default_value`. `pxf docs build` accepts a v9 registry export without the every-build version warning (the gate is a documented floor: only exports *newer* than the mirrored version warn), and a `bind` prop anchor resolves per-widget — on Bindable specs only — per the v8 move off `common_props`. Composition props and transitions are carried like `action_funcs`: provenance and future anchor kinds, not resolvable as widget anchors, since the widget-anchor grammar addresses typed widgets. Wire-compatible: existing catalogs and packs are unchanged.

### Fixed

- **`pxf` runtime errors no longer print the usage block** (issue [#188](https://github.com/trendvidia/protowire/issues/188)). Usage prints for exactly the errors it can help with — unknown flags, wrong argument counts — and never after a runtime failure. Also fixed alongside: `pxf query` and `pxf infer-schema` runtime failures used to exit 1 in complete silence (per-command `SilenceErrors` with nothing else printing); their errors now print like every other command's.

## [1.5.0] – 2026-07-25

Feature train on the v1.0 freeze line: the documentation platform's first layer and the M8 OpenAPI boundary land together with the v1.2 toolchain's CLI entry points. Two additive schema-language changes (`@http` operation surface, `@sensitive` classification), two new typed artifact families (`protowire.docs.v1` doc packs, `protowire.openapi.v1` generator config — new packages, no extension numbers), three new `pxf` subcommands (`build`, `docs build`/`docs digest`, `openapi`), and a set of conformance tightenings: §6.4 map-key validation is explicit, the §7 reserved-code fallback messages are pinned, §5.1/§8.1 message-literal list elements are corpus-pinned now that the reference parser accepts them, and unreferenced broken/cyclic aliases MUST be rejected at the declaration site. No wire-format changes — PXF, `pb`, SBE, and envelope outputs are byte-identical to v1.4.0 for every schema and document, and `report.proto` is byte-compatible (comment-only edits). See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **`pxf openapi` — the OpenAPI boundary renderer.** The M8 generator lands as decided in `docs/RFC-001-issues.md` §#080 (issues [#93](https://github.com/trendvidia/protowire/issues/93)/[#173](https://github.com/trendvidia/protowire/issues/173)): lowered image (`pxf build`) plus optional doc pack (`pxf docs build`) in, byte-stable OpenAPI 3.1 YAML/JSON out, with JSON/YAML existing only at this edge per the binding format principle. The schema half maps messages, enums and §8.2 type aliases to FQN-keyed `components/schemas` (alias chains via `allOf`, fields `$ref` their alias); common `@validate` shapes become native keywords (`pattern`, `minLength`/`maxLength`/`minItems`/`maxProperties`, `enum`, `minimum`/`maximum`/`exclusive*`) and every non-mappable rule is carried through under `x-validation`; `@description`/`@deprecated`/`@example`/`@default`/`@required` map to their counterparts, presence follows §6.1 nullability, and `@sensitive` honors the §6.7 doc-emit minima (no values or examples). The operation half renders `@http` per the §5.2 binding rules with **derived responses** (GH [#177](https://github.com/trendvidia/protowire/issues/177)): `200` from the return type, `default` from the §7 `Report`, reachable violation codes under `x-error-codes`. Audience tiers are generator configuration (FQN globs in the new `protowire.openapi.textproto`, `protowire.openapi.v1.GeneratorConfig`), restricted further by doc-pack topics anchoring an element; `--audience` filters and never strips, and a closure reaching a more restricted element fails generation naming both ends. `x-since` derives from protoregistry history when coordinates are configured, omitted otherwise. CLI-only and additive: no grammar, wire-format, or report change.

- **Conformance corpus: message-literal list elements + declaration-site alias-cycle rejection.** Fixture `10_literal_args.proto` and the `11_literal_carrier_golden.textproto` carrier golden pin the §8.1 list-of-message-literals shape (`LiteralValue.literal` wrapping an `Any` per element, issue [#176](https://github.com/trendvidia/protowire/issues/176)) now that the reference parser accepts it (trendvidia/protocompile#127 → #128; fork pinned at v0.22.0). The golden is realigned to actual reference lowering — positional args carry no `name` (§8.1 "empty for positional") and `location.file` is the fixture's import path. New MUST-NOT-COMPILE fixtures: `invalid/untyped_list_element.proto` (§5.1 explicit-typing rule 1) and `invalid/cyclic_alias.proto` (§6.3 — an unreferenced alias cycle is diagnosed at the declaration site, issue [#181](https://github.com/trendvidia/protowire/issues/181)). The `reserved_sensitive_class.proto` known-gap skip is gone: `protowire.`-prefixed sensitivity classes are genuinely rejected (trendvidia/protocompile#123, closed in v0.21.0).

- **`@http` gains the operation surface — OpenAPI operation metadata in the canonical library.** The framework annotation library's `@http` grows four defaulted parameters — `summary`, `operation_id`, `tags`, `security` — so the OpenAPI boundary renderer (`pxf openapi`, issue [#173](https://github.com/trendvidia/protowire/issues/173)) has an authored operation surface rather than a bare routing skeleton (RFC-001 §5.2; decision record `docs/RFC-001-issues.md` §#080). Binding rules are pinned with it: `{name}` path segments bind to same-named top-level request fields, remaining fields bind to the query string for bodyless methods and to the request body otherwise, `operation_id` defaults to `<Service>_<Method>`, and `summary` falls back to the first sentence of `@description`. `tags` and `security` take list literals of strings, because the annotation grammar admits no `repeated` parameter type (§5.1) — list-shaped values ride `any` plus a `Literal.list` (§8.1); the security-*scheme definitions* they name stay in generator configuration, per the §9.4/#112 argument that keeps deployment topology out of descriptors.

  Every added parameter is defaulted, so `@http("GET", "/orders")` keeps its v1.2.0 meaning and no existing schema changes shape — pinned by new fixture `21_http_operation.proto`, which compiles the bare form beside fully-parameterized uses. The parameters carry **no validation semantics** and impose no port obligation beyond carrying them through the §8.1 carrier: a port that renders no REST surface parses them and interprets nothing. No grammar production, extension number, or report change; `annotations.proto` remains parseable by any v1.2 parser.

  Recorded trade-off: a generator-owned `openapi.*` annotation library (gnostic-style, zero spec involvement) was the considered alternative, and the cost of the chosen route is that OpenAPI vocabulary now lives in the library every port mirrors and the IETF draft describes, growing only additively from here. Also settled in the same record: `@http` gets **no `responses` parameter** — responses are derived from the return type plus `@error_code`/§7, and authored per-status entries would need a list of message literals, a shape the carrier represents but the reference parser rejects at an annotation argument today; visibility tiers are **filtered, never stripped**, with tiers assigned by generator configuration and transitive inconsistency (`public` reaching `internal`) an error; and `x-since` is **derived from protoregistry history**, with no canonical `@since` annotation added, because an authored availability claim is unverifiable.

- **Typed documentation model + `pxf docs build` — the doc-pack compiler.** Layer 1 of the application-documentation platform lands: `proto/docs/v1/{topic,pack,registry}.proto` defines the typed documentation model, and `pxf docs build` compiles authored topics into a **doc pack** — the documentation analog of the lowered image ([#164](https://github.com/trendvidia/protowire/issues/164)), consumed by appviewer's runtime help/search (trendvidia/appviewer#364), the goed authoring flow (trendvidia/goed#321), and the boundary renderers `pxf openapi` ([#173](https://github.com/trendvidia/protowire/issues/173)) and the static-HTML export ([#171](https://github.com/trendvidia/protowire/issues/171)). Issue [#170](https://github.com/trendvidia/protowire/issues/170); design record in [`docs/DOC-PACK.md`](docs/DOC-PACK.md).

  Topics are authored as PXF documents bound to `protowire.docs.v1.TopicFile`, with identity `(key, locale)` rather than file path, and locale variants following the §7 catalog pattern (one entity per key+locale, no inheritance, source locale as sole origin of truth). Prose is typed — blocks and inline runs, never a markdown blob — because every consumer needs structure and a renderer that re-parses prose is one that disagrees with its siblings. Per the binding format rule of 2026-07-25, the only JSON in the pipeline is the appviewer registry-export adapter at the integration boundary; `WidgetCatalog` is protowire's typed mirror of that export, so the adapter drops out when appviewer emits the message natively.

  **Anchors carry a stability contract.** Anchor kinds are classified by how well they survive evolution, because the failure mode is proven rather than hypothetical — protocompile v0.18.0 re-keyed `elementPath[fqn#ordinal]` paths and broke exactly this class of persisted reference (trendvidia/protolsp#260). Schema FQNs, registry widget IDs, topic keys and routes are stable by construction; descriptor paths are derived and fragile, so `DescriptorPathAnchor` is authored in *element* terms (element FQN, annotation FQN, ordinal), re-derived on every build through the shared §8.3.1 `fdp.DescriptorPath` formatter, verified against the image's embedded source map (50404), and stamped with the image digest. A dangling anchor is a compile error, a moved target is a `Redirect` whose chains the compiler resolves to their terminus (cycles rejected), and the `Audience` taxonomy — defined once here and inherited by every renderer, including #173's community/enterprise filtering — is enforced transitively between topics.

  **The revisor gate is compiler policy, not IDE machinery.** Approval requires an `APPROVED` state, a revisor who is not the author (always an error otherwise), and an `approved_digest` that still matches the topic's content — a bare approved flag is a claim nobody can check. The content digest covers title, summary and body only, so re-tagging or re-recording an approval does not churn it; `pxf docs digest` prints the value the compiler checks, so the authoring flow never reimplements the canonical encoding. State and digest drift warn while authoring (goed runs the compiler on its diagnostics debounce) and refuse under `--release`, which also requires an explicit audience tier. Translation staleness is a compiler signal on the same terms, escalatable with `--stale-translations-fatal`.

  The pack embeds a prebuilt inverted index — indexing needs parsed content, and the index must be consumable beyond appviewer (goed help, client-side search on static hosting, docs-site export) with no search service anywhere in that list. Occurrences record their field class and positions but no ranking: a help panel and a docs site rank differently and both are right. Output is byte-stable and timestamp-free, with `PackProvenance` recording image and catalog digests, per-source digests, and whether release policy applied. New conformance corpus under `testdata/docs/` (valid corpus, thirteen invalid fixtures, three policy fixtures). CLI-only and additive: no grammar, wire-format, or report change, and no existing schema or document is affected.

- **`pxf build` — the v1.2 compiler front door.** Compiles RFC-001 v1.2 schema sources through the reference toolchain to the lowered `FileDescriptorSet` image every downstream consumer reads (issue [#164](https://github.com/trendvidia/protowire/issues/164), PRs [#167](https://github.com/trendvidia/protowire/pull/167)/[#168](https://github.com/trendvidia/protowire/pull/168)). Directory arguments are import roots (a trailing `/...` is accepted); the canonical annotation library resolves from the bundled embed with no `-p` flags; §9.4 engine configuration follows the pinned precedence (`--function-library` > `--config` > `PROTOWIRE_CONFIG` > nearest `protowire.config.textproto` > defaults); `--check` is the CI entry point; output is deterministic, byte-stable, and loadable by stock protobuf machinery. CLI-only and additive.

- **`@sensitive` classification parameter.** The canonical marker gains its long-deferred class: `annotation sensitive(class: string = "")` (RFC-001 §6.7, issue [#111](https://github.com/trendvidia/protowire/issues/111)). The vocabulary is open and org-defined — sensitivity taxonomies are org policy, and protection-layer consumers (the chameleon editor's key management is the consumer that triggered the deferral's "until needed" clause) map class names to key domains in their own configuration. The spec pins the mechanics only: the `protowire.` prefix is reserved (compile-time rejection, new `invalid/reserved_sensitive_class.proto` fixture); `class` is a single string, never a list, so field → class → key-domain routing stays deterministic; a field's *effective class* is the class of the nearest `@sensitive` that specifies one (field, then alias chain most-derived first, then message), and a bare `@sensitive` reasserts sensitivity without reclassifying; the §6.7 redaction minima remain class-invariant, including for `""` (sensitive but unclassified). New compile-only fixture `20_sensitive_class.proto` pins every arm of the effective-class rule. Additive, defaulted parameter riding the standard `AnnotationArg` carrier — no grammar production, extension number, or report change; existing bare `@sensitive` schemas are untouched.

- **`@encrypted(key_ref)` rejected — protection metadata never enters the schema.** The §6.7 companion deferral resolves as a terminal rejection (RFC-001 §6.7/§13, issue [#112](https://github.com/trendvidia/protowire/issues/112)): key references, algorithms, and rotation state are deployment topology, and annotations lower into `FileDescriptorSet` artifacts that ship across org boundaries — a key reference there would leak key topology into every descriptor artifact (the §9.4 config-out-of-file-options reasoning). The sanctioned contract is split: the schema declares *what* is sensitive and *which class* it belongs to; the protection layer (PXF / chameleon) maps class → key domain in its own configuration. Spec text only.

- **protovalidate migration story (RFC-001 Appendix C).** The last "TBD" from ratification — how `buf.validate`-using projects reach `@validate` — is resolved adapter-first (issue [#66](https://github.com/trendvidia/protowire/issues/66)): Phase 0 validates unchanged schemas at the protowire seam via `github.com/trendvidia/protocheck/protovalidate`; Phase 1 rewrites per-file (custom CEL rules carry over verbatim under the default `cel` engine; the appendix pins the mapping table, including the `required`-vs-`@required` presence delta); Phase 2 retires the `buf.validate` imports. The `--compat` compiler flag is rejected — `buf.validate` options are ordinary custom options that already round-trip opaquely (§8.5) — and no in-place rewriter ships (revisit on demand). One normative pin: protowire engines MUST NOT interpret `buf.validate` options, so mixed-form schemas during transition are well-defined — both validators may run at the same seam with reports disjoint by rule-ID namespace. Docs plus that single coexistence sentence; no grammar, wire, or report change.

### Changed

- **Map per-element validation covers the key dimension.** §6.4 said `map<K,V>` validates "per-element" without saying whether *element* includes the entry **key** — while the machinery had already taken the position twice: `EnrichedViolation.for_key` (v1.3.0, [#125](https://github.com/trendvidia/protowire/issues/125)) exists specifically to express key violations, and protocompile deliberately preserves key-type alias rules on the synthetic entry's `key` field. One clarifying sentence now ratifies it (RFC-001 §6.4, issue [#141](https://github.com/trendvidia/protowire/issues/141)): entry values validate against the value type's rules, entry keys against the key type's rules, key violations set `for_key` with the map subscript addressing the entry as usual. Engines skipping key validation were silently non-conformant before; now it's explicit. Spec text only; no wire change.

- **Reserved-code `fallback_message` strings pinned.** The four reserved `protowire.*` violation codes now carry spec-pinned fallback messages (RFC-001 §7, issue [#151](https://github.com/trendvidia/protowire/issues/151)): `protowire.required` → `field is required` (already pinned de facto by the report golden), `protowire.depth_exceeded` → `recursion depth limit exceeded`, and templates `<function>: not implemented` / `<function>: expected <n> argument(s)` / `<function>: argument <i> is not <type>` for the `protowire.function.*` pair, with `<function>` the declared function's FQN and `<type>` the schema-declared parameter type (never a host-language type name). Spec-defined violations have no schema author, so cross-port equality on their messages must come from the spec — the same argument that made #140's golden string schema-authored. Runtimes MUST expose the strings and template helpers alongside the existing code constants; ports' current emissions (bare declaration names, host-language type names) align in the reserved-namespace fan-out. Spec text and a `report.proto` comment only; no wire change.

### Fixed

- **Worked-example fixture: `country` rule now authors its `fallback_message`.** The `07_report_golden.textproto` golden pins `fallback_message: "country not supported"` on the `user.invalid_country` violation, but the fixture schema authored the rule with only a `code` override — no engine could produce that string, since fallback-message synthesis for inline expressions is engine-specific and non-normative (issue [#140](https://github.com/trendvidia/protowire/issues/140), found by trendvidia/protocheck#34's golden-equality run). `07_report_golden/myco/users/user.proto` and the RFC-001 §5.3 prose now add `message = "country not supported"` at the use site, making the pinned string schema-authored. Golden and cited source lines unchanged; ports that vendored the fixture with this one-line delta can drop it.

## [1.4.0] – 2026-07-24

Semantics-only minor on the v1.0 freeze line: two normative pins on validation-report semantics (RFC-001 §6.4 and §7, issues [#133](https://github.com/trendvidia/protowire/issues/133)/[#134](https://github.com/trendvidia/protowire/issues/134)) plus the executable §5.3 worked-example conformance fixtures ([#135](https://github.com/trendvidia/protowire/issues/135)). No grammar, descriptor, wire-format, or report-shape changes — `report.proto` is byte-compatible with v1.3.0 (comment-only edits) and PXF, `pb`, SBE, and envelope outputs are byte-identical for every schema and document. The bump exists because the cross-port conformance surface tightened: engines claiming v1.4 conformance emit `RULE_KIND_DEFAULT` on substituted-default failures and empty `params` on inline-expression violations. See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **Executable §5.3 worked-example fixtures.** The `07_report_golden.textproto` report golden was keyed to the RFC-001 §5.3 worked example (`myco.users.User`), but the schema existed only as RFC prose — no engine could execute the golden without reconstructing it, and per-port reconstructions would drift (issue [#135](https://github.com/trendvidia/protowire/issues/135)). New `testdata/schema-extensions/07_report_golden/` ships the schema (`myco/users/user.proto` + the `myco/commons` type library and declared functions) and `instance.textproto`, the invalid instance the golden was computed from; the golden's source locations now cite the committed files, and the fixture README pins the always-pass conformance stubs for the declared functions. Ports diff their emitted `Report` against the golden (trendvidia/protocheck#34).

### Changed

- **`RULE_KIND_DEFAULT` semantics pinned.** A rule evaluated against a `@default`-substituted value (RFC-001 §6.1 absent-with-default) reports its violations with `rule_kind: RULE_KIND_DEFAULT`, superseding the `VALIDATE`/`TYPE_REFINEMENT` kind the same rule would carry for a producer-set value; `actual_value` carries the substituted default (RFC-001 §6.4, issue [#133](https://github.com/trendvidia/protowire/issues/133)). The distinct kind marks a schema-authoring error — the declared default fails the field's own rules — not an instance error. Spec text and proto comments only; no wire change.
- **`Violation.params` provenance pinned.** `params` is populated from exactly two sources — function-returned `Violation`s (RFC-001 §6.5) and spec-defined synthetic violations (`protowire.depth_exceeded`); engines MUST leave `params` empty on violations from inline expression rules (RFC-001 §7, issue [#134](https://github.com/trendvidia/protowire/issues/134)). Engine-synthesized params from expression pattern-matching are ruled out — expressions are opaque engine source (§5.1) — so `params` participates in cross-port report equality with no carve-out. The `07_report_golden.textproto` fixture drops its two synthesized params blocks accordingly. Spec text, fixture, and proto comments only; no wire change.

## [1.3.0] – 2026-07-23

Single-train minor on the v1.0 freeze line: one strictly-additive extension to the validation report wire shape (RFC-001 §7, issue [#125](https://github.com/trendvidia/protowire/issues/125)). No grammar, descriptor, or wire-format changes — PXF, `pb`, SBE, and envelope outputs are byte-identical to v1.2.0 for every schema and document. See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Added

- **`EnrichedViolation.for_key` (field 8).** A subscripted map segment in a `FieldPath` addresses the entry's *value*, so a map-key violation (e.g. a length rule on `map<string, V>` keys) and a value violation on the same entry serialized to identical paths. The new bool flags violations whose rule applied to the key itself (RFC-001 §7, issue [#125](https://github.com/trendvidia/protowire/issues/125)). Strictly additive to the runtime report shape; existing reports are unaffected.

## [1.2.0] – 2026-07-20

First minor on the v1.0 freeze line, shipping two strictly-additive spec trains together: the **Protowire Schema Extensions** (RFC-001) and **keyed repeated fields** ([#116](https://github.com/trendvidia/protowire/issues/116)). Every valid v1.1 schema and document remains valid with unchanged meaning; PXF, `pb`, SBE, and envelope outputs are byte-identical for anything that does not use the new constructs. See [`STABILITY.md`](STABILITY.md) for the compatibility contract.

### Protowire Schema Extensions

Strictly additive — every valid v1.1 schema remains a valid v1.2 schema. Driven by [RFC-001 — Protowire Schema Extensions](docs/RFC-001-schema-extensions.md); formal text lands in IETF draft `-01`. Issues tracked at [`docs/RFC-001-issues.md`](docs/RFC-001-issues.md).

- **Three new top-level declarations.** `type`, `function`, and `annotation` extend the schema grammar with refinement aliases, validation-function signatures, and user-declarable annotation kinds. See RFC-001 §5 for grammar deltas and worked examples.
- **`@annotation(...)` use-site syntax.** Unified annotation framework subsuming validation rules, descriptions, examples, deprecation, OpenAPI hints, and future metadata. Hybrid placement: leading on block declarations (`message`, `service`, `rpc`, `enum`, `oneof`); trailing on single-line declarations (`type`, `field`, `function`). Existing `[(option) = value]` brackets coexist permanently — annotations are first-class sugar, brackets remain the raw escape hatch.
- **PXF presence semantics promoted into the validation layer.** `(pxf.required)` and `(pxf.default)` gain canonical annotation forms `@required` and `@default(value)`. Bracket forms retain their extension numbers (`50000`, `50001`) and behavior unchanged.
- **New contextual keywords.** `type`, `function`, `annotation` — recognized as keywords only at the start of a top-level declaration; accepted as identifiers everywhere else. Existing schemas that use these words as message/oneof/field/enum-value names (e.g., `oneof type { ... }`, common in Google APIs) remain valid v1.2 schemas without modification. The `@` sigil is reserved as the annotation-use-site marker. No source-level incompatibility is introduced.
- **New extension number sub-range.** `50400`–`50499` reserved in [`proto/schema/v1/descriptor.proto`](proto/schema/v1/descriptor.proto) for schema-extension carriers. Allocated in this release: `50400` (annotation carrier on every Options message, named per kind — `file_annotations`, `message_annotations`, …), `50401` (functions), `50402` (annotation declarations), `50403` (type declarations), `50404` (embedded source map). The `50100`–`50101` numbers are avoided because SBE already uses them on `FileOptions`. See [`STABILITY.md`](STABILITY.md) for the renumbering prohibition.
- **Structured error model.** Functions return `(bool, *Violation)` with stable codes, structured parameters, and engine-enriched type-chain provenance. Per-locale message catalogs handle i18n at render time. Report wire shapes (`Report`, `EnrichedViolation`, `Violation`, structured `FieldPath`, typed `Value`) are pinned in [`proto/schema/v1/report.proto`](proto/schema/v1/report.proto) so all 10 ports emit equivalent reports (RFC-001 §7, issue [#65](https://github.com/trendvidia/protowire/issues/65)). Params and captured values use the typed `Value` oneof — never `google.protobuf.Value` — preserving int64/uint64 width, bytes, and the set/null/absent presence distinction.
- **Project-level engine configuration.** Engine selection and engine knobs live in `protowire.config.textproto` at the project root — a text-format `protowire.schema.config.v1.EngineConfig` message pinned in [`proto/schema/config/v1/config.proto`](proto/schema/config/v1/config.proto) (RFC-001 §9.4, issue [#60](https://github.com/trendvidia/protowire/issues/60)). Discovery walks upward to the nearest config (no merging); precedence is per-setting CLI flags > `--config` > `PROTOWIRE_CONFIG` (file pointer only) > discovered file > defaults (`cel`, lenient, collect-all). Unknown engine names are startup errors, never fallbacks.
- **Well-known type semantics in refinement rules.** `google.protobuf.Timestamp`/`Duration` bind `this` to the engine-native temporal value (parallel to wrapper unwrap; comparisons mandatory); `google.protobuf.Any` never unwraps — `this.type_url` refinement is the canonical pattern and auto-unpacking is forbidden; all other messages bind structurally; engine `now()` builtins must be run-stable within one `Report` (RFC-001 §6.2, issue [#61](https://github.com/trendvidia/protowire/issues/61)). Spec-text only — no descriptor or lowering change.
- **Normative recursion-depth limit.** Nested-message validation is depth-limited: root at depth 0, each message-typed value +1, default **64**, configurable via `EngineConfig.max_recursion_depth` (RFC-001 §6.4, issue [#62](https://github.com/trendvidia/protowire/issues/62)). At the limit the engine records a fail-closed `protowire.depth_exceeded` violation, sets `Report.truncated`, and continues with siblings — never a silent accept, never a report-destroying hard error. Depth definition, default, and at-limit behavior are normative across ports. Violation codes under the `protowire.` prefix are now reserved for spec-defined violations (`protowire.required`, `protowire.depth_exceeded`).
- **Streaming RPC validation contract.** Per-message validation at the RPC boundary (receiver MUST, sender MAY); the first invalid message terminates the stream with an error carrying the structured `Report` — no skip-and-continue, no lenient-mode knob, no rollback of already-delivered messages (RFC-001 §6.6, issue [#63](https://github.com/trendvidia/protowire/issues/63)). Direction-asymmetric gRPC mapping: invalid request-direction messages → `INVALID_ARGUMENT`; server-produced invalid response messages → `INTERNAL`; client-side receiver checks surface local errors — each with the `Report` in `google.rpc.Status.details`. Stream-level invariants deferred (§13 #12).
- **`Literal` carrier shape finalized.** [`proto/schema/v1/descriptor.proto`](proto/schema/v1/descriptor.proto)'s placeholder `Literal` is fleshed out (RFC-001 §8.1, issue [#64](https://github.com/trendvidia/protowire/issues/64)): enum references lower **resolved** (`EnumLiteral {enum_type, value_name, number}` — consumers never re-resolve bare names); message literals stay `google.protobuf.Any`, always explicitly typed (param-declared type or use-site type name, never shape inference); list literals become homogeneous `ListLiteral { repeated LiteralValue }` — elements are values, not arguments (no names, no expressions; mixed kinds rejected at compile). In-place revision of the unreleased carrier; `protocompile` re-vendors `descriptor.pb.go` as M1 follow-up (no shipped grammar path emits these variants yet).
- **Backward compatibility.** Wire format unchanged for v1.1 schemas. Stock `protoc`, `protobuf-go`, and every existing protowire port round-trip the new carrier extensions byte-identically as opaque options. v1.1 ports parse v1.1 schemas as before; v1.2 grammar requires v1.2+ ports.

### New files

- `docs/RFC-001-schema-extensions.md` — design RFC (this release's driving spec).
- `docs/RFC-001-issues.md` — tracking-issue scaffold for the v1.2.0 work, paste-ready across consuming repos.
- `proto/schema/v1/descriptor.proto` — descriptor lowering targets (stock-proto3, parseable by any v1.x port).
- `proto/schema/v1/annotations.proto` — canonical user-facing annotation declarations (requires v1.2 grammar; lands once parser support exists, see [#004](docs/RFC-001-issues.md)).
- `proto/schema/v1/report.proto` — validation report wire shapes (stock-proto3; runtime artifact emitted by engines, allocates no extension numbers).
- `proto/schema/config/v1/config.proto` — project-level engine configuration (stock-proto3; build-time artifact loaded from `protowire.config.textproto`, never embedded in descriptors).

### Per-port implementation status

Adoption is independent per port. Reference Go implementation lands in `protocompile`/`protocheck`/`protolsp`/`pxfed`/`protowire-go` (M1–M5); other ports adopt on their own schedule (M9+). See RFC-001 Appendix B for tracker.

### Keyed repeated fields

Strictly additive per [`STABILITY.md`](STABILITY.md) — the parser accepts strictly more input than before; every valid earlier document remains valid and means the same thing. Driven by [#116](https://github.com/trendvidia/protowire/issues/116).

- **`field_entry` accepts a quoted entry name.** The production becomes `field_entry = (identifier | string), (assignment_tail | block_tail)`, mirroring `map_key`. The grammar accepts quoted entry names everywhere (parsers are schema-independent); the schema layer rejects them outside keyed repeated fields. Updated in [`docs/grammar.ebnf`](docs/grammar.ebnf), the railroad diagram, and IETF draft `-01` (§3.3 ABNF, §3.5 entries, new §3.13, appendix ABNF-additions note).
- **`(pxf.key) = 50002`.** New `FieldOptions` extension in [`proto/pxf/annotations.proto`](proto/pxf/annotations.proto). Valid only on repeated message-typed fields; the value names a singular **string** field of the element message (bind-time errors otherwise; non-string key types are a possible future additive extension). Standard renumbering prohibition applies.
- **Keyed surface form and semantics** (draft `-01` §3.13). A keyed repeated field MAY be written as a block of named blocks: entry name = key-field value (unquoted value for string-literal names), entry order = list order. Decode errors: duplicate entry names in one block (compared by unquoted value — `"foo"` ≡ `foo`), the empty string as a key (either surface form), and an explicit key-field assignment disagreeing with its entry name (agreeing is redundant but legal). Encode/fmt: keyed form whenever all keys are present, non-empty, and distinct — quoted only when not identifier-safe, key field not re-emitted inside the entry — anonymous list form otherwise. Entry names are atoms (dots are not path separators). `pb`/SBE wire outputs are untouched.
- **Cross-port fixtures.** New [`testdata/keyed/`](testdata/keyed/) corpus: keyed and quoted round-trips, fmt canonicalization pairs (unquote + anonymous→keyed), anonymous-form equivalence, redundant-key and anonymous-duplicate accepts, and the duplicate-key / spelling-equivalence / key-conflict / empty-key / quoted-name-outside-keyed rejects.
- **Port reach.** As a syntax change this lands in every from-scratch parser: reference implementation in `protowire-go#50`, per-port issues cross-linked from #116 (cpp, rust, java, typescript, csharp, swift, dart; kotlin and python inherit).
- **`cmd/pxf` keyed support (landed).** The shared CLI is bumped to `protowire-go v1.3.0` (the reference keyed implementation) and now handles keyed repeated fields across `encode` / `decode` / `validate` / `fmt`. `validate` surfaces the typed keyed decode errors (duplicate/empty/conflict/quoted-name-outside-keyed) with source positions. `fmt` is reworked onto the reference `CanonicalizeKeyed` + `FormatDocument` pipeline: it now formats losslessly (comments preserved), canonicalizes eligible collections to keyed form, and binds the schema from `--message` or the document's own `@type` (formatting still works with no schema; only keyed canonicalization needs one). Drive-by: `pxf encode` now marshals deterministically, so the same document always produces the same bytes — previously the `proto.Marshal` field order over a `dynamicpb` message varied run to run, which would defeat the byte-level cross-port wire checks.

## [1.1.1] – 2026-07-15

Tooling patch on the v1.0 spec line. No grammar, wire-format, or
envelope changes.

### Spec changes

None.

### Tooling

- **`pxf query` binds and emits non-finite floats as the spec's
  identifiers** (#86). With a schema in scope, a `@dataset` cell
  holding bare `nan`/`inf` bound as the *string* `"nan"`/`"inf"`
  instead of a non-finite float (§3.8: the bare forms lex as
  identifiers, and the schema layer never re-bound them on
  float/double fields; the signed forms already bound correctly).
  On output, non-finite floats emitted via Go's `NaN`/`+Inf`/`-Inf`
  spellings, which are not PXF literals and did not re-parse — they
  now emit as `nan`/`inf`/`-inf`. Also bumps the CLI's protowire-go
  pin to v1.2.2, since v1.0.0's lexer rejected the signed forms
  (`-inf`) in dataset cells outright.

## [1.1.0] – 2026-05-15

Tooling release on the v1.0 spec line. No grammar, wire-format, or
envelope changes — the spec is frozen at 1.0. This tag bundles the
post-freeze CLI consolidation, the new `pxq` query tool, and the
editor extension's rewrite as a thin LSP client.

### Spec changes

None. Existing v1.0 parsers and ports continue to work without
modification.

### Tooling

- **`cmd/protowire` and `cmd/pxq` unified into a single `pxf`
  binary** (#42). The old `protowire` and `pxq` commands are gone;
  every subcommand now lives under `pxf <subcommand>`. **Breaking
  for users with scripts that invoke `protowire` or `pxq`
  directly** — update them to `pxf`. The grammar / wire format /
  envelope are unchanged, so on-disk documents remain compatible.
- **`pxq` query subcommand** (now `pxf q`): jq-style transforms
  over `.pxf` documents with three input adapters (JSON, YAML,
  CSV) and protoregistry-backed schema resolution. Lands in
  stages A (#32), B (#33), C (#34), plus follow-ups for in-document
  `@proto` binding (#35, #36), bundled canonical schemas (#37),
  protoregistry `-s` / `-n` / `--schema` flags (#38), schema
  inference (#39), and a strict-mode AST validator (#40). Design
  doc at `docs/design/pxq.md`.

### Editor extensions

- **VS Code 2.0.0 → 2.1.0** — the extension rewrites as a thin
  `vscode-languageclient` host spawning the
  [protolsp](https://github.com/trendvidia/protolsp) Language
  Server over stdio (#47). 2.1.0 adds a `TextDocumentContentProvider`
  for the `registry:` URI scheme so go-to-definition can open
  `.proto` sources fetched from protoregistry when they aren't on
  disk (#48). The in-extension `@trendvidia/protowire` parser is
  gone — diagnostics, hover, completion, code actions, and
  go-to-definition all come from the LSP now.
- **JetBrains**: gradle-wrapper bump only (#44); plugin version
  unchanged at 1.0.0.

### Internal

- Schema-resolver helpers extracted to `internal/schemaresolve` so
  `pxf check` and `pxf q` (and future tools) share one
  protoregistry-resolution code path (#41).

### Pre-built editor bundles

- `editors/vscode/dist/pxf-2.1.0.vsix` (replaces 2.0.0).
- `editors/jetbrains/plugin/dist/pxf-jetbrains-1.0.0.zip` (unchanged).

## [1.0.0] – 2026-05-13

First major-version cut and spec freeze line. Three one-time spec
changes ship in lockstep across the whole `protowire-*` stack.
**Breaking** — there is no alias period; v1.0 is itself the major bump.
See `STABILITY.md` for the rules-of-engagement after v1.0.

### Spec changes

- **`@table` → `@dataset` rename** (draft §3.4.4). Same semantics; the
  directive represents the dataset (rows), not the storage container.
  v1 reserves `@table` for a future storage-definition meaning in the
  database export/import direction sketched in §3.4.6.
- **`@proto` directive added** (draft §3.4.5). Four body shapes
  lexically distinguished: anonymous (`@proto { ... }`), named
  (`@proto pkg.Type { ... }`), source (`@proto """..."""`), descriptor
  (`@proto b"..."`). Descriptor form is the MUST-support shape; the
  other three are QoI. Anonymous `@proto` is consumed one-shot by the
  next directive that requires a typed binding.
- **Reserved directive names expanded from 5 to 13** (draft §3.4.6).
  Adds `dataset`, `proto`, `entry` (promoted from spec-registered),
  and the future-allocated `table`, `datasource`, `view`, `procedure`,
  `function`, `permissions`.

### Source switch

The IETF draft authoring source moves from raw `.txt` to
kramdown-rfc Markdown:
`docs/draft-trendvidia-protowire.md` is now the source of truth;
the paginated `.txt` is regenerated via `scripts/build_rfc.sh`.

### Port releases at v1.0 freeze

- `protowire-go` v1.0.0
- `protowire-java` v1.0.1 (the v1.0.0 tag exists in git but the
  Maven Central publish failed on two leftover javadoc `Result#tables()`
  references that survived the rename; v1.0.1 ships the fix)
- `protowire-typescript` v1.0.0 (published to npm as
  `@trendvidia/protowire@1.0.0`)

### Editor extensions

Both extensions bump to v1.0.0 in lockstep and pick up the new
parser bundles:

- **VS Code** `editors/vscode/dist/pxf-1.0.0.vsix` — embeds
  `@trendvidia/protowire@1.0.0`. TextMate grammar highlights
  `@dataset` and `@proto`.
- **JetBrains** `editors/jetbrains/plugin/dist/pxf-jetbrains-1.0.0.zip`
  — embeds the `protowire-pxf-1.0.1.jar` parser. Same grammar
  changes as VS Code.

Documents that use `@table` will get red squiggles under v1.0.0
extensions; rename to `@dataset` to clear them.

## [0.74.0] – 2026-05-12

Streaming-consumption release. One informational addition to §3.4.4:
implementations MAY expose a row-by-row streaming API alongside the
materializing one (and for `@table`'s CSV-replacement use case
typically SHOULD), with a pinned contract on row order, per-row
enforcement, and working-set memory. No grammar change, no wire
change. First-port implementation: `protowire-go` v0.74.0
(`pxf.TableReader` over `io.Reader`).

### Changed

- **PXF spec — `@table` streaming consumption.** Section 3.4.4 grows
  a "Streaming consumption" paragraph stating that implementations
  MAY (and for the CSV-replacement use case typically SHOULD) expose
  a row-by-row streaming API alongside the materializing one. The
  contract: rows in source order, per-row arity + cell-grammar
  enforced as each row is consumed (not deferred), working-set
  memory bounded by the largest single row. Streaming and
  materializing APIs that coexist in the same implementation MUST
  produce byte-identical row sequences for the same input.

  No grammar change, no wire change — this is informational, making
  explicit what §3.4.4's existing "consumer-interpreted side-channel"
  framing already permitted. Without it, port maintainers reasonably
  read "rows are exposed through a parser API" as mandating full
  materialization.

  Spec changes:
  - `docs/draft-trendvidia-protowire-00.txt` §3.4.4: new paragraph
    after "Consumer contract."
  - `docs/grammar.ebnf`, `docs/grammar.svg`: unchanged.

  First-port implementation: `protowire-go` v0.74.0 (`pxf.TableReader`
  over `io.Reader`, `NewTableReader`/`Type`/`Columns`/`Directives`/
  `Next`). Other ports add streaming when their CSV-replacement
  consumers ask for it; no conformance obligation to expose it.

## [0.73.0] – 2026-05-11

Schema-constraint + directive-expansion release. Three additions to
the PXF text format, all strictly additive on the wire: a new
schema-level reserved-name rule (Section 3.13), the `@entry` bundle
directive (Section 3.4.3) with a generalized zero-or-more
prefix-identifier list on every named directive, and the `@table`
bulk-rows directive (Section 3.4.4) — the protowire-native
replacement for CSV. Wire format unchanged. First-port
implementation: `protowire-go` v0.73.0.

### Added

- **PXF grammar — `@table` directive (CSV replacement).** New top-level
  directive form for representing many instances of a single message
  type in a single PXF document. Syntax:

  ```
  @table <type> ( <col1>, <col2>, ... )
  ( <val1>, <val2>, ... )
  ( <val1>, <val2>, ... )
  ```

  The header names the row message type and the column list (top-level
  field names on `<type>`); each subsequent parenthesized tuple is a
  row whose values bind positionally to the columns. Empty cells
  (between two commas) denote absent fields and engage the existing
  `pxf.default` / `pxf.required` machinery; `null` literals denote
  present-but-null; any other value is present-with-value. Same
  three-state semantics as the keyed form, just spelled positionally.

  v1 restrictions (relaxed in a future revision):
  - Cell values are scalar-shaped (`value − list − block_value`).
    List literals `[...]` and block values `{...}` are NOT permitted
    in cells.
  - Column entries are unqualified field names. Dotted paths
    (`addr.city`) are NOT permitted.
  - Strict row arity: row arity MUST equal column count. No
    trailing-empty shorthand.
  - Standalone: a document containing `@table` MUST NOT also contain
    `@type` or top-level field entries. The `@table` header is the
    document's type declaration.

  `@table` is consumer-interpreted in the same side-channel manner as
  `@header` / `@entry`: rows are exposed through a parser API distinct
  from the body's schema layer. This spec does NOT mandate a canonical
  "decode-as-`repeated <type>`" semantics — applications that want
  that one-liner construct it on top of the rows API.

  Wire format unchanged. Strictly additive — any v0.74.0-valid PXF
  document remains valid (the new productions occupy fresh top-level
  surface).

  Spec changes:
  - `docs/grammar.ebnf`: new `table_directive`, `column_list`, `row`,
    `row_cell`, `row_value` productions; `directive` choice extended;
    `directive_name` excludes `table`.
  - `docs/draft-trendvidia-protowire-00.txt` §3.3 ABNF: matching
    additions. New §3.4.4 "The @table Directive" with normative
    conformance rules.
  - `docs/grammar.svg`: regenerated (47 rules; +5 over the prior
    revision).

  Editor support:
  - `editors/vscode/syntaxes/pxf.tmLanguage.json`: new
    `table-directive` pattern highlights `@table <type>` consistently
    with `@type`; new paren punctuation rules.
  - JetBrains bundle regenerated.

  Testdata:
  - `testdata/example-table.pxf` — happy path over `test.v1.AllTypes`
    with five columns and four rows, exercising the three cell states.
  - `testdata/table/` — adversarial fixtures: short/long row arity,
    `@table` + `@type`, `@table` + body field, list cell, block cell,
    dotted column. Each fixture states its violation in the leading
    comment. Conformance-harness wiring deferred.

  Ports: implementing `@table` is a new lexer entry (`@table`
  keyword), a new parser path (header + row loop), and a thin
  consumer-API surface (`parser.tables()` or equivalent returning
  an ordered list of `TableBlock` records). The schema layer never
  receives table rows as message body entries.

  Carry-forward fix: PR #16's grammar.ebnf comment referenced
  "Section 3.4.4" for `@entry`; the draft places it at §3.4.3.
  Corrected here while updating the same notes block for `@table`.

- **PXF grammar — `@entry` directive + generalized prefix list.** The
  `named_directive` production grows from a single optional prefix
  identifier to zero-or-more: `@<name> *(<prefix-id>) [ { ... } ]`.
  Any v0.72.0-valid named directive remains valid; the change is
  strictly additive. Spec registers `@entry` as the first
  in-spec-defined named directive (Section 3.4.3), with four
  permitted shapes:

  ```
  @entry { ... }                       ; anonymous, typeless
  @entry name { ... }                  ; labeled, typeless
  @entry some.pkg.Type { ... }         ; typed only (dotted ident)
  @entry name some.pkg.Type { ... }    ; labeled and typed
  ```

  `@entry` is consumer-interpreted; this document defines no meaning
  for the label beyond preservation in directive order. The canonical
  use case is manifest documents that bundle heterogeneous, typed
  sub-messages alongside a body.

  Wire format unchanged — text-format-only sugar. Ports MUST relax
  their `named_directive` lexer to accept zero-or-more prefix
  identifiers before claiming the next-version conformance (~1-line
  change in most ports: replace "accept one optional identifier"
  with "loop accepting identifiers until you hit `{` or end-of-
  directive"). Ports MUST enforce the `@entry`-specific cardinality
  (0–2 prefix identifiers) at the consumer layer.

  Spec changes:
  - `docs/grammar.ebnf`: `named_directive` uses `{ identifier }`
    instead of `[ identifier ]`. New notes block describes the
    per-directive cardinality convention.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3 ABNF:
    `named-directive` uses `*( 1*WSP identifier )`. Section 3.4.2
    rewritten to describe per-directive prefix semantics. New
    Section 3.4.3 defines `@entry`.
  - `docs/grammar.svg`: regenerated.

  Editor support:
  - `editors/vscode/syntaxes/pxf.tmLanguage.json`: new
    `named-directive` pattern highlights `@<name>` plus prefix
    identifiers (dotted → type; bare → tag).
  - JetBrains bundle regenerated via
    `scripts/sync_jetbrains_grammar.py`.

  Testdata: `testdata/example-entries.pxf` demonstrates the four
  shapes alongside a `@type` body declaration.

### Changed

- **PXF schema constraint — reserved names.** A protobuf schema bound
  for PXF use MUST NOT declare a message field, oneof, or enum value
  whose name is case-sensitively equal to `null`, `true`, or `false`.
  These names lex as PXF value keywords (Section 3.9 of the draft), so
  a field/oneof/enum value bearing such a name is unreachable from
  PXF surface syntax — the tokenizer always resolves the bare token
  to the keyword branch. The directive-name exclusion is widened
  symmetrically: `@null`, `@true`, and `@false` join `@type` as
  reserved directive names.

  Wire format unchanged — this is a static-time schema check, not a
  wire migration. Schemas that violate the constraint were never
  round-trippable through PXF; rejecting them at descriptor-bind time
  surfaces a pre-existing latent bug rather than introducing one.

  Spec changes:
  - `docs/grammar.ebnf`: `directive_name` production now excludes
    `null`/`true`/`false` in addition to `type`. New "Schema
    Constraints" notes block formalizes the field/oneof/enum-value
    rule.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3 ABNF:
    `directive-name` exclusion list updated. New Section 3.13
    ("Schema Constraints") states the rule with MUST-language and
    bind-time conformance requirements. Cross-references added from
    Sections 3.9 and 6.1. (Section page numbering downstream of
    §3.13 has drifted; re-paginate before submission.)
  - `docs/grammar.svg`: regenerated.

  Tooling:
  - New `internal/pxfschema` Go package with `ValidateReflect` /
    `ValidateProto` entry points covering both descriptor shapes
    used in this repo.
  - `cmd/protoc-gen-pxf-java-meta`: rejects non-conforming
    `FileDescriptorSet`s at the top of `generateFile`.
  - `cmd/pxf`: new `lint` subcommand that runs the same check
    standalone against `--proto` or `--server` schemas.

  Ports MUST add the equivalent descriptor-bind check before
  claiming the next-version conformance. The check is ~30 lines per
  port (walk messages/oneofs/enum-values, case-sensitive set
  match).

## [0.72.0] – 2026-05-11

Named-directive release. Extends the PXF text format with
application-extensible `@<name> [<type>] [{ ... }]` blocks at
document root, alongside the existing `@type` directive. Wire format
unchanged. First-port implementation: `protowire-go` v0.72.0.

### Added

- **PXF grammar — named directives.** The document grammar grows a
  generic `@<name> [<type>] [{ ... }]` form alongside the existing
  `@type` directive. `@type` retains its current meaning (declares
  the body's message type, at most one per document); other names
  carry side-channel metadata that the consumer's runtime
  interprets — never the body's schema layer.

  Decoders that don't recognize a directive name MAY skip the
  directive after parsing its block for syntactic well-formedness,
  or MAY reject. The inner block uses the same entry grammar as a
  message body, so brace-matching, string-literal, and comment
  rules apply uniformly.

  Wire format unchanged — this is text-format-only sugar. Ports
  that already shipped a stricter "only @type recognized" lexer
  MUST relax it to accept the new form before claiming v0.72.0
  conformance.

  Spec changes:
  - `docs/grammar.ebnf`: new `directive`, `named_directive`, and
    `directive_name` productions; `document` rule generalized to
    accept any sequence of directives before the body.
  - `docs/draft-trendvidia-protowire-00.txt` Section 3.3: ABNF
    grammar updated.  Section 3.4 split into 3.4.1 ("@type
    Directive") and 3.4.2 ("Named Directives") with conformance
    rules for unrecognized names.

  Motivating use case in the wild: chameleon's `@header
  chameleon.v1.LayerHeader { id = "x" }` preamble at the top of
  layer files, carrying per-file sanity-check fields the resolver
  cross-checks against its chain spec.

  First-port implementation: `protowire-go` v0.72.0.

## [0.70.0] – 2026-05-06

First tagged baseline of the spec repo, matching the `v0.70.x` release
line cut by every sibling port. Establishes the cross-port wire-equivalence
reference point and the adversarial corpus all ports must accept (or
reject, where the corpus probes hardening).

### Changed

- **PXF grammar (breaking)** — `docs/grammar.ebnf` now distinguishes
  `field_entry` (identifier key + `=` or `{ … }`) from `map_entry`
  (id/string/integer key + `:`). At the document top level only
  `field_entry` is accepted; `map_entry` is reserved for the inside of
  `{ … }` blocks where it represents map literal entries. Inputs like
  `123 = 234` and top-level `123: 234` are now parse errors.
  - All ports' parsers must mirror this; new adversarial fixtures
    `testdata/adversarial/pxf/{integer-key-assignment,integer-key-in-block,top-level-map-entry}.pxf`
    fail any port that hasn't caught up.

### Added

- **Editor extensions** under [`editors/`](editors/) — VS Code (`.vsix`)
  and JetBrains (`.zip`) plugins shipping prebuilt for offline install.
  Both bundle their port's own parser (`protowire-typescript` and
  `protowire-java` respectively) for inline parse-error squiggles, plus
  a TextMate grammar for syntax highlighting.
- **`docs/HARDENING.md`** adversarial corpus + per-port `check-decode`
  conformance harness, gated by
  [`scripts/cross_security_check.sh`](scripts/cross_security_check.sh).
- **Project security policy** at [`SECURITY.md`](SECURITY.md) with a
  contact and a 30-day coordinated-disclosure embargo for cross-port
  issues.
