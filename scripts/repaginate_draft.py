#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Re-paginate docs/draft-trendvidia-protowire-00.txt.

The .txt is hand-edited (no kramdown/xml2rfc source upstream); inserts
into the middle of the document drift the existing page-footer chrome
out of alignment. This script strips all page chrome from page 2
onwards, then re-emits the body at 58-line page intervals with the
canonical IETF-text running header and "[Page N]" footer.

Page 1 (the title block and abstract) is preserved verbatim, including
its trailing footer. Page 2+ chrome layout:

    [blank]
    Internet-Draft                  protowire                       May 2026
    [blank]
    [blank]
    <50 content lines>
    [blank]
    [blank]
    [blank]
    Franco Jr.                Expires 5 November 2026                [Page N]

Run via the project venv (or any environment):

    python3 scripts/repaginate_draft.py docs/draft-trendvidia-protowire-00.txt

Idempotent: re-running on already-paginated output produces byte-
identical text.
"""

from __future__ import annotations

import re
import sys

PAGE_CONTENT_LINES = 50
TARGET_COL = 72
RUNNING_HEADER = (
    "Internet-Draft                  protowire                       May 2026"
)
FOOTER_PREFIX = "Franco Jr.                Expires 5 November 2026"

FOOTER_RE = re.compile(
    r"^Franco Jr\.\s+Expires 5 November 2026\s+\[Page\s+\d+\]\s*$"
)
HEADER_RE = re.compile(r"^Internet-Draft\s+protowire\s+May 2026\s*$")


def emit_footer(page_num: int) -> str:
    token = f"[Page {page_num}]"
    pad = TARGET_COL - len(FOOTER_PREFIX) - len(token)
    if pad < 1:
        pad = 1
    return FOOTER_PREFIX + " " * pad + token


def repaginate(path: str) -> None:
    with open(path, encoding="utf-8") as f:
        text = f.read()
    # Keep an explicit trailing-newline marker so we can preserve it.
    trailing_nl = text.endswith("\n")
    lines = text.split("\n")
    if trailing_nl:
        # split() leaves an empty string at the end; drop it so the page-
        # break loop doesn't trip on it.
        lines = lines[:-1]

    # --- 1. Locate page 1's footer ------------------------------------------
    p1_idx = next(
        (i for i, line in enumerate(lines) if FOOTER_RE.match(line)),
        None,
    )
    if p1_idx is None:
        print("no page footers found; nothing to do", file=sys.stderr)
        return

    page1 = lines[: p1_idx + 1]

    # --- 2. Skip page 1's trailing chrome -----------------------------------
    i = p1_idx + 1
    while i < len(lines):
        if lines[i].strip() == "" or HEADER_RE.match(lines[i]):
            i += 1
            continue
        break

    # --- 3. Strip chrome from body, build a clean content stream -----------
    # Chrome shape per page (this script's own output):
    #   <50 content lines, possibly padded with blanks on a short page>
    #   blank, blank, blank             ← bottom chrome
    #   Franco Jr. ... [Page N]         ← footer
    #   blank                           ← top chrome (next page)
    #   Internet-Draft ... May 2026
    #   blank, blank
    #   <next 50 content lines>
    #
    # The strip removes the chrome but MUST NOT eat legitimate trailing
    # blank lines that belong to the content (paragraph separators at the
    # bottom of a page). We strip exactly 3 trailing blanks before the
    # footer to match the emit shape, then move on.
    body: list[str] = []
    while i < len(lines):
        line = lines[i]
        if FOOTER_RE.match(line):
            # Remove exactly 3 blank-line slots immediately before the
            # footer, preserving any non-blank content beyond that. If
            # there are fewer than 3 trailing blanks (shouldn't happen on
            # well-formed input), strip what's there.
            for _ in range(3):
                if body and body[-1] == "":
                    body.pop()
                else:
                    break
            i += 1
            # Skip post-footer chrome: blank, header, blank, blank.
            # Be lenient: skip up to 4 lines matching blank-or-header.
            skipped = 0
            while i < len(lines) and skipped < 4:
                if lines[i].strip() == "" or HEADER_RE.match(lines[i]):
                    i += 1
                    skipped += 1
                    continue
                break
            continue
        body.append(line)
        i += 1

    # If the original document ended with a short last page, the emitter
    # padded the page with blank lines so the footer landed at the right
    # row. After stripping that page's chrome (3 blanks + footer), those
    # pad-blanks remain in `body` as trailing whitespace. Trim them — but
    # only at the very end (we've already preserved per-page trailing
    # blanks above).
    while body and body[-1] == "":
        body.pop()

    # --- 4. Emit page 2+ ---------------------------------------------------
    output: list[str] = list(page1)
    page_num = 2
    pos = 0
    while pos < len(body):
        output.append("")
        output.append(RUNNING_HEADER)
        output.append("")
        output.append("")
        chunk = body[pos : pos + PAGE_CONTENT_LINES]
        pos += len(chunk)
        pad = PAGE_CONTENT_LINES - len(chunk)
        output.extend(chunk)
        # Pad short last page so the footer lands on the right column.
        for _ in range(pad):
            output.append("")
        output.append("")
        output.append("")
        output.append("")
        output.append(emit_footer(page_num))
        page_num += 1

    serialized = "\n".join(output)
    if trailing_nl:
        serialized += "\n"
    with open(path, "w", encoding="utf-8") as f:
        f.write(serialized)
    pages_written = page_num - 1
    print(f"wrote {path} ({pages_written} pages, {len(body)} body lines)")


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: repaginate_draft.py <path>", file=sys.stderr)
        return 2
    repaginate(argv[1])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
