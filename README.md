# suve

[![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/suve.svg)](https://pkg.go.dev/github.com/mpyw/suve)
[![Test](https://github.com/mpyw/suve/actions/workflows/test.yml/badge.svg)](https://github.com/mpyw/suve/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> [!NOTE]
> This project was written by AI (Claude Code).

A Git-like CLI for AWS Parameter Store and Secrets Manager.

## Features

- Git-like command structure (`show`, `log`, `diff`, `cat`, `ls`, `set`, `rm`)
- Version specification syntax (`@N`, `~N`, `:LABEL`)
- Colored diff output
- Supports both SSM Parameter Store and Secrets Manager

## Installation

### Using [Releases](https://github.com/mpyw/suve/releases) (Recommended)

Download pre-built binaries from the [Releases](https://github.com/mpyw/suve/releases) page. Binaries are signed with [sigstore/cosign](https://docs.sigstore.dev/cosign/overview/).

```bash
# Example for macOS (Apple Silicon)
curl -LO https://github.com/mpyw/suve/releases/latest/download/suve-darwin-arm64
chmod +x suve-darwin-arm64
mv suve-darwin-arm64 /usr/local/bin/suve
```

### Using [`go install`](https://pkg.go.dev/cmd/go#hdr-Compile_and_install_packages_and_dependencies)

```bash
GOEXPERIMENT=jsonv2 go install github.com/mpyw/suve/cmd/suve@latest
```

> [!IMPORTANT]
> This project uses Go 1.25's experimental `encoding/json/v2`. The `GOEXPERIMENT=jsonv2` flag is required.

## Usage

```bash
# Basic structure
suve <service> <command> [options] [arguments]

# Services
suve ssm ...  # Parameter Store
suve sm ...   # Secrets Manager
```

### Parameter Store (ssm)

```bash
# Show parameter with metadata
suve ssm show /my/param
suve ssm show /my/param@3      # Specific version
suve ssm show /my/param~1      # 1 version ago

# Raw value output (for piping)
suve ssm cat /my/param

# Show version history
suve ssm log /my/param [-n 10]

# Show diff between versions
suve ssm diff /my/param @1 @2

# List parameters
suve ssm ls /my/prefix/ [-r]

# Set parameter
suve ssm set /my/param "value" [--type SecureString]

# Delete parameter
suve ssm rm /my/param
```

### Secrets Manager (sm)

```bash
# Show secret with metadata
suve sm show my-secret
suve sm show my-secret:AWSCURRENT    # By staging label
suve sm show my-secret:AWSPREVIOUS

# Raw value output
suve sm cat my-secret

# Show version history
suve sm log my-secret [-n 10]

# Show diff between versions
suve sm diff my-secret :AWSPREVIOUS :AWSCURRENT

# List secrets
suve sm ls [--filter prefix]

# Create new secret
suve sm create my-secret "value" [--description "desc"]

# Update secret value
suve sm set my-secret "new-value"

# Delete secret
suve sm rm my-secret [--force] [--recovery-window 7]

# Restore deleted secret
suve sm restore my-secret
```

## Version Specification

Git-like revision syntax for specifying versions:

```
<name>[@<version>][~<shift>][:label]

Examples:
/my/param           # Latest version
/my/param@3         # Version 3
/my/param~1         # 1 version ago
/my/param@5~2       # 2 versions before version 5 (= version 3)
my-secret:AWSCURRENT
my-secret:AWSPREVIOUS
```

## Output Examples

### show command

```
Name: /my/parameter
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  my-secret-value
```

### diff command

```diff
--- /my/param@2
+++ /my/param@3
@@ -1 +1 @@
-old-value
+new-value
```

## Verifying Release Signatures

Releases are signed with [sigstore/cosign](https://docs.sigstore.dev/cosign/overview/):

```bash
# Download checksums and signature
curl -LO https://github.com/mpyw/suve/releases/latest/download/checksums.txt
curl -LO https://github.com/mpyw/suve/releases/latest/download/checksums.txt.sig
curl -LO https://github.com/mpyw/suve/releases/latest/download/checksums.txt.pem

# Verify signature
cosign verify-blob \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/mpyw/suve/.github/workflows/release.yml@refs/tags/' \
  checksums.txt

# Verify binary checksum
sha256sum -c checksums.txt
```

## Documentation

- [CLAUDE.md](./CLAUDE.md) - AI assistant guidance for development

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Build CLI
make build

# Run E2E tests (requires Docker)
make e2e
```

## License

MIT License
