#!/usr/bin/env python3
"""Unit tests for the docs-site pipeline's fragile pure logic — the splitter's
fence/heading handling, link rewriting, fail-closed guards, and the link
checker's URL resolution.

Run: `python .github/scripts/test-docs-pipeline.py` (used by the Pages workflow). These
guard the behaviors most likely to break — or to wrongly pass — under a
README/docs restructuring.
"""

from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

_HERE = Path(__file__).resolve().parent


def _load(mod_name: str, filename: str):
    """Load a sibling script by path (their filenames use hyphens, so they can't
    be imported by name)."""
    spec = importlib.util.spec_from_file_location(mod_name, _HERE / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


b = _load("build_docs_site", "build-docs-site.py")
c = _load("check_site_links", "check-site-links.py")


def resolve(page: str, url: str, existing: set[str]):
    """Compose the checker's two steps to a final target (or None), ignoring
    externals — the shape the checker uses per link."""
    rel, _frag, external = c.to_out_relpath(page, url)
    if external:
        return "EXTERNAL"
    return c.find_target(rel, existing) if rel is not None else None


class FenceTests(unittest.TestCase):
    def test_h2_inside_fenced_block_is_not_a_split(self):
        readme = "intro\n```sh\n## not a heading\n~~~ inside the ``` fence\n```\n## Real Section\nbody\n"
        errors: list[str] = []
        pages, anchor_page, _dups = b.build_readme_pages(readme, errors.append)
        self.assertEqual([n for n, _, _ in pages], ["index.md", "real-section.md"])
        self.assertIn("real-section", anchor_page)
        self.assertNotIn("not-a-heading", anchor_page)
        self.assertEqual(errors, [])

    def test_unclosed_fence_is_an_error(self):
        errors: list[str] = []
        b.build_readme_pages("## A\n```\nnever closed\n## B\nx\n", errors.append)
        self.assertTrue(any("unclosed code fence" in e for e in errors))

    def test_mismatched_fence_marker_does_not_close(self):
        fences = b.Fences()
        self.assertTrue(fences.is_code("```"))
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
        errors: list[str] = []
        out = b.rewrite_links("real [a](#x) but `[a](#x)` code\n", {"x": "page.md"}, from_readme=True, err=errors.append)
        self.assertIn("[a](page.md#x)", out)
        self.assertIn("`[a](#x)`", out)
        self.assertEqual(errors, [])

    def test_unmapped_anchor_is_an_error(self):
        errors: list[str] = []
        b.rewrite_links("[bad](#does-not-exist)\n", {}, from_readme=True, err=errors.append)
        self.assertTrue(any("does-not-exist" in e for e in errors))

    def test_ambiguous_duplicate_anchor_is_an_error(self):
        errors: list[str] = []
        b.rewrite_links("[d](#dup)\n", {"dup": "a.md"}, from_readme=True, err=errors.append, duplicates=frozenset({"dup"}))
        self.assertTrue(any("ambiguous anchor #dup" in e for e in errors))

    def test_doc_readme_backlinks_rewritten(self):
        amap = {"staging-workflow": "getting-started.md", "": "index.md"}
        errors: list[str] = []
        out = b.rewrite_links(
            "[w](../README.md#staging-workflow) [home](../README.md)\n", amap, from_readme=False, err=errors.append
        )
        self.assertIn("(getting-started.md#staging-workflow)", out)
        self.assertIn("(index.md)", out)
        self.assertEqual(errors, [])

    def test_img_size_moved_to_style(self):
        errors: list[str] = []
        out = b.rewrite_links('<img src="x.png" height="16" alt="">\n', {}, from_readme=True, err=errors.append)
        self.assertIn('style="height:16px"', out)

    def test_img_with_gt_in_quoted_attr_not_corrupted(self):
        errors: list[str] = []
        out = b.rewrite_links('<img alt="a > b" width="10" src="x.png">\n', {}, from_readme=True, err=errors.append)
        self.assertIn('alt="a > b"', out)  # attribute preserved intact
        self.assertIn("width:10px", out)

    def test_data_width_not_mistaken_for_width(self):
        errors: list[str] = []
        out = b.rewrite_links('<img data-width="9" src="x.png">\n', {}, from_readme=True, err=errors.append)
        self.assertNotIn("style=", out)  # no bogus dimension applied

    def test_div_gets_markdown_attr_once(self):
        errors: list[str] = []
        out = b.rewrite_links('<div align="center">\n', {}, from_readme=True, err=errors.append)
        self.assertIn('<div markdown="1" align="center">', out)
        # A div that already declares markdown is left alone.
        out2 = b.rewrite_links('<div markdown="span">\n', {}, from_readme=True, err=errors.append)
        self.assertEqual(out2.count("markdown="), 1)

    def test_summary_gets_an_id(self):
        errors: list[str] = []
        out = b.rewrite_links("<summary>Building From Source</summary>\n", {}, from_readme=True, err=errors.append)
        self.assertIn('id="building-from-source"', out)

    def test_unclosed_fence_in_a_doc_page_is_an_error(self):
        # docs/*.md are not split, so their fences are checked in rewrite_links.
        errors: list[str] = []
        b.rewrite_links("intro\n```sh\nnever closed\n", {}, from_readme=False, err=errors.append, label="docs/x.md")
        self.assertTrue(any("unclosed code fence in docs/x.md" in e for e in errors))


class GuardTests(unittest.TestCase):
    def test_reference_link_split_from_definition_is_an_error(self):
        readme = "## One\nSee [important][ref].\n## Two\n[ref]: #target\n### Target\ntext\n"
        errors: list[str] = []
        b.build_readme_pages(readme, errors.append)
        self.assertTrue(any("reference-style link" in e for e in errors))

    def test_reference_link_with_no_definition_is_ignored(self):
        # e.g. `#VERSION][~SHIFT]` version-syntax notation, not a real ref link.
        errors: list[str] = []
        b.build_readme_pages("## One\nUse [#VERSION][~SHIFT] syntax.\n", errors.append)
        self.assertEqual(errors, [])


class ResolveTests(unittest.TestCase):
    def test_directory_url_maps_to_index(self):
        self.assertEqual(resolve("command-reference/index.html", "../aws/", {"aws/index.html"}), "aws/index.html")

    def test_dotdot_resolves_to_home(self):
        self.assertEqual(resolve("aws/index.html", "..", {"index.html"}), "index.html")
        self.assertEqual(resolve("index.html", ".", {"index.html"}), "index.html")

    def test_extensionless_file_not_mistaken_for_dir(self):
        self.assertEqual(resolve("index.html", "LICENSE", {"LICENSE"}), "LICENSE")

    def test_query_string_is_stripped(self):
        self.assertEqual(resolve("index.html", "aws/?v=1", {"aws/index.html"}), "aws/index.html")

    def test_escape_above_root_is_rejected(self):
        self.assertIsNone(resolve("index.html", "../../etc/passwd", set()))

    def test_same_site_absolute_and_qualified(self):
        existing = {"aws/index.html"}
        self.assertEqual(resolve("index.html", "/suve/aws/", existing), "aws/index.html")
        self.assertEqual(resolve("index.html", "https://mpyw.github.io/suve/aws/", existing), "aws/index.html")

    def test_external_is_skipped(self):
        self.assertEqual(resolve("index.html", "https://example.com/x", set()), "EXTERNAL")
        self.assertEqual(resolve("index.html", "mailto:x@y.z", set()), "EXTERNAL")


if __name__ == "__main__":
    unittest.main()
