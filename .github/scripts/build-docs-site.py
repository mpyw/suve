#!/usr/bin/env python3
"""Assemble the MkDocs source tree for the GitHub Pages site.

The site is generated from the repo's existing Markdown — README.md plus
docs/**/*.md — WITHOUT committing anything: this writes a throwaway source tree
under .site/src that `mkdocs build` (see mkdocs.yml) turns into .site/out.

Because the README is large, it is split into one page per top-level `## `
section (the content before the first section becomes the home page). Splitting
would break the many intra-README anchor links (`#provider-selection`, …) and
the docs' `../README.md#…` links, so an anchor→page map is built from every
heading and used to rewrite those links to the page each section landed on.

Fail-closed by design: anything that would silently produce a broken or wrong
site — an unmapped anchor, a missing asset, an output-path collision, an empty
page slug, or a Git-LFS pointer left un-materialised — is collected and makes the
script exit non-zero, so CI catches doc-restructuring breakage deterministically
(no AI/human review). The rendered HTML is validated separately by
.github/scripts/check-site-links.py after the build.

Run via `mise docs-build` / `mise docs-serve`, or directly; it is idempotent.
"""

from __future__ import annotations

import re
import shutil
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]  # .github/scripts/<file> → repo root
SRC = REPO / ".site" / "src"

# Local assets referenced (by relative path) from the README, copied into the
# site so those links resolve. Each entry is a path relative to the repo root.
ASSETS = ["demo", "gui/build/appicon.png"]

# Committed stylesheet copied into the docs source at assets/docs-extra.css and
# wired via mkdocs.yml extra_css.
EXTRA_CSS = HERE / "docs-extra.css"

# A fenced code block opens with ``` or ~~~ (>=3), indented at most 3 spaces
# (CommonMark), and closes with a marker of the SAME character and >= the opening
# length, on a line that is only that marker (+ optional trailing text on open).
FENCE_RE = re.compile(r"^ {0,3}(`{3,}|~{3,})\s*(.*)$")
H2_RE = re.compile(r"^## +(.*\S)\s*$")
HEADING_RE = re.compile(r"^(#{1,6}) +(.*\S)\s*$")
MD_LINK_RE = re.compile(r"\[([^\]]*)\]\([^)]*\)")
HTML_TAG_RE = re.compile(r"<[^>]+>")
# Inline code spans: a run of N backticks, content, then N backticks. Used to
# avoid rewriting links that live inside inline code.
CODE_SPAN_RE = re.compile(r"(`+)(?:.*?)\1")
# A <details> summary is not a heading, so MkDocs (and GitHub) give it no anchor.
# We map its text and inject a matching id so #anchor links to it still resolve.
SUMMARY_RE = re.compile(r"<summary(\s[^>]*)?>(.*?)</summary>", re.IGNORECASE)
# Reference-style link definition (`[label]: url`) and usage (`[text][label]`,
# collapsed `[text][]`). Splitting the README can separate a usage from its
# definition, which would silently render as plain text; we detect that.
REF_DEF_RE = re.compile(r"^ {0,3}\[([^\]]+)\]:\s+\S")
REF_USE_RE = re.compile(r"\[([^\]]+)\]\[([^\]]*)\]")
# Material's stylesheet forces `.md-typeset img { height: auto }`, which overrides
# the README's inline width=/height= attributes (logos blow up to full size). Move
# those attributes into an inline style, which wins over the stylesheet.
# Match a full <img …> / <div …> open tag, tolerating '>' inside quoted attribute
# values (so an alt="a > b" cannot truncate the tag mid-attribute).
IMG_TAG_RE = re.compile(r"""<img\b(?:[^>"']|"[^"]*"|'[^']*')*>""", re.IGNORECASE)
DIV_OPEN_RE = re.compile(r"""<div\b(?:[^>"']|"[^"]*"|'[^']*')*>""", re.IGNORECASE)
# Attribute matchers use a non-name lookbehind so data-width / data-style / …
# are not mistaken for the bare attribute.
IMG_WIDTH_RE = re.compile(r'(?<![-\w])width\s*=\s*"(\d+)"', re.IGNORECASE)
IMG_HEIGHT_RE = re.compile(r'(?<![-\w])height\s*=\s*"(\d+)"', re.IGNORECASE)
HAS_STYLE_RE = re.compile(r'(?<![-\w])style\s*=', re.IGNORECASE)
HAS_MARKDOWN_ATTR_RE = re.compile(r'(?<![-\w])markdown\s*=', re.IGNORECASE)
# Content wrapped in <!-- site:skip --> … <!-- /site:skip --> is GitHub-only: it
# renders on the repo's README (e.g. a "Documentation" link back to this very
# site, which would be self-referential here) but is dropped from the built site.
SITE_SKIP_RE = re.compile(
    r"[^\S\n]*<!--\s*site:skip\s*-->.*?<!--\s*/site:skip\s*-->[^\S\n]*\n?",
    re.IGNORECASE | re.DOTALL,
)


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


