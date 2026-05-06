---
name: Feature request
about: Propose a new annotation, grammar addition, envelope field, or tooling improvement
title: "feat: "
labels: enhancement
---

<!--
Spec-level changes affect every port and need careful design. Proposals
that touch the wire contract (annotation field numbers, envelope fields,
PXF grammar) should outline the migration path for existing data.
-->

## Problem

What can't you express today, or what's awkward to express?

## Proposal

What you'd like to add or change. If it's a wire-format change, include:

- A draft of the grammar / proto / SBE additions.
- Whether existing valid input remains valid (additive vs. breaking).
- How the change is detected and serialised on the wire.

## Alternatives considered

What else you tried, and why it isn't enough.

## Cross-port impact

Which of the nine ports would need to change, and roughly how much
work each one looks like.

## Out of scope (optional)

Things this proposal is **not** trying to do, to keep review focused.
