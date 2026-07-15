#!/usr/bin/env python3
"""Unit tests for the docs-site pipeline's fragile pure logic — the splitter's
fence/heading handling and link rewriting, and the link checker's path resolver.

Run: `python scripts/test_docs_pipeline.py` (used by the Pages workflow). These
guard the behaviors most likely to break under a README/docs restructuring.
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import build_docs_site as b  # noqa: E402
import check_site_links as c  # noqa: E402


class FenceTests(unittest.TestCase):
    def test_h2_inside_fenced_block_is_not_a_split(self):
        readme = (
            "intro\n"
            "```sh\n"
            "## not a heading\n"
            "~~~ still inside the ``` fence\n"
            "```\n"
            "## Real Section\n"
            "body\n"
        )
        errors: list[str] = []
        pages, anchor_page = b.build_readme_pages(readme, errors.append)
        names = [name for name, _, _ in pages]
        self.assertEqual(names, ["index.md", "real-section.md"])
        self.assertIn("real-section", anchor_page)
        self.assertNotIn("not-a-heading", anchor_page)
        self.assertEqual(errors, [])

    def test_mismatched_fence_marker_does_not_close(self):
        fences = b.Fences()
        self.assertTrue(fences.is_code("```"))  # open with backticks
        self.assertTrue(fences.is_code("~~~"))  # tildes do NOT close a ``` fence
        self.assertTrue(fences.is_code("## still code"))
        self.assertTrue(fences.is_code("```"))  # real close
        self.assertFalse(fences.is_code("## now a heading"))


class RewriteTests(unittest.TestCase):
    def test_readme_anchor_and_docs_prefix_rewrites(self):
        amap = {"provider-selection": "command-reference.md"}
        errors: list[str] = []
        out = b.rewrite_links(
            "See [sel](#provider-selection) and [aws](docs/aws.md#suve-aws-secret-log).\n",
            amap,
            from_readme=True,
            err=errors.append,
        )
        self.assertIn("(command-reference.md#provider-selection)", out)
        self.assertIn("(aws.md#suve-aws-secret-log)", out)
        self.assertEqual(errors, [])

    def test_links_inside_inline_code_are_untouched(self):
        amap = {"x": "page.md"}
        errors: list[str] = []
        out = b.rewrite_links("real [a](#x) but `[a](#x)` code\n", amap, from_readme=True, err=errors.append)
        self.assertIn("[a](page.md#x)", out)  # the real link rewritten
        self.assertIn("`[a](#x)`", out)  # the inline-code one left verbatim
        self.assertEqual(errors, [])

    def test_unmapped_anchor_is_an_error(self):
        errors: list[str] = []
        b.rewrite_links("[bad](#does-not-exist)\n", {}, from_readme=True, err=errors.append)
        self.assertTrue(any("does-not-exist" in e for e in errors))

    def test_doc_readme_backlinks_rewritten(self):
        amap = {"staging-workflow": "getting-started.md", "": "index.md"}
        errors: list[str] = []
        out = b.rewrite_links(
            "[w](../README.md#staging-workflow) [home](../README.md)\n",
            amap,
            from_readme=False,
            err=errors.append,
        )
        self.assertIn("(getting-started.md#staging-workflow)", out)
        self.assertIn("(index.md)", out)
        self.assertEqual(errors, [])

    def test_summary_gets_an_id(self):
        errors: list[str] = []
        out = b.rewrite_links("<summary>Building From Source</summary>\n", {}, from_readme=True, err=errors.append)
        self.assertIn('id="building-from-source"', out)


class ResolveTests(unittest.TestCase):
    def test_directory_url_maps_to_index(self):
        self.assertEqual(c.resolve("command-reference/index.html", "../aws/"), "aws/index.html")

    def test_dotdot_resolves_to_home(self):
        self.assertEqual(c.resolve("aws/index.html", ".."), "index.html")
        self.assertEqual(c.resolve("index.html", "."), "index.html")

    def test_asset_with_extension_kept(self):
        self.assertEqual(c.resolve("index.html", "demo/cli-demo.gif"), "demo/cli-demo.gif")

    def test_escape_above_root_is_rejected(self):
        self.assertIsNone(c.resolve("index.html", "../../etc/passwd"))

    def test_same_page_fragment(self):
        self.assertEqual(c.resolve("aws/index.html", "#frag"), "aws/index.html")


if __name__ == "__main__":
    unittest.main()
