# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**suve** (**S**ecret **U**nified **V**ersioning **E**xplorer) is a Git-like CLI (and Wails/Svelte GUI) for multi-cloud secret and parameter management across AWS (Parameter Store + Secrets Manager), Google Cloud (Secret Manager), and Azure (Key Vault + App Configuration). It offers Git-style commands (`show`, `log`, `diff`, `list`, `tag`, `untag`) with revision syntax (`#VERSION`, `~SHIFT`, `:LABEL`) and a staging workflow (`stage add|edit|delete|status|diff|apply|reset`, alias `stg`) for batched writes.

## Where things live

- **Domain model** — `internal/domain`: the provider-neutral types (`Entry` with `Extra []Field`, `Version`, `Tag`, `ValueType`, `Field`) that every layer speaks.
- **Provider seam** — `internal/provider`: the `Reader`/`Writer`/`Tagger`/`Store` interfaces, `Registry`, `Scope`, opaque `VersionRef`, and errors. Provider selection lives in `internal/provider/detect` (env-detected flat aliases) + `registry.go`.
- **Provider adapters** — `internal/provider/{aws,gcloud,azure}`: each cloud's SDK-backed implementation. AWS client init sits at `internal/provider/aws/infra`.
- **Use cases** — `internal/usecase/{param,secret,staging,gcloud,azure}`: business logic. `param`/`secret` are the neutral service-axis core (the GUI consumes them for every provider); `gcloud`/`azure` add provider-specific presentation.
- **CLI commands** — `internal/cli/commands/{aws,gcloud,azure}` for provider groups plus the provider-neutral scaffold under `internal/cli/commands/generic` (show/diff/list/log/tag); wiring in `internal/cli/commands/app.go` and `internal/cli/commands/internal`.
- **Staging** — `internal/staging`: the reducer-based transition state machine (`transition/`) over a keychain-encrypted file store (`store/file`, the only backend).
- **Version parsers** — `internal/version/*` (`awsparamversion`, `awssecretversion`, `gcloudversion`, `azurekvversion`, `azureappconfigversion`) resolve version/shift/label per provider.
- **GUI** — `internal/gui` (Wails app) + `internal/gui/frontend` (Svelte + Playwright).

## Non-obvious invariants

- **SDK confinement**: only `internal/provider/<cloud>/**` may import that cloud's SDK. Enforced by `internal/architecture_test.go` and the depguard linter (`.golangci.yaml`).
- **Interface segregation**: `provider.Store = Reader + Writer + Tagger`. Versions are opaque `provider.VersionRef` values produced by an adapter and passed back to it — never parsed by callers.
- **io.Writer everywhere**: commands write to an injected `io.Writer`, never directly to stdout, so output is testable.
- **Staging store**: keychain-encrypted (zalando/go-keyring), scope-keyed under `~/.suve/staging/{scope.Key()}/` with per-service `param.json`/`secret.json`; `SUVE_STAGING_KEY` overrides the keychain data key (used in CI/tests).
- **Export/import**: each snapshot is a plaintext JSON envelope `{version, provider, scope, service, payload}` whose `payload` is Argon2id-passphrase-encrypted (or plaintext when the passphrase is empty); the scope is embedded and validated on import. `export` flags: `--keep`, `--yes`/`--force`, `--passphrase-stdin` (no `--merge`/`--overwrite`). `import` flags: `--merge`/`--overwrite` (mutually exclusive), `--yes`, `--passphrase-stdin`, `--allow-scope-mismatch` (no `--keep`).
- **Two naming axes**: provider axis (`aws`/`gcloud`/`azure`) × service axis (`param`/`secret`/`stage`). Service-axis names are shared by all providers and are never provider-marked; only the provider axis carries a marker. Flat `param`/`secret`/`stage` CLI commands are env-detected aliases that may resolve to any active provider.

## Development commands

