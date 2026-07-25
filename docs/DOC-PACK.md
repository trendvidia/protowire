# The doc pack — typed documentation model and compiler

Layer 1 of the application-documentation platform
([#170](https://github.com/trendvidia/protowire/issues/170)): the typed
documentation model in `proto/docs/v1/` and `pxf docs build`, the
compiler that turns authored topics into a **doc pack**.

A doc pack is to documentation what the lowered `FileDescriptorSet`
image ([#164](https://github.com/trendvidia/protowire/issues/164)) is to
schemas — one typed interchange artifact that every downstream surface
reads and none of them re-derives.

```
topics/*.pxf ─┐
schema image ─┼─► pxf docs build ─► doc pack ─► bundle publish ─► runtime help/search
registry data ┘                              └─► pxf openapi, static HTML
```

The other two layers live elsewhere and depend on this one: runtime
consumption in [appviewer#364](https://github.com/trendvidia/appviewer/issues/364),
authoring and review in [goed#321](https://github.com/trendvidia/goed/issues/321).
The boundary renderers are [#173](https://github.com/trendvidia/protowire/issues/173)
(`pxf openapi`) and [#171](https://github.com/trendvidia/protowire/issues/171)
(static HTML + wasm islands).

## Format rule

JSON and YAML are **integration-boundary formats only**. Topics,
anchors, review metadata, the search index and the pack itself are
PXF/protobuf-typed end to end. The single JSON reader in the pipeline is
the appviewer registry-export adapter (§ Data inputs), and it converts
to the typed model on read.

Prose is typed too — a tree of blocks and inline runs, never a markdown
blob. Every consumer needs structure: the runtime help panel renders it,
the HTML exporter emits DOM from it, the indexer walks it. A renderer
that has to re-parse prose is a renderer that disagrees with its
siblings.

Internal models need not be feature-complete against upstream
counterparts; boundary renderers map what they map (precedents: #164,
#66/Appendix C).

## Files

| File | What it defines |
|---|---|
| `proto/docs/v1/topic.proto` | The authoring model: `TopicFile`, `Topic`, prose blocks, anchors, review, translation, redirects, the `Audience` taxonomy |
| `proto/docs/v1/pack.proto` | The compiled artifact: `DocPack`, `CompiledTopic`, `ResolvedAnchor`, `SearchIndex`, provenance |
| `proto/docs/v1/registry.proto` | `WidgetCatalog` — protowire's typed mirror of the appviewer registry export |
| `internal/docpack/` | The compiler |
| `cmd/pxf/docs.go` | `pxf docs build`, `pxf docs digest` |

All three schemas are stock proto3, like `schema/v1/report.proto`: they
are read by runtimes and editors across every port, so they must parse
without a v1.2-capable parser. The structural rules a v1.2 `@validate`
would carry live in the compiler instead — which is also where #170 asks
for the load-bearing ones to live, so that no authoring tool can decide
to skip them.

## Authoring

A topic source is a PXF document bound to `protowire.docs.v1.TopicFile`:

```pxf
@type protowire.docs.v1.TopicFile

topics = [
  {
    key = "widgets.button"
    locale = "en"
    title = "Button"
    summary = "A tappable button with a text label."
    meta { audience = AUDIENCE_PUBLIC  tags = ["widgets"]  since = "1.5.0" }
    anchors = [
      { widget { type = "Button" } }
      { widget { type = "Button" event = "onTapped" } }
    ]
    body {
      blocks = [
        { paragraph { runs = [{ text = "A button fires " } { code = "onTapped" }] } }
      ]
    }
    review {
      state = REVIEW_STATE_APPROVED
      author = "author@example.com"
      revisor = "revisor@example.com"
      approved_digest = "…"
    }
  }
]
```

Topic identity is **(key, locale)**, never the file path: files group
topics for authoring convenience only, and moving a topic between files
breaks nothing. Repeated fields use PXF list syntax, which makes deep
prose verbose by hand — the authoring layer (goed#321) is the intended
editor, and `testdata/docs/topics/` shows the shape.

Locale variants follow the §7 catalog pattern of the error model: one
entity per (key, locale), no inheritance between locales, and the source
locale is the single origin of truth.

## Anchors and the stability contract

An anchor is how a topic says what it documents, and how a runtime
answers "show help for what I am pointing at". Anchors are typed, not
stringly, and every one is resolved at build time: **a dangling anchor
is a compile error**, and a moved target is a redirect.

Anchor kinds are classified by stability, because the failure mode is
proven rather than hypothetical — protocompile v0.18.0 re-keyed
`elementPath[fqn#ordinal]` descriptor paths and broke exactly this class
of persisted reference
([protolsp#260](https://github.com/trendvidia/protolsp/issues/260)).

**Stable by construction** — `ANCHOR_STABILITY_STABLE`. Identity is the
name, and the name is already governed by a compatibility process.

| Kind | Target | Resolved against |
|---|---|---|
| `schema` | message, field, enum, enum value, service, method, or `type` alias FQN | the lowered image |
| `widget` | registry type, prop or event | the registry export |
| `topic` | another topic's key | the build |
| `route` | an app route | nothing — shape-checked only |

`route` is the one kind whose target the compiler cannot prove exists.
That is why it is a distinct kind rather than a stringly escape hatch:
the model says plainly which references are checked and which are the
app's business.

**Derived and fragile** — `ANCHOR_STABILITY_DERIVED`. A descriptor path
is valid only against the image it was resolved with.

`DescriptorPathAnchor` is therefore authored in **element terms** —
element FQN, annotation FQN, ordinal — never as a path string. The
compiler derives the canonical §8.3.1 path through the shared
`fdp.DescriptorPath` formatter, verifies it against the image's own
embedded source map (extension 50404), and stamps the resolved anchor
with the image digest. A toolchain that re-keys the grammar therefore
invalidates these anchors instead of silently serving stale ones.

### Redirects

A rename is a documented event, not a wave of dangling links:

```pxf
redirects = [
  {
    from { schema { fqn = "myco.orders.Person" } }
    to   { schema { fqn = "myco.orders.User" } }
    since_version = "1.5.0"
    note = "Person was renamed to User"
  }
]
```

Redirects may chain; the compiler resolves chains to their terminus and
records the hops in `ResolvedAnchor.redirect_chain`, so no consumer ever
walks a chain — and no consumer can loop, because cycles are a compile
error. A redirect must not cross anchor kinds: a rename moves a thing, it
does not change what kind of thing it is.

### Audience

`Audience` is the visibility taxonomy, defined once here and inherited
by every renderer — runtime help, the HTML export, and #173's
community/enterprise API filtering. Renderers filter; they never invent
policy.

Tiers widen in restriction: `PUBLIC` → `COMMUNITY` → `PARTNER` →
`ENTERPRISE` → `INTERNAL`. Transitive consistency is enforced between
topics: a topic may not anchor or link to a more restricted one, because
render-time filtering would otherwise produce a link to a page the
reader may not see — a broken link *and* a leak about what exists.

Audience consistency **between a topic and a schema element** is not
modeled: the image carries no visibility tier today. That is the
schema-side half of the same rule and belongs to #173, which has to
decide artifact filtering versus descriptor stripping anyway.

## Content digest and the review gate

Both gates in the model hinge on one value:

> **Content digest** — the lowercase hex SHA-256 of a `Topic` message
> carrying only `title`, `summary` and `body`, marshaled
> deterministically.

Metadata, review state and translation provenance are deliberately
excluded. Retagging a topic or re-recording who approved it does not
change what a revisor read, and invalidating approvals for those edits
would train authors to treat the gate as noise.

`pxf docs digest` prints it, so the authoring flow records the value the
compiler will check rather than reimplementing the canonical encoding:

```
$ pxf docs digest ./topics/...
9c108a85…	guide.overview	en	overview.pxf
```

The **revisor gate** has three parts, and all three are needed for
approval to mean anything:

1. `state == REVIEW_STATE_APPROVED`,
2. a `revisor` who is not the `author` — self-approval defeats the gate,
3. an `approved_digest` that still matches the content — otherwise
   approval survives any later edit.

Part 2 is always an error. Parts 1 and 3 are **policy-sensitive**: a
warning by default, an error under `--release`. The normal authoring
loop passes through both states — you edit an approved topic and the
sign-off no longer covers it — and goed runs this compiler on its
diagnostics debounce, so the authoring loop stays usable while release
stays strict.

`--release` additionally requires an explicit audience tier: visibility
should be a decision, not a default.

### Translation staleness

A translated topic records `translation.source_digest`. When the
source-locale topic's current digest differs, the translation is stale:
a warning by default, an error under `--stale-translations-fatal`. The
i18n workflow gets a compiler signal instead of tribal knowledge, and
`CompiledTopic.translation_status` carries the verdict into the pack.

## Data inputs

The compiler resolves anchors against **data only** — protowire consumes
appviewer data, never appviewer code.

**`--image`** is the lowered `FileDescriptorSet` from `pxf build`. It is
walked as `FileDescriptorProto`s rather than hydrated through
`protodesc`: an image is a self-contained interchange artifact that may
legitimately omit files it does not need, and a documentation build must
not fail because a transitive import is absent from an image whose names
it can read perfectly well. Type aliases come from the `FileTypeDecls`
carrier (50403) so `components/schemas/<Name>` anchors resolve (§8.2);
descriptor paths come from the embedded source maps (50404).

**`--registry`** is the appviewer registry export
([appviewer#33](https://github.com/trendvidia/appviewer/issues/33)) in
any of three encodings: `.binpb` or `.pxf` carrying
`protowire.docs.v1.WidgetCatalog`, or `.json` — appviewer's
`--dump-registry` output, which the boundary adapter converts on read.
When appviewer emits the typed message natively, the adapter drops out
and nothing else changes.

Widget anchors resolve against a widget's own props, the props its
structural builder reads on children (`child_props`, appviewer#51), and
the common node props; the resolved anchor carries the entry's
since-version, which is what makes a topic's runtime applicability
checkable against the ADR-0011 bundle contract.

Either input may be omitted when the corpus uses no anchors of that
kind. An anchor with no input to resolve against is an error naming the
flag it needs, never a silent pass.

## The search index

The pack embeds a prebuilt inverted index. Indexing lives in the
compiler because it needs parsed content, and because the index must be
consumable beyond appviewer — goed's help, the HTML export's
client-side search, a docs-site export. None of those runs a search
service: the index is data, and it loads on fully static hosting.

- Terms are lowercased and split on everything that is not a letter,
  digit, or one of `_ . -`. A token containing a joiner is emitted whole
  *and* split into its parts at the same position, so `Button.text` is
  findable as `button.text`, `button` or `text` — documentation about
  code works without a second, code-specific index.
- Occurrences record the `FieldClass` they came from (title, summary,
  heading, body, code, tag) and their positions. The index bakes in **no
  ranking**: a runtime help panel and a docs site rank differently and
  both are right. Weighting and phrase matching are the consumer's.
- `Tokenization` records the algorithm, case folding and minimum token
  length, so a consumer tokenizes queries the way the index was built. A
  query tokenized differently is a search that quietly misses.

## Determinism

The pack is cached, diffed in CI and shipped as bundle data, so byte
stability is an acceptance bar. Topics sort by (key, locale); anchors
follow document order; postings sort by term and occurrences by
(document, field, position); catalog entries are sorted on the way in,
so a producer that reorders its export cannot change the output. Nothing
in the pack records a build timestamp.

`PackProvenance` records the image and catalog digests, every source
file's digest, the tool version, and whether release policy applied — a
consumer publishing a pack can check the last of those rather than
trusting the pipeline that handed it over. Provenance paths are recorded
as given on the command line; reproducible pipelines should pass stable
(relative) paths, since the digests, not the paths, are the identity.

## CLI

```bash
pxf docs build -o docs.binpb \
  --image image.binpb \
  --registry registry.json \
  ./topics/...

pxf docs build --check ./topics/...             # CI gate, no output
pxf docs build --check --release ./topics/...   # the revisor gate
pxf docs digest ./topics/...                    # digests for approvals
```

`--check` reports diagnostics and writes nothing. A build with errors
emits no pack at all: a partially resolved pack would be worse than
none, because its consumers cannot tell which parts were checked.

Inspect a pack with the rest of the toolchain:

```bash
pxf decode -p proto/docs/v1/pack.proto -m protowire.docs.v1.DocPack docs.binpb
```

## Fixtures

`testdata/docs/` is the conformance corpus:

- `topics/` — the valid corpus: every prose block kind, every anchor
  kind, a redirect-resolved anchor, a derived descriptor path, in-pack
  links with fragments, and both a current and a deliberately stale
  translation. Anchors resolve against an image built from
  `testdata/schema-extensions/01_basic.proto`.
- `invalid/` — one file per way a corpus can be wrong; each is expected
  to fail with a specific diagnostic.
- `policy/` — fixtures that warn while authoring and fail under
  `--release`.
- `registry.json` — a small widget catalog in appviewer's export shape.

## Non-goals

Bundle packaging and runtime surfaces (appviewer#364), authoring UX
(goed#321), OpenAPI rendering (#173), HTML rendering (#171). This layer
defines the model and compiles the artifact those consume.
