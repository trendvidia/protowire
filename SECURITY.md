# Security Policy

## Reporting a vulnerability

Email **security@trendvidia.com** with a description, reproduction steps,
and the affected version(s) or commit(s). PGP key on request.

Please do **not** file public GitHub issues for vulnerabilities, and do
**not** post details in pull request comments.

You can expect:

- An acknowledgement within **3 business days**.
- A triage decision (accepted / not-a-vulnerability / needs-more-info)
  within **10 business days**.
- A coordinated fix on the timeline below.

## Scope

This policy covers the canonical spec repository
(`github.com/trendvidia/protowire`) and the per-language ports under
`github.com/trendvidia/protowire-*`. Issues that affect more than one
port are tracked here per [`STABILITY.md`](STABILITY.md) and
[`docs/HARDENING.md`](docs/HARDENING.md).

In scope:

- Decoder crashes, hangs, infinite loops, unbounded memory, or OOMs
  triggered by adversarial PXF / PB / SBE / envelope input.
- Wire-format divergences between ports for the same input that could
  be exploited (e.g. authorization bypass via parser disagreement).
- Schema-validation bypasses that let invalid messages reach
  application code.
- Buffer overflows or use-after-free in native ports (C++, Rust unsafe,
  Swift, Dart FFI).

Out of scope:

- Denial-of-service via legitimately large inputs that respect the
  limits in [`docs/HARDENING.md`](docs/HARDENING.md).
- Issues in upstream `protobuf-java`, `protobuf-go`, or other
  third-party libraries — file those upstream and CC us.

## Coordinated disclosure

For vulnerabilities affecting **more than one port**, a **30-day
embargo** applies from the date we acknowledge your report, extendable
by mutual agreement when a fix needs more time. During the embargo:

- The reporter and TrendVidia coordinate fixes across all affected
  ports so they ship simultaneously.
- A new corpus entry under [`testdata/adversarial/`](testdata/adversarial/)
  is prepared and lands together with the per-port fixes.
- A `SECURITY-ADVISORY-YYYY-NN.md` advisory is drafted for publication
  on the embargo end date.

Single-port issues follow the affected port's own disclosure timeline,
typically 7–14 days, but always at least long enough for a fix to be
released.

## Hall of fame

Reporters who follow coordinated disclosure are credited in
`SECURITY-ADVISORY-*.md` advisories and (with permission) in the
release notes. We do not currently run a paid bug-bounty program.