```bash
mise test                  # unit tests
mise lint                  # golangci-lint (+ deadcode gate)
mise build-cli             # build bin/suve (CLI)
mise build-gui             # build bin/suve with the GUI frontend embedded
mise generate-gui-bindings # regenerate Wails bindings (rebuild GUI afterward)
mise e2e-aws               # e2e against localstack (AWS param/secret/staging)
mise e2e-gcloud            # e2e against the Google Cloud emulator
mise e2e-azure-appconfig   # e2e against Azure App Configuration
mise e2e-azure-keyvault    # e2e against Azure Key Vault
mise coverage              # unit coverage
mise coverage-e2e-aws      # AWS e2e coverage
mise coverage-all          # combined coverage
mise clean                 # sweep leftover test containers/volumes

# Dev shell: start emulators + inject env for the chosen cloud(s), then a shell.
mise run bash --aws --gcloud --azure
```

See `mise.toml` for the full task list, including local seeding (`seed-*`, `seed-build`) and demo-recording tasks.

## Testing

- **Unit**: each package has `*_test.go`; provider behavior is mocked via `internal/provider/providermock`.
- **E2E**: `e2e/` runs against emulators (localstack for AWS, plus Google Cloud and Azure), covering each provider's available services (param and/or secret) plus staging; select a suite with the `e2e-*` tasks above.
- **GUI**: `internal/gui/frontend/tests` uses Playwright (`npm run test` / `npm run test:ui` from that directory).
- User-visible behavior is expected at all three layers (Go unit + e2e + GUI Playwright) where applicable.
- Test helpers use `github.com/samber/lo` (pointer helpers) and `github.com/stretchr/testify` (assertions).

## Skills & docs

Load the matching skill under `.claude/skills/` before non-trivial work; each holds the *why* so this file stays lean.

- **provider-seam-invariants** — touching `internal/provider/**` or `internal/domain/**`: neutral model, opaque version refs, typed write/delete options, interface segregation, SDK confinement.
- **provider-selection-and-registry** — wiring a top-level command group, adding a cloud, or touching `registry.go`/`detect/`/`internal/cli/commands/internal/client.go`: how a provider is selected and how the registry composes backends and scopes.
- **staging-state-machine** — changing reducers/executor in `internal/staging/transition/` or reasoning about add/edit/delete/tag transitions, auto-skip/unstage, tag cascade, or conflict detection.
- **suve-cli-map** — the per-provider capability, versioning, and alias matrix without reading the full option tables.
- **add-provider-adapter** — adding a cloud adapter (or a new service under one): the `Store` interface, SDK confinement, error mapping, version parsing, CLI/registry wiring, staging.
- **add-e2e-emulator** — wiring an emulator-backed e2e seam: env gate, `compose.test.yaml` service, `mise e2e-<provider>` task, closed-network CI job, coverage upload.
- **gui-change** — changing `internal/gui/**`: bindings-regenerate-then-rebuild rule, capability-driven UI, server-side guards, async-load checklist.
- **refresh-demo-gifs** — re-recording CLI/TUI/GUI demo GIFs: record scripts, robust selectors, output-drift triggers, frame verification, Git LFS.
- **bug-audit-epic** — running a codebase- or subsystem-wide bug audit: parallel investigators, adversarial verification, severity-grouped epic with linked sub-issues.
- **bug-fix-pr** — turning one filed bug into a fix PR: reproduce, fix at the cited site, add regression tests at every layer, run gates.
- **feature-epic-breakdown** — planning a multi-PR feature or cross-cutting refactor as a layer-ordered epic with standalone sub-issues.
- **docs-audit** — keeping docs in sync: sweep command tables/env-vars/paths against the real CLI, current-state-only, consistent terminology, aqua-registry tracking.
- **collection-idioms** — writing/reviewing Go that builds or transforms slices/maps: prefer `samber/lo`, `lo/it`, and stdlib `slices`/`maps` utilities over hand-rolled `make`/`range`/`append`; keep explicit loops only for complex side effects/control flow.

For user-facing reference see `README.md` and `docs/{aws,azure,gcloud}.md`. The staging state machine's authoritative reference is `docs/staging-state-transitions.md`.

## Code style

- Follow standard Go conventions; use `urfave/cli/v3` for CLI structure.
- Commands write to `io.Writer`, not directly to stdout.
- Error messages should be user-friendly.
- Reference docs are current-state-only: no historical/migration language (that belongs in release notes).

## Refactoring gates

1. **Tests pass**: `mise test` after changes.
2. **Lint passes**: `mise lint` after changes.
3. **E2E** (optional, for command-behavior changes; needs Docker): `mise e2e-aws` (and provider-specific `e2e-*` as relevant).
