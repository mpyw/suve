# staging

## Scope

```yaml
path: internal/staging
type: integration
parent: ../CLAUDE.md
children:
  - cli/CLAUDE.md
  - transition/CLAUDE.md
  - store/CLAUDE.md
```

## Overview

Core staging domain package. Defines staging data structures (Entry, TagEntry, State), service types, and conflict detection. Sub-packages handle CLI wrappers, state transitions, and storage backends.

## Architecture

```yaml
key_types:
  - name: State
    role: Container for all staged entries and tags across services
  - name: Entry
    role: Single staged parameter/secret with operation type and value
  - name: TagEntry
    role: Staged tag changes (add/remove) for a resource
  - name: Operation
    role: Enum for Create/Update/Delete operations
  - name: Service
    role: Enum for param/secret service types

layer_structure:
  domain:
    - stage.go         # State, Entry, TagEntry structs
    - conflict.go      # Conflict detection between local and remote
    - service.go       # Service type enum
  strategies:
    - param.go         # Param-specific staging strategy
    - secret.go        # Secret-specific staging strategy
  presentation:
    - printer.go       # Status/diff output formatting

dependencies:
  internal:
    - internal/api/paramapi
    - internal/api/secretapi
  external: []
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - State operations: Direct struct manipulation
  - Conflict detection: Mock API responses for remote state
focus_areas:
  - State.Merge behavior with overlapping keys
  - Conflict detection edge cases
  - Service filtering in ExtractService/RemoveService
skip_areas:
  - CLI wrappers (delegated to cli/)
  - Storage operations (delegated to store/)
```

## Notes

### State Design

- Version field for future schema migrations
- Entries/Tags maps: `map[Service]map[name]Entry/TagEntry`
- Merge: later state wins on key conflicts

### Conflict Detection

`CheckConflicts` compares staged BaseModifiedAt with current remote version to detect concurrent modifications.

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - cli/CLAUDE.md
  - store/CLAUDE.md
```
