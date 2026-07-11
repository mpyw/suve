---
name: bug-fix-pr
description: Use when turning one filed bug issue into a fix PR — reproduce from the issue's Failure-scenario section, fix at the cited site, add a regression test at every layer the three-layer rule names, run the gates, and write a PR whose Verification section shows real command output.
---

# Bug fix PR

One filed bug = one PR. Evidence for this workflow: the fix-PR waves #363–#408, #494–#517, #571–#613, each closing a single audit child.

## Procedure

### 1. Reproduce first

Read the issue. Its `## Failure scenario` gives the exact reproduction; its `## Where` gives the `file:line` anchors. Replay the failure before touching anything, so the fix is validated against a real reproduction rather than a guess. Confirm the cites still resolve at current `main`.

### 2. Fix at the cited site

Make the minimal correct change at the anchors the issue names. If the true root cause is elsewhere, fix the root cause and say so in the PR body — do not paper over it at the symptom site.

### 3. Regression test at every layer the three-layer rule names

A feature/bug that is visible at multiple layers must be tested at each: **Go unit** test, **CLI e2e** test, and **GUI Playwright** test. Anything visible in the GUI needs Playwright coverage — a Go-only fix for a GUI-visible bug is incomplete (cf. #417 tag-only staged changes made visible in the Staging Area; #606 auto-unstaged notice; #607 inline-row double-fire guard, all with Playwright coverage). Add the regression test at the layer the bug actually lives in, plus any layer that surfaces it.

### 4. Run the gates

- `mise test` and `mise lint` — always.
- `mise e2e-aws` (and the `-gcloud` / `-azure-appconfig` / `-azure-keyvault` variants for the affected providers) when command behavior changed.
- `mise build-gui` when GUI code or the wailsjs bindings were touched.

### 5. Write the PR

- Title: `fix(<area>): <imperative summary> (#<issue>)`.
- Body opens with `Closes #<issue>`.
- Body ends with a **Verification** section showing real command output (the actual test/gate runs, or a before/after transcript) — not a claim that it passes. #231 and #619 are the exemplars for a Verification section.
- Before opening the PR, have a context-isolated subagent critically review the change and apply the improvements it surfaces (repo convention).
