---
name: provider-seam-invariants
description: >-
  Load when adding or modifying a provider adapter, or when touching
  internal/provider/** or internal/domain/**. Captures the invariants that keep
  the provider-neutral seam honest: the neutral domain model, opaque version
  refs, the typed write/delete option pattern, interface segregation, and SDK
  confinement.
---

# Provider seam invariants

suve talks to every cloud through a provider-neutral seam. The `internal/domain`
value types and the `internal/provider` interfaces are what the usecase, CLI,
staging, and GUI layers depend on; each cloud's specifics live behind an adapter
under `internal/provider/<cloud>/**`. The rules below bind any change to that
seam.

## 1. No provider-specific fields on `domain.Entry`

`domain.Entry` (`internal/domain/domain.go`) carries only cross-provider
essentials (Name, Value, Type, Version, Description, Tags, Modified). It has no
ARN, no KMS key, no metadata `any` bag, and no `[M]` generic.

Provider-specific, display-only metadata is surfaced through `Entry.Extra`
(`domain.go:108`), a `[]Field` where `Field` (`domain.go:72`) holds a
human-facing `Label` and a pre-formatted string `Value`. It is neutral in shape
(no cloud types, no `any`): adapters populate it, consumers render it verbatim
and never interpret it. Add new provider metadata by appending an `Extra` field
inside the adapter, never by widening `Entry`.

## 2. Version selection is opaque

`provider.VersionRef` (`internal/provider/provider.go:16-29`) is an opaque
reference produced and consumed by the same provider. Its zero value means
latest/current; it exposes no id or staging-label semantics to generic callers.

- Syntactic parsing of `#VERSION` / `~SHIFT` / `:LABEL` lives in the flat
  `internal/version` package: a product grammar value (`version.AWSParameterStore`,
  `version.AWSSecretsManager`, `version.GoogleCloudSecretManager`, `version.AzureKeyVault`,
  `version.AzureAppConfiguration`) built from one of three grammars (numeric,
  opaque, bare). It imports nothing from the CLI.
- Version *resolution* (mapping a parsed spec plus history onto a concrete
  version) lives behind `Reader.Resolve` (`provider.go:77`), inside the adapter.
  Generic code calls `Resolve` and passes the returned `VersionRef` to
  `Reader.Get`; it never interprets version ids or labels.
- The `internal/usecase/{param,secret}` use cases take a name plus the version
  suffix string (`#3`, `:LABEL`, `~2`, or `""`). The caller parses with its
  provider's grammar and rebuilds the suffix with that grammar's `Suffix`. The
  use cases never import `internal/version`.
- Which version is current is the adapter's call: `History` sets
  `domain.Version.Current` on exactly one version (AWS SSM: highest number; AWS
  Secrets Manager: the `AWSCURRENT` label; Google Cloud and Key Vault: newest).
  Consumers read `Current`. They never infer it from `Labels` or from position.

## 3. One typed write/delete-option mechanism

Provider-interpreted options use the sealed marker pattern
(`provider.go:31-71`):

- `WriteOption` is a closed interface satisfied by embedding `WriteOptionMarker`;
  `DeleteOption` by embedding `DeleteOptionMarker` (e.g. `ForceDelete`).
- Consumers (usecases, CLI) build and pass options through **without**
  type-asserting them. The adapter type-switches over the options it understands
  and silently ignores the rest.
- `provider.ForceDelete` is only honored by AWS Secrets Manager, but it stays in
  the neutral package. The GUI and TUI set it for any service whose
  `capability.HasForceDelete` is true, so they never import an adapter package.

This keeps provider-specific options out of the neutral domain model while
staying strongly typed. Do not add an `any` metadata parameter to widen a signature.

## 4. Interface segregation

The contract is split so a provider or command implements only what it needs
(`internal/provider/provider.go`):

| Interface | Methods | Lines |
|-----------|---------|-------|
| `Reader` | `Resolve` + `Get` + `History` + `List` | 102-113 |
| `Writer` | `Create` + `Put` + `Delete` | 116-133 |
| `Tagger` | `Tag` + `Untag` | 136-141 |
| `Store` | `Reader` + `Writer` + `Tagger` | 146-150 |
| `Restorer` (optional) | `Restore` | 153-156 |

`Writer` carries both `Create` and `Put`: `Create` returns a wrapped
`provider.ErrAlreadyExists` and never overwrites, while `Put` is the upsert.
Choose `Create` when a caller must not clobber an existing entry, `Put` when it
should. `Restorer` (soft-delete restore: AWS Secrets Manager, Azure Key Vault)
is an optional capability a provider may add.

## 5. SDK confinement

Each cloud SDK is confined to its own adapter root. `internal/domain` and
`internal/provider` import only `internal/domain` and the standard library — zero
cloud SDK. Enforcement is twofold:

- `internal/architecture_test.go` fails the build if a non-test package outside
  `internal/provider/{aws,gcloud,azure}` imports a banned SDK
  (`aws-sdk-go-v2/service/ssm`, `.../secretsmanager`,
  `cloud.google.com/go/secretmanager`, `github.com/Azure/azure-sdk-for-go`).
- depguard (`.golangci.yaml`) enforces the same confinement at lint time.

Each provider root package (`internal/provider/{aws,gcloud,azure}`) holds its
`Factory` and client bootstrap; AWS config loading and the STS identity lookup
are `aws.LoadConfig` and `aws.LoadIdentity`. Service adapters are subpackages
named after the cloud product (`aws/parameterstore`, `aws/secretsmanager`,
`gcloud/secretmanager`, `azure/appconfig`, `azure/keyvault`). Where an adapter
package shares its name with the SDK package it wraps, alias the SDK import
with an `sdk` suffix (`secretsmanagersdk`) in files that import both.

Vocabulary that the CLI, TUI or GUI imports directly lives in an SDK-free
package under the adapter: `aws/paramtype` and `azure/appconfig/namespaces`.
`TestSDKFreeProviderVocabulary` and the `*-sdk-free-vocabulary` depguard rules
keep them SDK-free.

When adding an adapter, keep every SDK import inside your
`internal/provider/<cloud>/**` root and run `mise lint` to confirm the guard
passes.
