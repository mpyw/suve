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
- Put each service adapter in a subpackage named after the cloud product
  (`aws/parameterstore`, `aws/secretsmanager`, `gcloud/secretmanager`,
  `azure/appconfig`, `azure/keyvault`), not after the service axis: the
  service axis is already `provider.Kind`. The provider root package holds the
  `Factory`, the client bootstrap, and any identity lookup (`aws.LoadIdentity`).
- Hide SDK paginators and iterators behind a `WrapClient()` adapter in the
  package's `client.go`, so the seam returns neutral `domain` types, never SDK
  types. The adapter's main file (`keyvault.go`, ...) is the package's
  `//declscope:core`, and `client.go` keeps its own namespace.
- Map SDK errors to the neutral sentinels in `internal/provider/err.go`:
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

- Add the product's grammar value to `internal/version/products.go`, built from
  one of the three grammars: `NumericGrammar` (integer versions),
  `OpaqueGrammar` (opaque ids, `Labels` for staging labels) or `BareGrammar`
  (unversioned). It parses `#VERSION` / `~SHIFT` / `:LABEL` per the product's
  capabilities and **cleanly rejects unsupported specifiers before any API
  call**: Google Cloud and Key Vault set `LabelError` so a `:LABEL` fails at
  parse time (#231). App Configuration is unversioned, and its keys may contain
  `#`, `:` and `~`, so the whole argument is the key. Add a new grammar only
  when none of the three fits.
- A versioned grammar has `Suffix(spec)` next to `Parse`, and every grammar has
  `Split(input)` (name plus suffix in one call). The CLI calls the neutral
  `usecase/{param,secret}` with `spec.Name` plus `version.<Product>.Suffix(spec)`.
  The GUI takes the grammar from the product's `internal/staging/binding` entry
  (`SplitSpec`), so a new product's grammar goes there too. There is no
  per-provider use case package.
- The diff command's argument parsing lives in the CLI, not in
  `internal/version`: `diff.go` in each service package defines
  `parseDiffArgs`, which wraps `generic.ParseDiffArgs` with the
  grammar and the usage string.

## Wiring

- Wire a `Factory` (`internal/provider/registry.go`) that returns
  `provider.ErrUnsupportedKind` for services the provider does not offer.
- Expose `Register(reg)` from the adapter package and call it from
  `builtin.NewRegistry()` (`internal/provider/builtin/builtin.go`), which the
  CLI, GUI and TUI all build from; wire the command group in
  `internal/cli/commands/app.go`. The provider root package
  (`internal/cli/commands/<cloud>`) exports `Command()` plus a
  `Flat<Service>Command(name)` for each service and for stage, which `app.go`
  uses for the env-detected flat aliases.
- Put the provider's context keys, store resolvers (a scope built from the
  flag/env value, then `cliinternal.Store(ctx, scope, kind)`) and staging-scope
  resolvers in `internal/cli/commands/<cloud>/internal`, shared by the root
  group and its service packages. `internal/cli/commands/internal` holds only
  the provider-neutral registry and staging wiring.
- Lay out each service's CLI as one flat package,
  `internal/cli/commands/<cloud>/<service>`, even when the provider has only one
  service, with one file per subcommand
  (`show.go` exposes `ShowCommand()`, `create.go` exposes `CreateCommand()`,
  and so on). Build the read commands on the flat
  `internal/cli/commands/generic` package (`generic.ShowCommand`,
  `generic.LogCommand`, `generic.DiffCommand`, `generic.ListCommand`,
  `generic.TagCommand`, `generic.UntagCommand`). Do not add a subpackage per
  subcommand: declscope already scopes each file, and `//declscope:package`
  marks what sibling files share.
- Add the provider's scope flag plus its environment-variable fallback
  (`detect.HydrateScope` for the TUI/GUI launch, `launchScope` in
  `internal/cli/commands/launch.go` for the `--tui`/`--gui` flags).

## SDK confinement guards

- Extend `internal/architecture_test.go` and the `depguard` block in
  `.golangci.yaml` to gate the new SDK module so it can only be imported from
  the provider directory. Confinement is enforced across all of `internal/`
  (#488, #502) — keep that breadth.
- Provider vocabulary that the CLI, TUI or GUI imports directly (such as
  `internal/provider/aws/paramtype` and
  `internal/provider/azure/appconfig/namespaces`) lives in its own SDK-free
  package under the adapter. List it in `TestSDKFreeProviderVocabulary` and in
  a `<cloud>-sdk-free-vocabulary` depguard rule so it stays SDK-free.

## Staging support (separate work)

Staging is a distinct increment on top of read/write (#247 → #261, #262):

- Add the provider's staging strategy in `internal/staging/<cloud>_<service>.go`.
  A versioned service embeds `versionedStrategy[H]` (`internal/staging/versioned.go`)
  and supplies a zero-size hooks type `H` (traits, `parse` via its version
  package's `Parse` + `Suffix`, and any delete-option or write differences; a
  secret service embeds `versionedSecretHooks` for the defaults). The strategy
  built over a nil store must work as a store-less parser, because that is the
  parser every surface uses. An unversioned service writes its own strategy, as
  `azure_appconfig_param.go` does.
- Register it in the descriptor table in `internal/staging/binding/binding.go`:
  parser, strategy constructor, staging-scope derivation (and an identity lookup
  if the scope needs a network call), and the namespace flag if the service has
  one. The CLI, GUI and TUI all read it from there; do not add a per-provider
  switch in any UI.
- Implement the provider `ScopeResolver` (in
  `internal/cli/commands/<cloud>/internal`, a flag/env check plus
  `binding.StagingScope`) and set it on every staging config; there is no
  default resolver. Build the config's `Factory` / `ParserFactory` with
  `cliinternal.StrategyFactory` / `cliinternal.ParserFactory`.
- Set `ProviderLabel` (e.g. `"Google Cloud"`) and `CommandPath` (the explicit
  stage path, e.g. `"suve gcloud stage"`) on every `stgcli.CommandConfig` and
  `stgcli.GlobalConfig`. The shared staging help, usage errors, and prompts
  render from them, so a missing value shows up as a blank path or "remote".
- Put the staging wiring in one `stage.go` in the provider root package, not
  in a subpackage (`aws/stage.go` and `azure/stage.go` each hold both
  services).
- Register the service spec in `GlobalConfig` (#261).

## PR conventions

- Record any friction with the neutral abstraction in a "Design feedback"
  section of the PR body (#231 — this feedback fed the seam epic #419).
- Close against #231's acceptance checklist: every op works, unsupported specs
  are rejected before the API call, no foreign SDK appears outside the provider
  directory, and `mise test` + `mise lint` are green.
