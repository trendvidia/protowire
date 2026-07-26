# Documentation conformance corpus

Fixtures for the typed documentation model (`proto/docs/v1/`) and
`pxf docs build` — issue
[#170](https://github.com/trendvidia/protowire/issues/170). The design
record is [`docs/DOC-PACK.md`](../../docs/DOC-PACK.md); the tests that
drive this corpus are `cmd/pxf/docs_test.go` and
`docpack/docpack_test.go`.

## Layout

| Path | Compiled | Expectation |
|---|---|---|
| `topics/` | as one corpus | builds clean, including under `--release`; one warning for the deliberately stale translation |
| `invalid/*.pxf` | one file at a time | each fails with a specific diagnostic |
| `policy/*.pxf` | one file at a time | warns by default, fails under `--release` |
| `registry.json` | — | the widget catalog anchors resolve against (catalog schema v3 — proves older exports keep resolving) |
| `registry_v9.json` | — | a catalog-schema-v9 export shaped like the live appviewer registry: per-widget `bind`, composition props, transitions, authoring hints ([#186](https://github.com/trendvidia/protowire/issues/186)) |

Schema and descriptor-path anchors resolve against an image built from
`testdata/schema-extensions/01_basic.proto`, so the anchor tests
exercise the real lowering pipeline rather than a hand-written
descriptor set:

```bash
pxf build -o /tmp/image.binpb testdata/schema-extensions/01_basic.proto
pxf docs build --check --image /tmp/image.binpb \
  --registry testdata/docs/registry.json testdata/docs/topics/...
```

## What `topics/` covers

- `overview.pxf` — every prose block kind (paragraph, heading, list,
  admonition, code, table), inline runs, in-pack links with heading
  fragments, a topic anchor, and the parent/child navigation relation.
- `widgets.pxf` — widget anchors (type, prop, event), a structural
  widget's child prop, a route anchor, an `ExampleBlock` with live PXF,
  and an image with alt text.
- `schema.pxf` — schema FQN anchors (message, field, type alias), a
  descriptor-path anchor authored in element terms, an anchor that only
  resolves through a redirect, and the redirect that carries it.
- `de/overview.pxf` — locale variants: one current translation and one
  whose recorded source digest has gone stale.

## Adding a fixture

`invalid/` and `policy/` are enumerated by the tests, and an entry with
no expected diagnostic fails the suite — a new fixture must come with
the message it is expected to produce.

Approval and translation digests are content digests; compute them with
`pxf docs digest <file>` rather than by hand. Editing a topic's title,
summary or body changes its digest and therefore invalidates any
`approved_digest` recorded against it — which is the gate working, so
re-run `docs digest` and update the fixture.
