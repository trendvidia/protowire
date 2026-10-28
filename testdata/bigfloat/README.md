# pxf.BigFloat text-form fixtures

Cross-port conformance fixtures for how a `pxf.BigFloat` is written as PXF
text and how a literal is read back ([issue #406](https://github.com/trendvidia/protowire/issues/406),
decided by the owner; [issue #393](https://github.com/trendvidia/protowire/issues/393);
IETF draft `-01` § PXF Annotations).
- **Writing:** N = (decimal digits of 2^prec) + 1 significant digits, round half even, in C's `%g` form.
- **Reading:** at max(256, bit length of 10^d) bits for d significant digits, round half even. The stored mantissa is the significand as a full-precision integer, and the exponent is the binary exponent minus that precision.
- **Zero** is stored with every field 0.

Every case binds to [`bigfloat.proto`](bigfloat.proto) (`bf.v1.H`, one field `bf`).
- An `enc-<name>` case is a PB message (`.binpb`) and the PXF text an encoder MUST write for it (`.pxf`).
- A `dec-<name>` case is a PXF document (`.pxf`) and the PB bytes a decoder MUST produce (`.binpb`, deterministic field order, proto3 zero fields omitted).

[`generate.py`](generate.py) computes every expected value with exact rational arithmetic, from the rule alone, independently of any port. Edit it, not the outputs, and commit both; it is idempotent. `cmd/pxf/bigfloat_test.go` drives the corpus through protowire-go.

| Case | Value or literal | Asserts |
|---|---|---|
| `enc-2pow1024-prec64` | 2^1024, prec 64 | 21 digits: `1.79769313486231590773e+308` |
| `enc-tenth-as-double` | the double nearest 0.1, prec 53 | 17 digits: `0.10000000000000001`. 16 would read back as a different value. |
| `enc-long-mantissa` | (2^100 − 1)·2^−50, prec 100, a 13-byte mantissa | 32 digits, positional |
| `enc-three-quarters-negative` | −0.75 (`negative = true`) | `-0.75`, trailing zeros removed |
| `enc-one-and-a-half` | 1.5, prec 53 | `1.5` |
| `enc-integer` | 1000, prec 64 | `1000`, positional below N digits |
| `enc-tiny-negative-exponent` | 2^−16385, at the `MaxBigFloatExponent` bound | exponent form |
| `enc-prec-zero` | prec 0 | `0` |
| `dec-tenth` | `0.1` | 1 digit, read at 256 bits |
| `dec-minus-two-and-a-half` | `-2.5` | `negative` set |
| `dec-1e4932` | `1e4932` | binary exponent 16384, the bound |
| `dec-small-positional` | `0.000125` | leading zeros are not significant |
| `dec-hundred-digits` | a 100-digit literal | read at 333 bits, past 256 |
| `dec-zero` | `0` | every field 0 |
