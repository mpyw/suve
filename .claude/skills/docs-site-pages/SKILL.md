---
name: docs-site-pages
description: Use when restructuring the README or the GitHub Pages site generated from it — the left-nav/left-pane, section reorg, or per-provider command docs. Covers the build-docs-site.py split + literate-nav SUMMARY pipeline, its HTML-comment directives and site-only transforms (nav-group, site:skip, hero <h1>, asset ../ prefix, DOC_NAV grouping), the python-markdown gotchas GitHub hides, and the `mise docs-build` strict gate.
---

# GitHub Pages docs-site

The Pages site is generated from `README.md` + `docs/**/*.md` by `.github/scripts/build-docs-site.py`; nothing under `.site/` is committed. `mise docs-serve` previews at http://127.0.0.1:8000/suve/ (watches `.site/src`; it does **not** re-run on edits to `build-docs-site.py`/CSS/`mkdocs.yml` — re-run the script or `mise docs-build` for those). `mise docs-build` is the gate. Evidence: nav/docs restructure PR #869.

## The pipeline — how a page and its nav come to exist

- The README is split into **one page per top-level `## ` section**; everything before the first `## ` is the home page (`index.md`). `docs/**/*.md` are flattened to root-level pages (`docs/aws.md` → `aws.md`), so README's `docs/X.md` links keep resolving.
- `literate-nav` reads a **generated `SUMMARY.md`** for nav order and titles. A docs page's nav title is its first `#` heading.
- Anchors are **text-derived, lowercase slugs** (pymdownx, matching GitHub). Rely on this:
  - Changing a heading's **capitalization** or **level** does NOT move its anchor.
  - **Moving** a `## ` section only moves its page — the anchor→page map is rebuilt and every `](#anchor)` / `](../README.md#anchor)` link is rewritten to wherever the heading landed. Reordering sections is safe.
  - **Renaming a heading's text** DOES change the slug, so every inbound `#old-slug` becomes unmapped and the build fails. Fix all references in the same change.
- `mise docs-build` = unit tests (`test-docs-pipeline.py`) → assemble → `mkdocs build --strict` → `check-site-links.py`. `--strict` turns any unresolved nav entry / link / anchor into a hard error. Run it after every structural change.

## build-docs-site.py — its HTML-comment directives & site-only transforms

The script does a lot with no GitHub equivalent. Know these before editing README/docs:

- **`<!-- nav-group: NAME -->`** groups the `## ` pages that follow under a link-less "NAME" parent (nested `SUMMARY.md`). Sticky until the next marker; `none`/`-`/empty returns to top level. **Must be flush-left** — an indented copy is CommonMark indented code and stays a literal comment, not a directive. Invisible on GitHub.
- **`DOC_NAV`** (dict in the script) nests a docs page under a nav group and inserts it right **after a named page** instead of the default (appended last, top-level). Within a group, order follows the dict's key order, not alphabetical discovery order. Fail-closed if it names a page that does not exist.
- **`<!-- site:skip --> … <!-- /site:skip -->`** is GitHub-only content dropped from the site (e.g. a "Documentation" link back to the site itself, self-referential there).
- **Hero `<h1>` demotion**: the decorative centered banner `<h1>` on the home page is rewritten to `<p class="hero-title">` on the site so mkdocs' toc gives it no id/permalink; `docs-extra.css` restores the large look. Keep it an `<h1>` in the source — GitHub wants it.
- **Raw-HTML local asset paths**: mkdocs does **not** rewrite `src=`/`href=` inside raw HTML. A split sub-page is served one directory deep (`/suve/<name>/`), so the script prefixes relative raw-HTML asset refs with `../`. (Markdown-syntax links/images *are* rewritten by mkdocs — this only bites raw `<img>`/`<a>`, e.g. the centered demo GIFs.)
- Other tag fix-ups: `<img width/height>` → inline `style=` (Material forces `.md-typeset img { height:auto }`); `<div>` → `markdown="1"` so `md_in_html` renders Markdown inside centered wrappers; `<summary>` → injected `id` so `#anchor` links to a `<details>` resolve.
- **Fail-closed** by design: unmapped/ambiguous anchor, missing asset, output-path collision, empty page slug, un-materialised Git-LFS pointer, or unclosed code fence → non-zero exit. When you add a directive or transform, add a unit test to `test-docs-pipeline.py` — it guards the fragile pure logic, and `--strict` does not catch semantically-valid-but-wrong Markdown.

## python-markdown gotchas GitHub hides

The site renders with python-markdown (mkdocs), stricter than GitHub's CommonMark. These render fine on GitHub and **wrong on the site** — the #1 source of surprises:

- **Blank line before a list or table.** A list/table directly under a paragraph line is folded into it: bullets become literal text, and a heading placed right after a table row with no blank line becomes an extra one-cell **table row** (a real PR #869 bug — the "Behavior & Diagnostics" heading was swallowed by the preceding Staging table).
- **Nested list = 4-space indent.** GitHub nests at 2 spaces; python-markdown needs 4. Inside a blockquote, count the indent *after* the `> `.
- **Exactly one `<h1>` per page.** Multiple `<h1>` (a page title plus `# service` dividers) breaks Material's "on this page" ToC — it renders empty (real PR #869 bug on the Azure page). Use one `<h1>` (the title) with `##`/`###` below.
- `--strict` will **not** catch these — they produce valid HTML, just wrong structure. **Verify the rendered HTML** (`curl` the served page, or read `.site/out/<page>/index.html`) after any structural change, not just the source Markdown.

## Editorial conventions for a restructure

- **Left-nav mirrors how the tool is used**: Home + Installation top-level; onboarding under a "Getting Started" group; the three UIs under "Basic Usage" (Using CLI/GUI/TUI); reference material together (Command Reference + a per-provider "Command Details" group); Environment Variables / Development / License last.
- **One canonical home per fact, linked, not duplicated.** Consolidate scattered copies — e.g. five near-identical per-provider staging tables → one `suve <provider> stage <service> <command>` table with footnotes for the differences; per-cloud auth folded into one "Authentication & Scopes". Slim an over-detailed doc to match its peers rather than bloating the peers.
- **Symmetric provider docs**: the aws/gcloud/azure command references share one section skeleton; keep only genuinely provider-specific nuances (`:LABEL` on AWS Secrets Manager; App Configuration's literal `#`/`~`/`:` keys). Generic commands (`internal/cli/commands/generic/…`) behave identically across providers, so a behavior true for one is usually true for all — level up by copying the shared behavior, not inventing per-provider claims.
- **Title Case headings** with minor words (`for`, `of`, `and`, `with`, …) lowercase; consistent singular/plural (enumerations plural — Providers, Examples, Version Specifiers; single concepts singular — Command Reference, Conflict Detection). All anchor-safe (slugs are lowercased).
- Reference bodies are **current-state-only** — that content-sync discipline lives in the **docs-audit** skill. This skill is about *structure*; docs-audit is about *content accuracy vs the implementation*.

## Verify behavioral claims against the code

Documentation claims about CLI behavior are easy to get subtly wrong, and a large restructure multiplies the risk. In PR #869 an external review (Codex `mcp__codex__codex`) caught: a consolidated table presenting per-provider flags as universal (`--description` is gated on whether the writer stores one; App Configuration staging **does** have `tag`/`untag`), a dropped `reset <name>#<VERSION>` restore behavior, and an over-promised identical-content warning (self-comparison prints a hint; two distinct-but-equal versions do not). Cross-check flag lists and behavior against `internal/cli`, `internal/staging/cli`, and `internal/cli/commands/generic` before asserting them, and prefer a second reviewer for large docs PRs.
