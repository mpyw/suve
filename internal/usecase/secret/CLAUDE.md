# usecase/secret

## Scope

```yaml
path: internal/usecase/secret
type: package
parent: ../CLAUDE.md
```

## Overview

Business logic for AWS Secrets Manager operations. Implements use cases for show, log, list, diff, create, update, delete, restore, and tag operations.

## Architecture

```yaml
key_types:
  - name: ShowUseCase
    role: Get secret value with metadata
  - name: LogUseCase
    role: Get version history
  - name: ListUseCase
    role: List secrets
  - name: DiffUseCase
    role: Compare two versions
  - name: CreateUseCase
    role: Create new secret
  - name: UpdateUseCase
    role: Update existing secret
  - name: DeleteUseCase
    role: Delete secret (with recovery window)
  - name: RestoreUseCase
    role: Restore deleted secret
  - name: TagUseCase
    role: Add/remove tags

files:
  - show.go, log.go, list.go, diff.go
  - create.go, update.go, delete.go, restore.go
  - tag.go

dependencies:
  internal:
    - internal/api/secretapi
    - internal/version/secretversion
  external:
    - github.com/aws/aws-sdk-go-v2/service/secretsmanager
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - Mock secretapi.API interface
  - Test version/label resolution with mock API responses
focus_areas:
  - Version ID and staging label resolution
  - Recovery window handling in delete
  - Binary vs string secret handling
skip_areas:
  - AWS SDK behavior
```

## Notes

### Secrets Manager Specifics

- Version IDs are UUIDs (not integers like SSM)
- Staging labels (AWSCURRENT, AWSPREVIOUS, custom)
- Soft delete with recovery window (7-30 days)
- Binary and string secret types

### Version Resolution

```
my-secret:AWSCURRENT    -> Current version via label
my-secret#abc123        -> Specific version ID
my-secret:AWSCURRENT~1  -> One version before current
```

## References

```yaml
related_docs:
  - ../CLAUDE.md
  - ../../version/CLAUDE.md
```
