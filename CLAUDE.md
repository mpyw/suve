# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**suve** (**S**ecret **U**nified **V**ersioning **E**xplorer) is a Git-like CLI for multi-cloud secret and parameter management, covering AWS (Parameter Store + Secrets Manager), Google Cloud (Secret Manager), and Azure (Key Vault + App Configuration). It provides familiar Git-style commands (`show`, `log`, `diff`, `list`, `tag`, `stash`) with version specification syntax (`#VERSION`, `~SHIFT`, `:LABEL`).

### Core Concepts

1. **Git-like Commands**: Commands mirror Git behavior for familiarity
   - `show` - Display value with metadata (like `git show`); use `--raw` for piping
   - `log` - Version history (like `git log`)
   - `diff` - Compare versions (like `git diff`)
   - `list` - List parameters/secrets (aliased as `ls`)
   - `tag` - Add or update tags on a resource
   - `untag` - Remove tags from a resource

2. **Staging Commands**: Git-like staging workflow for batch operations
   - `stage add` - Stage a new parameter/secret for creation
   - `stage edit` - Stage modifications to existing resources
   - `stage delete` - Stage resources for deletion
   - `stage status` - Show staged changes
   - `stage diff` - Show diff of staged changes vs AWS
   - `stage apply` - Apply all staged changes to AWS
   - `stage reset` - Unstage changes

3. **Stash Commands**: Save/restore staging state to file
   - `stage stash push` - Save staged changes to encrypted file
   - `stage stash pop` - Restore staged changes from file (deletes file)
   - `stage stash pop --keep` - Restore staged changes (keeps file)
   - `stage stash show` - Preview stashed changes
   - `stage stash drop` - Delete stash file
   - Mode flags (mutually exclusive):
     - `--merge` - Combine with existing data (default)
     - `--overwrite` - Replace existing data
   - `--yes` - Confirm without prompt

4. **Version Specification**: Git-like revision syntax
   ```
   # SSM Parameter Store
   <name>[#VERSION][~SHIFT]*
   where ~SHIFT = ~ | ~N  (repeatable, cumulative)

   /my/param           # Latest
   /my/param#3         # Version 3
   /my/param~1         # 1 version ago (like HEAD~1)
   /my/param#5~2       # Version 5, then 2 back = Version 3
   /my/param~~         # 2 versions ago (same as ~1~1)

   # Secrets Manager
   <name>[#VERSION | :LABEL][~SHIFT]*
   where ~SHIFT = ~ | ~N  (repeatable, cumulative)

   my-secret              # Current version
   my-secret#abc123       # Specific version ID
   my-secret:AWSCURRENT   # Staging label
   my-secret:AWSCURRENT~1 # 1 version before AWSCURRENT
   user@example.com~1     # @ in name is allowed
   ```

5. **Command Groups**:
   - `param` (aliases: `ssm`, `ps`) - AWS Systems Manager Parameter Store
   - `secret` (aliases: `sm`) - AWS Secrets Manager
   - `gcloud` - Google Cloud (`secret` = Secret Manager)
   - `azure` - Azure (`secret` = Key Vault, `param` = App Configuration)
   - `stage` (alias: `stg`) - Staging operations (AWS + Google Cloud + Azure)

## Architecture

