---
name: add-provider-adapter
description: Use when adding a new cloud provider adapter, or extending an existing one with a new service, in internal/provider/<name>/. Covers the Store interface, SDK confinement, error mapping, version-spec parsing, CLI/registry wiring, and staging support.
---

# Adding or extending a provider adapter

A provider adapter implements the neutral seam in `internal/provider` so the rest
of suve never sees a cloud SDK. Google Cloud (PR #231), Azure Key Vault + App
Configuration (PR #239), and the AWS re-implementation (PR #224) all follow the
same shape.

## Core adapter

- Implement `provider.Store` — the composition of `Reader`, `Writer`, and
  `Tagger` (`internal/provider/provider.go`) — over a **narrow in-package client
  wrapper**. Keep the cloud SDK confined to `internal/provider/<name>/**`.
- Hide SDK paginators and iterators behind a `Wrap()` adapter so the seam
  returns neutral `domain` types, never SDK types.
- Map SDK errors to the neutral sentinels in `internal/provider/errors.go`:
  `provider.ErrNotFound` and `provider.ErrAlreadyExists`. Missed error mappings
  are a recurring defect source (#318, #481) — enumerate every not-found and
  already-exists SDK error shape and cover it in unit tests.
- Do **pagination and deterministic ordering from day one**. List and history
  operations must page through all results and break timestamp ties with a
  stable secondary key. Skipping this produced #311, #312, and #314.
- In `History`, mark the version the provider serves as current with
  `domain.Version.Current`. The shared use cases read it for `IsCurrent`. They
  do not guess it from labels or from position.

## Version-spec parser

- Add a parser package `internal/version/<name>version` (see the existing
  `gcloudversion`, `azurekvversion`, `azureappconfigversion`). It resolves
  `#VERSION` / `~SHIFT` / `:LABEL` per the provider's capabilities and **cleanly
  rejects unsupported specifiers before any API call**. Google Cloud rejects
  labels with a dedicated error at parse time (`ErrLabelUnsupported`, #231);
  App Configuration is unversioned and rejects all specifiers.
- For a versioned grammar, add `Suffix(spec)` next to `Parse`. The CLI and GUI
  call the neutral `usecase/{param,secret}` with `spec.Name` plus
  `Suffix(spec)`. There is no per-provider use case package.

## Wiring

- Wire a `Factory` (`internal/provider/registry.go`) that returns
  `provider.ErrUnsupportedKind` for services the provider does not offer.
- Expose `Register(reg)` from the adapter package and call it from
  `builtin.NewRegistry()` (`internal/provider/builtin/builtin.go`), which the
  CLI, GUI and TUI all build from; wire the command group in
  `internal/cli/commands/app.go`.
- Add the provider's scope flag plus its environment-variable fallback.

## SDK confinement guards

- Extend `internal/architecture_test.go` and the `depguard` block in
  `.golangci.yaml` to gate the new SDK module so it can only be imported from
  the provider directory. Confinement is enforced across all of `internal/`
  (#488, #502) — keep that breadth.

## Staging support (separate work)

Staging is a distinct increment on top of read/write (#247 → #261, #262):

- Implement the provider `ScopeResolver` (next to the others in
  `internal/cli/commands/internal/client.go`) and set it on every staging
  config; there is no default resolver.
- Add the provider's staging strategy in `internal/staging/<cloud>_<service>.go`.
  A versioned service embeds `versionedStrategy[H]` (`internal/staging/versioned.go`)
  and supplies a zero-size hooks type `H` (traits, `parse` via its version
  package's `Parse` + `Suffix`, and any delete-option or write differences; a
  secret service embeds `versionedSecretHooks` for the defaults). The zero value
  must work as a store-less parser, because the GUI and TUI build parsers as
  `&staging.XStrategy{}`. An unversioned service writes its own strategy, as
  `azure_appconfig_param.go` does.
- Set `ProviderLabel` (e.g. `"Google Cloud"`) and `CommandPath` (the explicit
  stage path, e.g. `"suve gcloud stage"`) on every `stgcli.CommandConfig` and
  `stgcli.GlobalConfig`. The shared staging help, usage errors, and prompts
  render from them, so a missing value shows up as a blank path or "remote".
- Register the service spec in `GlobalConfig` (#261).

## PR conventions

- Record any friction with the neutral abstraction in a "Design feedback"
  section of the PR body (#231 — this feedback fed the seam epic #419).
- Close against #231's acceptance checklist: every op works, unsupported specs
  are rejected before the API call, no foreign SDK appears outside the provider
  directory, and `mise test` + `mise lint` are green.
