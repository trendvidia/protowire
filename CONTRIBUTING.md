# Contributing to protowire

> **Steward integration is rolling out.** The governance described below
> is the steady-state model. While the integration is being finalised,
> pull requests are reviewed by human maintainers in the conventional
> way — please open a PR, expect review comments, and iterate. The
> domain-vector / reputation / escrow mechanics in [`GOVERNANCE.md`](GOVERNANCE.md)
> become enforced once Steward goes live; clauses marked `(roadmap)` in
> [`governance.pxf`](governance.pxf) are aspirational until then.

Welcome. This repository operates differently than most open-source projects. To ensure mathematical fairness, prevent maintainer burnout, and keep cross-port wire-equivalence verifiable, **this project is governed by an autonomous AI agent named [Steward](https://steward-dev.ai).**

There are no human gatekeepers. Pull requests are evaluated, scored, and merged based strictly on objective rules defined in [`GOVERNANCE.md`](GOVERNANCE.md) and its machine-readable source of truth, [`governance.pxf`](governance.pxf).

This repo is the **canonical, language-neutral specification** for PXF, PB, SBE, and the response envelope. The nine language ports live in sibling repositories — see the [Implementations table in the README](README.md#implementations). Per-port library bugs go to the port repo; cross-port wire-format issues come here.

## 1. How code gets merged

1. **Open a Pull Request.**
2. **Steward evaluates it.** Static analysis, cyclomatic complexity, dependency-license check, and the required reputation threshold for the touched domains (see [§ Domain Vectors](GOVERNANCE.md#2-domain-vectors--voting-thresholds)).
3. **Steward posts a Diagnostic Log** on the PR with the breakdown.
4. **Community voting** (when required). PRs that pass all gates open for weighted voting.
5. **Auto-merge** once consensus is reached.

## 2. The Escrow Pipeline (new contributors)

First-time contributors start with a Reputation Vector of `0`.

**Don't submit large changes to `encoding/**` or `proto/**` as a first PR.** Steward places zero-reputation PRs to restricted domains into **Escrow**.

To unlock escrow:

1. Steward auto-generates 2 `sandbox`-labeled issues for you. The seed templates for this repo are in [`governance.pxf`](governance.pxf) under `escrow.sandbox_templates` — typical examples: write a regression test for a PXF parsing edge case, tighten a serialization error message, or document an undocumented exported function.
2. Merge the sandbox issues. Each one accrues reputation in the relevant domain.
3. Your original PR unlocks for the community vote automatically.

## 3. Private Mentorship Mode

Open the PR as a **Draft** and Steward will send its evaluation as a private review instead of a public PR comment. Iterate on style/perf/coverage privately, then mark **Ready for Review** to trigger the public evaluation. First contributions get private feedback rather than public friction.

## 4. How to read a rejection log

Steward does not reject code based on opinions. A blocked PR's diagnostic log states the mathematical or constitutional reason.

Example:

> ❌ **Action: Blocked**
> * **Reason:** Cyclomatic complexity threshold exceeded.
> * **Details:** `encoding/pxf/parser.go` introduced a function with complexity 11. The Constitution caps `core-encoding` at 8.
> * **Resolution:** Flatten the AST dispatch logic and request re-evaluation.

Don't argue with Steward in the comments. Refactor, push, and Steward re-evaluates automatically.

## 5. Wire-format and schema changes

Changes under `proto/**` (annotation extension numbers, the envelope schema, `pxf.BigInt` / `Decimal` / `BigFloat`) and to `governance.pxf` itself are **constitutional amendments**: they require a 75% supermajority of the top 10 reputation holders, with a 14-day voting period. See [`GOVERNANCE.md` § 6](GOVERNANCE.md#6-constitutional-amendments) and [`STABILITY.md`](STABILITY.md) for the wire-stability contract these protect.

Adversarial-corpus additions under `testdata/adversarial/` follow the disclosure rules in [`docs/HARDENING.md`](docs/HARDENING.md): a corpus file that exposes a live vulnerability in a published port lands together with the per-port fix under a 30-day embargo.

## 6. Useful Steward commands

Comment on any issue or PR:

* `/steward evaluate` — re-run the 9-dimension check on your latest commit.
* `/steward check-reputation` — Steward replies with your decayed reputation vectors and your effective voting weight in the current PR's domains.
* `/steward sandbox-me` — assigns you an unassigned, low-risk issue to start building reputation.

## 7. Reporting bugs

Open an issue with a reproducible test case. If Steward verifies that a bug was introduced in a previously merged PR, it will retroactively slash the original author's reputation in the relevant domain — please write thorough tests.

For cross-port wire-format divergence, file here. For library bugs in a specific port, file against that port's repo.
