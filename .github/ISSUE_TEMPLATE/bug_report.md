---
name: Bug report
about: Report a defect — wrong output, crash, parse error on valid input, etc.
title: "bug: "
labels: bug
---

<!--
Cross-port issues (the same input produces different output on different
ports, or one port crashes where others succeed) belong here, not in
the per-port repo. See STABILITY.md and HARDENING.md.

Security issues — decoder crashes/hangs/OOMs on adversarial input —
go to security@trendvidia.com instead. See SECURITY.md.
-->

## What happened

A clear description of the bug.

## How to reproduce

Smallest possible PXF / PB / SBE / envelope input that triggers it.
Inline if short, or attach as a file.

```pxf
@type your.package.Type
field = "value"
```

## What you expected

What you thought should happen.

## Affected ports

- [ ] Go (`protowire-go`)
- [ ] C++ (`protowire-cpp`)
- [ ] Rust (`protowire-rust`)
- [ ] Java (`protowire-java`)
- [ ] TypeScript (`protowire-typescript`)
- [ ] Python (`protowire-python`)
- [ ] C# (`protowire-csharp`)
- [ ] Swift (`protowire-swift`)
- [ ] Dart (`protowire-dart`)
- [ ] Spec / grammar / documentation only (no port code involved)

## Versions

- Affected port version(s):
- `protowire` spec commit (if known):
- OS / arch (only if it might matter):
