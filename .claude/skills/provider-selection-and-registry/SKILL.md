---
name: provider-selection-and-registry
description: >-
  Load when wiring a top-level command group, adding a cloud, or touching
  internal/provider/registry.go, internal/provider/detect/, or
  internal/cli/commands/internal/client.go. Explains how a provider is selected
  (explicit groups plus env-detected flat aliases), how the registry composes
  backends, how each provider's scope is built, and the SDK-confinement boundary.
---

# Provider selection and registry

There is no `--provider` flag. A command chooses its provider through the seam:
the CLI and staging layers talk to a provider-neutral `provider.Store`, and a
`provider.Registry` maps a `provider.Scope` to the concrete backend.

## Selection model

Two ways to reach a provider coexist:

1. **Explicit groups — always present.** `suve aws`, `suve gcloud`, and
   `suve azure` are registered unconditionally (`internal/cli/commands/app.go:47-51`).

2. **Flat aliases — env-detected.** The bare `param`, `secret`, and `stage`
   commands are aliases added only when exactly one provider is active for that
   service. Detection runs at process start
   (`detect.Resolve(detect.OSEnvironment())`, invoked at `app.go:39`) and is
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
   AWS, Google Cloud, or Azure depending on the environment, and each flat alias
   reuses its provider's real command implementation (`app.go:220-260`).

## Registry composition

The registry is built once, with every backend registered
(`internal/cli/commands/internal/client.go:22-28`):

```go
reg := aws.NewRegistry()
gcloud.Register(reg)
azure.Register(reg)
```

Each command group resolves its store through this shared registry via
`registry.Store(ctx, scope, kind)` (`kind` is `provider.KindParam` or
`provider.KindSecret`). A `Factory` returns `provider.ErrUnsupportedKind` when a
provider does not offer a requested kind, and the registry returns
`provider.ErrNoFactory` for an unregistered provider
(`internal/provider/registry.go:37-56`).

## Scope construction per provider

Each group builds a provider-specific `provider.Scope` (`internal/provider/scope.go`):

- **AWS** — read/write commands use `provider.Scope{Provider: provider.ProviderAWS}`
  (`client.go:114`). Only the provider field is needed because the AWS factory
  builds its client from the ambient AWS config (region from env/profile), so no
  STS `GetCallerIdentity` call is made on the read/write path. The full
  account/region identity (`infra.GetAWSIdentity`,
  `internal/provider/aws/infra/client.go:164` → `provider.AWSScope(accountID, region)`)
  is resolved separately, only where staging state must be keyed.
- **Google Cloud** — the project id from `--project` or `GOOGLE_CLOUD_PROJECT`
  (`provider.GoogleCloudScope(project)`).
- **Azure** — the Key Vault name (`--vault-name` / `AZURE_KEYVAULT_NAME`) via
  `provider.AzureKeyVaultScope(vault)`, or the App Configuration store name
  (`--store-name` / `AZURE_APPCONFIG_NAME`) via
  `provider.AzureAppConfigScope(store)`. Each is a globally-unique name that
  fully identifies the resource, so no subscription/resource group is needed.

Staging is available on AWS, Google Cloud, and Azure. Each scope keys its
on-disk staging state (`provider.Scope.Key`, `scope.go:44`), partitioning staged
changes per scope under `~/.suve/staging/<scope key>/`.

## SDK-confinement boundary

`internal/architecture_test.go` fails the build if any non-test package outside a
provider adapter imports a cloud service SDK directly. depguard
(`.golangci.yaml`) enforces the same at lint time.

| SDK (confined) | Allowed only in |
|----------------|-----------------|
| `aws-sdk-go-v2/service/ssm`, `.../secretsmanager` | `internal/provider/aws/**` |
| `cloud.google.com/go/secretmanager` | `internal/provider/gcloud/**` |
| `github.com/Azure/azure-sdk-for-go` | `internal/provider/azure/**` |

`internal/provider/aws/infra` is the low-level AWS client bootstrap and is an
allowed AWS importer. The `internal/gui` tree is guarded too: it constructs
stores through the registry rather than a cloud SDK.

## Adding another cloud

1. Implement `provider.Reader` / `Writer` / `Tagger` in a new
   `internal/provider/<cloud>/**` adapter (keep every SDK import inside it).
2. Register it (`registry.Register(provider.Provider<Cloud>, ...)`) at the
   composition point in `client.go`.
3. Add its version-spec parser under `internal/version/`.
4. Add its detection signal in `internal/provider/detect/detect.go` and wire a
   command group in `internal/cli/commands/<cloud>/` that builds the provider's
   `provider.Scope`.

Everything downstream — the generic command presenters and version resolution —
is provider-neutral and reused unchanged.
