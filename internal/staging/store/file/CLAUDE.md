# staging/store/file

## Scope

```yaml
path: internal/staging/store/file
type: package
parent: ../CLAUDE.md
```

## Overview

File-based staging storage with optional encryption. Stores staging state as separate JSON files per service:
- `~/.suve/{accountID}/{region}/param.json` for Parameter Store changes
- `~/.suve/{accountID}/{region}/secret.json` for Secrets Manager changes

Supports Argon2id key derivation with AES-256-GCM encryption for secure stash persistence.

## Architecture

```yaml
key_types:
  - name: Store
    role: Service-specific FileStore implementation with encryption support
  - name: CompositeStore
    role: Wrapper for multiple service stores for global operations

files:
  - store.go           # Store struct, NewStore, NewStoreWithPassphrase
  - internal/crypt/    # Encryption primitives (Argon2, AES-GCM)

dependencies:
  internal:
    - internal/staging       # State type
    - internal/staging/store # FileStore interface
  external:
    - golang.org/x/crypto/argon2
```

## Testing Strategy

```yaml
coverage_target: 85%
mock_strategy: |
  - Test with temp directories
  - Test encrypted/unencrypted round-trips
focus_areas:
  - Encryption/decryption correctness
  - File existence checks
  - Error handling (wrong passphrase, corrupted file)
skip_areas:
  - Argon2 algorithm correctness (external library)
```

## Notes

### File Format

Each service file is self-describing with the V3 format:

Unencrypted:
```json
{"version":3,"service":"param","entries":{...},"tags":{...}}
```

Encrypted: Binary format with salt prefix + AES-GCM ciphertext

### Passphrase Handling

- Empty passphrase = plaintext storage
- Non-empty passphrase = encrypted storage
- Wrong passphrase returns authentication error

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
