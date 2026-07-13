---
name: refresh-demo-gifs
description: Use when re-recording the CLI, TUI, or GUI demo GIFs after a UI change or a user-visible output change. Covers the record scripts, robust Playwright selectors, output-drift triggers, frame verification, and Git LFS.
---

# Refreshing the demo GIFs

The demos are recorded and committed as GIFs (PRs #619, #618, #61):

- **CLI demo**: `demo/cli-demo.tape` recorded via `./demo/cli-record.sh` (vhs).
- **TUI demo**: `demo/tui-demo.tape` recorded via `./demo/tui-record.sh` (vhs, same
  terminal-recording path as the CLI demo — the TUI is pure Go, no browser).
- **GUI demo**: `demo/gui-demo.spec.ts` recorded via `./demo/gui-record.sh`
  (Playwright-driven).

## Selector drift is the dominant failure mode

After a UI change the GUI recording breaks on selectors:

- Prefer role/label selectors over positional ones — use
  `page.getByRole('checkbox', { name: 'Show Values' })`, not `.first()`.
- Re-check navigation labels against the provider display names in
  `internal/gui/providers.go` (AWS {Param, Secret}, Google Cloud {Secret},
  Azure {App Configuration, Key Vault}).

## CLI output drift also invalidates the GIF

- User-visible CLI output changes invalidate the CLI GIF even without a "UI"
  change — e.g. `param ls` sorting (#480, PR #508). Re-record after such changes.

## Verify and commit

- Verify by **frame inspection** of the produced GIF — confirm each intended
  step is visible (PR #619 lists per-frame confirmations).
- The GIFs are tracked as **Git LFS** pointers (`*.gif filter=lfs` in
  `.gitattributes`). Confirm the committed `demo/cli-demo.gif` /
  `demo/gui-demo.gif` are LFS pointer files, not raw binaries.
