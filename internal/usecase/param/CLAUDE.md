# usecase/param

## Scope

```yaml
path: internal/usecase/param
type: package
parent: ../CLAUDE.md
```

## Overview

Business logic for SSM Parameter Store operations. Implements use cases for show, log, list, diff, create, update, delete, and tag operations.

## Architecture

```yaml
key_types:
  - name: ShowUseCase
    role: Get parameter value with metadata
  - name: LogUseCase
    role: Get version history
  - name: ListUseCase
    role: List parameters by path
  - name: DiffUseCase
    role: Compare two versions
  - name: CreateUseCase
    role: Create new parameter
  - name: UpdateUseCase
    role: Update existing parameter
  - name: DeleteUseCase
    role: Delete parameter
  - name: TagUseCase
    role: Add/remove tags

files:
  - show.go, log.go, list.go, diff.go
  - create.go, update.go, delete.go
  - tag.go

dependencies:
  internal:
    - internal/api/paramapi
    - internal/version/paramversion
  external:
    - github.com/aws/aws-sdk-go-v2/service/ssm
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - Mock paramapi.API interface
  - Test version resolution with mock API responses
focus_areas:
  - Version specification resolution
  - Error mapping (not found, access denied)
  - Diff output formatting
skip_areas:
  - AWS SDK behavior
```

## Notes

### Version Resolution

All read operations support version specs:
- `show /app/config#3` -> Get version 3
- `log /app/config~2` -> History starting 2 versions ago
- `diff /app/config#1 /app/config#3` -> Compare versions

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../version/CLAUDE.md
```
