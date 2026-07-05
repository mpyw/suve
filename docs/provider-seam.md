# Provider Seam Design

Status: design (issue #199) — **historical**. This is the original design note
for the seam. It has since shipped and evolved: the AWS/GoogleCloud/Azure adapters, the
usecase/CLI migration, and multi-cloud support all landed. Notably, the
`internal/api/paramapi` / `internal/api/secretapi` wrappers referenced below
were **removed** — the AWS adapter now imports the AWS SDK directly (confined to
`internal/provider/aws/**`). See `docs/provider-selection.md` for the current
architecture.

Scope of this issue: the provider-neutral **seam** — the `domain` value types and
the `provider` interfaces/registry — plus this document. No AWS adapters, no
usecase/CLI migration, no additional cloud providers.

## Motivation

suve began as an AWS-only tool. To support multiple cloud backends (AWS SSM,
AWS Secrets Manager, and later GoogleCloud/Azure) we need a **seam**: a small set of
provider-neutral types and interfaces that the usecase and CLI layers depend
on, with each cloud's specifics hidden behind an adapter.

An earlier attempt lives on the `multicloud` branch. It got the broad shape
right (interface segregation, a generic model, adapters) but leaked AWS
concepts into the generic layer and hedged its type decisions. This design
takes the lessons from that branch and commits to a single, clean shape.

The rest of this document is organized around the concrete flaws that branch
exhibited, so that each design decision here is traceable to a real problem it
prevents.

## Hard requirements (and the multicloud-branch flaw each prevents)

### 1. No AWS fields in generic result types

**Branch flaw.** In `internal/model/secret.go` the branch put a top-level
`ARN string` field directly on the generic result types `SecretWriteResult`,
`SecretDeleteResult`, and `SecretRestoreResult`. `ARN` is an Amazon Resource
Name — a concept that only exists on AWS. Baking it into a "provider-agnostic"
result type means every non-AWS provider must either fake an ARN or leave it
blank, and every consumer is tempted to read `.ARN` and thereby couple itself
to AWS.

**How this design addresses it.** The generic retrieved type is
`domain.Entry`, and it has **no `ARN` field** and no other AWS-only field. The
provider write path returns a `domain.Version` (id/label/created) rather than
an AWS-shaped result struct — there is no generic `WriteResult`/`DeleteResult`/
`RestoreResult` carrying an ARN at all. `Writer.Delete` and `Restorer.Restore`
return only `error`. If a future feature genuinely needs an ARN, it is reached
through the provider-specific path (requirement 3), never through the generic
seam.

### 2. No AWS staging labels in generic interfaces

**Branch flaw.** The branch's reader contract was
`SecretReader.GetSecret(ctx, name, versionID, versionStage)` (see
`internal/provider/secret.go` and the usecase/mocks that mirror it). Both
`versionID` **and** `versionStage` are AWS concepts — `versionStage` is a
Secrets Manager staging label such as `AWSCURRENT`/`AWSPREVIOUS`. The generic
interface therefore hard-coded the AWS version model into its signature, so any
non-AWS provider inherits parameters it cannot honor, and callers must know AWS
label semantics to call a "generic" method.

**How this design addresses it.** Version selection is expressed through an
**opaque** `provider.VersionRef`, produced by the provider and consumed by the
same provider. Generic callers never see a version id or a staging label:

- `Reader.Resolve(ctx, name, spec)` takes a **provider-specific spec string**
  (e.g. `#3~1`, `#abc123`, `:AWSCURRENT`) and returns a `VersionRef`.
- `Reader.Get(ctx, name, ref)` consumes that `VersionRef`.

`VersionRef` deliberately exposes no id/label semantics to generic callers; its
zero value means "latest/current". AWS staging labels live **only** inside the
AWS adapter (where `Resolve` parses `:AWSCURRENT` etc.). The generic seam knows
nothing about them.

### 3. A single typed-metadata mechanism

**Branch flaw.** `internal/model/parameter.go` (and its secret twin) carried
**two** competing mechanisms at once:

- a generic `TypedParameter[M]` with a `ToBase()` method that **erased** the
  `[M]` generic down to a base `Parameter`, and
- a base `Parameter{ Metadata any }` bag plus `AWSMeta()` /
  `TypedMetadata[M]()` helpers that **type-assert** the `any` back to a
  concrete type at the consuming layer.

This is the worst of both worlds: the generic parameter promises type safety,
then `ToBase()` throws it away, and consumers recover provider metadata with
runtime type assertions on an `any`. Two mechanisms, both lossy.

**How this design addresses it.** We choose **one** mechanism, and it is
"neither of those": the generic `domain.Entry` has **no metadata bag** (`any`)
and **no `[M]` generic**. It carries only the handful of fields that are
essential across every provider, each as a first-class typed field (Name,
Value, Type, Version, Description, Tags, Modified).

Provider-specific metadata (ARN, KMS key, SSM tier, allowed-pattern, Azure
label/etag, …) is deliberately **out of the generic seam**. It is deferred to
issue #210 (per-provider options), where it will be handled through a **typed,
provider-specific path** — a caller that wants AWS metadata will talk to an
AWS-typed API and receive an AWS-typed value, with no `any` erasure at any
consuming layer. We accept that the generic layer simply cannot express
provider-specific metadata; that is the point of a seam. This keeps the generic
types honest (every field is meaningful for every provider) and pushes the
irreducibly-specific data to a place where it can stay strongly typed.

### 4. Base model carries cross-provider essentials

**Branch flaw.** The branch's base `model.Parameter` had **no `Type` field**;
whether a value was a `SecureString` lived only inside `AWSParameterMeta.Type`
(reachable via `AWSMeta()`). So a fundamental, cross-provider distinction —
"is this value secret/encrypted or plaintext?" — was only accessible through an
AWS-specific metadata assertion. A non-AWS provider had nowhere neutral to say
"this is a secret".

**How this design addresses it.** `domain.Entry` has a first-class
`Type domain.ValueType` field. `ValueType` is a neutral enum
(`ValueTypePlaintext`, `ValueTypeSecret`, `ValueTypeList`) that every provider
maps its native types onto (AWS `String`→plaintext, `SecureString`→secret,
`StringList`→list; Secrets Manager→secret). The essential distinction is now a
typed field on the generic type, reachable without any provider assertion.

### 5. Keep interface segregation

**Branch decision worth keeping.** The branch split the contract into
`Reader` / `Writer` / `Tagger` plus optional `Restorer` / `Describer`. That
segregation is correct: not every provider or command needs every capability,
and small interfaces are easy to mock and to implement partially.

**Branch flaw within it.** The branch also shipped **dead adapter code** around
this shape: `WrapTypedParameterReader[M]` was unused, and the
`typedParameterReaderAdapter.ListParameters` it produced simply
`return nil, nil` — a stub that silently returns no data. Wiring generic ↔
typed with an adapter whose methods are stubs defeats the segregation it claims
to provide.

**How this design addresses it.** We keep the segregation with cleaner
contracts:

- `Reader` = `Resolve` + `Get` + `History` + `List`
- `Writer` = `Put` + `Delete`
- `Tagger` = `Tag` + `Untag`
- `Store` = `Reader` + `Writer` + `Tagger` (the full contract for one service)
- optional `Restorer` (soft-delete restore, e.g. Secrets Manager) and
  `Describer` (metadata without value)

We explicitly do **not** carry forward the branch's `TypedXReader` /
`WrapTypedXReader` adapters or any stub method that returns `(nil, nil)`. There
is no generic↔typed bridging adapter in this design because there is no `[M]`
generic to bridge (requirement 3).

### 6. Version-spec parsing stays generic; resolution moves into adapters

**Existing good code to preserve.** `internal/version` already has a generic
`Spec[A any]` (with `paramversion` and `secretversion` supplying the
provider-specific `AbsoluteSpec` type parameter). The *syntactic* parsing of
`#VERSION`, `~SHIFT`, `:LABEL` is genuinely shared and should stay generic.

**How this design addresses it.** Parsing stays in `internal/version`
(`Spec[A]`), unchanged by this issue. What moves is version **resolution** —
turning a parsed spec plus the entry's history into a concrete version. That is
inherently provider-specific (AWS labels, AWS version ids, shift-against-history
rules) and now lives behind `Reader.Resolve`, which returns an opaque
`VersionRef`. Generic code calls `Resolve`; only the adapter knows how a spec
maps onto real versions.

### 7. Provider registry

**Branch flaw / current-code smell.** Commands construct AWS clients directly
via `infra.NewParamClient` / `infra.NewSecretClient` — there are ~19 such call
sites across the CLI (and more across staging/usecase). Every one of those is a
hard-coded dependency on AWS; adding a second provider would mean editing all
of them.

**How this design addresses it.** We define a provider **registry**: a
`Registry` mapping a `Provider` to a `Factory`, where
`Factory.Store(ctx, scope, kind)` builds a `provider.Store` for a given
`Scope` + `Kind` (`param`/`secret`). Commands will ask the registry for a store
instead of calling `infra.NewXClient`. A `Factory` returns `ErrUnsupportedKind`
when a provider does not offer a requested kind (e.g. a provider with secrets
but no param store), and the registry returns `ErrNoFactory` for an
unregistered provider.

Note: the **full** `Scope` (multi-field, used as a key for scope-keyed storage)
is ported in issue #200. This issue defines only the minimal contract the
registry needs (`Provider` + AWS `AccountID`/`Region`), enough to prove the
seam compiles and dispatches.

## Target architecture

```
                 internal/domain/         (ValueType, Version, Tag, TagChange, Entry)
                        ▲   ▲                 provider-neutral value types; ZERO cloud SDK
                        │   │
        ┌───────────────┘   └───────────────┐
        │                                     │
 internal/provider/                    internal/provider/aws/   (issue #201)
   Reader/Writer/Tagger,                  AWS SSM adapter  ─┐
   Store, Restorer, Describer,            AWS SM adapter   ─┤ implement provider.Store,
   VersionRef,                            AWS Factory      ─┘ Resolve parses AWS Spec[A]
   Registry / Factory / Scope                                (labels), talks to paramapi/
        ▲                                                    secretapi behind the seam
        │ (depend on the seam, not on AWS)
        │
 internal/usecase/  ──────────────►  business logic against provider.Store
        ▲                            (migration: issues #202-204)
        │
 internal/cli/commands/  ─────────►  ask the registry for a Store instead of
        ▲                            infra.NewParamClient/NewSecretClient
        │
 internal/infra (registry wiring)    build a Registry, Register(ProviderAWS, awsFactory)
```

Dependency direction: `provider` depends on `domain`; `usecase` and `cli`
depend on `provider` (and `domain`); the AWS adapter depends on both plus the
existing `paramapi`/`secretapi` wrappers. Nothing generic depends on AWS.

### How the AWS adapter (#201) will implement `Reader.Resolve`

The AWS adapter is where all AWS version semantics live. For a call
`Resolve(ctx, name, spec)` it will:

1. **Parse** the `spec` using the existing generic parser for its service —
   `paramversion` (`Spec[paramversion.AbsoluteSpec]`) for SSM, or
   `secretversion` (`Spec[secretversion.AbsoluteSpec]`) for Secrets Manager.
   This is where `:AWSCURRENT` / `:AWSPREVIOUS` labels, `#VERSION`, and
   `~SHIFT` are understood — inside the adapter, not in the seam.
2. **Resolve shifts against history.** If the spec includes `~N` shifts, the
   adapter fetches the entry's version history (the same data behind
   `Reader.History`) and walks back the requested number of versions from the
   selected anchor (a specific version id, a staging label, or latest).
