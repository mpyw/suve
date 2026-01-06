# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**suve** (**S**ecret **U**nified **V**ersioning **E**xplorer) is a Git-like CLI for AWS Parameter Store and Secrets Manager. It provides familiar Git-style commands (`show`, `log`, `diff`, `list`, `create`, `update`, `delete`) with version specification syntax (`#VERSION`, `~SHIFT`, `:LABEL`).

### Core Concepts

1. **Git-like Commands**: Commands mirror Git behavior for familiarity
   - `show` - Display value with metadata (like `git show`); use `--raw` for piping
   - `log` - Version history (like `git log`)
   - `diff` - Compare versions (like `git diff`)
   - `list` - List parameters/secrets (aliased as `ls`)
   - `tag` - Add or update tags on a resource
   - `untag` - Remove tags from a resource

2. **Version Specification**: Git-like revision syntax
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

3. **Two Services**:
   - `param` (aliases: `ssm`, `ps`) - AWS Systems Manager Parameter Store
   - `secret` (aliases: `sm`) - AWS Secrets Manager

## Architecture

```
suve/
├── cmd/suve/main.go           # Entry point
│
├── internal/
│   ├── cli/
│   │   ├── commands/
│   │   │   ├── app.go         # urfave/cli v3 app definition
│   │   │   ├── param/         # param subcommands (create, delete, diff, log, ls, show, tag, untag, update)
│   │   │   ├── secret/        # secret subcommands (create, delete, diff, log, ls, restore, show, tag, untag, update)
│   │   │   └── stage/         # staging subcommands
│   │   └── ...
│   │
│   ├── api/
│   │   ├── paramapi/          # SSM API interface (for testing)
│   │   └── secretapi/         # SM API interface (for testing)
│   │
│   ├── version/
│   │   ├── internal/          # Shared utilities (char checks)
│   │   ├── paramversion/      # SSM version spec parser (#VERSION, ~SHIFT)
│   │   └── secretversion/     # SM version spec parser (#VERSION, :LABEL, ~SHIFT)
│   │
│   ├── staging/               # Staging functionality
│   ├── usecase/               # Business logic layer
│   │   ├── param/             # SSM use cases (show, log, list, create, update, delete, tag)
│   │   ├── secret/            # SM use cases (show, log, list, create, update, delete, restore, tag)
│   │   └── staging/           # Staging use cases (add, edit, delete, status, diff, apply, reset)
│   ├── maputil/               # Generic map utilities (Set type)
│   ├── tagging/               # Tag operations (add/remove tags)
│   ├── output/                # Output formatting (diff, colors)
│   ├── jsonutil/              # JSON formatting
│   └── infra/                 # AWS client initialization
│
├── e2e/                       # E2E tests (requires localstack)
│
├── .github/workflows/
│   └── test.yml               # CI: test + lint on push/PR
│
└── Makefile                   # build, test, lint, e2e, up, down
```

### Key Design Patterns

1. **Subcommand packages**: Each command (show, etc.) is its own package under `internal/cli/commands/{param,secret}/`
2. **Interface-based testing**: API interfaces in `internal/api/` enable mock testing
3. **Version resolution**: `paramversion` and `secretversion` handle version/shift/label resolution
4. **Output abstraction**: Commands write to `io.Writer` for testability

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
```

## Testing Strategy

- **Unit tests**: Each command package has `*_test.go` with mock AWS clients
- **E2E tests**: `e2e/e2e_test.go` runs against localstack (SSM only, SM requires Pro)
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

## Code Style

- Follow standard Go conventions
- Use `urfave/cli/v3` for CLI structure
- Commands write to `io.Writer`, not directly to stdout
- Error messages should be user-friendly

## Refactoring Guidelines

1. **Tests must pass**: Run `make test` after changes
2. **Lint must pass**: Run `make lint` after changes
3. **E2E tests**: Run `make e2e` for command behavior changes (optional, requires Docker)
