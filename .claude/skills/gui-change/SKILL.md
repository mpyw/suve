---
name: gui-change
description: Use when changing the Wails/Svelte GUI in internal/gui/ — backend bindings, capability-driven UI, provider/scope switching, or async loads. Covers the bindings-regenerate-then-rebuild rule, server-side guards, and the recurring async-bug checklist.
---

# Changing the GUI

The GUI is Wails (Go bindings in `internal/gui/*.go`) plus a Svelte frontend
(`internal/gui/frontend/src/`). Multi-cloud parity was tracked in #250 and
landed across #273–#282.

## Bindings: regenerate, then rebuild immediately

- Any backend binding change requires `mise generate-gui-bindings`, then an
  immediate `mise build-gui`. Stale bindings make every API arity mismatch, so
  **verification is always `mise build-gui`, never the CLI build**.

## Capability-driven UI

- Drive control visibility from the per-provider capability descriptor
  (`ProviderCapability` / `ServiceCapability` in `internal/gui/providers.go`,
  #264/#274). Hide unsupported controls via the descriptor; never hardcode
  provider conditionals in Svelte.
- Security-relevant guards live **server-side in the Go bindings**, not only in
  frontend hiding (#276) — e.g. staging guards and scope validation/readback.

## Provider/scope switching

- A provider or scope switch is a **full view remount** driven by
  `{#key scopeKey}` (`internal/gui/frontend/src/App.svelte`). Reset badges and
  cancel pending debounces on switch so no stale state or stray request survives
  the remount (#266).

## Testing (three-layer rule)

- Playwright coverage is per-feature definition-of-done (#250). Any
  GUI-visible behavior requires a Playwright test
  (`internal/gui/frontend/tests/`) in addition to Go unit and CLI e2e coverage.

## Recurring async-bug checklist

Check every GUI change against these classes:

- Stale-response guards on async loads — ignore a resolved load whose request
  has been superseded by a newer one (#539, #566).
- Busy-guards against double-fire on repeated clicks/actions (#568).
- Modal dismissal gating while an operation is mid-flight (#565).
- Errors must not be swallowed at the binding boundary; surface them to the UI
  (#550, #447).
