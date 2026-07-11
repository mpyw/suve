---
name: docs-audit
description: Use when keeping docs in sync with the implementation — sweep command tables/env-vars/paths against the real CLI, keep reference docs current-state-only (migrations go to release notes), enforce identical terminology across CLI help / README / GUI, and track the README install/aqua section against the external aqua-registry.
---

# Docs audit

Sweep documentation for divergence from the implementation and fix it. Evidence: audit #448 → fix PR #442; self-review sweep PR #245; aqua-version drift #544; export/import docs PR #461; README restructure PR #614; terminology work #409 → PR #412; release/aqua coupling PRs #303, #304, #309.

## Procedure

### 1. Sweep for divergence

Compare docs against the real implementation and file/fix each divergence (#448 catalogued them; #442 fixed them):

- Command tables and flag lists vs the actual `--help` output.
- Documented env vars, paths, and package names vs the code (missing packages, renamed dirs, drifted interface descriptions).
- Claims about behavior vs what the code does (e.g. an "E2E is SSM-only" claim the current suite contradicts).

### 2. Reference docs describe current state only

Reference/README bodies describe **the current state only**. Do not write "removed", "no longer", "renamed from", or "BREAKING" in a reference body — describe what the thing *is* now, in the present tense. Migrations and breaking-change narratives go to **release notes**, not reference docs (the #456 / PR #461 pattern: the export/import reference docs describe export/import as they exist; the stash→export/import migration lives in the release note).

### 3. Terminology identical across surfaces

The same concept uses the same word in CLI help, README, and the GUI. Maintain the native→suve term mapping explicitly (#409 / PR #412): Google Cloud "labels" are surfaced as suve **tags**; the Azure App Config "label" axis is surfaced as a **namespace** and is distinct from metadata tags. When one surface changes wording, change all of them in the same PR.

### 4. Track the README install/aqua section against the external registry

The README install/aqua section must track the external aqua-registry package `pkgs/mpyw/suve` (regenerated with `argd gr`; `version_constraint` floor currently `v1.6.1`). This drifts independently of the code, so check it every sweep — #544 was exactly this drift (the aqua section advertised an outdated minimum version). Release-archive and install-method changes couple here too (PRs #303, #304, #309).
