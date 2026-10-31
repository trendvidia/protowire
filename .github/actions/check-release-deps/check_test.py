#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2026 TrendVidia, LLC.
"""Tests for check.py. Run: python3 .github/actions/check-release-deps/check_test.py"""
import pathlib
import subprocess
import sys
import tempfile
import unittest

CHECK = pathlib.Path(__file__).with_name("check.py")
PSEUDO = "v0.36.2-0.20261030225431-6b62d46387c5"


def repo(tmp, files):
    root = pathlib.Path(tmp)
    for name, text in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    return root


def run(root, *args):
    return subprocess.run(
        [sys.executable, str(CHECK), *args, str(root)], capture_output=True, text=True
    )


class CheckTest(unittest.TestCase):
    def check(self, files, *args):
        with tempfile.TemporaryDirectory() as tmp:
            return run(repo(tmp, files), *args)

    def test_tagged_requires_pass(self):
        r = self.check({"go.mod": "module x\nrequire github.com/trendvidia/protos v0.1.2\n"})
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("1 go.mod checked, 0 problem(s)", r.stdout)

    def test_pseudo_version_in_block_fails(self):
        r = self.check({"go.mod": f"module x\nrequire (\n\tgithub.com/trendvidia/protocompile {PSEUDO} // indirect\n)\n"})
        self.assertEqual(r.returncode, 1)
        self.assertIn(f"untagged version {PSEUDO}", r.stderr)

    def test_pseudo_on_protobuf_fails(self):
        r = self.check({"go.mod": "module x\nrequire google.golang.org/protobuf v1.36.13-0.20261030225431-6b62d46387c5\n"})
        self.assertEqual(r.returncode, 1)

    def test_every_pseudo_form_fails(self):
        for version in (
            "v0.0.0-20261030225431-6b62d46387c5",
            "v1.2.3-pre.0.20261030225431-6b62d46387c5",
            "v2.0.0-20261030225431-6b62d46387c5+incompatible",
        ):
            r = self.check({"go.mod": f"module x\nrequire github.com/trendvidia/a {version}\n"})
            self.assertEqual(r.returncode, 1, version)

    def test_prerelease_tag_passes(self):
        r = self.check({"go.mod": "module x\nrequire github.com/trendvidia/a v1.0.0-rc.4\n"})
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_non_stack_pseudo_passes(self):
        r = self.check({"go.mod": f"module x\nrequire golang.org/x/net {PSEUDO}\n"})
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_fork_replace_fails(self):
        r = self.check({"go.mod": "module x\nreplace google.golang.org/protobuf => github.com/trendvidia/protobuf-go v1.36.14\n"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("replace google.golang.org/protobuf", r.stderr)

    def test_replace_block_fails(self):
        r = self.check({"go.mod": "module x\nreplace (\n\tgithub.com/trendvidia/a v1.0.0 => github.com/other/a v1.0.0\n)\n"})
        self.assertEqual(r.returncode, 1)

    def test_replace_inside_the_repository_passes(self):
        r = self.check({
            "go.mod": "module x\n",
            "sops/go.mod": "module x/sops\nreplace github.com/trendvidia/x => ../\n",
        })
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_replace_outside_the_repository_fails(self):
        r = self.check({"go.mod": "module x\nreplace github.com/trendvidia/x => ../sibling\n"})
        self.assertEqual(r.returncode, 1)

    def test_comment_is_not_a_directive(self):
        r = self.check({"go.mod": "module x\n// replace github.com/trendvidia/x => ../y\n"})
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_testdata_is_skipped(self):
        r = self.check({
            "go.mod": "module x\n",
            "testdata/go.mod": f"module y\nrequire github.com/trendvidia/a {PSEUDO}\n",
        })
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_allowlist_needs_a_reason(self):
        gomod = f"module x\nrequire github.com/trendvidia/a {PSEUDO}\n"
        ok = self.check({"go.mod": gomod, ".release-deps-allow": ". github.com/trendvidia/a  # opted in\n"})
        self.assertEqual(ok.returncode, 0, ok.stderr)
        bad = self.check({"go.mod": gomod, ".release-deps-allow": ". github.com/trendvidia/a\n"})
        self.assertEqual(bad.returncode, 1)

    def test_stale_allowlist_entry_fails_a_full_run(self):
        r = self.check({"go.mod": "module x\n", ".release-deps-allow": ". github.com/trendvidia/a  # gone\n"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("stale entry", r.stderr)

    def test_module_scope(self):
        files = {
            "go.mod": "module x\n",
            "internal/h/go.mod": f"module x/h\nrequire github.com/trendvidia/a {PSEUDO}\n",
            "connect/go.mod": f"module x/c\nrequire github.com/trendvidia/x {PSEUDO}\n",
        }
        self.assertEqual(self.check(files).returncode, 1)
        # A root tag covers internal modules, which ship with it...
        self.assertEqual(self.check(files, "--module", ".").returncode, 1)
        # ...but not a module with its own tags, which must follow the root's.
        del files["internal/h/go.mod"]
        self.assertEqual(self.check(files, "--module", ".").returncode, 0)
        self.assertEqual(self.check(files, "--module", "connect").returncode, 1)

    def test_unknown_module_fails(self):
        r = self.check({"go.mod": "module x\n"}, "--module", "nope")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no go.mod found", r.stderr)


if __name__ == "__main__":
    unittest.main()
