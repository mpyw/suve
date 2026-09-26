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
  immediate `mise build-gui`. The task runs the wails CLI pinned in `mise.toml`,
  which must match the `wailsapp/wails/v2` version in `go.mod`/`gui/go.mod`;
  bump both together. It needs `internal/gui/frontend/dist/index.html` to
  exist (a placeholder is enough) and deletes it when it exits. Stale bindings make every API arity mismatch, so
  **verification is always `mise build-gui`, never the CLI build**.

## Capability-driven UI

- Drive control visibility from the per-provider capability descriptor
  (`capability.ProviderCapability` / `capability.ServiceCapability` from
  `internal/capability`, bound by `Capabilities` in `internal/gui/capability.go`;
  the generated TypeScript types live in the `capability` namespace of
  `wailsjs/go/models.ts`). Hide unsupported controls via the descriptor; never hardcode
  provider conditionals in Svelte. The descriptor also carries the data a
  provider switch used to hold: `hasValueType` (the param Type dropdown),
  `hasNamespaces` (the namespace axis), `hasRecursiveList` (the Recursive
  toggle, Parameter Store only), `scopeField` (the scope field a service
  needs, so Azure shows a tab only for the vault or store that is set),
  `nativeTagName` (the "(= Google Cloud: labels)" hint), and `itemNoun`
  (the "parameter"/"setting"/"secret" word in titles and prompts; it equals the
  staging strategy's `ItemName`, and `displayName` equals its `ServiceName`,
  both pinned by tests in `internal/staging/binding`).
- The sidebar scope form is built from the provider's `scopeFields`, and a
  scope is complete once at least one service's `scopeField` is set. The
  helpers live in `internal/gui/frontend/src/lib/scopeFields.ts`: its
  `SCOPE_FIELDS` table maps each field name to the `ScopeSelection` property,
  input id, label and hint. Add a scope field there, not a provider branch in
  `App.svelte` or `Sidebar.svelte`, and the matching entry in the Go
  `scopeFields` table (`internal/gui/scope.go`), which `scopeFromSelection`
  uses to copy and validate the same fields server-side.
- Go code looks capabilities up with `capability.Service(p, service)`,
  `capability.Provider(p)` and `capability.DisplayName(p)`. The bindings gate
  on `a.serviceCapability(kind)` (`internal/gui/capability.go`) rather than on
  `Provider == provider.ProviderX`.
- The sidebar's scope target comes from `GetScopeTarget` (no network) and,
  while it is pending, `ResolveScopeTarget` (AWS: STS). Both wrap
  `provider.Target` (`internal/gui/target.go`); render its segments, never a
  per-provider block. "Change scope" shows when the provider has `scopeFields`.
- Binding DTOs use neutral names. A secret carries `version` and `labels`
  (a version's movable labels, such as `AWSCURRENT`; not suve's staging), and
  provider-specific display metadata arrives as `extra` (`{label, value}[]`
  from `domain.Entry.Extra`). Render `extra` verbatim; never add a
  cloud-specific DTO field such as an ARN. The TUI renders the same fields as
  detail meta rows.
- Lists are not paginated: every provider lists all names in one call, so
  `ParamList`/`SecretList` take no page size or cursor and return no token.
- Security-relevant guards live **server-side in the Go bindings**, not only in
  frontend hiding (#276) — e.g. staging guards and scope validation/readback.

## Provider-specific staging rules

- Do not switch on the provider in `internal/gui` for version grammars,
  staging parsers, strategies, staging scopes, or the App Configuration
  namespace. Look them up in `internal/staging/binding`, which the CLI and
  TUI share (the GUI's `stagingBinding`, `stagingScopeForKindScoped`,
  `effectiveParamScopeScoped`, and `parseSpec` in `internal/gui/spec.go`,
  which uses `Binding.SplitSpec`).
- "Apply All" is the `StagingApplyAll` binding (`internal/gui/staging_apply.go`)
  over `usecase/staging.GlobalApplyUseCase`, the same use case as the CLI's
  all-service `stage apply`: every service is conflict-checked before any is
  applied. Never loop the per-service `StagingApply` from the frontend.
- The launch scope and the bare-launch provider come from the shared helpers:
  `RegisterLaunchMode` (`internal/cli/commands/launch.go`),
  `detect.HydrateScope`, and `detect.Result.UniqueProvider`.

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
