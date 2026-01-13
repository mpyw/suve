# staging/transition

## Scope

```yaml
path: internal/staging/transition
type: package
parent: ../CLAUDE.md
```

## Overview

State machine for staging operations. Defines valid state transitions and provides helper functions for determining next states based on current state and requested operation.

## Architecture

```yaml
key_types:
  - name: State
    role: Enum representing staging states
  - name: Transition
    role: Valid state transition definition

states:
  - Empty      # No staged changes
  - Staged     # Has staged changes (not yet applied)
  - Applying   # Currently applying changes
  - Applied    # Changes successfully applied
  - Failed     # Apply failed (partial or complete)

transitions:
  Empty   -> Staged   (add/edit/delete)
  Staged  -> Staged   (add/edit/delete more)
  Staged  -> Empty    (reset)
  Staged  -> Applying (apply start)
  Applying -> Applied (apply success)
  Applying -> Failed  (apply error)
  Failed  -> Staged   (retry)
  Failed  -> Empty    (reset)

dependencies:
  internal: []
  external: []
```

## Testing Strategy

```yaml
coverage_target: 95%
mock_strategy: |
  - Pure unit tests, no external dependencies
focus_areas:
  - All valid transitions
  - Invalid transition rejection
  - Edge case state combinations
skip_areas: []
```

## Notes

### Usage Pattern

```go
currentState := transition.Staged
if transition.CanTransition(currentState, transition.Applying) {
    // Start apply operation
}
```

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
