# staging/cli

## Scope

```yaml
path: internal/staging/cli
type: package
parent: ../CLAUDE.md
```

## Overview

Shared runners and command builders for staging subcommands. Provides reusable Runner structs and command factory functions that are composed into service-specific commands (param/secret) in `cli/commands/stage/`.

## Architecture

```yaml
key_types:
  - name: CommandConfig
    role: Configuration for building service-specific commands
  - name: "*Runner"
    role: Executor structs (AddRunner, EditRunner, ApplyRunner, etc.)
  - name: "*Options"
    role: Input options for each runner

files:
  command_builders:
    - command.go      # CommandConfig, shared factory helpers
    - add.go          # AddRunner, newAddCommand
    - edit.go         # EditRunner, newEditCommand
    - delete.go       # DeleteRunner, newDeleteCommand
    - status.go       # StatusRunner, newStatusCommand
    - diff.go         # DiffRunner, newDiffCommand
    - apply.go        # ApplyRunner, newApplyCommand
    - reset.go        # ResetRunner, newResetCommand
    - tag.go          # TagRunner, UntagRunner
  stash_commands:
    - stash.go        # newStashCommand group builder
    - stash_push.go   # StashPushRunner
    - stash_pop.go    # StashPopRunner
    - stash_show.go   # StashShowRunner
    - stash_drop.go   # StashDropRunner

dependencies:
  internal:
    - internal/usecase/staging
    - internal/staging/store/agent
    - internal/staging/store/file
    - internal/cli/output
    - internal/cli/confirm
    - internal/cli/editor
  external:
    - github.com/urfave/cli/v3
```

## Testing Strategy

```yaml
coverage_target: 75%
mock_strategy: |
  - Mock usecase layer via dependency injection
  - Mock store interfaces for stash commands
  - Use bytes.Buffer for Stdout/Stderr capture
focus_areas:
  - Runner.Run() logic and error handling
  - Flag parsing and option mapping
  - Service-specific vs global command behavior
skip_areas:
  - urfave/cli framework behavior
  - Usecase layer logic (tested separately)
```

## Notes

### Command Composition Pattern

Runners are service-agnostic. Service-specific commands are created by passing `CommandConfig` with appropriate `ParserFactory`:

```go
// In cli/commands/stage/param/command.go
cfg := cli.CommandConfig{
    ParserFactory: paramversion.NewParser,
    ItemName:      "parameter",
}
cmd := cli.NewAddCommand(cfg)
```

### Stash Commands

- `stash` (default action = push)
- `stash push` - agent memory -> file
- `stash pop` - file -> agent memory (deletes file)
- `stash pop --keep` - file -> agent memory (keeps file)
- `stash show` - preview file contents
- `stash drop` - delete file

### Stash Mode Flags

Both `stash push` and `stash pop` support mutually exclusive mode flags:
- `--merge` - Combine source data with existing destination data (default)
- `--overwrite` - Replace destination data with source data
- `--yes` - Confirm without prompt (implies merge mode)

These flags use urfave/cli v3's `MutuallyExclusiveFlags` to ensure only one can be specified.

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../cli/commands/stage/CLAUDE.md
```