3. **Return an opaque `VersionRef`.** The resolved concrete version id is
   wrapped with `provider.NewVersionRef(id)` (or the zero `VersionRef` for
   latest). `Reader.Get` then consumes that ref. Generic callers never learn
   whether the id came from a label, a version number, or a shift.

## Out of scope

This issue intentionally delivers only the seam and this document. The
following are explicitly **out of scope** here and tracked separately:

- **AWS adapters** — implementing `provider.Store` for SSM and Secrets Manager,
  including `Resolve`: issue **#201**.
- **Usecase / CLI migration** — porting the usecase layer and CLI commands off
  `infra.NewXClient` and onto the registry + `provider.Store`: issues
  **#202-204**.
- **Per-provider options / provider-specific metadata** — the typed
  provider-specific path for ARNs, KMS keys, SSM tiers, etc.: issue **#210**.
- **Full multi-field `Scope` and scope-keyed storage**: issue **#200**.
- **Additional providers** — GoogleCloud (**#207**) and Azure (**#208**).

No consumers are wired to the new packages in this issue; nothing in the repo
imports `internal/domain` or `internal/provider` except the new packages' own
tests.

## Acceptance criteria

- `internal/domain` and `internal/provider` contain **zero** AWS SDK imports
  (no `aws-sdk-go`, `paramapi`, `secretapi`, `ssm`, `secretsmanager`).
- The seam compiles and passes lint with the repo's `default: all` config.
- The packages have real test coverage proving the interfaces are
  implementable (a `VersionRef` test and a `Registry` test with an in-test fake
  `Factory`), with no AWS code in the tests.
- No existing command/usecase imports the new packages yet.
