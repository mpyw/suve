# usecase/staging

## Scope

```yaml
path: internal/usecase/staging
type: package
parent: ../CLAUDE.md
```

## Overview

Business logic for staging operations. Handles staging workflow: add/edit/delete entries, status/diff inspection, apply/reset execution, and stash push/pop for file persistence.

## Architecture

```yaml
key_types:
  - name: AddUseCase
    role: Stage new parameter/secret for creation
  - name: EditUseCase
    role: Stage modifications to existing resources
  - name: DeleteUseCase
    role: Stage resources for deletion
  - name: StatusUseCase
    role: Retrieve current staging state
  - name: DiffUseCase
    role: Compare staged vs remote state
  - name: ApplyUseCase
    role: Execute staged changes to AWS
  - name: ResetUseCase
    role: Clear staged changes
  - name: StashPushUseCase
    role: Save agent memory to file (persist)
  - name: StashPopUseCase
    role: Load file to agent memory (drain)
  - name: TagUseCase
    role: Stage tag add/remove operations
  - name: StashMode
    role: Enum for stash conflict handling (Merge, Overwrite)

files:
  staging_ops:
    - add.go, edit.go, delete.go
    - status.go, diff.go
    - apply.go, reset.go
    - tag.go
  stash_ops:
    - stash_mode.go   # StashMode enum (Merge, Overwrite)
    - stash_push.go   # Agent -> File
    - stash_pop.go    # File -> Agent

dependencies:
  internal:
    - internal/staging
    - internal/staging/store
    - internal/api/paramapi
    - internal/api/secretapi
  external: []
```

## Testing Strategy

```yaml
coverage_target: 90%
mock_strategy: |
  - testutil.MockStore for store interfaces
  - Direct struct manipulation for State testing
  - MockStore.PingErr for Ping-first pattern tests
focus_areas:
  - Add/Edit with conflict detection
  - Apply with rollback on partial failure
  - Stash merge/overwrite modes
  - Service-specific filtering
  - Ping-first pattern (daemon lifecycle)
skip_areas:
  - AWS API behavior
```

## Notes

### Ping-First Pattern

The `EditUseCase` and `AddUseCase` implement a "Ping-first" pattern to avoid unnecessary
daemon auto-start and AWS requests:

```go
// Before accessing staged state, check if daemon is running
if pinger, ok := u.Store.(store.Pinger); ok {
    if pinger.Ping(ctx) != nil {
        // Daemon not running → nothing staged → skip to AWS/default
    }
}
// Proceed to GetEntry only if daemon is running
```

This pattern applies to:
- `EditUseCase.Baseline()`: Returns AWS value if daemon not running
- `EditUseCase.Execute()`: Skips staged check if daemon not running, fetches from AWS directly
- `AddUseCase.Draft()`: Returns "not staged" if daemon not running

Benefits:
- Avoids daemon auto-start for pure read operations
- Reduces unnecessary AWS requests when editing staged CREATE entries
- Maintains correct behavior for FileStore (which doesn't implement Pinger)

### StashMode

Both `StashPush` and `StashPop` use a unified `StashMode` enum:
- `StashModeMerge` (default): Combines data from source with existing data at destination
- `StashModeOverwrite`: Replaces destination data with source data

CLI flags `--merge` and `--overwrite` are mutually exclusive (using urfave/cli v3's `MutuallyExclusiveFlags`).
Default behavior is `Merge` for safer operation.

### Stash Operations

**StashPush (agent -> file):**
- Uses StashMode for conflict handling with existing file
- Service filter: Push only param or secret entries
- `--keep`: Don't clear agent memory after push

**StashPop (file -> agent):**
- Uses StashMode for conflict handling with existing agent memory
- Service filter: Pop only param or secret entries
- `--keep`: Don't delete file after pop (same as `stash apply`)

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../staging/CLAUDE.md
```
