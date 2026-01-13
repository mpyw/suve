# version

## Scope

```yaml
path: internal/version
type: group
parent: ../CLAUDE.md
```

## Overview

Version specification parsing for Git-like revision syntax. Supports `#VERSION`, `~SHIFT`, and `:LABEL` (Secrets Manager only) modifiers. Separate implementations for param and secret due to different version semantics.

## Architecture

```yaml
packages:
  internal/:       # Shared utilities (char checks, shift parsing)
  paramversion/:   # SSM Parameter Store version parser
  secretversion/:  # Secrets Manager version parser

key_types:
  - name: paramversion.Parser
    role: Parse param version specs (#N, ~N)
  - name: secretversion.Parser
    role: Parse secret version specs (#ID, :LABEL, ~N)
  - name: Spec
    role: Parsed version specification (Name, Version/Label, Shifts)

version_syntax:
  param: "<name>[#VERSION][~SHIFT]*"
  secret: "<name>[#VERSION | :LABEL][~SHIFT]*"

examples:
  - "/my/param"           # Latest
  - "/my/param#3"         # Version 3
  - "/my/param~1"         # 1 version ago
  - "my-secret:AWSCURRENT" # Staging label
  - "my-secret#abc123~1"  # Version abc123, 1 back

dependencies:
  internal:
    - internal/staging  # Service type
  external: []
```

## Testing Strategy

```yaml
coverage_target: 90%
mock_strategy: |
  - Direct parser testing with various input strings
  - Mock API for version resolution tests
focus_areas:
  - Edge cases in version syntax
  - Shift resolution (cumulative ~)
  - Error messages for invalid syntax
skip_areas:
  - AWS API version listing (tested via e2e)
```

## Notes

### Shift Resolution

Shifts are cumulative: `~1~2` = `~3`
Bare `~` equals `~1`

### Parser.Resolve()

Takes API client to resolve shifts to actual version:
1. Parse name and modifiers
2. List versions from AWS
3. Apply shifts to find target version

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
