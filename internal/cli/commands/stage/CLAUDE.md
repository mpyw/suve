# cli/commands/stage

## Scope

```yaml
path: internal/cli/commands/stage
type: group
parent: ../CLAUDE.md
```

## Overview

Staging subcommands for `suve stage`. Composes commands from staging/cli runners with global and service-specific variants.

## Architecture

```yaml
structure:
  command.go:     # Main stage command, global subcommands
  agent/:         # agent start/stop commands
  apply/:         # apply command
  diff/:          # diff command
  reset/:         # reset command
  status/:        # status command
  param/:         # param-specific staging commands
  secret/:        # secret-specific staging commands

command_hierarchy:
  stage:
    - add, edit, delete     # From staging/cli
    - status, diff          # From staging/cli
    - apply, reset          # Wired to dedicated packages
    - stash                 # Group from staging/cli
    - agent                 # start, stop
    - param                 # Service-specific variants
    - secret                # Service-specific variants

dependencies:
  internal:
    - internal/staging/cli    # Shared runners and command builders
    - internal/usecase/staging
  external:
    - github.com/urfave/cli/v3
```

## Testing Strategy

```yaml
coverage_target: 60%
mock_strategy: |
  - Minimal wiring tests
  - Main logic tested in staging/cli and usecase/staging
focus_areas:
  - Command wiring correctness
  - Flag propagation
skip_areas:
  - Business logic (tested in usecase)
  - Runner behavior (tested in staging/cli)
```

## Notes

### Command Composition

Global commands use `staging/cli` directly:
```go
// In command.go
stagingcli.NewGlobalStashCommand()
```

Service-specific commands use CommandConfig:
```go
// In param/command.go
cfg := stagingcli.CommandConfig{
    ParserFactory: paramversion.NewParser,
    ItemName:      "parameter",
}
stagingcli.NewStashCommand(cfg)
```

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../../staging/cli/CLAUDE.md
```
