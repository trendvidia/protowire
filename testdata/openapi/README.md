# `pxf openapi` fixtures

Reference inputs for the OpenAPI boundary renderer (RFC-001 §#080,
issue #173). Unlike `testdata/schema-extensions/`, this is **not** a
cross-port conformance corpus — the OpenAPI rendering is a protowire
tool feature, and these fixtures pin its behavior in
`cmd/pxf/openapi_test.go`.

| File | Purpose |
|---|---|
| `store.proto` | One schema exercising every arm of the §#080 mapping: mappable and unmappable `@validate` shapes, alias chains, §6.7 sensitivity minima, presence, derived error codes, and the §5.2 operation surface (path/query/body binding, defaulted and explicit operation metadata). |
| `protowire.openapi.textproto` | The generator config: info/servers, the `bearerAuth` scheme definition, and the `*Audit*` → internal audience glob. |
| `inconsistent.textproto` | Deliberately broken tier assignment (only `AuditRecord` restricted): generation MUST fail naming both ends of the first violated edge. |
| `topics/customer.pxf` | A partner-tier doc topic anchored to `demo.store.Customer` — the doc-pack tier contribution (docs and API surface cannot disagree). |
