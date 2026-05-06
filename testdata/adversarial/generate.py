#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Regenerate the adversarial corpus fixtures from spec.

Run from any working directory:

    python3 testdata/adversarial/generate.py

Idempotent — produces byte-identical files on every run for the same input
parameters. Edit this script (not the outputs) when adjusting parameters.

Inputs that don't need parameterisation (small, hand-written PXF) are NOT
managed by this script — they live in the corpus directly. See README.md.
"""
import pathlib
import shutil
import struct
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent


def write_text(path: pathlib.Path, data: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(data, encoding="utf-8")
    print(f"  wrote {path.relative_to(HERE)} ({len(data)} chars)")


def write_bytes(path: pathlib.Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    print(f"  wrote {path.relative_to(HERE)} ({len(data)} bytes)")


def pxf_nested_tree(depth: int) -> str:
    """PXF text: `depth` levels of `child { ... }` against adversarial.v1.Tree.

    Whitespace is intentionally minimised (no indentation) to keep the corpus
    file size linear in `depth`. The decoder must not care.
    """
    return "@type adversarial.v1.Tree\n\n" + "child{" * depth + "}" * depth + "\n"


def pb_nested_tree(depth: int) -> bytes:
    """PB binary: `depth` levels of length-delimited Tree.child=1 (wire-type 2).

    The innermost message is empty (label="" omitted, child absent).
    """
    payload = b""
    for _ in range(depth):
        # Tag for field 1, wire-type 2 (LEN) = (1 << 3) | 2 = 0x0a.
        # Then varint-encoded length, then the inner payload.
        n = len(payload)
        length = bytearray()
        while True:
            b = n & 0x7F
            n >>= 7
            if n:
                length.append(b | 0x80)
            else:
                length.append(b)
                break
        payload = b"\x0a" + bytes(length) + payload
    return payload


def varint(n: int) -> bytes:
    """Standard protobuf varint encoding."""
    out = bytearray()
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


# --- SBE wire-format helpers ---
#
# SBE is little-endian fixed-layout. Every adversarial input below targets a
# specific HARDENING.md § SBE invariant by hand-rolling the wire bytes — no
# encoder library is needed because the whole point is to construct headers
# the encoder would refuse to emit.
#
# Layout for adversarial.v1.SbeWithGroup (template_id=9001, schema_id=9001):
#   header        : block_length(2) | template_id(2) | schema_id(2) | version(2)
#   root_block    : root_value      (uint32 LE — template block_length = 4)
#   group_header  : entry_block(2)  | count(2)
#   entry_block * count
SBE_TEMPLATE_ID = 9001
SBE_SCHEMA_ID = 9001
SBE_VERSION = 0
SBE_TEMPLATE_BLOCK = 4   # one uint32 root_value
SBE_GROUP_BLOCK = 4      # one uint32 entry_value


def sbe_header(block_length: int) -> bytes:
    """8-byte SBE message header (little-endian uint16s)."""
    return struct.pack(
        "<HHHH", block_length, SBE_TEMPLATE_ID, SBE_SCHEMA_ID, SBE_VERSION
    )


def sbe_group_header(block_length: int, count: int) -> bytes:
    """4-byte SBE group header (little-endian uint16s)."""
    return struct.pack("<HH", block_length, count)


def main() -> None:
    print(f"writing corpus under {HERE}")

    # --- PXF: nesting cases ---
    # 100k levels reliably blows growable native stacks (Rust's 8 MB main, Go's
    # ~1 GB cap, Swift's iOS 512 KB). The 200/1000 entries surface the missing
    # depth check via FAIL_VERDICT; 100k surfaces it via FAIL_CRASH on ports
    # whose stack actually trips before the input runs out.
    write_text(HERE / "pxf" / "nesting-baseline-10.pxf", pxf_nested_tree(10))
    write_text(HERE / "pxf" / "deep-nesting-200.pxf", pxf_nested_tree(200))
    write_text(HERE / "pxf" / "deep-nesting-1000.pxf", pxf_nested_tree(1000))
    write_text(HERE / "pxf" / "deep-nesting-100000.pxf", pxf_nested_tree(100_000))

    # --- PXF: numeric-literal digit cap ---
    write_text(
        HERE / "pxf" / "long-numeric.pxf",
        "@type adversarial.v1.BigIntHolder\n\nvalue = " + "1" * 5000 + "\n",
    )

    # --- PB: deep submessage ---
    write_bytes(HERE / "pb" / "deep-submessage-200.binpb", pb_nested_tree(200))

    # --- PB: length-prefix truncation ---
    # Tag 0x0a (field 1, wire-type LEN) + varint length 100, then no payload.
    # The decoder must reject because the declared length exceeds the bytes
    # actually available. This is the most basic length-prefix bounds check.
    write_bytes(HERE / "pb" / "length-prefix-truncated.binpb", b"\x0a\x64")

    # --- PB: length-prefix integer overflow ---
    # Tag 0x0a + max-varint length (10-byte 0xFFFFFFFFFFFFFFFF). A decoder
    # that narrows the uint64 length to int32/int64 without a checked cast
    # may compute a negative or wrapped offset, then index out-of-bounds or
    # allocate gigabytes. HARDENING.md § API contract item 3 (no trapping
    # integer conversions) and item 4 (length math is checked) both apply.
    write_bytes(
        HERE / "pb" / "length-prefix-overflow.binpb",
        b"\x0a" + b"\xff\xff\xff\xff\xff\xff\xff\xff\xff\x01",
    )

    # --- SBE: short root block_length ---
    # Wire block_length=2, template requires 4 (one uint32 root_value).
    # Per HARDENING.md § SBE: a wire block strictly smaller than template
    # means at least one field's offset+size exceeds the wire block, so the
    # decoder must reject before reading any field. A naive decoder reads
    # past the block boundary and either returns garbage or segfaults.
    write_bytes(
        HERE / "sbe" / "short-block-length.sbe",
        sbe_header(block_length=2) + b"\x00\x00",  # only 2 bytes of root.
    )

    # --- SBE: group count × block_length overflow ---
    # entry_block_length = 0xFFFF, count = 0xFFFF — product 0xFFFE_0001 (~4
    # GB). With no group body in the input, a decoder that doesn't validate
    # `pos + 4 + count*block_length ≤ data.length` before iterating either
    # OOMs allocating writer state or indexes far past end of buffer.
    write_bytes(
        HERE / "sbe" / "group-count-overflow.sbe",
        sbe_header(SBE_TEMPLATE_BLOCK)
        + b"\x00\x00\x00\x00"  # root_value = 0
        + sbe_group_header(block_length=0xFFFF, count=0xFFFF),
    )

    # --- SBE: zero block_length with non-zero count ---
    # entry_block_length = 0, count = 10000. Product is zero so the bounds
    # math succeeds, but the decoder is still asked to allocate 10000 entry
    # writers from zero further bytes of input — an attacker amplification
    # pattern. HARDENING.md § SBE step 4 requires explicit rejection.
    write_bytes(
        HERE / "sbe" / "group-zero-blocklength-nonzero-count.sbe",
        sbe_header(SBE_TEMPLATE_BLOCK)
        + b"\x00\x00\x00\x00"  # root_value = 0
        + sbe_group_header(block_length=0, count=10000),
    )

    # --- FileDescriptorSet for ports that can't compile .proto at runtime ---
    # Rust (prost-reflect) and a few others load DescriptorPool from a
    # FileDescriptorSet binary. Generated via `protoc --include_imports
    # --descriptor_set_out=...`; both files must move together when the .proto
    # changes. Skipped silently if protoc isn't on PATH so this script remains
    # runnable without a protobuf toolchain.
    if shutil.which("protoc"):
        # Resolve the repo's `proto/` root so `import "sbe/annotations.proto"`
        # in adversarial.proto can be located. testdata/adversarial → repo
        # root is two levels up; proto/ sits beside testdata/.
        repo_root = HERE.parent.parent
        subprocess.run(
            [
                "protoc",
                f"-I{HERE}",
                f"-I{repo_root / 'proto'}",
                "--include_imports",
                f"--descriptor_set_out={HERE / 'adversarial.binpb'}",
                "adversarial.proto",
            ],
            check=True,
        )
        size = (HERE / "adversarial.binpb").stat().st_size
        print(f"  wrote adversarial.binpb ({size} bytes)")
    else:
        print("  (protoc not on PATH — skipping adversarial.binpb regeneration)",
              file=sys.stderr)


if __name__ == "__main__":
    main()
