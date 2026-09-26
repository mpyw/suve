---
name: feature-epic-breakdown
description: Use when planning a multi-PR feature or cross-cutting refactor as an epic — write a design-complete epic body, slice into layer-ordered sub-issues that each restate their slice standalone with DELETE/ADD/MODIFY tasks and dependencies, use an integration branch when the change must land coherently, and require a context-isolated review before every PR.
---

# Feature epic breakdown

Plan a feature so that each sub-issue is a self-contained PR and the whole lands coherently. Evidence: #451 (the gold standard — export/import, sub-issues #452–457), #419 (phased domain→adapters→usecase→CLI→GUI refactor), #410 (integration-branch epic).

## Epic body

Write the epic so a work agent needs nothing else:

- **Summary** → **Motivation**.
- **Finalized design** — schemas and tables inline, not linked. #451 embeds the full JSON envelope schema and a complete CLI-surface table; #419 tables the per-provider semantics being modeled.
- **Sub-issues in dependency order**, with the order stated explicitly (`#452 → #453 → #454 → (#455 and #456 in parallel) → #457`; each green under its gates before the next starts).
- **Edge cases shared across all steps** — called out once, centrally.
- **Blast radius** — file counts per layer, so scope is legible.
- **Validation gates** — the concrete `mise` commands.
- **"Done when"** acceptance criteria (#410 closes with an explicit Done-when list).

## Slicing

Slice by layer, in dependency order:

1. **Storage** (schema, serialization; delete the thing being replaced).
2. **Usecase** (business logic).
3. **CLI + e2e** (commands, wiring, e2e coverage).
4. **GUI** (Go DTO/API, regenerated wailsjs bindings, Svelte view, Playwright).
5. **Docs + breaking-change note** (reference docs current-state-only; migration to release notes).
6. **Final integration & comprehensive review** — its own last sub-issue (#457).

Each sub-issue must: restate its slice standalone (a fixer reads only that issue), list **DELETE / ADD / MODIFY** tasks against concrete `file:line` sites, name its gates, and state its dependency (`Do not start until Step 2 (Usecase) is merged` — #454). Link children as GitHub sub-issues and mirror them as a body checklist.

## Integration branch

When the change must land as one coherent concept rather than piecemeal, sub-PRs target an **integration branch** `epic/<name>` and a final PR merges it to `main` (`epic/version-state-vs-staging-labels` in #419; `epic/appconfig-namespace-terminology` in #410). Otherwise each sub-PR targets `main` directly.

## Mandatory review clause

Include this clause verbatim in the epic (and echo it in each sub-issue, as #454 does):

> Before opening the PR, have a **context-isolated subagent** (one that does **not** share this conversation's context) perform a **critical review** of the changes, apply the quality improvements it surfaces, and only then open the pull request.
