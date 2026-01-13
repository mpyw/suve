# internal

## Scope

```yaml
path: internal
type: integration
parent: ../CLAUDE.md
children:
  - staging/CLAUDE.md
  - cli/CLAUDE.md
  - usecase/CLAUDE.md
  - version/CLAUDE.md
  - gui/CLAUDE.md
```

## Overview

Root package for all internal implementation. Contains business logic, CLI commands, staging system, and GUI components. No code at this level - only sub-packages.

## Architecture

```yaml
key_types: []

layer_structure:
  presentation:
    - cli/commands    # CLI entry points
    - gui             # GUI entry points
  application:
    - usecase         # Business logic orchestration
    - staging/cli     # Staging CLI wrappers
  domain:
    - staging         # Staging core (strategies, state)
    - version         # Version specification parsing
    - tagging         # Tag operations
  infrastructure:
    - staging/store   # Storage backends (agent, file)
    - infra           # AWS client initialization
    - api             # AWS API interfaces
  utilities:
    - cli/*           # CLI utilities (output, confirm, editor, etc.)
    - maputil         # Map/Set utilities
    - jsonutil        # JSON formatting
    - timeutil        # Time utilities
    - parallel        # Parallel execution

dependencies:
  external:
    - github.com/aws/aws-sdk-go-v2
    - github.com/urfave/cli/v3
    - github.com/wailsapp/wails/v2
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  Each layer has its own mocking strategy:
  - CLI commands: mock usecase layer
  - Usecase: mock API interfaces and store
  - Staging store: mock IPC/file operations
focus_areas:
  - Cross-package integration
  - Error propagation between layers
skip_areas:
  - Individual package unit tests (delegated to children)
```

## Notes

### Package Organization Principles

1. **Dependency direction**: Upper layers depend on lower layers, never reverse
2. **Interface boundaries**: Each layer communicates through interfaces
3. **Testability**: All packages are designed for isolated unit testing

### Complexity Hotspots

High complexity packages requiring careful attention:
- `staging/cli` (2,493 LOC) - Many CLI wrappers
- `staging/store/agent` - IPC, daemon, security
- `usecase/staging` (1,643 LOC) - Complex business logic

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
