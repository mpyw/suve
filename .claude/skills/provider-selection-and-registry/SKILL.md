---
name: provider-selection-and-registry
description: >-
  Load when wiring a top-level command group, adding a cloud, or touching
  internal/provider/registry.go, internal/provider/detect/,
  internal/staging/binding/, internal/cli/commands/launch.go, or
  internal/cli/commands/internal/client.go or
  internal/cli/commands/<cloud>/internal. Explains how a provider is selected
  (explicit groups plus env-detected flat aliases), how the registry composes
  backends, how each provider's scope and staging binding are built, how the
  TUI/GUI launch scope is resolved, and the SDK-confinement boundary.
---

# Provider selection and registry

There is no `--provider` flag. A command chooses its provider through the seam:
the CLI and staging layers talk to a provider-neutral `provider.Store`, and a
`provider.Registry` maps a `provider.Scope` to the concrete backend.

## Selection model

Two ways to reach a provider coexist:

1. **Explicit groups — always present.** `suve aws`, `suve gcloud`, and
   `suve azure` are registered unconditionally (`internal/cli/commands/app.go:50-54`).

2. **Flat aliases — env-detected.** The bare `param`, `secret`, and `stage`
   commands are aliases added only when exactly one provider is active for that
   service. Detection runs at process start
   (`detect.Resolve(detect.OSEnvironment())`, invoked at `app.go:42`) and is
   implemented in `internal/provider/detect/detect.go`. It reads only env vars
   (no network, no credential-chain resolution):

   | Provider | Service | Active when env set |
   |----------|---------|---------------------|
   | AWS | param + secret | `AWS_ACCESS_KEY_ID` \| `AWS_VAULT` \| `AWS_PROFILE` (final fallback: `~/.aws/credentials` exists) |
   | Google Cloud | secret | `GOOGLE_CLOUD_PROJECT` |
   | Azure | secret (Key Vault) | `AZURE_KEYVAULT_NAME` |
   | Azure | param (App Config) | `AZURE_APPCONFIG_NAME` |

   A flat alias for a service is exposed only when exactly ONE provider is active
   for it — zero or two-plus active means no alias, and the user picks an
   explicit group. There is no priority order. So `suve secret` may resolve to
   AWS, Google Cloud, or Azure depending on the environment. Each flat alias is
   built by the provider's `Flat{Param,Secret,Stage}Command(name)`
   (`aws.FlatParamCommand`, `gcloud.FlatSecretCommand`, `azure.FlatStageCommand`,
   and so on), which reuses the real command and folds in any group-level flag
   (`gcloud` adds `--project`). `app.go`'s `flatCommand`/`flatStageCommand`
   pick one per provider.

## Registry composition

The registry is built once per process by `builtin.NewRegistry()`
(`internal/provider/builtin/builtin.go`), which starts from an empty
`provider.NewRegistry()` and registers every cloud on equal footing:

```go
reg := provider.NewRegistry()
aws.Register(reg)
gcloud.Register(reg)
azure.Register(reg)
```

The CLI (`internal/cli/commands/internal/client.go`, which exposes it as
`Store(ctx, scope, kind)`), GUI (`internal/gui/app.go`)
and TUI (`internal/tui/run.go`) all use it. No provider is a default: an
unknown or unselected provider is an error everywhere (registry lookup, staging
scope resolution, strategy selection), never a silent fallback to AWS.

Each command group resolves its store through this shared registry via
`cliinternal.Store(ctx, scope, kind)` (`kind` is `provider.KindParam` or
`provider.KindSecret`). A `Factory` returns `provider.ErrUnsupportedKind` when a
provider does not offer a requested kind, and the registry returns
`provider.ErrNoFactory` for an unregistered provider
(`internal/provider/registry.go:17-25`).

## Scope construction per provider

Each group builds a provider-specific `provider.Scope` (`internal/provider/scope.go`):

Each provider's store and staging-scope resolvers, and any context keys they
read, live in `internal/cli/commands/<cloud>/internal` (imported as
`awsinternal`, `gcloudinternal`, `azureinternal`), shared by that provider's
root group, service packages and stage commands.

- **AWS** — read/write commands resolve stores through `awsinternal.ParamStore` /
  `awsinternal.SecretStore` with `provider.Scope{Provider: provider.ProviderAWS}`.
  Only the provider field is needed because the AWS factory builds its client
  from the ambient AWS config (region from env/profile), so no STS
  `GetCallerIdentity` call is made on the read/write path. The full
  account/region identity (`infra.GetAWSIdentity` → `provider.AWSScope(accountID, region)`)
  is resolved separately by `binding.StagingScope` (the CLI's
  `awsinternal.StagingScopeResolver`, the GUI and the TUI all go through it), only where
  staging state must be keyed.
