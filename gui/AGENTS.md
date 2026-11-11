# gui

## Scope

```yaml
path: gui
type: wails-entry
parent: ../AGENTS.md
```

## Overview

Standalone entry point for the GUI. It is a thin wrapper that calls `internal/gui.Run()`, the same function `suve --gui` calls. `mise gui-dev` runs it for frontend hot reload, and declscope checks it as its own build configuration.

## Why This Directory Exists

`suve --gui` needs CLI flags to choose the provider and scope. This module starts the GUI with no CLI layer, the scope coming from the environment as for a bare `suve --gui`, so a GUI-only dev build stays small and fast to rebuild. The actual implementation lives in `internal/gui/`.

## Files

```yaml
main.go: Entry point, calls internal/gui.Run()
go.mod: Separate module for the GUI entry point
build/appicon.png: The application icon (also used by the README and the docs site)
```

## Development Commands

```bash
# Run the GUI with frontend hot reload (Vite dev server + a dev build of this module)
mise gui-dev

# Build the single CLI+GUI binary (bin/suve)
mise build-gui

# Regenerate TypeScript bindings after changing Go backend
mise generate-gui-bindings
```

## Relationship to internal/gui

```
gui/main.go ──calls──> internal/gui.Run()
                              │
                              ▼
                       internal/gui/run.go (Wails v3 application and window)
                       internal/gui/app.go (the App service)
                              │
                              ▼
                       internal/gui/frontend/ (Svelte app, bindings/)
```

- `gui/`: Standalone GUI entry point only
- `internal/gui/`: Actual Go backend implementation
- `internal/gui/frontend/`: Svelte frontend

## Notes

- The `go.mod` here is separate from the root module
- It replaces `github.com/mpyw/suve` with `../`, so re-run `go mod tidy` here after a root dependency change. `mise tidy-check` (part of `mise lint` and CI) fails when it is not tidy
- Build tags `production` or `dev` are required (see `//go:build` in main.go). `dev` keeps the Wails devtools and makes the app load the frontend from `FRONTEND_DEVSERVER_URL`
