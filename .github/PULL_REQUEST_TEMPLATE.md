<!--
Spec-level PRs (grammar, proto, envelope, hardening corpus) usually
require coordinated changes in every port. Mention which ports you've
also opened PRs against, or flag the ones that are still pending.

For first-time contributors: see CONTRIBUTING.md for the workflow and
the Steward governance rollout note.
-->

## Summary

What this PR changes, in 1–3 sentences.

## Why

Link to the issue or discussion that motivated this. If there isn't
one, briefly state the user-visible problem this solves.

## Scope

- [ ] Spec / grammar (`docs/grammar.ebnf`, `proto/`, `governance.pxf`)
- [ ] Tooling (`scripts/`, `cmd/`, `editors/`)
- [ ] Hardening corpus (`testdata/adversarial/`)
- [ ] Documentation only

## Cross-port coordination

If this is a wire-format or grammar change, list every port it affects
and the status of the matching PR there:

- [ ] protowire-go — link / status
- [ ] protowire-cpp — link / status
- [ ] protowire-rust — link / status
- [ ] protowire-java — link / status
- [ ] protowire-typescript — link / status
- [ ] protowire-python — link / status
- [ ] protowire-csharp — link / status
- [ ] protowire-swift — link / status
- [ ] protowire-dart — link / status

## Test plan

- [ ] `go vet ./... && go build ./... && go test -race ./...` passes
- [ ] If this changes wire behaviour: relevant cross-port script has
      been re-run locally and produces identical output across all
      checked-out ports.
- [ ] If this touches the adversarial corpus: every port's
      `check-decode` produces the verdict declared in `MANIFEST.jsonl`.
- [ ] If this touches the editor extensions: the prebuilt `.vsix` /
      `.zip` in `dist/` has been refreshed.