class Fences:
    """Tracks fenced-code state so headings/links inside code are ignored,
    honoring the opening marker's character and length (mixed ```/~~~ safe)."""

    def __init__(self) -> None:
        self._char = ""
        self._len = 0
        self.open_line = 0  # 1-based line where the currently-open fence started

    @property
    def open(self) -> bool:
        return bool(self._char)

    def is_code(self, line: str, lineno: int = 0) -> bool:
        """Feed the next line; return True if it is a fence marker or inside a
        fenced block (i.e. not ordinary prose)."""
        m = FENCE_RE.match(line)
        if not self._char:
            if m:
                self._char = m.group(1)[0]
                self._len = len(m.group(1))
                self.open_line = lineno
                return True
            return False
        # Inside a fence: only a closing marker of the same char, >= the opening
        # length, with nothing but the marker on the line, ends it.
        if m and m.group(1)[0] == self._char and len(m.group(1)) >= self._len and not m.group(2).strip():
            self._char = ""
            self._len = 0
        return True


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


def size_img(tag: str) -> str:
    """Move an <img>'s width=/height= attributes into an inline style so Material's
    `height: auto` rule cannot override them. Leaves tags with an explicit style
    (or no numeric dimensions) untouched."""
    if HAS_STYLE_RE.search(tag):
        return tag
    dims = []
    w = IMG_WIDTH_RE.search(tag)
    h = IMG_HEIGHT_RE.search(tag)
    if w:
        dims.append(f"width:{w.group(1)}px")
    if h:
        dims.append(f"height:{h.group(1)}px")
    if not dims:
        return tag
    style = f' style="{";".join(dims)}"'
    if tag.endswith("/>"):
        return tag[:-2].rstrip() + style + " />"
    return tag[:-1].rstrip() + style + ">"


def add_div_markdown(m: re.Match) -> str:
    """Add markdown="1" to a <div> open tag that lacks it, so md_in_html renders
    Markdown inside it."""
    tag = m.group(0)
    if HAS_MARKDOWN_ATTR_RE.search(tag):
        return tag
    return '<div markdown="1"' + tag[len("<div") :]


BULLET_RE = re.compile(r"^[-*+] ")
# A flush-left line that is NOT a paragraph (so a following list is already fine):
# blank, a list item, heading, table row, blockquote, or raw HTML.
NON_PARAGRAPH_RE = re.compile(r"^([-*+] |#{1,6} |\||>|<|\d+[.)] |\s*$)")


