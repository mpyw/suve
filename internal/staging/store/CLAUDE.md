# staging/store

## Scope

```yaml
path: internal/staging/store
type: integration
parent: ../CLAUDE.md
children:
  - agent/CLAUDE.md
  - file/CLAUDE.md
```

## Overview

Storage interface definitions and backend implementations for staging state. Defines common interfaces (ReadOperator, WriteOperator, Drainer, etc.) implemented by agent (in-memory daemon) and file (encrypted JSON) backends.

## Architecture

```yaml
key_types:
  - name: ReadOperator
    role: Interface for reading staged entries/tags
  - name: WriteOperator
    role: Interface for staging/unstaging entries/tags
  - name: ReadWriteOperator
    role: Combined read/write interface
  - name: Drainer
    role: Interface for bulk state retrieval
  - name: Writer
    role: Interface for bulk state writing
  - name: FileStore
    role: Drainer + Writer for file backend
  - name: AgentStore
    role: Full interface for agent backend
  - name: HintedUnstager
    role: Unstage with operation hints for shutdown messages

constants:
  - HintApply    # Operation was apply
  - HintReset    # Operation was reset
  - HintPersist  # Operation was stash push

dependencies:
  internal:
    - internal/staging  # State, Entry, TagEntry types
  external: []
```

## Testing Strategy

```yaml
coverage_target: 90%
mock_strategy: |
  - testutil/mock.go provides MockStore for all interfaces
  - Allows setting error injection via DrainErr, WriteStateErr, etc.
focus_areas:
  - Interface contract compliance
  - Error propagation
skip_areas:
  - Implementation details (delegated to agent/, file/)
```

## Notes

### Interface Hierarchy

```
ReadOperator ─┐
              ├─> ReadWriteOperator ─┐
WriteOperator ┘                      │
                                     ├─> AgentStore
Drainer ─────────────────────────────┤
Writer ──────────────────────────────┘

Drainer ─┬─> FileStore
Writer ──┘
```

### Hints for Shutdown Messages

When clearing agent memory, hints are passed to control the daemon's shutdown message:
- `HintApply` -> "all changes applied"
- `HintReset` -> "all changes unstaged"
- `HintPersist` -> "state saved to file"

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - agent/CLAUDE.md
  - file/CLAUDE.md
```
