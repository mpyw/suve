# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**suve** is a Git-like CLI for AWS Parameter Store and Secrets Manager. It provides familiar Git-style commands (`show`, `log`, `diff`, `cat`, `ls`, `set`, `delete`) with version specification syntax (`#VERSION`, `~SHIFT`, `:LABEL`).

### Core Concepts

1. **Git-like Commands**: Commands mirror Git behavior for familiarity
   - `show` - Display value with metadata (like `git show`)
   - `cat` - Raw value output for piping (like `git cat-file -p`)
   - `log` - Version history (like `git log`)
   - `diff` - Compare versions (like `git diff`)

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
   - `ssm` - AWS Systems Manager Parameter Store
   - `sm` - AWS Secrets Manager

## Architecture

```
suve/
├── cmd/suve/main.go           # Entry point
│
├── internal/
│   ├── cli/
│   │   ├── app.go             # urfave/cli v2 app definition
│   │   ├── ssm/               # SSM subcommands (cat, delete, diff, log, ls, set, show)
│   │   └── sm/                # SM subcommands (cat, create, delete, diff, log, ls, restore, set, show)
│   │
│   ├── api/
│   │   ├── ssmapi/            # SSM API interface (for testing)
│   │   └── smapi/             # SM API interface (for testing)
│   │
│   ├── version/
│   │   ├── internal/          # Shared utilities (char checks)
│   │   ├── shift/             # Shift parser (~SHIFT)
│   │   ├── ssmversion/        # SSM version spec parser (#VERSION, ~SHIFT)
│   │   └── smversion/         # SM version spec parser (#VERSION, :LABEL, ~SHIFT)
│   │
│   ├── diff/                  # Diff argument parsing
│   ├── output/                # Output formatting (diff, colors)
│   ├── jsonutil/              # JSON formatting
│   └── awsutil/               # AWS client initialization
│
├── e2e/                       # E2E tests (requires localstack)
│
├── .github/workflows/
│   └── test.yml               # CI: test + lint on push/PR
│
└── Makefile                   # build, test, lint, e2e, up, down
```

### Key Design Patterns

1. **Subcommand packages**: Each command (cat, show, etc.) is its own package under `internal/cli/{ssm,sm}/`
2. **Interface-based testing**: API interfaces in `internal/api/` enable mock testing
3. **Version resolution**: `ssmversion` and `smversion` handle version/shift/label resolution
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
make e2e     # Run SSM E2E tests
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
- Use `urfave/cli/v2` for CLI structure
- Commands write to `io.Writer`, not directly to stdout
- Error messages should be user-friendly

## Refactoring Guidelines

1. **Tests must pass**: Run `make test` after changes
2. **Lint must pass**: Run `make lint` after changes
3. **E2E tests**: Run `make e2e` for command behavior changes (optional, requires Docker)
