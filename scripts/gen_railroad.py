#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""
gen_railroad.py — generate a single SVG railroad diagram for the PXF grammar.

Mirrors docs/grammar.ebnf rule-for-rule. Run via the project venv:

    /tmp/rr-venv/bin/python3 scripts/gen_railroad.py docs/grammar.svg

Or any environment where `pip install railroad-diagrams` has been run.
"""

import sys
from railroad import (
    Diagram, Sequence, Choice, Optional, ZeroOrMore, OneOrMore,
    NonTerminal, Terminal, Comment, Group,
)


def t(s):  # terminal
    return Terminal(s)


def n(s):  # nonterminal
    return NonTerminal(s)


def rule_diagram(name, body, comment=None):
    """Produce one Diagram for `name = body`. Returns the SVG fragment as text."""
    items = [body]
    d = Diagram(*items, css=None)
    out = []
    d.writeSvg(out.append)
    svg = "".join(out)
    return svg


# --- Rule definitions (mirroring docs/grammar.ebnf) -------------------------

RULES = []

def add(name, body, note=None):
    RULES.append((name, body, note))


add(
    "document",
    Sequence(
        Optional(n("type_directive")),
        ZeroOrMore(n("field_entry")),
    ),
)

add(
    "type_directive",
    Sequence(t("@type"), n("identifier")),
)

add(
    "entry",
    Choice(0, n("field_entry"), n("map_entry")),
)

add(
    "field_entry",
    Sequence(
        n("identifier"),
        Choice(
            0,
            Sequence(t("="), n("value")),
            Sequence(t("{"), ZeroOrMore(n("entry")), t("}")),
        ),
    ),
)

add(
    "map_entry",
    Sequence(n("map_key"), t(":"), n("value")),
)

add(
    "map_key",
    Choice(0, n("identifier"), n("string"), n("integer")),
)

add(
    "value",
    Choice(
        0,
        n("string"), n("integer"), n("float"), n("bool"), n("null"),
        n("bytes"), n("timestamp"), n("duration"),
        n("identifier"), n("list"), n("block_value"),
    ),
)

add(
    "list",
    Sequence(
        t("["),
        Optional(
            Sequence(
                n("value"),
                ZeroOrMore(Sequence(Optional(t(",")), n("value"))),
            ),
        ),
        t("]"),
    ),
)

add(
    "block_value",
    Sequence(t("{"), ZeroOrMore(n("entry")), t("}")),
)

add(
    "identifier",
    Sequence(
        Choice(0, n("letter"), t("_")),
        ZeroOrMore(Choice(0, n("letter"), n("digit"), t("_"), t("."))),
    ),
)

add("bool", Choice(0, t("true"), t("false")))
add("null", t("null"))

add(
    "integer",
    Sequence(Optional(t("-")), OneOrMore(n("digit"))),
)

add(
    "float",
    Sequence(
        Optional(t("-")),
        OneOrMore(n("digit")),
        Choice(
            0,
            Sequence(t("."), ZeroOrMore(n("digit")), Optional(n("exponent"))),
            n("exponent"),
        ),
    ),
)

add(
    "exponent",
    Sequence(
        Choice(0, t("e"), t("E")),
        Optional(Choice(0, t("+"), t("-"))),
        OneOrMore(n("digit")),
    ),
)

add(
    "duration",
    OneOrMore(n("duration_segment")),
)

add(
    "duration_segment",
    Sequence(
        OneOrMore(n("digit")),
        Optional(Sequence(t("."), OneOrMore(n("digit")))),
        n("time_unit"),
    ),
)

add(
    "time_unit",
    Choice(0, t("ns"), t("us"), t("µs"), t("ms"), t("s"), t("m"), t("h")),
)

add(
    "timestamp",
    Comment("RFC 3339 date-time"),
)

add(
    "string",
    Choice(0, n("simple_string"), n("triple_string")),
)

add(
    "simple_string",
    Sequence(
        t('"'),
        ZeroOrMore(Choice(0, n("string_char"), n("escape_seq"))),
        t('"'),
    ),
)

add(
    "string_char",
    Comment('any byte except " or \\ or LF'),
)

add(
    "triple_string",
    Sequence(
        t('"""'),
        Comment('any text not containing """'),
        t('"""'),
    ),
)

add(
    "escape_seq",
    Sequence(
        t("\\"),
        Choice(
            0,
            n("simple_escape"),
            n("hex_escape"),
            n("octal_escape"),
            n("unicode_4_escape"),
            n("unicode_8_escape"),
        ),
    ),
)

add(
    "simple_escape",
    Choice(
        0,
        t('"'), t("\\"), t("'"), t("?"),
        t("a"), t("b"), t("f"), t("n"), t("r"), t("t"), t("v"),
    ),
)

add(
    "hex_escape",
    Sequence(t("x"), n("hex_digit"), n("hex_digit")),
)

add(
    "octal_escape",
    Sequence(n("oct_lead"), n("oct_digit"), n("oct_digit")),
)

add(
    "unicode_4_escape",
    Sequence(t("u"), n("hex_digit"), n("hex_digit"), n("hex_digit"), n("hex_digit")),
)