```
suve/
├── cmd/suve/main.go              # Entry point
│
├── internal/
│   ├── cli/
│   │   ├── colors/               # ANSI color codes
│   │   ├── confirm/              # User confirmation prompts
│   │   ├── diffargs/             # Diff argument parsing
│   │   ├── editor/               # External editor integration ($EDITOR)
│   │   ├── output/               # Output formatting (diff, colors)
│   │   ├── pager/                # Pager integration ($PAGER)
│   │   ├── passphrase/           # Passphrase input (for stash encryption)
│   │   ├── terminal/             # Terminal utilities (TTY detection)
│   │   └── commands/
│   │       ├── app.go            # urfave/cli v3 app definition
│   │       ├── generic/          # provider-neutral command scaffold (show/diff/list/log/tag) + per-provider presenters
│   │       ├── internal/         # registry composition + provider/scope wiring for commands
│   │       ├── param/            # AWS param subcommands
│   │       ├── secret/           # AWS secret subcommands
│   │       ├── gcloud/           # Google Cloud command group (secret)
│   │       ├── azure/            # Azure command group (secret=Key Vault, param=App Config)
│   │       └── stage/            # staging subcommands
│   │           ├── command.go    # stage command group definition
│   │           ├── apply/        # apply staged changes
│   │           ├── diff/         # diff staged vs AWS
│   │           ├── param/        # param-specific staging
│   │           ├── reset/        # unstage changes
│   │           ├── secret/       # secret-specific staging
│   │           └── status/       # show staged changes
│   │
│   ├── domain/                   # Neutral model (Entry, Version, Tag, ValueType, Field) shared across providers
│   │
│   ├── provider/                 # Provider seam: Reader/Writer/Tagger/Store interfaces, Registry, Scope, errors
│   │   ├── aws/                  # AWS adapter (SSM + Secrets Manager); AWS SDK confined here
│   │   ├── gcloud/                  # Google Cloud Secret Manager adapter
│   │   ├── azure/                # Azure Key Vault + App Configuration adapters
│   │   └── providermock/         # In-memory provider mock for tests
│   │
│   ├── gui/                      # GUI application (Wails + Svelte)
│   │   ├── app.go                # Wails app definition
│   │   ├── param.go              # Param operations for GUI
│   │   ├── secret.go             # Secret operations for GUI
│   │   ├── staging.go            # Staging operations for GUI
│   │   └── frontend/             # Svelte frontend
│   │       ├── src/              # Svelte components
│   │       └── tests/            # Playwright tests
│   │
│   ├── infra/                    # AWS client initialization (SDK confinement boundary w/ provider/aws)
│   │
│   ├── jsonutil/                 # JSON formatting utilities
│   │
│   ├── maputil/                  # Generic map utilities (Set type)
│   │
│   ├── parallel/                 # Parallel execution utilities
│   │
│   ├── timeutil/                 # Time utilities (timezone handling)
│   │
│   ├── updatecheck/              # Non-blocking update-check notification (#209)
│   │
│   ├── staging/                  # Staging core functionality (AWS + Google Cloud + Azure)
│   │   ├── cli/                  # Staging CLI wrappers (service-specific)
│   │   ├── transition/           # Reducer-based state machine (state/action/reducer/executor)
│   │   └── store/                # Storage backend
│   │       ├── store.go          # Storage interfaces
│   │       ├── file/             # File-based storage (ONLY backend); scope-keyed split param.json/secret.json + stash.json
│   │       │   └── internal/
│   │       │       ├── crypt/        # Argon2 + AES-GCM (stash, passphrase) and raw-key (working) encryption
│   │       │       └── keyprovider/  # OS-keychain data-key provider (zalando/go-keyring); SUVE_STAGING_KEY override
│   │       └── testutil/         # Mock store for testing
│   │
│   ├── usecase/                  # Business logic layer
│   │   ├── param/                # AWS SSM use cases
│   │   ├── secret/               # AWS SM use cases
│   │   ├── staging/              # Staging use cases
│   │   ├── gcloud/                  # Google Cloud use cases
│   │   └── azure/                # Azure use cases
│   │
│   ├── version/                  # Version specification parsing
│   │   ├── parse.go              # Shared generic spec parsing
│   │   ├── shift.go              # Shift (~N) handling
│   │   ├── internal/             # Shared utilities (char checks)
│   │   ├── paramversion/         # AWS SSM version spec parser
│   │   ├── secretversion/        # AWS Secrets Manager version spec parser
│   │   ├── gcloudversion/           # Google Cloud integer-version parser
│   │   ├── azurekvversion/       # Azure Key Vault opaque-id parser
│   │   └── azureappconfigversion/ # Azure App Config (rejects specifiers; unversioned)
│   │
│   └── architecture_test.go      # Arch-guard: forbids cloud SDKs outside their provider/{aws,gcloud,azure} + infra
│
├── e2e/                          # E2E tests (requires localstack)
│
├── .github/workflows/
│   └── test.yml                  # CI: test + lint on push/PR
│
└── mise.toml                     # toolchain + tasks: build-cli/build-gui, test, lint, e2e(+ -gcloud/-azure-appconfig/-azure-keyvault), generate-gui-bindings, coverage/coverage-all, clean, bash (run via `mise <task>` / `mise run <task>`)
```

