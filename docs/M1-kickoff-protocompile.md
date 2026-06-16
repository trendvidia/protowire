# M1 Kickoff — `protocompile` Extended Grammar (RFC-001 / v1.2)

| | |
|---|---|
| **Scope** | First atomic PR on `protocompile` toward issue #030 (extended grammar parser) |
| **Goal** | Reserve the 5 new keywords at the lexer level; everything else deferred to follow-up PRs |
| **Estimated effort** | 1–2 days for an engineer familiar with the codebase |
| **Reviewers** | Anyone familiar with `parser/` |
| **Reference docs** | [RFC-001](https://github.com/trendvidia/protowire/blob/main/docs/RFC-001-schema-extensions.md), [Issue scaffold](https://github.com/trendvidia/protowire/blob/main/docs/RFC-001-issues.md), [Umbrella #55](https://github.com/trendvidia/protowire/issues/55) |

## Status: ✅ COMPLETE (2026-06-15)

**M1 is done.** The full RFC-001 compiler surface — the three new
declarations (`type`, `function`, `annotation`) plus the `@name(args)`
annotation framework — is implemented and landed in the
`trendvidia/protocompile` fork. Every existing v1.x schema still parses
unchanged.

The plan below is preserved as a historical record. **Two things turned
out differently from this kickoff scope:**

1. **The work was not split into the five legacy-parser PRs described
   below.** Rather than extend the yacc `parser/proto.y` + `y.go` + legacy
   `ast/` package, the implementation was built on protocompile's
   **experimental** compiler pipeline, which was promoted to the default
   (`protocompile#41`, "M1 B4'") and then made the only pipeline by
   deleting the legacy parser, options, sourceinfo, linker, and `ast/`
   package (`protocompile#42`–`#44`). The "nameOrKeyword vs. per-production
   expansion" decision in PR 1 below was therefore moot — the new grammar
   lives in the experimental parser, not the yacc grammar.

2. **Scope ran past M1's original parser-only goal** straight through IR,
   linking, and descriptor lowering — i.e. issues #030–#032 in one
   sustained push rather than stopping after #030.

### What actually landed (in `trendvidia/protocompile`)

| RFC-001 capability | protocompile PR(s) |
|---|---|
| New keywords / grammar (experimental parser) | landed with the experimental pipeline (`#41`–`#44`) |
| `annotation` declarations registered as IR symbols | `#50` |
| Annotation use sites resolved against the symbol table | `#51` |
| Annotation parameter types resolved + use-site args validated | `#52` |
| Annotation use sites lowered into the PSE descriptor carrier | `#53` |
| `FileAnnotationDecls` emitted on `FileOptions` | `#54` |
| Default-value expressions lowered on annotation params | `#55` |
| PSE annotation fixture sweep coverage | `#56` |
| Enum-typed annotation arguments type-checked | `#57` |
| `function` declarations registered; `FileFunctions` emitted | `#58` |
| `type` aliases registered; `FileTypeDecls` emitted | `#60` |
| Type-alias field types resolved; alias annotations propagated | `#61` |
| Type-alias annotations propagated across files | `#62` |
| `TYPE_REFINEMENT` source-map entries with alias chain links | `source-map-type-refinement` branch |

### Next

Issues #030–#032 are effectively closed. Remaining RFC-001 follow-ups
live downstream of the compiler: language ports (`protowire-*`), the
`protowire-go` runtime, and editor-grammar support for the new syntax.

---

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

Three layers, kept as a single atomic PR because they only work together:

1. **`parser/lexer.go`** — extend the `keywords` map (around line 122)
   with three new entries:

   ```go
   var keywords = map[string]int{
       // ... existing entries
       "type":       _TYPE,
       "function":   _FUNCTION,
       "annotation": _ANNOTATION,
   }
   ```

   `expression` and `this` are NOT added — per RFC-001 §5.1, neither is
   reserved in protobuf namespace. `expression` is recognized only at
   annotation-parameter-type position by the grammar (PR 4 wires that);
   `this` lives in engine-language bodies that protocompile captures
   opaquely.

2. **`parser/proto.y`** — add three token declarations alongside
   `%token <id> _MESSAGE _SERVICE …` (around line 139):

   ```yacc
   %token <id> _TYPE _FUNCTION _ANNOTATION
   ```

3. **`parser/proto.y`** — add the contextual-keyword bridge so existing
   schemas using these words as identifiers (`oneof type { ... }`,
   `string function = N`, etc.) continue to parse. Audit current
   productions for `_NAME` in name-binding positions and either:

   - extend each one to also accept `_TYPE | _FUNCTION | _ANNOTATION`, or
   - introduce a single `nameOrKeyword` non-terminal and replace `_NAME`
     with `nameOrKeyword` at name-binding sites.

   The second is cleaner; expected positions include message names,
   field names, oneof names, enum-value names, service names, rpc names,
   option key path segments, qualified-name path segments, and any
   `identifier` production that already exists.

   Then regenerate `y.go` via the existing `make generate` target. The
   `@` sigil is **already a token** in `proto.y` (around line 143).

### What this PR does NOT change

- No grammar productions consuming the new tokens as *keywords* yet
  (they're available; PRs 2–5 use them).
- No AST nodes.
- No linker changes.
- No descriptor lowering.
- No semantic behavior — every existing valid schema continues to parse
  byte-identically.

After this PR merges, the parser remains backward-compatible: existing
v1.x schemas (including `parser/testdata/largeproto.proto`, which uses
`oneof type` 8 times) continue to parse unchanged. New top-level
productions for `type Email = ...`, `function f(...);`, and
`annotation a(...);` are not yet recognized.

### Tests to add

- **Lexer unit test**: feed `type`, `function`, `annotation` to the
  lexer in isolation; assert each yields the corresponding token.
- **Lexer test update**: `lexer_test.go:124` currently expects `type`
  to lex as `_NAME`; update to `_TYPE`. Same for any analogous lines.
- **Grammar regression**: `parser/testdata/largeproto.proto` must
  continue to parse without errors (it uses `oneof type` 8 times,
  exercising the contextual-keyword bridge).
- **Identifier-bridge fixtures**: small `.proto` files exercising
  `message function { ... }`, `oneof annotation { ... }`,
  `string type = 1;`, `enum Foo { type = 0; }` — each must parse
  cleanly under PR 1.

### Acceptance criteria

- [ ] Three new keyword tokens declared and lexed.
- [ ] Generated `y.go` rebuilds without warnings (`make generate`).
- [ ] `make checkgenerate` passes (CI gate).
- [ ] Lexer unit tests pass for each new keyword.
- [ ] `lexer_test.go:124`-style expectations updated to the new tokens.
- [ ] `largeproto.proto` and all existing fixtures in
  `parser/testdata/` and `internal/testdata/` continue to parse
  without regression.
- [ ] New identifier-bridge fixtures parse cleanly (verifying contextual
  behavior).
- [ ] No new dependencies added to `go.mod`.

### Estimate

2–3 days. The lexer change is ~3 lines; the grammar bridge is the bulk
of the work (audit existing productions, decide on `nameOrKeyword`
approach vs. expanding each production, update tests).

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
| Contextual-keyword bridge missed at some name-binding production | A pre-flight grep finds all `_NAME` occurrences in `proto.y`; verify each is covered by the `nameOrKeyword` non-terminal (or the per-production extension) |
| New keywords collide with future proto3 evolutions upstream | Low likelihood — upstream proto3 grammar is frozen; tracking is via #69 (upstream fork compatibility) |

## Identifier audit before merge

The contextual-keyword design preserves backward compatibility, so this
audit is a sanity check (not a soft-break gate). Run the same grep
commands originally drafted as a hard-break audit to confirm the
contextual bridge actually covers the patterns it claims to:

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

Each finding is a concrete fixture for the contextual-keyword test
suite. Add (or extend) a parser test that confirms the matched
pattern parses cleanly under PR 1. If anything fails to parse, the
contextual-keyword bridge is incomplete — fix the grammar, don't
rename the identifier.

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
Last updated: 2026-06-15 (marked complete; see Status section at top)
