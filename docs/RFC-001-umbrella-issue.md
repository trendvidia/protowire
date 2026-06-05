# Umbrella tracking issue — paste-ready

This file contains the paste-ready body for the umbrella tracking issue
for **Protowire Schema Extensions** (RFC-001, targeting protowire v1.2.0).

**How to use:**

1. Open a new issue in the `trendvidia/protowire` tracker.
2. Suggested title:
   `[META] Protowire Schema Extensions (v1.2.0) — RFC-001`
3. Suggested labels: `meta`, `schema-extensions`, `tracking`
4. Paste the content below the `--- begin paste ---` marker.
5. As child issues are filed in their respective trackers, replace the
   `#TBD` placeholders with the real issue numbers (and repo paths,
   since several issues live in repos other than `protowire`).

The body below renders cleanly in both GitHub Issues and Linear.

---

--- begin paste ---

# Protowire Schema Extensions (v1.2.0) — RFC-001

| | |
|---|---|
| **Status** | Draft — RFC awaiting ratification |
| **Target spec version** | protowire v1.2.0 (strictly additive minor) |
| **IETF draft** | `draft-trendvidia-protowire-01` (in preparation) |
| **Driving design doc** | [RFC-001](docs/RFC-001-schema-extensions.md) |
| **Implementation issues** | [RFC-001-issues.md](docs/RFC-001-issues.md) |
| **Conformance fixtures** | [`testdata/schema-extensions/`](testdata/schema-extensions/) |

## Overview

This umbrella tracks the v1.2.0 protowire schema-extension work — three
new top-level declarations (`type`, `function`, `annotation`), a
unified `@annotation(...)` use-site syntax, and a structured validation
error model. The additions promote message validation to a first-class
schema concern.

All additions are **strictly additive**: every schema valid under v1.1
remains valid under v1.2. Wire formats (PXF, PB, SBE, envelope) are
unchanged. The new constructs lower to standard `FileDescriptorSet`
plus custom-option carriers in extension numbers `50100`–`50104`
(reserved in [`STABILITY.md`](STABILITY.md)), so existing tooling
round-trips them transparently.

Per-port adoption is independent. Reference Go implementation lands
across `protocompile`, `protocheck`, `protolsp`, `protobuf-go` (custom
fork), and `pxfed`; other ports follow once the Go reference is proven.

## Reference documents

- **[RFC-001 — Schema Extensions](docs/RFC-001-schema-extensions.md)** — design rationale, locked decisions, open questions.
- **[IETF draft `-01`](docs/draft-trendvidia-protowire-01.md)** — formal spec text (companion to `-00`).
- **[RFC-001 issue scaffold](docs/RFC-001-issues.md)** — paste-ready bodies for every child issue.
- **[`STABILITY.md`](STABILITY.md)** — v1.2 additive surface and extension-number reservation policy.
- **[`CHANGELOG.md`](CHANGELOG.md)** — `[Unreleased]` entry summarizing v1.2.0.
- **[`proto/schema/v1/descriptor.proto`](proto/schema/v1/descriptor.proto)** — descriptor lowering targets (stock-proto3).
- **[`proto/schema/v1/annotations.proto`](proto/schema/v1/annotations.proto)** — canonical annotation library (requires v1.2 parser).
- **[`testdata/schema-extensions/`](testdata/schema-extensions/)** — initial conformance fixtures.

## Status

### M0 — Spec freeze

