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
  - name: Spec
    role: Type alias for parsed version spec (includes Name, AbsoluteSpec, Shifts)
  - name: AbsoluteSpec
    role: Parsed absolute version (param: version int, secret: version ID or label)

public_api:
  paramversion:
    - Parse(spec string) -> Spec  # Parse version spec string
    - ParseDiffArgs(args []string) -> (name, spec1, spec2)
  secretversion:
    - Parse(spec string) -> Spec  # Parse version spec string
    - ParseDiffArgs(args []string) -> (name, spec1, spec2)

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

### Version Resolution Flow

When using `Parse()` with shifts (~N):
1. Parse name and modifiers into Spec
2. Caller lists versions from AWS API
3. Caller applies shifts to find target version

Note: The `Spec.Shifts` field accumulates shift count (e.g., `~1~2` = 3 shifts)

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
