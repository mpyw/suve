# cli/commands

## Scope

```yaml
path: internal/cli/commands
type: integration
parent: ../CLAUDE.md
children:
  - param/CLAUDE.md
  - secret/CLAUDE.md
  - stage/CLAUDE.md
```

## Overview

CLI command definitions using urfave/cli v3. Contains app.go (root command) and service-specific subcommand packages (param, secret, stage).

## Architecture

```yaml
key_types:
  - name: App (app.go)
    role: Root CLI application definition

structure:
  app.go:         # NewApp(), root command with subcommands
  param/:         # param subcommands (show, log, list, create, etc.)
  secret/:        # secret subcommands (show, log, list, create, etc.)
  stage/:         # staging subcommands (add, edit, status, apply, etc.)
    - agent/      # daemon start/stop commands
    - param/      # param-specific staging
    - secret/     # secret-specific staging

dependencies:
  internal:
    - internal/cli/*           # CLI utilities
    - internal/usecase/*       # Business logic
    - internal/staging/cli     # Staging command builders
  external:
    - github.com/urfave/cli/v3
```

## Testing Strategy

```yaml
coverage_target: 70%
mock_strategy: |
  - Mock usecase layer
  - Capture Stdout/Stderr for output verification
focus_areas:
  - Command wiring and flag parsing
  - Error message formatting
skip_areas:
  - urfave/cli framework behavior
  - Usecase logic (tested separately)
```

## Notes

### Command Hierarchy

```
suve
├── param
│   ├── show, log, list, diff
│   ├── create, update, delete
│   └── tag, untag
├── secret
│   ├── show, log, list, diff
│   ├── create, update, delete, restore
│   └── tag, untag
└── stage
    ├── add, edit, delete
    ├── status, diff
    ├── apply, reset
    ├── stash (push, pop, apply, show, drop)
    ├── agent (start, stop)
    ├── param (add, edit, delete, status, diff, apply, reset, stash)
    └── secret (add, edit, delete, status, diff, apply, reset, stash)
```

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../staging/cli/CLAUDE.md
```
