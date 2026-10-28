# pxf.BigFloat text-form fixtures

Cross-port conformance fixtures for how a `pxf.BigFloat` is written as PXF
text and how a literal is read back ([issue #406](https://github.com/trendvidia/protowire/issues/406),
decided by the owner; [issue #393](https://github.com/trendvidia/protowire/issues/393);
IETF draft `-01` § PXF Annotations).
- **Writing:** N = (decimal digits of 2^prec) + 1 significant digits, round half even, in C's `%g` form, followed by the precision suffix `p<prec>`. Zero is `0`, without a suffix.
- **Reading:** a suffixed literal is read at exactly the suffix's precision, an unsuffixed one at 256 bits, round half even. The stored mantissa is the significand as a prec-bit integer, and the exponent is the binary exponent minus prec.
- **Zero** is stored with every field 0.
- **A suffix elsewhere,** on a field that isn't a `pxf.BigFloat`, is rejected, and so is `p0`.

Every case binds to [`bigfloat.proto`](bigfloat.proto) (`bf.v1.H`: `pxf.BigFloat bf`, `int32 n`).
- An `enc-<name>` case is a PB message (`.binpb`) and the PXF text an encoder MUST write for it (`.pxf`).
- A `dec-<name>` case is a PXF document (`.pxf`) and the PB bytes a decoder MUST produce (`.binpb`, deterministic field order, proto3 zero fields omitted).
- An `err-<name>` case is a document a decoder MUST reject.

[`generate.py`](generate.py) computes every expected value with exact rational arithmetic, from the rule alone, independently of any port. Edit it, not the outputs, and commit both; it is idempotent. `cmd/pxf/bigfloat_test.go` drives the corpus through protowire-go.

| Case | Value or literal | Asserts |
|---|---|---|
| `enc-2pow1024-prec64` | 2^1024, prec 64 | `1.79769313486231590773e+308p64`: 21 digits |
| `enc-tenth-as-double` | the double nearest 0.1, prec 53 | `0.10000000000000001p53`: 17 digits. 16 would read back as a different value. |
| `enc-long-mantissa` | (2^100 − 1)·2^−50, prec 100, a 13-byte mantissa | 32 digits, positional |
| `enc-three-quarters-negative` | −0.75 (`negative = true`) | `-0.75p64`, trailing zeros removed |
| `enc-one-and-a-half` | 1.5, prec 53 | `1.5p53` |
| `enc-integer` | 1000, prec 64 | `1000p64`, positional below N digits |
| `enc-tiny-negative-exponent` | 2^−16385, at the `MaxBigFloatExponent` bound | exponent form |
| `enc-prec-zero` | prec 0 | `0`, no suffix |
| `dec-tenth-p53` | `0.10000000000000001p53` | read at 53 bits: the double nearest 0.1 again |
| `dec-2pow1024-p64` | `1.79769313486231590773e+308p64` | exactly 2^1024 at 64 bits |
| `dec-hundred-digits-p333` | a 100-digit literal with `p333` | read at the suffix's 333 bits |
| `dec-hundred-digits-unsuffixed` | the same literal without a suffix | read at 256 bits |
| `dec-tenth-unsuffixed` | `0.1` | read at 256 bits |
| `dec-minus-two-and-a-half` | `-2.5` | `negative` set |
| `dec-1e4932` | `1e4932` | binary exponent 16384, the bound |
| `dec-small-positional` | `0.000125` | |
| `dec-zero` | `0` | every field 0 |
| `err-suffix-on-int32` | `n = 5p64` | a suffix on a field that isn't a BigFloat |
| `err-precision-zero` | `bf = 1.5p0` | a zero precision |
