# staging/transition

## Scope

```yaml
path: internal/staging/transition
type: package
parent: ../CLAUDE.md
```

## Overview

State machine for staging operations. Provides reducers (pure functions) that compute new states from current states and actions. Separates state computation logic from side effects for testability.

## Architecture

```yaml
key_types:
  - name: EntryState
    role: Current state of a staged entry (value + staged state)
  - name: EntryStagedState
    role: Interface for entry staged states (NotStaged, Create, Update, Delete)
  - name: StagedTags
    role: Staged tag changes (ToSet, ToUnset)
  - name: EntryTransitionResult
    role: Result of entry state transition (next state, discard tags flag)
  - name: TagTransitionResult
    role: Result of tag state transition (next tags)
  - name: Executor
    role: Applies transitions to store with error handling

entry_staged_states:
  - EntryStagedStateNotStaged  # Entry has no staged changes
  - EntryStagedStateCreate     # Entry staged for creation (with DraftValue)
  - EntryStagedStateUpdate     # Entry staged for update (with DraftValue)
  - EntryStagedStateDelete     # Entry staged for deletion

actions:
  - EntryActionAdd     # Stage new entry creation
  - EntryActionEdit    # Stage entry modification
  - EntryActionDelete  # Stage entry deletion
  - EntryActionReset   # Unstage entry
  - TagActionTag       # Stage tag addition
  - TagActionUntag     # Stage tag removal
  - TagActionReset     # Unstage tags

files:
  - state.go    # EntryState, EntryStagedState types, StagedTags
  - action.go   # Action types
  - reducer.go  # ReduceEntry(), ReduceTag() pure functions
  - executor.go # Executor applies transitions with store operations

dependencies:
  internal:
    - internal/staging/store  # Store interface for Executor
  external: []
```

## Testing Strategy

```yaml
coverage_target: 95%
mock_strategy: |
  - Pure unit tests for reducers (no mocks needed)
  - MockStore for Executor tests
focus_areas:
  - All valid state transitions
  - Error cases (invalid transitions)
  - Edge cases in state combinations
skip_areas: []
```

## Notes

### Reducer Pattern

```go
// Pure function: takes current state + action, returns new state
result := transition.ReduceEntry(currentState, action)
newState := result.NextState
shouldDiscardTags := result.DiscardTags

// Tags reducer
tagResult := transition.ReduceTag(currentTags, tagAction)
newTags := tagResult.NextTags
```

### State Transitions

```
Entry transitions:
  NotStaged + Add    -> Create
  NotStaged + Edit   -> Update (if current value exists)
  NotStaged + Delete -> Delete (if current value exists)
  Create + Edit      -> Create (updated draft)
  Create + Reset     -> NotStaged
  Update + Edit      -> Update (updated draft)
  Update + Reset     -> NotStaged
  Delete + Reset     -> NotStaged
```

### Executor

Executor wraps reducers with store operations:
- Loads current state from store
- Applies reducer
- Saves new state to store
- Returns error if transition is invalid

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