add(
    "unicode_8_escape",
    Sequence(
        t("U"),
        n("hex_digit"), n("hex_digit"), n("hex_digit"), n("hex_digit"),
        n("hex_digit"), n("hex_digit"), n("hex_digit"), n("hex_digit"),
    ),
)

add(
    "bytes",
    Sequence(t("b"), t('"'), ZeroOrMore(n("base64_char")), t('"')),
)

add(
    "base64_char",
    Choice(0, n("letter"), n("digit"), t("+"), t("/"), t("=")),
)

add(
    "comment",
    Choice(0, n("line_comment"), n("block_comment")),
)

add(
    "line_comment",
    Sequence(
        Choice(0, t("#"), t("//")),
        Comment("any byte except LF"),
    ),
)

add(
    "block_comment",
    Sequence(t("/*"), Comment("any bytes"), t("*/")),
)

add("hex_digit", Comment("0-9 | a-f | A-F"))
add("oct_digit", Comment("0-7"))
add("oct_lead", Comment("0-3  (so \\nnn <= 0xFF)"))
add("digit", Comment("0-9"))
add("letter", Comment("A-Z | a-z"))


# --- Render --------------------------------------------------------------

def render_all(out_path):
    parts = []
    parts.append(
        '<?xml version="1.0" encoding="UTF-8"?>\n'
        '<svg class="railroad-diagram" '
        'xmlns="http://www.w3.org/2000/svg" '
        'xmlns:xlink="http://www.w3.org/1999/xlink" '
        'font-family="monospace" font-size="14">\n'
    )
    parts.append(_default_style())

    y = 20
    x_pad = 20
    max_width = 0

    for name, body, note in RULES:
        # Title above each rule
        parts.append(
            f'<g transform="translate({x_pad},{y})">'
            f'<text x="0" y="0" font-weight="bold" font-size="16">{name}</text>'
            f'</g>\n'
        )
        y += 14

        d = Diagram(body, css=None)
        # Capture diagram SVG by writing into a list buffer.
        buf = []
        d.writeStandalone(buf.append, css=None)
        diag_svg = "".join(buf)
        # Extract <svg ...>...</svg> inner content + dimensions.
        inner, w, h = _extract_svg(diag_svg)
        max_width = max(max_width, w + 2 * x_pad)
        parts.append(
            f'<g transform="translate({x_pad},{y})">{inner}</g>\n'
        )
        y += int(h) + 28

    parts.append("</svg>\n")
    svg = "".join(parts)
    # Patch root <svg> with width/height/viewBox now that we know them.
    total_h = y
    total_w = int(max(max_width, 600))
    svg = svg.replace(
        '<svg class="railroad-diagram"',
        f'<svg class="railroad-diagram" width="{total_w}" '
        f'height="{total_h}" viewBox="0 0 {total_w} {total_h}"',
        1,
    )
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(svg)
    print(f"wrote {out_path} ({total_w}x{total_h}, {len(RULES)} rules)")


def _default_style():
    return (
        "<style>\n"
        "  .railroad-diagram { background: #fff; }\n"
        "  .railroad-diagram path { stroke-width: 2; stroke: black; fill: rgba(0,0,0,0); }\n"
        "  .railroad-diagram text { font: bold 12px monospace; text-anchor: middle; "
        "white-space: pre; fill: #000; }\n"
        "  .railroad-diagram text.diagram-text { font-size: 12px; }\n"
        "  .railroad-diagram text.diagram-arrow { font-size: 16px; }\n"
        "  .railroad-diagram text.label { text-anchor: start; }\n"
        "  .railroad-diagram text.comment { font: italic 12px monospace; }\n"
        "  .railroad-diagram g.non-terminal text { font: bold 12px monospace; }\n"
        "  .railroad-diagram rect { stroke-width: 2; stroke: black; fill: #f8f8ff; }\n"
        "  .railroad-diagram rect.group-box { stroke: gray; stroke-dasharray: 10 5; "
        "fill: none; }\n"
        "  .railroad-diagram path.diagram-text { stroke-width: 1.5; stroke: black; "
        "fill: white; cursor: help; }\n"
        "  .railroad-diagram g.diagram-text:hover path.diagram-text { fill: #eee; }\n"
        "</style>\n"
    )


def _extract_svg(svg):
    # Pull the root <svg ...> tag and its contents; return (inner, width, height).
    # railroad-diagrams emits the attributes in alphabetical order; we accept any.
    import re
    open_match = re.search(r"<svg\b([^>]*)>", svg, re.S)
    if not open_match:
        raise RuntimeError("could not find <svg> tag in generated output")
    attrs = open_match.group(1)
    w_match = re.search(r'\bwidth="([\d.]+)"', attrs)
    h_match = re.search(r'\bheight="([\d.]+)"', attrs)
    if not (w_match and h_match):
        raise RuntimeError(f"could not parse width/height: {attrs[:200]}")
    w, h = float(w_match.group(1)), float(h_match.group(1))
    open_end = open_match.end()
    close_start = svg.rfind("</svg>")
    inner = svg[open_end:close_start]
    return inner, w, h


if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else "docs/grammar.svg"
    render_all(out)
