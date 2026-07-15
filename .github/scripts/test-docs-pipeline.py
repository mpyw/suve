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
        self.assertEqual([n for n, *_ in pages], ["index.md", "real-section.md"])
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


class NavGroupTests(unittest.TestCase):
    def _groups(self, readme: str):
        errors: list[str] = []
        pages, _amap, _dups = b.build_readme_pages(readme, errors.append)
        self.assertEqual(errors, [])
        return [(name, group) for name, _title, _body, group in pages]

    def test_marker_groups_following_sections_and_is_stripped(self):
        readme = "intro\n<!-- nav-group: Basic Usage -->\n## Using CLI\nbody\n## Using TUI\nmore\n"
        errors: list[str] = []
        pages, _amap, _dups = b.build_readme_pages(readme, errors.append)
        by_name = {name: (group, "".join(body)) for name, _t, body, group in pages}
        self.assertIsNone(by_name["index.md"][0])
        self.assertEqual(by_name["using-cli.md"][0], "Basic Usage")
        self.assertEqual(by_name["using-tui.md"][0], "Basic Usage")
        # The comment marker does not leak into any page body.
        self.assertNotIn("nav-group", by_name["index.md"][1])

    def test_none_marker_returns_to_top_level(self):
        readme = "<!-- nav-group: G -->\n## A\nx\n<!-- nav-group: none -->\n## B\ny\n"
        self.assertEqual(self._groups(readme), [("index.md", None), ("a.md", "G"), ("b.md", None)])

    def test_marker_inside_fence_is_ignored(self):
        readme = "## A\n```\n<!-- nav-group: G -->\n```\n## B\ny\n"
        self.assertEqual(self._groups(readme), [("index.md", None), ("a.md", None), ("b.md", None)])


class ComposeNavTests(unittest.TestCase):
    def test_doc_group_inserted_after_named_page_and_contiguous(self):
        readme_nav = [
            ("index.md", "Home", None),
            ("command-reference.md", "Command Reference", None),
            ("license.md", "License", None),
        ]
        doc_entries = [
            ("aws.md", "AWS Commands", "Provider Commands", "command-reference.md"),
            ("azure.md", "Azure Commands", "Provider Commands", "command-reference.md"),
            ("gcloud.md", "GCloud", "Provider Commands", "command-reference.md"),
            ("staging-state-transitions.md", "Staging", None, None),
        ]
        errors: list[str] = []
        nav = b.compose_nav(readme_nav, doc_entries, errors.append)
        self.assertEqual(
            [n for n, _t, _g in nav],
            ["index.md", "command-reference.md", "aws.md", "azure.md", "gcloud.md", "license.md", "staging-state-transitions.md"],
        )
        # The grouped docs stay contiguous so literate-nav renders one parent.
        self.assertEqual([g for _n, _t, g in nav if g], ["Provider Commands"] * 3)
        self.assertEqual(errors, [])

    def test_missing_after_target_is_an_error(self):
        errors: list[str] = []
        b.compose_nav([("index.md", "Home", None)], [("aws.md", "AWS", "G", "nope.md")], errors.append)
        self.assertTrue(any("not in the nav" in e for e in errors))


class HeroTests(unittest.TestCase):
    def test_hero_h1_regex_demotes_to_styled_paragraph(self):
        out = b.HERO_H1_RE.sub(r'<p class="hero-title"\1>\2</p>', "  <h1>suve</h1>\n")
        self.assertEqual(out, '  <p class="hero-title">suve</p>\n')  # no heading -> no toc id


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

    def test_subpage_local_asset_src_gets_dotdot_prefix(self):
        errors: list[str] = []
        out = b.rewrite_links(
            '<img src="demo/x.gif"> <a href="https://z"> <img src="/root.png"> [md](y.md)\n',
            {"": "index.md"},
            from_readme=True,
            err=errors.append,
            sub_page=True,
        )
        self.assertIn('src="../demo/x.gif"', out)  # relative local ref fixed
        self.assertIn('href="https://z"', out)  # absolute left alone
        self.assertIn('src="/root.png"', out)  # root-absolute left alone
        self.assertIn("[md](y.md)", out)  # Markdown link untouched (MkDocs handles it)
        self.assertEqual(errors, [])

    def test_home_page_local_asset_src_unprefixed(self):
        errors: list[str] = []
        out = b.rewrite_links('<img src="demo/x.gif">\n', {}, from_readme=True, err=errors.append, sub_page=False)
        self.assertIn('src="demo/x.gif"', out)
        self.assertNotIn("../demo", out)

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
