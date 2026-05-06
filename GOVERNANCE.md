# Project Constitution & Governance

This repository is governed autonomously by [Steward](https://steward-dev.ai). Human maintainers do not merge Pull Requests. Code is merged exclusively through mathematically verified consensus, automated static analysis, and supply-chain checks as defined in this Constitution.

> **Note:** The machine-readable source of truth for this constitution is [`governance.pxf`](governance.pxf) at the repository root, validated against Steward's governance schema. This Markdown file is the human-readable preamble; if the two ever diverge, `governance.pxf` wins. Steward's audit log only claims to enforce clauses that the engine actually verifies — clauses marked `(roadmap)` are aspirational and will be activated once the corresponding subsystem ships.

## 1. Core Manifesto & Scope

protowire is a **language-neutral wire-format specification** for PXF (text), PB (protobuf binary, with PXF-specific annotations), SBE (FIX Simple Binary Encoding), and a shared response envelope. The mission is *human-friendly text serialization for protobuf schemas, plus PB/SBE bindings — tiny, fast, zero-framework.*

* **Mission Drift Gate.** Steward rejects PRs that introduce heavy frameworks into core paths. The current block-list lives under `manifesto.blocked_module_globs` in `governance.pxf` (e.g. `github.com/spf13/viper`, `github.com/gorilla/**`). Add to it as the project grows; don't remove without an amendment.
* **Performance budget.** Serialization libraries are perf primitives. A >10% regression on the canonical fixtures fails the PR (`manifesto.max_perf_regression_percent = 10`). The cross-port bench harness (`scripts/cross_pxf_bench.sh`, `scripts/cross_sbe_bench.sh`) is the source of measurement; CI gating is tracked under [ROADMAP M5](ROADMAP.md#m5--performance-regression-ci-target-0740).

## 2. Domain Vectors & Voting Thresholds

Reputation is strictly scoped to architectural domains. Voting weight applies only to the domains a PR actually modifies. Domain order matters when path globs overlap — most-specific (longest prefix) first.

| Domain Vector | Path Match | Min. Coverage | Max Cyclomatic Complexity | Approval Threshold | Supermajority |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **core-encoding** | `encoding/**` | 95% | 8 | 1500 | 75% |
| **proto-schemas** | `proto/**` | n/a | n/a | 1000 | 75% |
| **envelope** | `envelope/**` | 90% | 10 | 800 | 51% |
| **cli** | `cmd/**` | 70% | 12 | 500 | 51% |
| **editors-and-tooling** | `editors/**`, `scripts/**` | n/a | n/a | 300 | 51% |
| **documentation** | `**/*.md`, `testdata/**` | n/a | n/a | 100 | plurality |

`core-encoding`, `proto-schemas`, and `envelope` are **restricted** — zero-reputation contributors land in escrow for changes there. `.proto` files have no Go-style coverage so the coverage gate is exempt; the schema-change gate is the supermajority threshold above plus the constitutional-amendment process in § 6.

## 3. The Escrow Pipeline (new contributors)

All contributors start with a baseline reputation vector of `0`.

* **Quarantine trigger.** A `0`-reputation PR to any restricted domain (`core-encoding`, `proto-schemas`, `envelope`) goes into **Escrow**.
* **Unlock condition.** Steward auto-assigns 2 `sandbox`-labeled issues from `escrow.sandbox_templates` in `governance.pxf`. Merge those, and the original PR unlocks for community voting.

The sandbox templates are intentionally low-blast-radius: regression tests for known PXF parsing edge cases, error-message tightening in `encoding/{pxf,pb,sbe}`, doc comments on undocumented exported functions.

## 4. Time Decay & Reputation Kineticism

Reputation decays so the roadmap stays driven by active maintainers.

* **Half-life.** 720 hours (30 days). Points decay to 50% over each half-life of inactivity.
* **Reset.** Successfully merging a PR resets the decay timer for that domain.
* **Founder seed.** The original maintainer (`decoder`) is seeded with enough reputation to reach approval thresholds in every domain on day one; natural decay dilutes that over ~6 months as community reputation accrues. See `founders` in `governance.pxf`.

## 5. The Autonomous Immune System (security gates)

Steward enforces these non-negotiable configurations. Violations are rejected without a vote.

1. **Supply chain.** Only `MIT`, `Apache-2.0`, and `BSD-3-Clause` transitive dependencies are permitted. GPL/Copyleft dependencies trigger an immediate block.
2. **Pinned dependencies.** All external modules must be strictly pinned. Wildcard / caret versioning is blocked. (Currently relaxed for `trendvidia/protobuf-go` while it tags its first public release — see `immune_system.require_pinned_dependencies` for current state.)
3. **Domain Anomaly Quarantine.** A contributor whose reputation is >90% concentrated in `documentation` who attempts to modify `**/encoding/**` or `**/proto/**` triggers a 72-hour timelock before voting opens. This catches doc-only contributors who suddenly attempt cryptographic / wire-format changes.
4. **Slashing.** When a merged PR is later proven (via post-merge regression) to have introduced a bug, the original author's reputation is slashed in the relevant domain. Base 50 points; growth half-life 720h; max 100×.

## 6. Constitutional Amendments

To modify [`governance.pxf`](governance.pxf), this `GOVERNANCE.md`, or anything under `proto/**`, a PR must achieve a **75% supermajority across the top 10 reputation holders** in *all* defined domain vectors simultaneously. Voting period is hard-coded to **14 days** (`amendments.voting_period`).

Wire-format breaking changes — bumping the envelope from `v1` to `v2`, renumbering an annotation extension, narrowing the PXF grammar — require a major bump on the project as a whole, not just the affected port. See [`STABILITY.md`](STABILITY.md) for the full wire-compatibility contract this protects.

## 7. Equivalence-group taxonomy

The "alternatives" gate (which would force comparison against incumbent equivalents before adopting a new dependency) is **disabled** here. protowire *is* the library — there is no equivalence-group taxonomy that applies. See `alternatives.enabled = false` in `governance.pxf`.
