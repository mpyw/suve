#!/usr/bin/env python3
"""Deterministically validate the built site's internal links, anchors and assets.

Runs after `mkdocs build` over .site/out and, for every rendered page, checks
that each local `<a href>` and `<img src>`:

  * resolves to a file that exists in the output (directory URLs → index.html),
  * and, for `#fragment` links, that the target page actually contains an element
    with that id/name.

External links (http(s)/mailto/tel/data/protocol-relative) and absolute-path
links (theme-generated canonicals) are out of scope — network checks are
non-deterministic, and this gate's job is to guarantee every REPOSITORY-LOCAL
link/anchor/asset. Any broken local reference exits non-zero, so a documentation
restructuring that breaks a link fails CI without any AI/human review.
"""

from __future__ import annotations

import posixpath
import sys
import urllib.parse
from html.parser import HTMLParser
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
OUT = REPO / ".site" / "out"

# Pages whose links are theme-generated against the absolute site_url base rather
# than relative to the page, so they are not meaningfully checkable here.
SKIP_FILES = {"404.html"}

EXTERNAL_SCHEMES = ("http:", "https:", "mailto:", "tel:", "data:", "javascript:")


class PageParser(HTMLParser):
    """Collects element ids/names and (a href / img src) references from a page."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.ids: set[str] = set()
        self.refs: list[tuple[str, str]] = []  # (tag, url)

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        d = {k: (v or "") for k, v in attrs}
        if d.get("id"):
            self.ids.add(d["id"])
        if d.get("name"):
            self.ids.add(d["name"])
        if tag == "a" and d.get("href"):
            self.refs.append(("a", d["href"]))
        elif tag == "img" and d.get("src"):
            self.refs.append(("img", d["src"]))


def is_external(url: str) -> bool:
    u = url.strip()
    return (
        u.startswith("//")
        or u.startswith("/")  # absolute-path (site-base) links: out of scope
        or u.lower().startswith(EXTERNAL_SCHEMES)
    )


def resolve(page_rel: str, url: str) -> str | None:
    """Resolve a page-relative link to an output file path (posix, relative to
    OUT), or None if it points outside the tree."""
    path = urllib.parse.unquote(urllib.parse.urldefrag(url)[0])
    if path == "":
        return page_rel  # same-page fragment

    joined = posixpath.normpath(posixpath.join(posixpath.dirname(page_rel), path))
    if joined == ".":
        return "index.html"  # resolved to the site root (e.g. a `..`/`.` home link)
    if joined.startswith(".."):
        return None  # escaped the output root

    # A directory URL (no filename extension) maps to its index.html.
    if "." not in posixpath.basename(joined):
        return joined + "/index.html"
    return joined


def main() -> int:
    if not OUT.is_dir():
        print(f"check_site_links: {OUT} does not exist — build the site first", file=sys.stderr)
        return 1

    html_files = [p for p in OUT.rglob("*.html") if p.name not in SKIP_FILES]
    existing = {p.relative_to(OUT).as_posix() for p in OUT.rglob("*") if p.is_file()}

    ids_by_page: dict[str, set[str]] = {}

    def page_ids(rel: str) -> set[str]:
        if rel not in ids_by_page:
            target = OUT / rel
            parser = PageParser()
            parser.feed(target.read_text(encoding="utf-8", errors="replace"))
            ids_by_page[rel] = parser.ids
        return ids_by_page[rel]

    errors: list[str] = []

    for path in sorted(html_files):
        page_rel = path.relative_to(OUT).as_posix()
        parser = PageParser()
        parser.feed(path.read_text(encoding="utf-8", errors="replace"))
        ids_by_page[page_rel] = parser.ids

        for tag, url in parser.refs:
            if not url.strip() or url.strip() == "#" or is_external(url):
                continue

            target = resolve(page_rel, url)
            if target is None or target not in existing:
                errors.append(f"{page_rel}: {tag} -> {url!r} (missing file {target!r})")
                continue

            if tag == "a":
                fragment = urllib.parse.unquote(urllib.parse.urldefrag(url)[1])
                if fragment and fragment not in page_ids(target):
                    errors.append(f"{page_rel}: link -> {url!r} (no anchor #{fragment} in {target})")

    if errors:
        print(f"check_site_links: {len(errors)} broken local reference(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print(f"check_site_links: OK — {len(html_files)} pages, all local links/anchors/assets resolve")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
