# staging/store/agent

## Scope

```yaml
path: internal/staging/store/agent
type: integration
parent: ../CLAUDE.md
children:
  - daemon/CLAUDE.md
  - internal/client/CLAUDE.md
  - internal/server/CLAUDE.md
  - internal/protocol/CLAUDE.md
```

## Overview

In-memory staging storage using a daemon process with Unix socket IPC. Provides fast, persistent staging state that survives CLI invocations. Security features include peer authentication and memory protection.

## Architecture

```yaml
key_types:
  - name: Store (store.go)
    role: Public entry point, delegates to client or direct mode
  - name: Mode (mode.go)
    role: Enum for client/direct mode selection

layer_structure:
  public_api:
    - store.go        # NewStore, implements AgentStore interface
    - mode.go         # Mode enum (client/direct)
  daemon:
    - daemon/runner.go     # Daemon process main loop
    - daemon/launcher.go   # Start/stop daemon from client
  internal:
    - internal/client/     # Client-side store implementation
    - internal/server/     # Request handler, state management
    - internal/protocol/   # IPC message format, socket paths

dependencies:
  internal:
    - internal/staging      # State, Entry types
    - internal/staging/store # Interface definitions
  external: []
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - daemon/: Test runner and launcher separately
  - client/: Mock IPC responses
  - server/: Unit test handler with mock state
  - protocol/: Test serialization/deserialization
focus_areas:
  - Daemon lifecycle (start, shutdown, auto-shutdown)
  - IPC message round-trip
  - Security (peer auth, memory protection)
  - Error handling and recovery
skip_areas:
  - Platform-specific socket behavior (tested via e2e)
```

## Notes

### Daemon Architecture

```
┌─────────────────┐     Unix Socket      ┌─────────────────┐
│  CLI Process    │ ◄──────────────────► │  Daemon Process │
│  (client/)      │     JSON-RPC         │  (daemon/)      │
└─────────────────┘                      └─────────────────┘
                                                 │
                                                 ▼
                                         ┌─────────────────┐
                                         │  server/handler │
                                         │  (in-memory)    │
                                         └─────────────────┘
```

### Agent Lifecycle Rules

The agent daemon follows specific rules for auto-start and auto-shutdown based on command type.

**For detailed command classification and executor functions, see `daemon/lifecycle/CLAUDE.md`.**

#### Auto-Start Rules (Summary)

| Category | Should Auto-Start | Rationale |
|----------|-------------------|-----------|
| **WriteCommand** | ✅ Yes | Need agent to store changes |
| **ReadCommand** | ❌ No (Ping check) | If agent not running, nothing is staged |
| **FileCommand** | ❌ No | Only access file store, not agent |

#### Auto-Shutdown Rules

| Trigger | Shutdown? | Reason Code |
|---------|-----------|-------------|
| `apply` empties state | ✅ Yes | `applied` |
| `reset --all` empties state | ✅ Yes | `unstaged` |
| `reset <name>` empties state | ✅ Yes | `unstaged` |
| `stash push` empties state | ✅ Yes | `persisted` |
| `stash pop --keep` empties agent | ✅ Yes | `cleared` |

#### Implementation Pattern (Lifecycle Package)

Commands should use the `daemon/lifecycle` package executor functions:

```go
import "github.com/mpyw/suve/internal/staging/store/agent/daemon/lifecycle"

// Write command - auto-starts agent
result, err := lifecycle.ExecuteWrite(ctx, launcher, lifecycle.CmdAdd, func() (*Output, error) {
    return usecase.Execute(ctx, input)
})

// Read command - checks if agent running first
result, err := lifecycle.ExecuteRead(ctx, store, lifecycle.CmdStatus, func() (*Output, error) {
    return usecase.Execute(ctx, input)
})
if result.NothingStaged {
    // Handle "nothing staged" case
}

// File command - no agent needed
result, err := lifecycle.ExecuteFile(ctx, lifecycle.CmdStashShow, func() (*Output, error) {
    return usecase.Execute(ctx, input)
})
```

#### Store Method Auto-Start Behavior

| Method | Auto-Starts | Used By |
|--------|-------------|---------|
| `Ping()` | ❌ No | Lifecycle checks |
| `StageEntry()` | ✅ Yes | add, edit, delete |
| `StageTag()` | ✅ Yes | tag, untag |
| `UnstageEntry()` | ✅ Yes | reset (specific) |
| `UnstageTag()` | ✅ Yes | reset (specific) |
| `UnstageAll()` | ✅ Yes | reset --all, apply |
| `ListEntries()` | ✅ Yes | status, diff, apply |
| `ListTags()` | ✅ Yes | status, diff, apply |
| `GetEntry()` | ✅ Yes | diff |
| `Drain()` | ❌ No | stash push |
| `WriteState()` | ✅ Yes | stash pop |

**Important**: Even though `UnstageAll` and `ListEntries` auto-start, commands using them should do a Ping check first to provide better UX when nothing is staged.

### Auto-Shutdown

Daemon automatically shuts down when:
- All staged changes are cleared (via apply/reset/unstage)
- SetState results in empty state (via stash push)

Shutdown message indicates the reason (applied/unstaged/persisted/cleared).

### Security

- Peer authentication via SO_PEERCRED (Linux) / LOCAL_PEERPID (Darwin)
- Memory protection via mlock() to prevent swapping sensitive data

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - daemon/CLAUDE.md
  - ../../../../docs/staging-agent.md  # User-facing documentation
```
