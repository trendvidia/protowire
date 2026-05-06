# Adversarial corpus

This directory is the conformance corpus for [`docs/HARDENING.md`](../../docs/HARDENING.md), driven by [`scripts/cross_security_check.sh`](../../scripts/cross_security_check.sh) and gated in CI per [ROADMAP M8](../../ROADMAP.md#m8--hardening-conformance-corpus-target-0750).

Every input here is a hand-curated attacker payload. Each port's `check-decode` binary is run against every input and must produce the verdict declared in [`MANIFEST.jsonl`](MANIFEST.jsonl) — `accept` (decode succeeds) or `reject` (decode returns a clean error) — within a 5-second wall-clock and 256 MiB RSS budget, without crashing.

## Layout

```
adversarial.proto       schema referenced by every corpus entry
MANIFEST.jsonl          one JSON line per corpus file: format, schema, expected verdict, reason
generate.py             reproducibility helper — regenerates the larger fixtures from spec
pxf/                    PXF text inputs
pb/                     PB binary inputs
sbe/                    SBE binary inputs (corpus seeding scheduled for ROADMAP M8)
envelope/               Envelope binary inputs (corpus seeding scheduled for ROADMAP M8)
README.md               this file
```

## Manifest format

Each line of `MANIFEST.jsonl` is one JSON object:

```json
{
  "file":   "pxf/deep-nesting-200.pxf",
  "format": "pxf",
  "schema": "adversarial.v1.Tree",
  "expect": "reject",
  "reason": "200 levels exceeds MaxNestingDepth=100",
  "skip":   ["python"]
}
```

- `file` — path under `testdata/adversarial/`.
- `format` — one of `pxf`, `pb`, `sbe`, `envelope`. Selects the decoder.
- `schema` — fully-qualified message type from `adversarial.proto`.
- `expect` — `accept` or `reject`. The verdict every non-skipped port must produce.
- `reason` — human-readable; cited in test output on failure.
- `skip` (optional) — list of port names that legitimately can't reach this code path (e.g. a port whose decoder is a thin wrapper around another port's). Skips are tracked here, not in port repos, so the exemption is visible across the project.

## Adding a corpus entry

1. Add the input file under the appropriate `<format>/` directory.
2. Add a line to `MANIFEST.jsonl` declaring the expected verdict.
3. Run `python3 testdata/adversarial/generate.py` if you generated the file from spec; otherwise commit the file directly.
4. Run `scripts/cross_security_check.sh` locally against at least one port's `check-decode` binary to verify the verdict matches.
5. If the addition exposes a live vulnerability in a published port, hold the PR for the 30-day embargo per `SECURITY.md` (forthcoming, ROADMAP M6) and land the fix together.

## Regenerating

The PXF deep-nesting files and the PB binary fixture are produced by `generate.py`. Edit the script (not the outputs) when adjusting parameters; commit both. The script is idempotent — running it on a clean checkout produces byte-identical files.

```sh
python3 testdata/adversarial/generate.py
```
