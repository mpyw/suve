# gui

## Scope

```yaml
path: gui
type: wails-entry
parent: ../CLAUDE.md
```

## Overview

Wails entry point for GUI development and standalone builds. This is a thin wrapper that calls `internal/gui.Run()`. Required by Wails toolchain for `wails dev` and `wails build` commands.

## Why This Directory Exists

Wails expects a specific directory structure with `main.go` at the project root of the GUI app. This directory serves as that entry point while the actual implementation lives in `internal/gui/`.

## Files

```yaml
main.go: Entry point, calls internal/gui.Run()
wails.json: Wails configuration
go.mod: Separate module for Wails tooling
build/: Build assets (icons, Info.plist, etc.)
```

## Development Commands

```bash
# Start Wails dev server (hot reload at http://localhost:34115)
make gui-dev

# Build standalone GUI binary
make gui-build

# Regenerate TypeScript bindings after changing Go backend
make gui-bindings
```

## Relationship to internal/gui

```
gui/main.go ──calls──> internal/gui.Run()
                              │
                              ▼
                       internal/gui/app.go (Wails app definition)
                              │
                              ▼
                       internal/gui/frontend/ (Svelte app)
```

- `gui/`: Wails toolchain entry point only
- `internal/gui/`: Actual Go backend implementation
- `internal/gui/frontend/`: Svelte frontend

## Notes

- Do NOT delete this directory - Wails toolchain requires it
- The `go.mod` here is separate from the root module to isolate Wails dependencies
- Build tags `production` or `dev` are required (see `//go:build` in main.go)