def ensure_blank_before_lists(text: str) -> str:
    """Insert the blank line Python-Markdown needs before a bullet list that opens
    directly under a paragraph. GitHub renders such a list without the blank line;
    MkDocs would otherwise fold the bullets into the paragraph as literal text."""
    fences = Fences()
    out: list[str] = []
    prev_paragraph = False
    for i, line in enumerate(text.splitlines(keepends=True), start=1):
        if fences.is_code(line, i):
            out.append(line)
            prev_paragraph = False
            continue
        if prev_paragraph and BULLET_RE.match(line):
            out.append("\n")
        out.append(line)
        # The next line is "under a paragraph" only if this one is flush-left prose.
        prev_paragraph = bool(line.strip()) and not NON_PARAGRAPH_RE.match(line)
    return "".join(out)


def promote_headings(text: str) -> str:
    """Shift every ATX heading up one level (## → #, ### → ##, …) outside code, so
    a split section page has a single top-level <h1> and MkDocs does not synthesize
    a second title from the nav. Slugs are text-derived, so anchors are unchanged."""
    fences = Fences()
    out: list[str] = []
    for i, line in enumerate(text.splitlines(keepends=True), start=1):
        if not fences.is_code(line, i):
            line = re.sub(r"^(#{2,6})( )", lambda m: "#" * (len(m.group(1)) - 1) + m.group(2), line)
        out.append(line)
    return "".join(out)


def apply_outside_code(line: str, fn) -> str:
    """Apply fn to the parts of line that are NOT inside inline code spans."""
    out: list[str] = []
    pos = 0
    for m in CODE_SPAN_RE.finditer(line):
        out.append(fn(line[pos : m.start()]))
        out.append(m.group(0))  # inline code, left verbatim
        pos = m.end()
    out.append(fn(line[pos:]))
    return "".join(out)


def build_readme_pages(readme: str, err):
    """Split the README into (filename, title, body) pages at each `## ` heading,
    and build the anchor→filename map over every heading. The pre-first-section
    content is the home page (index.md).

    Returns (pages, anchor_page, duplicate_slugs). duplicate_slugs are slugs that
    occur on more than one heading/summary — a link to one is ambiguous after the
    split, so rewrite_links treats it as an error rather than guessing.
    """
    lines = readme.splitlines(keepends=True)
    pages: list[tuple[str, str, list[str]]] = []
    anchor_page: dict[str, str] = {}
    slug_counts: dict[str, int] = {}

    # Per-page reference-style link bookkeeping (see check below).
    page_defs: dict[str, set[str]] = {}
    page_uses: dict[str, set[str]] = {}
    global_defs: set[str] = set()

    current_name = "index.md"
    current_title = "Home"
    current_body: list[str] = []
    fences = Fences()

    def note_slug(slug: str) -> None:
        slug_counts[slug] = slug_counts.get(slug, 0) + 1
        anchor_page.setdefault(slug, current_name)

    def flush():
        pages.append((current_name, current_title, current_body))

    for i, line in enumerate(lines, start=1):
        if fences.is_code(line, i):
            current_body.append(line)
            continue

        m2 = H2_RE.match(line)
        if m2:
            flush()
            current_title = m2.group(1)
            current_name = section_filename(current_title)
            if current_name == ".md":
                err(f"README section {current_title!r} slugifies to an empty page name")
            current_body = [line]
            note_slug(heading_slug(current_title))
            continue

        mh = HEADING_RE.match(line)
        if mh:
            note_slug(heading_slug(mh.group(2)))

        for ms in SUMMARY_RE.finditer(line):
            note_slug(heading_slug(ms.group(2)))

        # Reference-style link accounting (ignore matches inside inline code).
        md = REF_DEF_RE.match(line)
        if md:
            label = md.group(1).strip().lower()
            page_defs.setdefault(current_name, set()).add(label)
            global_defs.add(label)

        def collect_uses(text: str) -> str:
            for mu in REF_USE_RE.finditer(text):
                label = (mu.group(2) or mu.group(1)).strip().lower()
                page_uses.setdefault(current_name, set()).add(label)
            return text

        apply_outside_code(line, collect_uses)
        current_body.append(line)

    flush()

    if fences.open:
        err(f"unclosed code fence opened at README line {fences.open_line}")

    # A reference-style usage whose definition landed on a different page renders
    # as plain text (no <a> for any downstream check to catch) — flag it.
    for name, uses in page_uses.items():
        for label in uses:
            if label in global_defs and label not in page_defs.get(name, set()):
                err(
                    f"reference-style link [...][{label}] on page {name} is separated from its "
                    f"[{label}]: definition by the section split — use an inline link or keep the "
                    f"definition in the same section"
                )

    duplicate_slugs = {s for s, n in slug_counts.items() if n > 1}
    return pages, anchor_page, duplicate_slugs


