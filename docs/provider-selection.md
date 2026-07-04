# Provider selection

suve is built around a provider seam: the CLI and staging layers talk to a
provider-neutral `provider.Store` (Reader / Writer / Tagger) instead of a cloud
SDK. A `provider.Registry` maps a `provider.Scope` to the concrete backend that
serves it. This document describes how a provider is selected today and how new
clouds will plug in.

## Today: AWS is the default (and only) provider

The top-level commands are unchanged and always target AWS:

```
suve param  ...   # AWS Systems Manager Parameter Store
suve secret ...   # AWS Secrets Manager
suve stage  ...   # staging for the above
```

There is **no `--provider` flag** — AWS is the only registered backend, so
adding a selection flag now would have nothing to select. Provider resolution
still goes through the registry:

1. The registry is built once with AWS registered
   (`internal/provider/aws.NewRegistry()`), reachable from the command layer
   via `internal/cli/commands/internal`.
2. Each command builds an AWS `provider.Scope` from the current AWS identity
   (`infra.GetAWSIdentity` → `provider.AWSScope(accountID, region)`).
3. It asks the registry for the store it needs:
   `registry.Store(ctx, scope, provider.KindParam)` (or `KindSecret`).

The returned `provider.Store` is handed to the generic show / diff / list / log
/ tag / untag commands, the create / update / delete / restore commands, and the
staging strategies (`staging.NewParamStrategy` / `NewSecretStrategy`). No command
or usecase constructs an AWS SDK client or adapter directly anymore.

The same AWS scope keys on-disk staging state (see `provider.Scope.Key`), so
staged changes are partitioned per account/region.

## Enforced boundary

An architecture test (`internal/architecture_test.go`) fails the build if any
non-test package under `internal/cli`, `internal/usecase`, or `internal/staging`
imports the AWS service SDK (`aws-sdk-go-v2/service/ssm`,
`.../secretsmanager`) or the `internal/api/paramapi` / `internal/api/secretapi`
aliases. The AWS SDK is confined to `internal/provider/aws/**`
(`internal/api` and `internal/infra` are low-level allowed importers).

> Note: `internal/gui/**` is intentionally excluded from this guard for now; its
> migration onto the provider registry is tracked by issue #206. Until then the
> GUI keeps constructing AWS clients directly.

## Future: additional clouds as top-level command groups

New providers will be added as their own top-level command groups that build a
provider-specific `provider.Scope` and reuse the *same* generic commands and the
*same* registry. Sketch:

```
suve gcloud secret ...   # Google Cloud Secret Manager  (provider.GoogleCloudScope)
suve azure  secret ...   # Azure Key Vault              (provider.AzureKeyVaultScope)
suve azure  param  ...   # Azure App Configuration      (provider.AzureAppConfigScope)
```

Each group differs only in how it builds its `provider.Scope` (project id,
subscription/resource-group/vault, …) and in registering its factory
(`registry.Register(provider.ProviderGoogleCloud, …)`). Everything downstream —
version-spec parsing, the generic command presenters, and the staging
strategies — is provider-neutral and is reused unchanged.

`suve param` / `suve secret` remain AWS-only shortcuts and keep their current
behavior. Implementing the GCP and Azure backends is out of scope here (tracked
by #207 and #208); this wiring only makes the selection pluggable.
