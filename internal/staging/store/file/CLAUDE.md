# staging/store/file

## Scope

```yaml
path: internal/staging/store/file
type: package
parent: ../CLAUDE.md
```

## Overview

File-based staging storage with optional encryption. Stores staging state as JSON in `~/.suve/{accountID}/{region}/stage.json`. Supports Argon2id key derivation with AES-256-GCM encryption for secure stash persistence.

## Architecture

```yaml
key_types:
  - name: Store
    role: FileStore implementation with encryption support
  - name: StoreOption
    role: Functional options for Store configuration

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

Unencrypted:
```json
{"version":1,"entries":{...},"tags":{...}}
```

Encrypted: Binary format with salt prefix + AES-GCM ciphertext

### Passphrase Handling

- Empty passphrase = plaintext storage
- Non-empty passphrase = encrypted storage
- Wrong passphrase returns authentication error

### Delete Operation

The `Delete()` method removes the stash file without reading its contents.
This allows global stash drop to work on encrypted files without requiring a passphrase.

## References

```yaml
related_docs:
  - ../CLAUDE.md
```
