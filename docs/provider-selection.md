# Provider selection

suve is built around a provider seam: the CLI and staging layers talk to a
provider-neutral `provider.Store` (Reader / Writer / Tagger) instead of a cloud
SDK. A `provider.Registry` maps a `provider.Scope` to the concrete backend that
serves it. This document describes how a provider is selected.

## Selection is by top-level command group

There is **no `--provider` flag**. The provider is chosen by which top-level
command group you invoke; each group builds a provider-specific `provider.Scope`
and reuses the *same* generic commands and the *same* registry:

```
suve param  ...          # AWS Systems Manager Parameter Store  (provider.AWSScope)
suve secret ...          # AWS Secrets Manager                  (provider.AWSScope)
suve stage  ...          # staging for the AWS services above
suve gcloud secret ...   # Google Cloud Secret Manager          (provider.GoogleCloudScope)
suve azure  secret ...   # Azure Key Vault                      (provider.AzureKeyVaultScope)
suve azure  param  ...   # Azure App Configuration              (provider.AzureAppConfigScope)
```

The registry is built once with all backends registered
(`internal/cli/commands/internal/client.go`):

```go
reg := aws.NewRegistry()
gcloud.Register(reg)
azure.Register(reg)
```

Each command group resolves its store through this shared registry:

1. It builds the provider-specific `provider.Scope`:
   - **AWS** — from the current AWS identity (`infra.GetAWSIdentity` →
     `provider.AWSScope(accountID, region)`). Read/write commands only need
     `provider.Scope{Provider: provider.ProviderAWS}` (the region comes from the
     ambient AWS config), so they do not pay for an STS call; the account/region
     identity is resolved separately, only where staging state must be keyed.
   - **Google Cloud** — the project id from `--project` or `GOOGLE_CLOUD_PROJECT`
     (`provider.GoogleCloudScope(project)`).
   - **Azure** — the Key Vault name (`--vault-name` / `AZURE_KEYVAULT_NAME`) or
     App Configuration store name (`--store-name` / `AZURE_APPCONFIG_NAME`); each
     is a globally-unique name that fully identifies the resource, so no
     subscription/resource group is needed
     (`provider.AzureKeyVaultScope(vault)` / `provider.AzureAppConfigScope(store)`).
2. It asks the registry for the store it needs:
   `registry.Store(ctx, scope, provider.KindParam)` (or `KindSecret`).

The returned `provider.Store` is handed to the generic show / diff / list / log
/ tag / untag commands and the create / update / delete / restore commands. No
command or usecase constructs a cloud SDK client or adapter directly.

Staging is AWS-only. The AWS scope keys on-disk staging state (see
`provider.Scope.Key`), so staged changes are partitioned per account/region
under `~/.suve/staging/aws/<account>/<region>/`.

## Enforced boundary

An architecture test (`internal/architecture_test.go`) fails the build if any
non-test package under `internal/cli`, `internal/usecase`, `internal/staging`,
or `internal/gui` imports a cloud service SDK directly. Each SDK is confined to
its own adapter:

| SDK (banned outside its adapter)                       | Allowed only in            |
| ----------------------------------------------------- | -------------------------- |
| `aws-sdk-go-v2/service/ssm`, `.../secretsmanager`     | `internal/provider/aws/**` |
| `cloud.google.com/go/secretmanager`                   | `internal/provider/gcloud/**` |
| `github.com/Azure/azure-sdk-for-go`                   | `internal/provider/azure/**` |

`internal/infra` is a low-level allowed importer for AWS client bootstrapping
and is not under the guarded roots. The `internal/gui` tree is guarded too: it
constructs stores through the provider registry rather than talking to a cloud
SDK. depguard (`.golangci.yaml`) enforces the same confinement at lint time.

## Adding another cloud

A new provider is another top-level command group plus a registered factory:
implement the `provider.Reader` / `Writer` / `Tagger` interfaces in a new
`internal/provider/<cloud>/**` adapter, register it
(`registry.Register(provider.Provider<Cloud>, ...)`), add its version-spec
parser under `internal/version/`, and wire a command group that builds the
provider's `provider.Scope`. Everything downstream — the generic command
presenters and version resolution — is provider-neutral and reused unchanged.
