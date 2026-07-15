#!/usr/bin/env python3
"""Assemble the MkDocs source tree for the GitHub Pages site.

The site is generated from the repo's existing Markdown — README.md plus
docs/*.md — WITHOUT committing anything: this script writes a throwaway source
tree under .site/src that `mkdocs build` (see mkdocs.yml) turns into .site/out.

Because the README is large, it is split into one page per top-level `## `
section (the content before the first section becomes the home page). Splitting
would break the many intra-README anchor links (`#provider-selection`, …) and
the docs' `../README.md#…` links, so an anchor→page map is built from every
heading and used to rewrite those links to the page each section landed on. The
heading slugs use the same GitHub-compatible slugifier MkDocs is configured with
(pymdownx.slugs.slugify), so the rewritten anchors resolve.

Run via `mise docs-build` / `mise docs-serve`, or directly; it is idempotent.
"""

from __future__ import annotations

import re
import shutil
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
SRC = REPO / ".site" / "src"

# Local assets referenced (by relative path) from the README intro, copied into
# the site so those links resolve. Each entry is a path relative to the repo root.
ASSETS = ["demo", "gui/build/appicon.png"]

# The docs/*.md pages, in nav order, as (source filename, nav title).
DOC_PAGES = [
    ("aws.md", "AWS commands"),
    ("azure.md", "Azure commands"),
    ("gcloud.md", "Google Cloud commands"),
    ("staging-state-transitions.md", "Staging state transitions"),
]

FENCE_RE = re.compile(r"^\s*(```|~~~)")
H2_RE = re.compile(r"^## +(.*\S)\s*$")
HEADING_RE = re.compile(r"^(#{1,6}) +(.*\S)\s*$")
MD_LINK_RE = re.compile(r"\[([^\]]*)\]\([^)]*\)")
HTML_TAG_RE = re.compile(r"<[^>]+>")
# A <details> summary is not a heading, so MkDocs (and GitHub) give it no anchor.
# We map its text and inject a matching id so #anchor links to it still resolve.
SUMMARY_RE = re.compile(r"<summary(\s[^>]*)?>(.*?)</summary>", re.IGNORECASE)


def _load_slugify():
    """Return the slugify used for anchors — pymdownx's GitHub-compatible one when
    available (matches mkdocs.yml), else a close local fallback."""
    try:
        from pymdownx.slugs import slugify as _s  # type: ignore

        return _s(case="lower")
    except Exception:  # pragma: no cover - fallback for a bare environment

        def _fallback(text: str, sep: str) -> str:
            text = text.strip().lower()
            text = re.sub(r"[^\w\s-]", "", text)
            return re.sub(r"[\s]+", sep, text)

        return _fallback


_SLUGIFY = _load_slugify()


def heading_slug(raw: str) -> str:
    """Slug for a heading's raw Markdown text, mirroring how MkDocs slugifies the
    rendered heading: strip HTML tags, reduce Markdown links to their text, drop
    inline-formatting punctuation, then slugify."""
    text = HTML_TAG_RE.sub("", raw)
    text = MD_LINK_RE.sub(r"\1", text)
    text = re.sub(r"[`*_~]", "", text)
    return _SLUGIFY(text, "-")


def section_filename(title: str) -> str:
    return heading_slug(title) + ".md"


def build_readme_pages(readme: str):
    """Split the README into (filename, title, body) pages at each `## ` heading,
    and build the anchor→filename map over every heading. The pre-first-section
    content is the home page (index.md)."""
    lines = readme.splitlines(keepends=True)
    pages: list[tuple[str, str, list[str]]] = []
    anchor_page: dict[str, str] = {}

    current_name = "index.md"
    current_title = "Home"
    current_body: list[str] = []
    in_fence = False

    def flush():
        pages.append((current_name, current_title, current_body))

    for line in lines:
        if FENCE_RE.match(line):
            in_fence = not in_fence
            current_body.append(line)
            continue

        if not in_fence:
            m2 = H2_RE.match(line)
            if m2:
                flush()
                current_title = m2.group(1)
                current_name = section_filename(current_title)
                current_body = [line]
                anchor_page[heading_slug(current_title)] = current_name
                continue

            mh = HEADING_RE.match(line)
            if mh:
                anchor_page.setdefault(heading_slug(mh.group(2)), current_name)

            for ms in SUMMARY_RE.finditer(line):
                anchor_page.setdefault(heading_slug(ms.group(2)), current_name)

        current_body.append(line)

    flush()
    return pages, anchor_page


