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
(`domain.go:107`), a `[]Field` where `Field` (`domain.go:71`) holds a
human-facing `Label` and a pre-formatted string `Value`. It is neutral in shape
(no cloud types, no `any`): adapters populate it, consumers render it verbatim
and never interpret it. Add new provider metadata by appending an `Extra` field
inside the adapter, never by widening `Entry`.

## 2. Version selection is opaque

`provider.VersionRef` (`internal/provider/provider.go:16-29`) is an opaque
reference produced and consumed by the same provider. Its zero value means
latest/current; it exposes no id or staging-label semantics to generic callers.

- Syntactic parsing of `#VERSION` / `~SHIFT` / `:LABEL` lives in
  `internal/version/*` (`awsparamversion`, `awssecretversion`, `gcloudversion`,
  `azurekvversion`, `azureappconfigversion`).
- Version *resolution* (mapping a parsed spec plus history onto a concrete
  version) lives behind `Reader.Resolve` (`provider.go:77`), inside the adapter.
  Generic code calls `Resolve` and passes the returned `VersionRef` to
  `Reader.Get`; it never sees version ids or labels.

## 3. One typed write/delete-option mechanism

Provider-interpreted options use the sealed marker pattern
(`provider.go:31-71`):

- `WriteOption` is a closed interface satisfied by embedding `WriteOptionMarker`;
  `DeleteOption` by embedding `DeleteOptionMarker` (e.g. `ForceDelete`).
- Consumers (usecases, CLI) build and pass options through **without**
  type-asserting them. The adapter type-switches over the options it understands
  and silently ignores the rest.

This keeps provider-specific options out of the neutral domain model while
staying strongly typed. Do not add an `any` metadata parameter to widen a signature.

## 4. Interface segregation

The contract is split so a provider or command implements only what it needs
(`internal/provider/provider.go`):

| Interface | Methods | Lines |
|-----------|---------|-------|
| `Reader` | `Resolve` + `Get` + `History` + `List` | 74-85 |
| `Writer` | `Create` + `Put` + `Delete` | 88-105 |
| `Tagger` | `Tag` + `Untag` | 107-113 |
| `Store` | `Reader` + `Writer` + `Tagger` | 118-122 |
| `Restorer` (optional) | `Restore` | 125-128 |
| `Describer` (optional) | `Describe` | 130-134 |

`Writer` carries both `Create` and `Put`: `Create` returns a wrapped
`provider.ErrAlreadyExists` and never overwrites, while `Put` is the upsert.
Choose `Create` when a caller must not clobber an existing entry, `Put` when it
should. `Restorer` (soft-delete restore, e.g. Secrets Manager) and `Describer`
(metadata without value) are optional capabilities a provider may add.

## 5. SDK confinement

Each cloud SDK is confined to its own adapter root. `internal/domain` and
`internal/provider` import only `internal/domain` and the standard library — zero
cloud SDK. Enforcement is twofold:

- `internal/architecture_test.go` fails the build if a non-test package outside
  `internal/provider/{aws,gcloud,azure}` imports a banned SDK
  (`aws-sdk-go-v2/service/ssm`, `.../secretsmanager`,
  `cloud.google.com/go/secretmanager`, `github.com/Azure/azure-sdk-for-go`).
- depguard (`.golangci.yaml`) enforces the same confinement at lint time.

`internal/provider/aws/infra` is the low-level AWS client bootstrap and is the
allowed importer for AWS config/client construction.

When adding an adapter, keep every SDK import inside your
`internal/provider/<cloud>/**` root and run `mise lint` to confirm the guard
passes.
