# Staging Agent/Daemon System

> [Back to README](../README.md#staging-workflow)

This document describes the in-memory staging daemon architecture, stash commands, security features, and configuration options.

## Overview

suve uses an in-memory daemon process to store staged changes. This provides:

- **Fast access**: No disk I/O for staging operations
- **Persistent state**: Survives CLI invocations within the same session
- **Security**: Memory protection prevents sensitive data from being swapped to disk
- **Automatic lifecycle**: Daemon starts when needed and stops when empty

## Architecture

```
┌─────────────────┐     Unix Socket      ┌─────────────────┐
│  CLI Process    │ ◄──────────────────► │  Daemon Process │
│  (client)       │     JSON messages    │  (background)   │
└─────────────────┘                      └─────────────────┘
                                                 │
                                                 ▼
                                         ┌─────────────────┐
                                         │  In-Memory      │
                                         │  State (mlock)  │
                                         └─────────────────┘
```

### Component Overview

| Component | Location | Description |
|-----------|----------|-------------|
| Client | `internal/staging/store/agent/internal/client/` | Connects to daemon via socket |
| Server | `internal/staging/store/agent/internal/server/` | Handles requests, manages state |
| Daemon Runner | `internal/staging/store/agent/daemon/runner.go` | Main daemon process loop |
| Launcher | `internal/staging/store/agent/daemon/launcher.go` | Starts/stops daemon from CLI |
| Protocol | `internal/staging/store/agent/internal/protocol/` | IPC message format, socket paths |

### Daemon Lifecycle

1. **Auto-Start**: When you run a staging command (e.g., `suve stage param edit`), the daemon starts automatically if not running
2. **Active**: Daemon stores staged changes in memory, responds to CLI requests
3. **Auto-Shutdown**: When all staged changes are cleared (via `apply`, `reset`, or `stash push`), the daemon shuts down automatically

### Socket Paths

The daemon communicates via Unix sockets. Socket location depends on the platform:

| Platform | Socket Path |
|----------|-------------|
| Linux | `$XDG_RUNTIME_DIR/suve/{accountID}/{region}/agent.sock` |
| Linux (fallback) | `/tmp/suve-{uid}/{accountID}/{region}/agent.sock` |
| macOS | `$TMPDIR/suve/{accountID}/{region}/agent.sock` |
| Windows | `$LOCALAPPDATA/suve/{accountID}/{region}/agent.sock` |

Socket directories are created with mode `0700` for security.

## Stash Commands

Stash commands allow you to save staged changes to a file for later restoration, similar to `git stash`.

### Command Overview

| Command | Description |
|---------|-------------|
| `suve stage stash` | Save staged changes to file (alias for `stash push`) |
| `suve stage stash push` | Save staged changes from memory to file |
| `suve stage stash pop` | Restore staged changes from file (deletes file) |
| `suve stage stash pop --keep` | Restore staged changes from file (keeps file) |
| `suve stage stash show` | Preview stashed changes without restoring |
| `suve stage stash drop` | Delete stash file without restoring |

### Data Flow

```
                    stash push
   Agent Memory ──────────────────► File (~/.suve/.../stage.json)
       ▲                                     │
       │                                     │
       │               stash pop             │
       └─────────────────────────────────────┘
```

### Usage Examples

**Save staged changes to file:**

```bash
# Save and clear from memory (default)
suve stage stash

# Save but keep in memory
suve stage stash push --keep

# Save with encryption (prompts for passphrase)
# Passphrase is prompted interactively in a TTY

# Save with passphrase from stdin (for scripts)
echo "my-passphrase" | suve stage stash push --passphrase-stdin
```

**Restore staged changes:**

```bash
# Restore and delete file
suve stage stash pop

# Restore but keep file
suve stage stash pop --keep

# Force overwrite existing memory (no prompt)
suve stage stash pop --force

# Merge with existing memory
suve stage stash pop --merge

# Decrypt with passphrase from stdin
echo "my-passphrase" | suve stage stash pop --passphrase-stdin
```

**Preview and delete:**

```bash
# Preview stashed changes
suve stage stash show
suve stage stash show -v  # Verbose mode

# Delete stash file
suve stage stash drop
suve stage stash drop --force  # Skip confirmation
```

### Service-Specific Stash

You can stash changes for a specific service:

```bash
# Stash only param changes
suve stage param stash

# Stash only secret changes
suve stage secret stash

# Pop only param changes
suve stage param stash pop
```

When using service-specific stash:
- `stash push` saves only that service's changes (other services remain in memory)
- `stash pop` restores only that service's changes (other services in file are preserved)

### Stash File Format

The stash file is stored at `~/.suve/{accountID}/{region}/stage.json`:

**Unencrypted:**
```json
{"version":1,"entries":{...},"tags":{...}}
```

**Encrypted:** Binary format with `SUVE_ENC` header, salt, and AES-GCM ciphertext.

### Merge and Conflict Handling

When restoring stashed changes with `stash pop`:

| Scenario | Default Behavior | Options |
|----------|-----------------|---------|
| Agent memory is empty | Restore directly | N/A |
| Agent has changes | Prompt for action | `--force` (overwrite), `--merge` (combine) |
| File has conflicts | User chooses | Interactive prompt in TTY |

When using `--merge`:
- File changes are combined with existing memory changes
- For duplicate keys, file values take precedence (newer wins)

## Security Features

### Peer Authentication

The daemon verifies that connecting clients are running as the same user:

| Platform | Mechanism |
|----------|-----------|
| Linux | `SO_PEERCRED` socket option |
| macOS | `LOCAL_PEERPID` / peer credentials |
| Windows | Named pipe security |

This prevents unauthorized processes from accessing your staged secrets.

### Memory Protection

Sensitive data in daemon memory is protected using the [memguard](https://github.com/awnumar/memguard) library:

- **mlock**: Prevents memory from being swapped to disk
- **Guard pages**: Detects buffer overflows/underflows
- **Secure destruction**: Overwrites memory on cleanup

### File Encryption

Stash files can be encrypted with a passphrase:

- **Key derivation**: Argon2id (memory-hard, resistant to GPU attacks)
- **Encryption**: AES-256-GCM (authenticated encryption)
- **Format**: Magic header + version + salt + ciphertext

```
┌──────────┬─────────┬──────────────┬─────────────────┐
│ SUVE_ENC │ Version │ Salt (32B)   │ AES-GCM Payload │
│ (8 bytes)│ (1 byte)│              │ (variable)      │
└──────────┴─────────┴──────────────┴─────────────────┘
```

### Security Best Practices

1. **Always use encryption** when stashing sensitive data:
   ```bash
   suve stage stash  # Will prompt for passphrase in TTY
   ```

2. **Clear stashed data** when no longer needed:
   ```bash
   suve stage stash drop
   ```

3. **Use `--passphrase-stdin`** for automation (avoid shell history):
   ```bash
   read -s PASS && echo "$PASS" | suve stage stash --passphrase-stdin
   ```

4. **Socket permissions** are automatically set to `0700` (owner only)

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SUVE_DAEMON_MANUAL_MODE` | Set to `1` to disable auto-start and auto-shutdown | Not set (auto mode) |

### Manual Mode

By default, the daemon starts and stops automatically. Enable manual mode for:

- Debugging daemon issues
- Keeping daemon running across sessions
- CI/CD environments where you want explicit control

```bash
# Enable manual mode
export SUVE_DAEMON_MANUAL_MODE=1

# Start daemon manually
suve stage agent start

# ... perform staging operations ...

# Stop daemon manually
suve stage agent stop
```

### Agent Commands

| Command | Description |
|---------|-------------|
| `suve stage agent start` | Start the daemon manually |
| `suve stage agent stop` | Stop the daemon (warning: unsaved changes are lost) |

### File Paths

| Item | Path |
|------|------|
| Stash file | `~/.suve/{accountID}/{region}/stage.json` |
| Socket | Platform-specific (see [Socket Paths](#socket-paths)) |

## Troubleshooting

### Daemon Won't Start

1. Check if another daemon is running:
   ```bash
   suve stage agent stop
   suve stage agent start
   ```

2. Check socket permissions:
   ```bash
   ls -la $TMPDIR/suve/  # macOS
   ls -la $XDG_RUNTIME_DIR/suve/  # Linux
   ```

3. Enable manual mode for debugging:
   ```bash
   export SUVE_DAEMON_MANUAL_MODE=1
   suve stage agent start
   ```

### Lost Staged Changes

If the daemon stopped unexpectedly:

1. Check if changes were auto-stashed (unlikely unless you used `stash push`)
2. Staged changes in memory are lost when daemon stops
3. Use `suve stage stash push` before closing your session to persist changes

### Encryption Issues

1. **Wrong passphrase**: Try again with the correct passphrase
2. **Corrupted file**: The file may be damaged; use `stash drop` and re-stage
3. **Automation**: Use `--passphrase-stdin` for scripts

## Implementation Details

For developers working on the staging system:

- **CLAUDE.md files**: Each sub-package has its own documentation
  - `internal/staging/CLAUDE.md` - Core staging domain
  - `internal/staging/store/CLAUDE.md` - Store interfaces
  - `internal/staging/store/agent/CLAUDE.md` - Agent store
  - `internal/staging/store/agent/daemon/CLAUDE.md` - Daemon runner/launcher
  - `internal/staging/store/file/CLAUDE.md` - File store with encryption

- **State transitions**: See [Staging State Transitions](staging-state-transitions.md)

- **Testing**: E2E tests for daemon IPC are in `e2e/staging_daemon_test.go`