def rewrite_links(
    body: str,
    anchor_page: dict[str, str],
    *,
    from_readme: bool,
    err,
    duplicates: frozenset[str] = frozenset(),
    label: str = "",
) -> str:
    """Rewrite cross-page links so they survive the split. Links inside fenced or
    inline code are left untouched; an unmapped or ambiguous anchor is an error."""

    def map_anchor(anchor: str, whole: str) -> str:
        key = anchor.lower()
        if key in duplicates:
            err(f"ambiguous anchor #{anchor} in {whole!r}: the slug occurs on more than one heading")
            return whole
        page = anchor_page.get(key)
        if page is None:
            err(f"unmapped anchor #{anchor} in {whole!r}")
            return whole
        return f"]({page}#{anchor})"

    def add_summary_id(m: re.Match) -> str:
        attrs, inner = m.group(1) or "", m.group(2)
        if re.search(r"\bid=", attrs, re.IGNORECASE):
            return m.group(0)
        return f'<summary{attrs} id="{heading_slug(inner)}">{inner}</summary>'

    def rewrite_prose(text: str) -> str:
        # Honor inline <img> dimensions on every page (README and docs).
        text = IMG_TAG_RE.sub(lambda m: size_img(m.group(0)), text)
        if from_readme:
            # Let Markdown inside raw <div> wrappers (centered badges/links) render:
            # md_in_html only processes it when the block is marked markdown="1".
            text = DIV_OPEN_RE.sub(add_div_markdown, text)
            # README lives at the repo root; docs/ links become root-level siblings.
            text = re.sub(r"\]\(docs/([^)#]+)((?:#[^)]*)?)\)", r"](\1\2)", text)
            # Give each <details> summary an id so #anchor links to it resolve.
            text = SUMMARY_RE.sub(add_summary_id, text)
            # Same-README anchors now live on whichever split page carries them.
            text = re.sub(r"\]\(#([^)\s]+)\)", lambda m: map_anchor(m.group(1), m.group(0)), text)
        else:
            # docs/*.md pages: rewrite their links back to the README, tolerating
            # ../README.md, ./README.md, and README.md spellings.
            text = re.sub(
                r"\]\((?:\.\./|\./)?README\.md#([^)\s]+)\)",
                lambda m: map_anchor(m.group(1), m.group(0)),
                text,
            )
            text = re.sub(r"\]\((?:\.\./|\./)?README\.md\)", "](index.md)", text)
        return text

    fences = Fences()
    out: list[str] = []
    for i, line in enumerate(body.splitlines(keepends=True), start=1):
        if fences.is_code(line, i):
            out.append(line)
            continue
        out.append(apply_outside_code(line, rewrite_prose))
    if fences.open:
        err(f"unclosed code fence in {label or 'page'} (opened at line {fences.open_line})")
    return "".join(out)


def first_heading_title(text: str, fallback: str) -> str:
    fences = Fences()
    for line in text.splitlines(keepends=True):
        if fences.is_code(line):
            continue
        mh = HEADING_RE.match(line)
        if mh:
            return HTML_TAG_RE.sub("", MD_LINK_RE.sub(r"\1", mh.group(2))).strip() or fallback
    return fallback


