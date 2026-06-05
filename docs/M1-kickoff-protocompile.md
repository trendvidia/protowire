# M1 Kickoff — `protocompile` Extended Grammar (RFC-001 / v1.2)

| | |
|---|---|
| **Scope** | First atomic PR on `protocompile` toward issue #030 (extended grammar parser) |
| **Goal** | Reserve the 5 new keywords at the lexer level; everything else deferred to follow-up PRs |
| **Estimated effort** | 1–2 days for an engineer familiar with the codebase |
| **Reviewers** | Anyone familiar with `parser/` |
| **Reference docs** | [RFC-001](https://github.com/trendvidia/protowire/blob/main/docs/RFC-001-schema-extensions.md), [Issue scaffold](https://github.com/trendvidia/protowire/blob/main/docs/RFC-001-issues.md), [Umbrella #55](https://github.com/trendvidia/protowire/issues/55) |

## Why a minimal first PR

M1 (parser + IR + linker) is the longest item on the critical path. The
temptation is to land a single large PR covering all of issue #030 at
once — five new keywords plus their grammar productions plus their AST
nodes plus annotation use sites — but that PR would be ~600 lines of
intertwined changes across `parser/`, `ast/`, and generated y.go,
hard to review and hard to revert.

The lower-risk path is **five sequential PRs**, each individually
reviewable, testable, and revertable. This document scopes **PR 1**.

## PR 1 — Lexer keyword reservation

### What changes

Two files:

1. **`parser/lexer.go`** — extend the `keywords` map (around line 122)
   with five new entries:

   ```go
   var keywords = map[string]int{
       // ... existing entries
       "type":       _TYPE,
       "function":   _FUNCTION,
       "annotation": _ANNOTATION,
       "expression": _EXPRESSION,
       "this":       _THIS,
   }
   ```

2. **`parser/proto.y`** — add five token declarations (around line 136,
   alongside `%token <id> _MESSAGE _SERVICE …`):

   ```yacc
   %token <id> _TYPE _FUNCTION _ANNOTATION _EXPRESSION _THIS
   ```

   Then regenerate `y.go` via the existing `go generate` target.

   The `@` sigil is **already a token** in `proto.y` (around line 143) —
   no lexer change needed for it.

### What this PR does NOT change

- No grammar productions consuming the new tokens.
- No AST nodes.
- No linker changes.
- No descriptor lowering.
- No fixture changes beyond what's needed for the new keyword tests.

After this PR merges, the parser still rejects every `.proto` file
that uses `type`/`function`/`annotation`/`expression`/`this` in any
position — those tokens are reserved but not consumed by any production.
Existing v1.1 files continue to parse identically.

### Tests to add

- **Lexer unit test**: feed `type` to the lexer, assert `_TYPE` token.
  Repeat for the other four keywords.
- **Regression fixture**: pick 3–5 existing valid `.proto` fixtures
  from `internal/testdata/` (whichever cover the most parser surface)
  and confirm they continue to parse without change.
- **Soft-break fixture**: a `.proto` file with `message type { … }`
  (using one of the new reserved words as a message name) — assert
  the parser rejects it with a clear error pointing at the reserved
  word. One such fixture per new keyword.

### Acceptance criteria

- [ ] Five new keywords reserved at the lexer level.
- [ ] Generated `y.go` rebuilds without warnings.
- [ ] Lexer unit tests pass for each new keyword.
- [ ] All existing fixtures in `internal/testdata/` continue to parse
  byte-identically (no regression).
- [ ] Soft-break fixtures fail at the lexer/parser boundary with
  clear error messages identifying the reserved word.
- [ ] No new dependencies added to `go.mod`.

### Estimate

1–2 days for someone familiar with the codebase. The actual lexer
change is ~5 lines; the rest is tests and `y.go` regeneration.

## What comes next — PRs 2–5

Sketched here so reviewers of PR 1 know what's coming. Each subsequent
PR is similarly atomic.

### PR 2 — `type` declarations

Smallest non-trivial grammar addition. Just `type Name = qualifiedIdent;`
with no annotations attached.

- `parser/proto.y` — add `typeDecl` production, dispatch from
  `fileElement`.
- `ast/type.go` — define `TypeDeclNode` embedding `compositeNode`,
  implement `fileElement()` marker.
- Add a constructor `NewTypeDeclNode(...)`.
- Add a couple of positive fixtures (`type Email = string;`) and a
  negative one (missing `=`).

Estimate: 2–3 days.

### PR 3 — `function` declarations

Same shape as PR 2 but with parameter lists.

- `parser/proto.y` — add `functionDecl`, `paramList`, `param` productions.
- `ast/function.go` — `FunctionDeclNode`, `FunctionParamNode`.
- Fixtures covering: zero-arg, one-arg, multi-arg, bracket-option suffix.

Estimate: 2–3 days.

### PR 4 — `annotation` declarations

Adds the declaration of annotations but not their use sites.

- `parser/proto.y` — `annotationDecl`, `annotParamList`, `annotParam`,
  including the `expression` parameter-type keyword.
- `ast/annotation_decl.go` — `AnnotationDeclNode`.
- Fixtures covering: zero-param, all primitive param types, qualified
  message-type params, default values.

Estimate: 2–3 days.

### PR 5 — `@annotation(args)` use sites

The largest of the M1 PRs. Adds annotation **use** at every supported
target.

- `parser/proto.y` — `annotation`, `annotArgList`, `annotArg`,
  `annotationList` productions.
- Modify existing productions to accept optional annotation lists at
  hybrid placements (trailing on `type`, `function`, `field`, `enum-value`;
  leading on `message`, `enum`, `service`, `rpc`, `oneof`).
- `ast/annotation.go` — `AnnotationNode`, `AnnotationArgNode`.
- Engine-expression body capture: delimiter balancing + FQN extraction
  for the function references the linker (#032) will later verify.
- Fixtures from `protowire/testdata/schema-extensions/01_basic.proto`
  through `05_error_overrides.proto` — they should parse cleanly after
  this PR.

Estimate: 5–7 days.

### After M1 PR 5

Issue #030 closes. Move to #031 (IR), then #032 (linker).

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `y.go` regeneration produces churn unrelated to this PR | Commit `y.go` regenerations in a separate prep PR if any are needed for tooling-version alignment |
| Soft-break breaks downstream `.proto` files using the new reserved words as identifiers | Pre-flight: `grep` `protocompile`'s consumers (`protolsp`, `protocheck`, etc.) for identifiers matching the new keywords; warn the maintainers before merging |
| New keywords collide with future proto3 evolutions upstream | Low likelihood — upstream proto3 grammar is frozen; tracking is via #69 (upstream fork compatibility) |

## Soft-break audit before merge

Before merging PR 1, run a quick search across the trendvidia
ecosystem for `.proto` files that would break:

```bash
# From a workspace containing all trendvidia repos
grep -rEn '\bmessage[[:space:]]+(type|function|annotation|expression|this)\b' \
  ~/projects/src/github.com/trendvidia/*/proto/ \
  ~/projects/src/github.com/trendvidia/*/testdata/ \
  ~/projects/src/github.com/trendvidia/*/internal/testdata/

grep -rEn '\boneof[[:space:]]+(type|function|annotation|expression|this)\b' \
  ~/projects/src/github.com/trendvidia/

grep -rEn '\b(type|function|annotation|expression|this)[[:space:]]*=[[:space:]]*[0-9]' \
  ~/projects/src/github.com/trendvidia/ | grep -v node_modules | grep -v vendor
```

Findings — if any — get tracked as part of the merge plan: either
rename the conflicting identifiers (the documented v1.2 migration) or
delay PR 1 until the renames land.

## Branch strategy

- **Branch name**: `m1-pr1-lexer-keywords`
- **Base**: `main`
- **Target**: `trendvidia/protocompile:main`
- **Merge mode**: squash (matches existing convention on the repo)

PR title: `parser: reserve type/function/annotation/expression/this for RFC-001`

PR body should include:
- Link to issue #030 on protowire (`trendvidia/protowire#030` — will need to be filed on protocompile too)
- One-paragraph description of the change
- Test plan (the acceptance criteria above)
- Note that this is PR 1 of 5 in the M1 sequence

## Open follow-ups (for the maintainer)

1. **File issue #030–#035 on `trendvidia/protocompile`** — they currently
   live only in the protowire scaffold doc. Filing them on the actual
   implementation repo gives the team a tracker home for the work.
2. **Decide upstream-fork strategy** (#69) — does this fork track
   upstream `buf/protocompile` or diverge cleanly? Affects how M1 is
   labeled and how upstream changes are absorbed.
3. **Schedule a 30-min sync** before PR 1 lands to confirm scope with
   anyone else touching `parser/`.

---

Document owner: TBD
Last updated: 2026-06-05
