# usecase

## Scope

```yaml
path: internal/usecase
type: integration
parent: ../CLAUDE.md
children:
  - param/CLAUDE.md
  - secret/CLAUDE.md
  - staging/CLAUDE.md
```

## Overview

Business logic layer containing use case implementations. Orchestrates domain operations and API calls. Each sub-package corresponds to a service domain (param, secret, staging).

## Architecture

```yaml
layer_structure:
  param/:
    - show.go, log.go, list.go    # Read operations
    - create.go, update.go        # Write operations
    - delete.go                   # Delete operations
    - diff.go                     # Version comparison
    - tag.go                      # Tag management
  secret/:
    - show.go, log.go, list.go    # Read operations
    - create.go, update.go        # Write operations
    - delete.go, restore.go       # Delete/restore operations
    - diff.go                     # Version comparison
    - tag.go                      # Tag management
  staging/:
    - add.go, edit.go, delete.go  # Staging modifications
    - status.go, diff.go          # Staging inspection
    - apply.go, reset.go          # Staging execution
    - stash_push.go, stash_pop.go # Stash operations
    - tag.go                      # Tag staging

dependencies:
  internal:
    - internal/api/paramapi
    - internal/api/secretapi
    - internal/staging
    - internal/staging/store
  external:
    - github.com/aws/aws-sdk-go-v2
```

## Testing Strategy

```yaml
coverage_target: 80%
mock_strategy: |
  - Mock API interfaces (paramapi.API, secretapi.API)
  - Mock store interfaces for staging
focus_areas:
  - Use case orchestration logic
  - Error mapping and propagation
  - Input validation
skip_areas:
  - AWS SDK behavior (tested via e2e)
```

## Notes

### Use Case Pattern

Each use case follows:
```go
type XxxUseCase struct {
    API       api.Interface
    Store     store.Interface  // for staging
}

type XxxInput struct { ... }
type XxxOutput struct { ... }

func (u *XxxUseCase) Execute(ctx, input) (*XxxOutput, error)
```

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