- **Google Cloud** — the project id from `--project` or `GOOGLE_CLOUD_PROJECT`
  (`provider.GoogleCloudScope(project)`). The `gcloud` root package owns the
  flag and the Before hook, which stores the id with `gcloudinternal.WithProject`
  for both `gcloud secret` and `gcloud stage`.
- **Azure** — the Key Vault name (`--vault-name` / `AZURE_KEYVAULT_NAME`) via
  `provider.AzureKeyVaultScope(vault)`, or the App Configuration store name
  (`--store-name` / `AZURE_APPCONFIG_NAME`) via
  `provider.AzureAppConfigScope(store)`. Each is a globally-unique name that
  fully identifies the resource, so no subscription/resource group is needed.

Staging is available on AWS, Google Cloud, and Azure. The per-(provider, kind)
staging rules live in one place, `internal/staging/binding`, which the CLI, GUI
and TUI all use:

| `binding` API | Owns |
|---------------|------|
| `Lookup(p, kind)` → `Binding` | `Parser()` / `ParserFactory()` (store-less parser), `Strategy(store)`, `Namespaced(sc)` / `NamespaceScope(sc, ns)` (App Configuration namespace override) |
| `StagingScope(ctx, sc, kind, lookup)` | the scope that keys staging state plus the confirmation target. Azure keys param by store and secret by vault; AWS resolves the STS identity (pass `lookup` to memoize or stub it; nil uses `DefaultIdentity`) |

An unknown provider is `binding.ErrUnknownProvider`; a known provider without
the kind is `provider.ErrUnsupportedKind`.

Every staging command config (`stgcli.CommandConfig`, `stgcli.GlobalConfig`,
`stgcli.GlobalServiceSpec`) must set a `ScopeResolver`. The CLI resolvers
(`awsinternal.StagingScopeResolver`, `gcloudinternal.StagingScopeResolver`,
`azureinternal.KeyVaultStagingScopeResolver`,
`azureinternal.AppConfigStagingScopeResolver`) check the flag/env value and
then call `binding.StagingScope`. Stage configs get `Factory` from
`cliinternal.StrategyFactory(p, kind, store)` and `ParserFactory` from
`cliinternal.ParserFactory(p, kind)`. A nil resolver fails the command. Each
scope keys its on-disk staging state (`provider.Scope.Key`), partitioning staged
changes per scope under `~/.suve/staging/<scope key>/`.

## UI launch (`--tui` / `--gui`)

Both launch flags register through `RegisterLaunchMode`
(`internal/cli/commands/launch.go`): the flag on the root, each provider group,
and Azure's param/secret subgroups; the launch scope from `--project` /
`--vault-name` / `--store-name` / `--namespace`. Env hydration is
`detect.HydrateScope`. A bare `suve --tui` / `suve --gui` uses
`detect.Result.UniqueProvider`: the sole provider active on any of the param,
secret, or stage axes. The TUI errors when it is not unique; the GUI opens its
provider picker (the frontend's `uniqueActiveProvider` in `App.svelte` applies
the same rule to `DetectProviders`).

## SDK-confinement boundary

`internal/architecture_test.go` fails the build if any non-test package outside a
provider adapter imports a cloud service SDK directly. depguard
(`.golangci.yaml`) enforces the same at lint time.

| SDK (confined) | Allowed only in |
|----------------|-----------------|
| `aws-sdk-go-v2/service/ssm`, `.../secretsmanager` | `internal/provider/aws/**` |
| `cloud.google.com/go/secretmanager` | `internal/provider/gcloud/**` |
| `github.com/Azure/azure-sdk-for-go` | `internal/provider/azure/**` |

The AWS config loading and STS identity lookup (`aws.LoadConfig`,
`aws.LoadIdentity`) sit in the `internal/provider/aws` root package. The `internal/gui` tree is guarded too: it constructs
stores through the registry rather than a cloud SDK.

## Adding another cloud

1. Implement `provider.Reader` / `Writer` / `Tagger` in a new
   `internal/provider/<cloud>/**` adapter (keep every SDK import inside it).
2. Expose a `Register(reg)` from the adapter package and call it from
   `builtin.NewRegistry()` (`internal/provider/builtin/builtin.go`).
3. Add its version-spec parser under `internal/version/`.
4. Add its detection signal and env hydration in
   `internal/provider/detect/detect.go`, its launch-scope flags in
   `internal/cli/commands/launch.go`, and wire a command group in
   `internal/cli/commands/<cloud>/` (with `Flat*Command` constructors) whose
   `<cloud>/internal` package builds the provider's `provider.Scope`.
5. Add its staging entry to the descriptor table in
   `internal/staging/binding/binding.go`.

Everything downstream — the generic command presenters and version resolution —
is provider-neutral and reused unchanged.
