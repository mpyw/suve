# staging/store/agent/daemon/lifecycle

## Scope

```yaml
path: internal/staging/store/agent/daemon/lifecycle
type: package
parent: ../CLAUDE.md
```

## Overview

Declarative agent lifecycle management for staging commands. Classifies commands into Write, Read, and File categories, each with different agent lifecycle requirements. Provides type-safe executor functions that enforce correct handling patterns at compile time.

## Architecture

```yaml
key_types:
  - name: WriteCommand
    role: Commands that require agent auto-start (add, edit, delete, tag, untag, etc.)
  - name: ReadCommand
    role: Commands that check agent status first (status, diff, apply, reset, etc.)
  - name: FileCommand
    role: Commands that only access file storage (stash show, stash drop)
  - name: Result[T]
    role: Generic result type for read commands with NothingStaged flag
  - name: Pinger
    role: Interface for checking agent status without starting
  - name: Starter
    role: Interface for ensuring agent is running

files:
  - command.go   # Command type definitions (WriteCommand, ReadCommand, FileCommand)
  - result.go    # Result[T] generic type
  - executor.go  # ExecuteWrite, ExecuteRead, ExecuteFile functions

dependencies:
  internal: []
  external: []
```

## Testing Strategy

```yaml
coverage_target: 100%
mock_strategy: |
  - mockStarter: Returns configurable error from Start()
  - mockPinger: Returns configurable error from Ping()
focus_areas:
  - All command constants are usable
  - Error propagation from Starter/Pinger
  - Action success/failure handling
  - Generic type correctness with complex types
skip_areas: []
```

## Command Categories

### WriteCommand (Auto-Start)

Commands that modify staging state. Agent is auto-started if not running.

| Command | Constant | Description |
|---------|----------|-------------|
| add | `CmdAdd` | Stage new parameter/secret |
| edit | `CmdEdit` | Stage modification to existing resource |
| delete | `CmdDelete` | Stage resource for deletion |
| tag | `CmdTag` | Stage tag addition |
| untag | `CmdUntag` | Stage tag removal |
| reset (version) | `CmdResetVersion` | Stage historical version (reset foo#3) |
| stash pop | `CmdStashPop` | Restore state from file to agent |
| agent start | `CmdAgentStart` | Explicit agent start |

### ReadCommand (Ping Check)

Commands that check agent status first. Returns `Result.NothingStaged=true` if agent not running.

| Command | Constant | Description |
|---------|----------|-------------|
| status | `CmdStatus` | Show staged changes |
| diff | `CmdDiff` | Compare staged vs AWS |
| apply | `CmdApply` | Apply staged changes |
| reset (all) | `CmdResetAll` | Clear all staged changes |
| reset (name) | `CmdReset` | Clear specific entry |
| stash push | `CmdStashPush` | Save agent state to file |
| agent stop | `CmdAgentStop` | Explicit agent stop |

### FileCommand (No Agent)

Commands that only interact with file storage, bypassing the agent entirely.

| Command | Constant | Description |
|---------|----------|-------------|
| stash show | `CmdStashShow` | Preview stashed changes |
| stash drop | `CmdStashDrop` | Delete stash file |

## Executor Functions

### ExecuteWrite[T]

```go
func ExecuteWrite[T any](
    ctx context.Context,
    starter Starter,
    _ WriteCommand,
    action func() (T, error),
) (T, error)
```

- Calls `starter.Start()` to ensure agent is running
- If Start fails, returns zero value and error
- If Start succeeds, executes action and returns result

### ExecuteRead[T]

```go
func ExecuteRead[T any](
    ctx context.Context,
    pinger Pinger,
    _ ReadCommand,
    action func() (T, error),
) (Result[T], error)
```

- Calls `pinger.Ping()` to check if agent is running
- If Ping fails, returns `Result{NothingStaged: true}` with nil error
- If Ping succeeds, executes action
- If action fails, returns empty Result and error
- If action succeeds, returns `Result{Value: value}` with nil error

### ExecuteFile[T]

```go
func ExecuteFile[T any](
    _ context.Context,
    _ FileCommand,
    action func() (T, error),
) (T, error)
```

- Simply executes the action and returns result
- No agent interaction

## Usage Example

```go
// Write command - auto-starts agent
result, err := lifecycle.ExecuteWrite(ctx, launcher, lifecycle.CmdAdd, func() (*AddOutput, error) {
    return usecase.Execute(ctx, input)
})

// Read command - checks if agent running first
result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdStatus, func() (*StatusOutput, error) {
    return usecase.Execute(ctx, input)
})
if result.NothingStaged {
    // Handle "nothing staged" case
    return
}

// File command - no agent needed
result, err := lifecycle.ExecuteFile(ctx, lifecycle.CmdStashShow, func() (*ShowOutput, error) {
    return usecase.Execute(ctx, input)
})
```

## Notes

### Type Safety

The command type parameter (WriteCommand, ReadCommand, FileCommand) is unused at runtime but enforces at compile time that callers use the correct executor function for each command.

### Result vs Error

For read commands, "nothing staged" is not an error - it's a valid state. The `Result[T]` type distinguishes this from actual errors:
- `Result{NothingStaged: true}` + `nil error` = Agent not running, nothing staged
- `Result{Value: v}` + `nil error` = Action succeeded
- `Result{}` + `error` = Action failed

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../CLAUDE.md  # Agent lifecycle rules
```