def rewrite_links(body: str, anchor_page: dict[str, str], *, from_readme: bool, warn) -> str:
    """Rewrite cross-page links so they survive the split. Links inside fenced code
    are left untouched."""

    def map_anchor(anchor: str, whole: str) -> str:
        page = anchor_page.get(anchor)
        if page is None:
            warn(f"unmapped anchor #{anchor} in {whole!r}")
            return whole
        return f"]({page}#{anchor})"

    def add_summary_id(m: re.Match) -> str:
        attrs, inner = m.group(1) or "", m.group(2)
        if re.search(r"\bid=", attrs, re.IGNORECASE):
            return m.group(0)
        return f'<summary{attrs} id="{heading_slug(inner)}">{inner}</summary>'

    def rewrite_line(line: str) -> str:
        if from_readme:
            # README lives at the repo root; docs/ links become root-level siblings.
            line = re.sub(r"\]\(docs/([^)]+)\)", r"](\1)", line)
            # Give each <details> summary an id so #anchor links to it resolve.
            line = SUMMARY_RE.sub(add_summary_id, line)
            # Same-README anchors now live on whichever split page carries them.
            line = re.sub(r"\]\(#([\w-]+)\)", lambda m: map_anchor(m.group(1), m.group(0)), line)
        else:
            # docs/*.md pages: rewrite their links back to the README.
            line = re.sub(
                r"\]\(\.\./README\.md#([\w-]+)\)",
                lambda m: map_anchor(m.group(1), m.group(0)),
                line,
            )
            line = re.sub(r"\]\(\.\./README\.md\)", "](index.md)", line)
        return line

    out: list[str] = []
    in_fence = False
    for line in body.splitlines(keepends=True):
        if FENCE_RE.match(line):
            in_fence = not in_fence
            out.append(line)
            continue
        out.append(line if in_fence else rewrite_line(line))
    return "".join(out)


def main() -> int:
    warnings: list[str] = []
    warn = warnings.append

    if SRC.exists():
        shutil.rmtree(SRC)
    SRC.mkdir(parents=True)

    readme = (REPO / "README.md").read_text(encoding="utf-8")
    pages, anchor_page = build_readme_pages(readme)

    # Home is always reachable even if the README ever loses its intro.
    anchor_page.setdefault("", "index.md")

    nav: list[tuple[str, str]] = []
    for name, title, body in pages:
        text = rewrite_links("".join(body), anchor_page, from_readme=True, warn=warn)
        (SRC / name).write_text(text, encoding="utf-8")
        nav.append((name, "Home" if name == "index.md" else title))

    for filename, title in DOC_PAGES:
        body = (REPO / "docs" / filename).read_text(encoding="utf-8")
        text = rewrite_links(body, anchor_page, from_readme=False, warn=warn)
        (SRC / filename).write_text(text, encoding="utf-8")
        nav.append((filename, title))

    for asset in ASSETS:
        src = REPO / asset
        dst = SRC / asset
        if src.is_dir():
            shutil.copytree(src, dst)
        elif src.is_file():
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)
        else:
            warn(f"asset not found: {asset}")

    # literate-nav reads this SUMMARY.md for page order/titles.
    summary = "".join(f"- [{title}]({name})\n" for name, title in nav)
    (SRC / "SUMMARY.md").write_text(summary, encoding="utf-8")

    print(f"assembled {len(pages)} README pages + {len(DOC_PAGES)} doc pages into {SRC.relative_to(REPO)}")
    for w in warnings:
        print(f"  warning: {w}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
