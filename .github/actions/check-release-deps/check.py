#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Fail when a go.mod would ship a replace or an untagged stack dependency.

Usage: check.py [--module DIR] [ROOT]

ROOT is the repository (default: the working directory). --module picks what
is being released: `.` is the root module and any module under internal/, a
directory is that module alone, and no flag means every module. See README.md.
"""
import argparse
import pathlib
import re
import sys

ALLOW_FILE = ".release-deps-allow"
SKIP_DIRS = {"testdata", "vendor", "node_modules"}
PSEUDO = re.compile(r"\d{14}-[0-9a-f]{12}(\+incompatible)?$")


def is_stack(module: str) -> bool:
    return module.startswith("github.com/trendvidia/") or module == "google.golang.org/protobuf"


def directives(text: str):
    """Yield (verb, fields) for each require and replace line of a go.mod."""
    block = None
    for raw in text.splitlines():
        line = raw.split("//", 1)[0].strip()
        if block:
            if line == ")":
                block = None
            elif line:
                yield block, line.split()
            continue
        fields = line.split()
        if not fields or fields[0] not in ("require", "replace"):
            continue
        if fields[1:] == ["("]:
            block = fields[0]
        else:
            yield fields[0], fields[1:]


def findings(gomod: pathlib.Path, root: pathlib.Path):
    """Yield (module, description) for each violation in one go.mod."""
    for verb, fields in directives(gomod.read_text(encoding="utf-8")):
        if verb == "require":
            module, version = fields[0], fields[1]
            if is_stack(module) and PSEUDO.search(version):
                yield module, f"untagged version {version}"
            continue
        arrow = fields.index("=>")
        old, new = fields[0], fields[arrow + 1]
        if new.startswith(("./", "../", "/")):
            target = (gomod.parent / new).resolve()
            if target.is_relative_to(root.resolve()):
                continue  # how a multi-module repository works
        if is_stack(old) or is_stack(new):
            yield old, f"replace {old} => {' '.join(fields[arrow + 1:])}"


def allowlist(root: pathlib.Path):
    """Read `<go.mod dir> <module>  # reason` lines; a reason is mandatory."""
    allowed, problems = set(), []
    path = root / ALLOW_FILE
    if not path.exists():
        return allowed, problems
    for n, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        entry, _, reason = raw.partition("#")
        fields = entry.split()
        if not fields:
            continue
        if len(fields) != 2 or not reason.strip():
            problems.append(f"{ALLOW_FILE}:{n}: want `<dir> <module>  # reason`")
            continue
        allowed.add((fields[0], fields[1]))
    return allowed, problems


def modules(root: pathlib.Path, scope):
    for gomod in sorted(root.rglob("go.mod")):
        rel = gomod.parent.relative_to(root)
        if SKIP_DIRS.intersection(rel.parts):
            continue
        name = rel.as_posix()
        if scope is None or name == scope:
            yield name, gomod
        elif scope == "." and rel.parts[:1] == ("internal",):
            yield name, gomod


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--module", help="module being released")
    parser.add_argument("root", nargs="?", default=".")
    args = parser.parse_args()
    root = pathlib.Path(args.root)

    allowed, problems = allowlist(root)
    used, checked = set(), 0
    for name, gomod in modules(root, args.module):
        checked += 1
        for module, description in findings(gomod, root):
            if (name, module) in allowed:
                used.add((name, module))
            else:
                problems.append(f"{gomod.relative_to(root)}: {description}")
    for name, module in sorted(allowed - used):
        if args.module is None:
            problems.append(f"{ALLOW_FILE}: stale entry: {name} {module}")

    if checked == 0:
        problems.append(f"no go.mod found for module {args.module or '(any)'}")
    for problem in problems:
        print(problem, file=sys.stderr)
    print(f"{checked} go.mod checked, {len(problems)} problem(s)")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
