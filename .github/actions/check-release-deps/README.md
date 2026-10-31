# check-release-deps

Fails a release when a `go.mod` still has a `replace` or an untagged version of a stack module (`github.com/trendvidia/*`, `google.golang.org/protobuf`). Those are fine while cross-repository work is in progress. They are a problem once shipped: Go ignores `replace` in dependencies, so every consumer would have to repeat it.

## When it runs

Each repository adds `.github/workflows/release-deps.yml`:

```yaml
name: Release dependencies

on:
  push:
    tags: ['v*', '**/v*']
  pull_request:
    types: [opened, edited, synchronize, reopened]
  workflow_dispatch:

permissions:
  contents: read

jobs:
  check:
    if: github.event_name != 'pull_request' || startsWith(github.event.pull_request.title, 'release:')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: trendvidia/protowire/.github/actions/check-release-deps@main
```

| Trigger | What it checks |
|---|---|
| A PR titled `release:` | The root module and modules under `internal/`, before the tag exists |
| Tag `v1.2.3` | The same |
| Tag `a/b/v1.2.3` | Module `a/b` only |
| Run by hand (`workflow_dispatch`) | Every module, and allowlist entries that no longer match |

**A tag run comes too late to stop the tag.** A pushed tag is already public through the Go proxy, so the run can only report it. The `release:` PR is the check that comes first, so cut a release through one. Run it by hand, or run `check.py` locally, in repositories that don't.

A root release doesn't check a module with its own tags. That module has to require the root's new tag, so it can only be fixed after the root is tagged.

## What it allows

- **A `replace` to a path inside the repository.** This is how multi-module repositories work, as in chameleon's `sops`. A path outside the repository fails.
- **Anything that isn't a stack module.**
- **Prerelease tags** such as `v1.0.0-rc.4`. These are tags, not pseudo-versions.
- **`testdata`, `vendor` and `node_modules`** are not read.
- **Entries in `.release-deps-allow`** at the repository root.

## The allowlist

`.release-deps-allow` has one entry per line, and the reason is required:

```
<directory of the go.mod> <module>  # why this may ship
.   google.golang.org/protobuf  # opted in to the performance fork, see #123
```

It's for binaries that opted in to the performance fork, and anything else that means to ship this way. A run over every module fails on an entry that no longer matches, so the list can't go stale unnoticed.

## Why it lives here

setup-go-plus is the composite-action precedent, but it's private, and public repositories can't use a private action. Copying the script to eight repositories would let them drift, and this repository is public and already the root of the stack.

Run the tests with `python3 .github/actions/check-release-deps/check_test.py`.