### Key Design Patterns

1. **Unified generic commands**: Commands (show, diff, list, log, tag) share a provider-neutral scaffold in `internal/cli/commands/generic/**` with per-provider presenters; AWS `param`/`secret` still register their own command groups.
2. **Provider seam**: Core interfaces (`Reader`/`Writer`/`Tagger`/`Store`) live in `internal/provider` and are mocked via `internal/provider/providermock` for testing.
3. **Version resolution**: `paramversion` and `secretversion` (plus `gcloudversion`, `azurekvversion`, `azureappconfigversion`) handle version/shift/label resolution per provider.
4. **Output abstraction**: Commands write to `io.Writer` for testability
5. **Staging state machine**: `staging/transition` implements a reducer-based state machine for staging operations
6. **Keychain-encrypted file store**: Staging is a keychain-encrypted file store, scope-keyed under `~/.suve/staging/{scope.Key()}/`

## Development Commands

```bash
# Run tests
mise test

# Run linter
mise lint

# Build CLI
mise build-cli

# E2E tests (each task starts its emulator automatically via docker compose)
mise e2e                  # AWS (localstack)
mise e2e-gcloud           # Google Cloud
mise e2e-azure-appconfig  # Azure App Configuration
mise e2e-azure-keyvault   # Azure Key Vault

# Dev verification shell: start emulators + inject their env for the chosen
# cloud(s), then open a shell (any combination of flags; 0 = plain shell).
mise run bash --aws --gcloud --azure

# Coverage
mise coverage

# GUI (requires Wails + Node.js)
mise build-gui             # Build the CLI+GUI binary (bin/suve) with the frontend embedded
mise generate-gui-bindings # Regenerate the GUI wailsjs bindings
(cd gui && wails dev)      # GUI hot-reload dev server
```

## Testing Strategy

- **Unit tests**: Each command package has `*_test.go` with provider-neutral mocking via `internal/provider/providermock`
- **E2E tests**: `e2e/e2e_test.go` runs against localstack (SSM only, SM requires Pro)
- **GUI tests**: `internal/gui/frontend/tests/` uses Playwright for component/integration testing
- **Test dependencies**: Uses `github.com/samber/lo` for pointer helpers and `github.com/stretchr/testify` for assertions

### Running E2E Tests

```bash
# Run E2E tests (starts the AWS emulator, localstack, automatically)
mise e2e

# Or with a custom port
SUVE_LOCALSTACK_EXTERNAL_PORT=4599 mise e2e

# Stop the emulator containers when done
docker compose down   # or: mise run clean
```

### Running GUI Tests

```bash
cd internal/gui/frontend
npm install
npm run test        # Run Playwright tests
npm run test:ui     # Run with UI mode
```

## Code Style

- Follow standard Go conventions
- Use `urfave/cli/v3` for CLI structure
- Commands write to `io.Writer`, not directly to stdout
- Error messages should be user-friendly

## Refactoring Guidelines

1. **Tests must pass**: Run `mise test` after changes
2. **Lint must pass**: Run `mise lint` after changes
3. **E2E tests**: Run `mise e2e` for command behavior changes (optional, requires Docker)
