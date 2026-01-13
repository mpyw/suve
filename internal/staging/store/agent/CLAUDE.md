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
```
