# staging/store/agent/daemon

## Scope

```yaml
path: internal/staging/store/agent/daemon
type: package
parent: ../CLAUDE.md
```

## Overview

Daemon process runner and launcher. Runner handles the daemon main loop, signal handling, and auto-shutdown logic. Launcher manages daemon lifecycle from client processes (start, stop, ensure running).

## Architecture

```yaml
key_types:
  - name: Runner
    role: Daemon process main entry point
  - name: RunnerOption
    role: Functional options (WithAutoShutdownDisabled)
  - name: Launcher
    role: Client-side daemon lifecycle manager
  - name: LauncherOption
    role: Functional options (WithAutoStartDisabled)

files:
  - runner.go      # Runner struct, Run(), Shutdown(), checkAutoShutdown()
  - launcher.go    # Launcher struct, EnsureRunning(), SendRequest()
  - internal/ipc/  # Low-level Unix socket server/client

dependencies:
  internal:
    - internal/staging/store/agent/internal/server
    - internal/staging/store/agent/internal/protocol
    - internal/staging/store/agent/daemon/internal/ipc
  external: []
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - runner_test.go: Test checkAutoShutdown() logic with mock requests/responses
  - launcher_test.go: Test EnsureRunning() with mock process checks
  - lifecycle_test.go: Integration tests for start/stop sequences
focus_areas:
  - Auto-shutdown triggers (UnstageEntry/Tag/All, SetState)
  - Shutdown reason mapping (apply/reset/persist/cleared)
  - Signal handling (SIGTERM, SIGINT)
skip_areas:
  - IPC server internals (tested in ipc/)
```

## Notes

### Auto-Shutdown Logic

```go
// In runner.go checkAutoShutdown()
switch req.Method {
case MethodUnstageEntry, MethodUnstageTag:
    if handler.IsEmpty() { resp.WillShutdown = true }
case MethodUnstageAll:
    if handler.IsEmpty() { resp.WillShutdown = true }
case MethodSetState:
    if handler.IsEmpty() { resp.WillShutdown = true }
}
```

### Launcher Flow

1. `EnsureRunning()` checks if daemon is running via ping
2. If not, spawns new daemon process with same credentials
3. Waits for daemon to become ready
4. Returns socket path for IPC

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
