#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Generate the pxf.BigFloat text-form corpus (issues #393, #406).

Expected values are computed here with exact rational arithmetic, from
the rule in draft -01 § PXF Annotations, independently of any port:

  * Writing: a BigFloat (mantissa M, exponent E, prec P, negative S) is
    the value (-1)^S * M * 2^E. It is written with N significant decimal
    digits, N = (number of decimal digits of 2^P) + 1, rounded to
    nearest with ties to even, trailing zeros removed, in the form of C's
    %g with precision N: exponent form (d.ddd e+XX, at least two exponent
    digits) when the decimal exponent is below -4 or at least N,
    positional otherwise. prec 0, or a zero mantissa, is "0".
  * Reading: a literal with d significant digits (the digits of its
    mantissa, leading zeros excluded) is read at P = max(256, bit length
    of 10^d) bits, rounded to nearest with ties to even. It is stored as
    mantissa = the significand as a P-bit integer (top bit set),
    exponent = binary exponent - P, prec = P. Zero is stored with every
    field 0 (negative set for -0).

Each enc-<name> case is a PB message (enc-<name>.binpb) and the PXF text
an encoder MUST write for it (enc-<name>.pxf). Each dec-<name> case is a
PXF document (dec-<name>.pxf) and the PB bytes a decoder MUST produce
(dec-<name>.binpb). Run from anywhere; it writes next to itself, and is
idempotent.
"""
from fractions import Fraction
from pathlib import Path

HERE = Path(__file__).resolve().parent


def varint(n: int) -> bytes:
    n &= 0xFFFFFFFFFFFFFFFF
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def bigfloat_pb(mant: int, exp: int, prec: int, neg: bool) -> bytes:
    """H { bf = BigFloat{...} } with proto3 zero fields omitted, in
    field-number order (deterministic marshaling)."""
    sub = b""
    if mant:
        mb = mant.to_bytes((mant.bit_length() + 7) // 8, "big")
        sub += b"\x0a" + varint(len(mb)) + mb
    if exp:
        sub += b"\x10" + varint(exp)
    if prec:
        sub += b"\x18" + varint(prec)
    if neg:
        sub += b"\x20\x01"
    return b"\x0a" + varint(len(sub)) + sub


def round_half_even(x: Fraction) -> int:
    q, r = divmod(x.numerator, x.denominator)
    twice = 2 * r
    if twice > x.denominator or (twice == x.denominator and q % 2 == 1):
        q += 1
    return q


def write_literal(mant: int, exp: int, prec: int, neg: bool) -> str:
    if prec == 0 or mant == 0:
        return "0"
    v = Fraction(mant) * (Fraction(2) ** exp)
    n = len(str(2 ** prec)) + 1
    k = 0  # decimal exponent: 10^k <= v < 10^(k+1)
    while Fraction(10) ** (k + 1) <= v:
        k += 1
    while Fraction(10) ** k > v:
        k -= 1
    q = round_half_even(v / Fraction(10) ** (k - n + 1))
    if q == 10 ** n:
        q //= 10
        k += 1
    digits = str(q).rstrip("0") or "0"
    sign = "-" if neg else ""
    if k < -4 or k >= n:
        body = digits[0] + ("." + digits[1:] if len(digits) > 1 else "")
        return f"{sign}{body}e{'-' if k < 0 else '+'}{abs(k):02d}"
    if k >= 0:
        ip, fp = digits[: k + 1].ljust(k + 1, "0"), digits[k + 1 :]
        return sign + ip + ("." + fp if fp else "")
    return sign + "0." + "0" * (-k - 1) + digits


def read_literal(lit: str):
    neg = lit.startswith("-")
    s = lit.lstrip("+-")
    mant_part, _, exp_part = s.lower().partition("e")
    ip, _, fp = mant_part.partition(".")
    d = len((ip + fp).lstrip("0"))
    v = Fraction(int(ip + fp or "0")) / (Fraction(10) ** len(fp))
    if exp_part:
        v *= Fraction(10) ** int(exp_part)
    if v == 0:
        return 0, 0, 0, neg
    p = max(256, (10 ** d).bit_length())
    e2 = 0  # 2^(p-1) <= v / 2^e2 < 2^p
    while v / Fraction(2) ** e2 >= 2 ** p:
        e2 += 1
    while v / Fraction(2) ** e2 < 2 ** (p - 1):
        e2 -= 1
    m = round_half_even(v / Fraction(2) ** e2)
    if m == 2 ** p:
        m //= 2
        e2 += 1
    return m, e2, p, neg


ENCODE = {
    # name: (mantissa, exponent, prec, negative)
    "2pow1024-prec64": (1, 1024, 64, False),
    "three-quarters-negative": (3, -2, 64, True),
    "tenth-as-double": (0x1999999999999A, -56, 53, False),
    "one-and-a-half": (3, -1, 53, False),
    "long-mantissa": (2 ** 100 - 1, -50, 100, False),
    "tiny-negative-exponent": (1, -16385, 64, False),
    "prec-zero": (0, 0, 0, False),
    "integer": (1000, 0, 64, False),
}

DECODE = ["0.1", "-2.5", "1e4932", "0", "1" + "2" * 99 + "e-50", "0.000125"]
DECODE_NAMES = ["tenth", "minus-two-and-a-half", "1e4932", "zero", "hundred-digits", "small-positional"]


def main() -> None:
    for name, (m, e, p, s) in ENCODE.items():
        (HERE / f"enc-{name}.binpb").write_bytes(bigfloat_pb(m, e, p, s))
        (HERE / f"enc-{name}.pxf").write_text(f"bf = {write_literal(m, e, p, s)}\n")
    for name, lit in zip(DECODE_NAMES, DECODE):
        (HERE / f"dec-{name}.pxf").write_text(f"@type bf.v1.H\n\nbf = {lit}\n")
        (HERE / f"dec-{name}.binpb").write_bytes(bigfloat_pb(*read_literal(lit)))
    print(f"wrote {len(ENCODE)} encode and {len(DECODE)} decode cases")


if __name__ == "__main__":
    main()
