# gui

## Scope

```yaml
path: internal/gui
type: package
parent: ../CLAUDE.md
```

## Overview

GUI application using Wails v2 with Svelte frontend. Provides desktop interface for parameter and secret management with staging support.

## Architecture

```yaml
key_types:
  - name: App
    role: Wails app binding Go backend to JS frontend

files:
  backend:
    - app.go        # Main app struct, lifecycle methods
    - param.go      # Parameter operations for GUI
    - secret.go     # Secret operations for GUI
    - staging.go    # Staging operations for GUI
  frontend:
    - frontend/src/        # Svelte components
    - frontend/tests/      # Playwright tests

dependencies:
  internal:
    - internal/api/*
    - internal/usecase/*
    - internal/staging/*
  external:
    - github.com/wailsapp/wails/v2
```

## Testing Strategy

```yaml
coverage_target: 60%
mock_strategy: |
  - Backend: Mock API interfaces
  - Frontend: Playwright for component/integration tests
focus_areas:
  - Go-JS binding correctness
  - State synchronization
  - Error display
skip_areas:
  - Wails framework behavior
  - Visual layout (manual testing)
```

## Notes

### Development

```bash
make gui          # Run in dev mode
make gui-build    # Build binary
make gui-test     # Run Playwright tests
```

### Frontend Stack

- Svelte 4
- TypeScript
- TailwindCSS
- Playwright for testing

### Staging API

The `StagingDrain` method uses a mode string parameter:
- `mode: "merge"` - Combine with existing agent memory (default)
- `mode: "overwrite"` - Replace agent memory

Frontend uses radio buttons for mode selection (mutually exclusive).

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
