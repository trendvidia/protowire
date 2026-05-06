#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Mirror the canonical PXF TextMate grammar into the JetBrains targets.

VS Code reads the grammar as JSON; JetBrains' TextMate plugin reads the
original XML plist format (.tmLanguage). Source of truth is the JSON file
under editors/vscode/syntaxes/. Two destinations are written:

  * editors/jetbrains/pxf.tmbundle/Syntaxes/{PXF.tmLanguage,pxf.tmLanguage.json}
        — the standalone bundle for "Add Bundle" installs.
  * editors/jetbrains/plugin/src/main/resources/pxf.tmbundle/...
        — the copy embedded in the prebuilt JetBrains plugin .zip.

Both are kept identical to avoid drift. Run from any working directory:

    python3 scripts/sync_jetbrains_grammar.py

The script is idempotent: byte-identical output for the same input.
"""
import json
import pathlib
import plistlib
import shutil

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parent
SRC = REPO / "editors" / "vscode" / "syntaxes" / "pxf.tmLanguage.json"
BUNDLE_DIR = REPO / "editors" / "jetbrains" / "pxf.tmbundle"
PLUGIN_BUNDLE_DIR = (
    REPO / "editors" / "jetbrains" / "plugin"
    / "src" / "main" / "resources" / "pxf.tmbundle"
)


def write_grammar(target_dir: pathlib.Path, grammar: dict) -> None:
    syntaxes = target_dir / "Syntaxes"
    syntaxes.mkdir(parents=True, exist_ok=True)

    plist_bytes = plistlib.dumps(grammar, fmt=plistlib.FMT_XML, sort_keys=True)
    plist_path = syntaxes / "PXF.tmLanguage"
    plist_path.write_bytes(plist_bytes)
    print(f"  wrote {plist_path.relative_to(REPO)} ({len(plist_bytes)} bytes)")

    json_text = json.dumps(grammar, indent=2, ensure_ascii=False) + "\n"
    json_path = syntaxes / "pxf.tmLanguage.json"
    json_path.write_text(json_text, encoding="utf-8")
    print(f"  wrote {json_path.relative_to(REPO)} ({len(json_text)} chars)")


def mirror_static_files() -> None:
    """Copy info.plist and Preferences/ from the canonical bundle into the plugin."""
    PLUGIN_BUNDLE_DIR.mkdir(parents=True, exist_ok=True)
    for rel in ("info.plist", "Preferences/Comments.tmPreferences"):
        src = BUNDLE_DIR / rel
        dst = PLUGIN_BUNDLE_DIR / rel
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(src, dst)
        print(f"  copied {dst.relative_to(REPO)}")


def main() -> None:
    grammar = json.loads(SRC.read_text(encoding="utf-8"))
    grammar.pop("$schema", None)
    grammar.setdefault("fileTypes", ["pxf"])

    write_grammar(BUNDLE_DIR, grammar)
    write_grammar(PLUGIN_BUNDLE_DIR, grammar)
    mirror_static_files()


if __name__ == "__main__":
    main()
