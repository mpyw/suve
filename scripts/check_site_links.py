#!/usr/bin/env python3
"""Deterministically validate the built site's internal links, anchors and assets.

Runs after `mkdocs build` over .site/out and, for every rendered page, checks
that each local URL-bearing HTML attribute (a/@href, img/@src+srcset, and the
other asset-bearing elements below):

  * resolves to a file that exists in the output (directory URLs → index.html,
    decided against the actual output tree), and
  * for `#fragment` links, that the target page contains that id/name.

Same-site links — relative, root-absolute under the site base path (/suve/…), or
fully-qualified under the site_url host — are all resolved and checked. Truly
external links (other hosts, mailto/tel/data/js) are out of scope: network
checks are non-deterministic, and this gate's job is to guarantee every
REPOSITORY-LOCAL link/anchor/asset. Any broken local reference exits non-zero, so
a documentation restructuring that breaks a reference fails CI without any
AI/human review.
"""

from __future__ import annotations

import posixpath
import re
import sys
import urllib.parse
from html.parser import HTMLParser
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
OUT = REPO / ".site" / "out"

# Pages whose links are theme-generated against the absolute site_url base rather
# than relative to the page, so they are not meaningfully checkable here.
SKIP_FILES = {"404.html"}

NON_HTTP_SCHEMES = ("mailto:", "tel:", "data:", "javascript:")

# Local URL-bearing element/attribute pairs to validate. srcset attributes are
# comma-separated candidate lists and handled specially.
URL_ATTRS = {
    ("a", "href"),
    ("img", "src"),
    ("source", "src"),
    ("video", "src"),
    ("video", "poster"),
    ("audio", "src"),
    ("script", "src"),
    ("link", "href"),
    ("object", "data"),
    ("iframe", "src"),
    ("embed", "src"),
}
SRCSET_TAGS = {"img", "source"}


def load_site_base() -> tuple[str, str]:
    """(host, base-path) parsed from mkdocs.yml's site_url, so same-site absolute
    links can be resolved. base-path always ends with '/'."""
    m = re.search(r"^site_url:\s*(\S+)", (REPO / "mkdocs.yml").read_text(encoding="utf-8"), re.M)
    if not m:
        return "", "/"
    parts = urllib.parse.urlsplit(m.group(1))
    base = parts.path if parts.path.endswith("/") else parts.path + "/"
    return parts.netloc, base or "/"


SITE_HOST, SITE_BASE = load_site_base()


class PageParser(HTMLParser):
    """Collects element ids/names and local URL references from a page."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.ids: set[str] = set()
        self.refs: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        d = {k: (v or "") for k, v in attrs}
        if d.get("id"):
            self.ids.add(d["id"])
        if d.get("name"):
            self.ids.add(d["name"])
        for attr, value in d.items():
            if (tag, attr) in URL_ATTRS and value:
                self.refs.append(value)
        if tag in SRCSET_TAGS and d.get("srcset"):
            for candidate in d["srcset"].split(","):
                url = candidate.strip().split(" ", 1)[0]
                if url:
                    self.refs.append(url)


def to_out_relpath(page_rel: str, url: str) -> tuple[str | None, str, bool]:
    """Map a URL found on page_rel to (output-relative path, fragment, external).

    Returns external=True for links this gate does not check (other hosts,
    mailto/tel/data/js, or absolute paths outside the site base). The path is a
    normalized relpath from the output root, or None if it escapes the root."""
    s = urllib.parse.urlsplit(url)
    frag = urllib.parse.unquote(s.fragment)

    if s.scheme in ("http", "https"):
        if SITE_HOST and s.netloc == SITE_HOST and s.path.startswith(SITE_BASE):
            return _from_root(s.path[len(SITE_BASE) :]), frag, False
        return None, frag, True
    if s.scheme or s.netloc:  # other scheme (mailto:, …) or //host
        return None, frag, True

    path = urllib.parse.unquote(s.path)
    if path == "":
        return page_rel, frag, False  # same-page fragment
    if path.startswith("/"):
        if SITE_BASE != "/" and path.startswith(SITE_BASE):
            return _from_root(path[len(SITE_BASE) :]), frag, False
        if SITE_BASE == "/":
            return _from_root(path[1:]), frag, False
        return None, frag, True  # absolute path outside the site base

    joined = posixpath.normpath(posixpath.join(posixpath.dirname(page_rel), path))
    return (None if joined.startswith("..") else joined), frag, False


def _from_root(rel: str) -> str | None:
    joined = posixpath.normpath(rel) if rel.strip("/") else "."
    return None if joined.startswith("..") else joined


def find_target(rel: str, existing: set[str]) -> str | None:
    """Resolve a normalized relpath to an existing output file, trying it as a
    file and then as a directory URL (rel/index.html). '.' is the site root."""
    if rel in (".", ""):
        return "index.html" if "index.html" in existing else None
    if rel in existing:
        return rel
    idx = posixpath.normpath(posixpath.join(rel, "index.html"))
    return idx if idx in existing else None


def main() -> int:
    if not OUT.is_dir():
        print(f"check_site_links: {OUT} does not exist — build the site first", file=sys.stderr)
        return 1

    html_files = [p for p in OUT.rglob("*.html") if p.name not in SKIP_FILES]
    existing = {p.relative_to(OUT).as_posix() for p in OUT.rglob("*") if p.is_file()}
    ids_by_page: dict[str, set[str]] = {}

    def page_ids(rel: str) -> set[str]:
        if rel not in ids_by_page:
            parser = PageParser()
            parser.feed((OUT / rel).read_text(encoding="utf-8", errors="replace"))
            ids_by_page[rel] = parser.ids
        return ids_by_page[rel]

    errors: list[str] = []

    for path in sorted(html_files):
        page_rel = path.relative_to(OUT).as_posix()
        parser = PageParser()
        parser.feed(path.read_text(encoding="utf-8", errors="replace"))
        ids_by_page[page_rel] = parser.ids

        for url in parser.refs:
            if not url.strip() or url.strip() == "#":
                continue
            rel, fragment, external = to_out_relpath(page_rel, url)
            if external:
                continue
            target = find_target(rel, existing) if rel is not None else None
            if target is None:
                errors.append(f"{page_rel}: {url!r} -> missing file (resolved {rel!r})")
                continue
            if fragment and fragment not in page_ids(target):
                errors.append(f"{page_rel}: {url!r} -> no anchor #{fragment} in {target}")

    if errors:
        print(f"check_site_links: {len(errors)} broken local reference(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print(f"check_site_links: OK — {len(html_files)} pages, all local links/anchors/assets resolve")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
