# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**suve** (**S**ecret **U**nified **V**ersioning **E**xplorer) is a Git-like CLI for AWS Parameter Store and Secrets Manager. It provides familiar Git-style commands (`show`, `log`, `diff`, `list`, `tag`, `stash`) with version specification syntax (`#VERSION`, `~SHIFT`, `:LABEL`).

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

5. **Two Services**:
   - `param` (aliases: `ssm`, `ps`) - AWS Systems Manager Parameter Store
   - `secret` (aliases: `sm`) - AWS Secrets Manager
   - `stage` (alias: `stg`) - Staging operations

## Architecture

```
suve/
├── cmd/suve/main.go              # Entry point
│
├── internal/
│   ├── api/
│   │   ├── paramapi/             # SSM API interface (for testing)
│   │   └── secretapi/            # SM API interface (for testing)
│   │
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
│   │       ├── param/            # param subcommands
│   │       ├── secret/           # secret subcommands
│   │       └── stage/            # staging subcommands
│   │           ├── agent/        # daemon start/stop commands
│   │           ├── apply/        # apply staged changes
│   │           ├── diff/         # diff staged vs AWS
│   │           ├── reset/        # unstage changes
│   │           ├── status/       # show staged changes
│   │           ├── param/        # param-specific staging
│   │           └── secret/       # secret-specific staging
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
│   ├── infra/                    # AWS client initialization
│   │
│   ├── jsonutil/                 # JSON formatting utilities
│   │
│   ├── maputil/                  # Generic map utilities (Set type)
│   │
│   ├── parallel/                 # Parallel execution utilities
│   │
│   ├── staging/                  # Staging core functionality
│   │   ├── param.go              # Param staging strategy
│   │   ├── secret.go             # Secret staging strategy
│   │   ├── conflict.go           # Conflict detection
│   │   ├── cli/                  # Staging CLI wrappers (service-specific)
│   │   ├── transition/           # State machine for staging operations
│   │   └── store/                # Storage backends
│   │       ├── store.go          # Storage interfaces
│   │       ├── agent/            # In-memory daemon storage
│   │       │   ├── daemon/       # Daemon process (runner, launcher)
│   │       │   │   └── internal/
│   │       │   │       └── ipc/  # Unix socket IPC
│   │       │   └── internal/
│   │       │       ├── client/   # Daemon client
│   │       │       ├── server/   # Request handler
│   │       │       │   └── security/  # Memory protection, peer auth
│   │       │       └── protocol/ # IPC protocol definitions
│   │       ├── file/             # File-based storage (encrypted)
│   │       │   └── internal/
│   │       │       └── crypt/    # Argon2 + AES-GCM encryption
│   │       └── testutil/         # Mock store for testing
│   │
│   ├── tagging/                  # Tag operations (add/remove tags)
│   │
│   ├── timeutil/                 # Time utilities (timezone handling)
│   │
│   ├── usecase/                  # Business logic layer
│   │   ├── param/                # SSM use cases
│   │   ├── secret/               # SM use cases
│   │   └── staging/              # Staging use cases
│   │
│   └── version/                  # Version specification parsing
│       ├── internal/             # Shared utilities (char checks)
│       ├── paramversion/         # SSM version spec parser
│       └── secretversion/        # SM version spec parser
│
├── e2e/                          # E2E tests (requires localstack)
│
├── .github/workflows/
│   └── test.yml                  # CI: test + lint on push/PR
│
└── Makefile                      # build, test, lint, e2e, gui, up, down
```

### Key Design Patterns

1. **Subcommand packages**: Each command (show, etc.) is its own package under `internal/cli/commands/{param,secret}/`
2. **Interface-based testing**: API interfaces in `internal/api/` enable mock testing
3. **Version resolution**: `paramversion` and `secretversion` handle version/shift/label resolution
4. **Output abstraction**: Commands write to `io.Writer` for testability
5. **Staging state machine**: `staging/transition` implements a state machine for staging operations
6. **Daemon architecture**: Staging uses an in-memory daemon process with IPC for performance and data persistence

## Development Commands

```bash
# Run tests
make test

# Run linter
make lint

# Build CLI
make build

# E2E tests with localstack
make up      # Start localstack
make e2e     # Run E2E tests
make down    # Stop localstack

# Coverage
make coverage

# GUI development (requires Wails)
make gui          # Run GUI in dev mode
make gui-build    # Build GUI binary
make gui-test     # Run Playwright tests
```

## Testing Strategy

- **Unit tests**: Each command package has `*_test.go` with mock AWS clients
- **E2E tests**: `e2e/e2e_test.go` runs against localstack (SSM only, SM requires Pro)
- **GUI tests**: `internal/gui/frontend/tests/` uses Playwright for component/integration testing
- **Test dependencies**: Uses `github.com/samber/lo` for pointer helpers and `github.com/stretchr/testify` for assertions

### Running E2E Tests

```bash
# Start localstack (SSM service)
make up

# Run E2E tests
make e2e

# Or with custom port
SUVE_LOCALSTACK_EXTERNAL_PORT=4599 make e2e

# Stop localstack
make down
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

1. **Tests must pass**: Run `make test` after changes
2. **Lint must pass**: Run `make lint` after changes
3. **E2E tests**: Run `make e2e` for command behavior changes (optional, requires Docker)