def discover_doc_pages(err) -> list[tuple[Path, str, str]]:
    """Every docs/**/*.md, as (source path, flat output filename, nav title).
    Flattening keeps README's `docs/X.md` links working as root-level `X.md`."""
    docs_dir = REPO / "docs"
    out: list[tuple[Path, str, str]] = []
    for path in sorted(docs_dir.rglob("*.md")):
        rel = path.relative_to(docs_dir)
        if len(rel.parts) > 1:
            err(f"nested docs file not supported (flatten it or extend the script): docs/{rel}")
            continue
        title = first_heading_title(path.read_text(encoding="utf-8"), rel.stem)
        out.append((path, rel.name, title))
    return out


def is_lfs_pointer(path: Path) -> bool:
    try:
        with path.open("rb") as fh:
            return fh.read(46).startswith(b"version https://git-lfs")
    except OSError:
        return False


def main() -> int:
    errors: list[str] = []
    err = errors.append
    claimed: dict[str, str] = {}  # output filename -> what claimed it (collision guard)

    def claim(name: str, who: str) -> None:
        if name in claimed:
            err(f"output collision: {who} and {claimed[name]} both map to {name}")
        else:
            claimed[name] = who

    if SRC.exists():
        shutil.rmtree(SRC)
    SRC.mkdir(parents=True)

    readme = (REPO / "README.md").read_text(encoding="utf-8")
    # Drop GitHub-only blocks (see SITE_SKIP_RE) before splitting into pages.
    readme = SITE_SKIP_RE.sub("", readme)
    pages, anchor_page, duplicates = build_readme_pages(readme, err)
    anchor_page.setdefault("", "index.md")  # bare ../README.md -> home
    dups = frozenset(duplicates)

    nav: list[tuple[str, str]] = []
    for name, title, body in pages:
        claim(name, f"README section {title!r}")
        text = rewrite_links("".join(body), anchor_page, from_readme=True, err=err, duplicates=dups, label=name)
        # Split section pages open at `## `; promote so each has a single <h1>
        # (the home page already carries the README's own top-level title).
        if name != "index.md":
            text = promote_headings(text)
        text = ensure_blank_before_lists(text)
        (SRC / name).write_text(text, encoding="utf-8")
        nav.append((name, "Home" if name == "index.md" else title))

    for path, name, title in discover_doc_pages(err):
        claim(name, f"docs/{path.name}")
        text = rewrite_links(
            path.read_text(encoding="utf-8"), anchor_page, from_readme=False, err=err, duplicates=dups, label=f"docs/{path.name}"
        )
        text = ensure_blank_before_lists(text)
        (SRC / name).write_text(text, encoding="utf-8")
        nav.append((name, title))

    for asset in ASSETS:
        src = REPO / asset
        dst = SRC / asset
        if src.is_dir():
            shutil.copytree(src, dst)
            for f in src.rglob("*"):
                if f.is_file() and is_lfs_pointer(f):
                    err(f"asset is an un-materialised Git-LFS pointer: {f.relative_to(REPO)}")
        elif src.is_file():
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)
            if is_lfs_pointer(src):
                err(f"asset is an un-materialised Git-LFS pointer: {asset}")
        else:
            err(f"asset not found: {asset}")

    # Copy the extra stylesheet into the docs source (wired via mkdocs extra_css).
    if EXTRA_CSS.is_file():
        css_dst = SRC / "assets" / "docs-extra.css"
        css_dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(EXTRA_CSS, css_dst)
    else:
        err(f"extra stylesheet not found: {EXTRA_CSS.relative_to(REPO)}")

    # literate-nav reads this SUMMARY.md for page order/titles.
    summary = "".join(f"- [{title}]({name})\n" for name, title in nav)
    (SRC / "SUMMARY.md").write_text(summary, encoding="utf-8")

    if errors:
        print(f"build_docs_site: {len(errors)} error(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print(f"assembled {len(pages)} README pages + {len(nav) - len(pages)} doc pages into {SRC.relative_to(REPO)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
