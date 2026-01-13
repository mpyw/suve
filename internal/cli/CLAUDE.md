# cli

## Scope

```yaml
path: internal/cli
type: group
parent: ../CLAUDE.md
children:
  - commands/CLAUDE.md
```

## Overview

CLI utilities package group. Contains shared utilities for CLI operations: output formatting, user confirmation, editor integration, terminal detection, passphrase handling, and diff rendering.

## Architecture

```yaml
packages:
  colors/:       # ANSI color codes, color detection
  confirm/:      # User confirmation prompts, choice selection
  diffargs/:     # Diff argument parsing (version specs)
  editor/:       # External editor integration ($EDITOR)
  output/:       # Formatted output (success, warn, error, diff)
  pager/:        # Pager integration ($PAGER, less)
  passphrase/:   # Passphrase input (for stash encryption)
  terminal/:     # TTY detection, terminal utilities

shared_patterns:
  - Prompter structs with Stdin/Stdout/Stderr fields
  - IsTerminal* functions for TTY detection
  - Printf/Warn/Success/Error for consistent output

dependencies:
  external:
    - github.com/mattn/go-isatty
    - golang.org/x/term
```

## Testing Strategy

```yaml
coverage_target: 70%
mock_strategy: |
  - Use bytes.Buffer for Stdin/Stdout/Stderr
  - Mock terminal.IsTerminal* for TTY tests
focus_areas:
  - Output formatting correctness
  - Confirmation prompt flow
  - Passphrase masking
skip_areas:
  - Actual terminal behavior (requires TTY)
  - External editor invocation
```

## Notes

### Package Sizes (LOC)

Small, focused packages:
- colors: ~50 LOC
- confirm: ~150 LOC
- output: ~200 LOC
- terminal: ~50 LOC
- passphrase: ~150 LOC

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - commands/CLAUDE.md
```