- [x] `proto/schema/v1/descriptor.proto` committed (#TBD)
- [x] `STABILITY.md` updated with v1.2 additive surface (#TBD)
- [x] `CHANGELOG.md` `[Unreleased]` populated (#TBD)
- [x] Initial conformance fixtures committed (`testdata/schema-extensions/`) (#TBD)
- [ ] **Ratify RFC-001** (#TBD)
- [ ] **Publish IETF draft `-01`** (#TBD)
- [ ] `proto/schema/v1/annotations.proto` (lands during M1 once v1.2 parser exists) (#TBD)

### Open questions (deferred or in-design)

Each is its own tracked issue. Resolution is required before publication
or deferral to a future minor must be formally agreed.

- [ ] Container-shaped type aliases (deferred to v1.3+) (#TBD)
- [ ] Engine-config file format (#TBD)
- [ ] Well-known types semantics (Timestamp, Duration, Any) (#TBD)
- [ ] Recursive message validation depth limits (#TBD)
- [ ] Streaming RPC validation contract (#TBD)
- [ ] `Literal` shape in `AnnotationArg` (#TBD)
- [ ] Validation report wire shape (#TBD)
- [ ] protovalidate migration story (#TBD)
- [ ] Performance budget + benchmark suite (#TBD)
- [ ] Conformance test corpus expansion (#TBD)
- [ ] Upstream `buf/protocompile` compatibility strategy (#TBD)

### M1 — Parser + IR (Go reference, `protocompile`)

- [ ] `protocompile`: extended grammar parser (#TBD — `trendvidia/protocompile`)
- [ ] `protocompile`: IR for type/function/annotation (#TBD — `trendvidia/protocompile`)
- [ ] `protocompile`: linker symbol resolution (#TBD — `trendvidia/protocompile`)

### M2 — Lowering + carrier (`protocompile`)

- [ ] `protocompile`: option-interpretation hook for `@annot` → carrier (#TBD)
- [ ] `protocompile`: descriptor lowering pass (#TBD)

### M3 — Source map + LSP foundation

- [ ] `protocompile`: source-map emission (#TBD)
- [ ] `protolsp`: extended grammar parsing (#TBD — `trendvidia/protolsp`)
- [ ] `protolsp`: source-map consumption + go-to-definition (#TBD)

### M4 — Engine SPI + runtime (`protocheck`)

- [ ] `protocheck`: engine SPI (Go interface) (#TBD — `trendvidia/protocheck`)
- [ ] `protocheck`: function registration + runtime-init verification (#TBD)
- [ ] `protocheck`: validation execution (#TBD)

### M5 — Codegen + runtime wiring

- [ ] `protobuf-go`: function-stub codegen plugin (Go) (#TBD — `trendvidia/protobuf-go`)
- [ ] `protowire-go`: runtime wiring through `protocheck` (#TBD)
- [ ] Performance budget + benchmark suite (#TBD)

### M6 — i18n

- [ ] `protocheck`: catalog support + i18n (#TBD)

### M7 — Tooling integration

- [ ] `protolsp`: annotation-aware diagnostics (#TBD)
- [ ] `protobuf-go`: annotation-aware codegen (#TBD)

### M8 — OpenAPI generator

- [ ] OpenAPI generator (separate tool, repo TBD) (#TBD)

### M9+ — Per-port adoption

Each port adopts the v1.2 spec independently once the Go reference is
proven. Schedule per port.

- [ ] `protowire-java`
- [ ] `protowire-typescript`
- [ ] `protowire-python`
- [ ] `protowire-cpp`
- [ ] `protowire-rust`
- [ ] `protowire-csharp`
- [ ] `protowire-kotlin`
- [ ] `protowire-swift`
- [ ] `protowire-dart`

## How to participate

- **Want the design context?** Start with [RFC-001](docs/RFC-001-schema-extensions.md).
- **Want to implement something?** Open the corresponding child issue and check its "Implementation notes" section in [RFC-001-issues.md](docs/RFC-001-issues.md) — each enriched issue has file-level entry points.
- **Have a design question?** Comment on the matching open-question issue above, or open a new one with the `schema-extensions` and `design` labels and link it here.
- **Found a bug?** Open against the implementing repo (e.g., `protocompile` for parser bugs, `protocheck` for runtime bugs); reference this umbrella for context.

## Maintainer notes

- **Backward compatibility.** v1.2 is strictly additive on the v1.0 spec-freeze line. The only soft-break is the new reserved keywords (`type`, `function`, `annotation`, `expression`, `this`) — application schemas using any of these as message/oneof/enum-value identifiers must rename. Document and search for this in any consumer that lints schemas.
- **Extension number range.** Reservation is `50100`–`50199` for schema-extension carriers. `50100`–`50104` are allocated. Allocations beyond this range MUST go through an RFC update; renumbering is a wire break.
- **Per-port adoption is decoupled.** A port at v1.1 reading a v1.2 schema rejects the new keywords at parse time. A port at v1.2 reading a v1.1 schema accepts it unchanged. Schemas pin to the highest minor they use; consumers must match.
- **Ratification gate.** This issue moves from `Draft` to `Active` once RFC-001 has at least one approving review from spec governance AND all open-question issues are either resolved or filed with a `v1.2 deferred` label.

--- end paste ---
