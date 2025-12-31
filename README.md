# suve

[![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/suve.svg)](https://pkg.go.dev/github.com/mpyw/suve)
[![Test](https://github.com/mpyw/suve/actions/workflows/test.yml/badge.svg)](https://github.com/mpyw/suve/actions/workflows/test.yml)
[![Codecov](https://codecov.io/gh/mpyw/suve/graph/badge.svg)](https://codecov.io/gh/mpyw/suve)
[![Go Report Card](https://goreportcard.com/badge/github.com/mpyw/suve)](https://goreportcard.com/report/github.com/mpyw/suve)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> [!NOTE]
> This project was written by AI (Claude Code).

A Git-like CLI for AWS Parameter Store and Secrets Manager.

## Features

- Git-like command structure (`show`, `log`, `diff`, `cat`, `ls`, `set`, `rm`)
- Version specification syntax (`#N`, `~N`, `:LABEL`)
- Colored diff output
- Supports both SSM Parameter Store and Secrets Manager

## Installation

### Using [`go install`](https://pkg.go.dev/cmd/go#hdr-Compile_and_install_packages_and_dependencies)

```bash
go install github.com/mpyw/suve/cmd/suve@latest
```

### Using [`go tool`](https://pkg.go.dev/cmd/go#hdr-Run_specified_go_tool) (Go 1.24+)

```bash
# Add to go.mod as a tool dependency
go get -tool github.com/mpyw/suve/cmd/suve@latest

# Run via go tool
go tool suve ssm show /my/param
```

## Quick Start

```bash
# Parameter Store
suve ssm show /my/param             # Show parameter with metadata
suve ssm cat /my/param              # Output raw value (for piping)
suve ssm set /my/param "value"      # Create or update (String)
suve ssm set -s /my/param "secret"  # Create or update (SecureString)

# Secrets Manager
suve sm show my-secret              # Show secret with metadata
suve sm cat my-secret               # Output raw value (for piping)
suve sm create my-secret "value"    # Create new secret
suve sm set my-secret "value"       # Update existing secret
```

## Version Specification

Git-like revision syntax for specifying versions:

```
# SSM Parameter Store
<name>[#<N>]<shift>*

# Secrets Manager
<name>[#<id> | :<label>]<shift>*

where <shift> = ~ | ~<N>  (repeatable, cumulative)
```

| Syntax | Description | Service |
|--------|-------------|---------|
| `/my/param` | Latest version | SSM |
| `/my/param#3` | Version 3 | SSM |
| `/my/param~1` | 1 version ago from latest | SSM |
| `/my/param#5~2` | 2 versions before version 5 (= version 3) | SSM |
| `/my/param~~` | 2 versions ago (same as `~1~1`) | SSM |
| `my-secret` | Latest version (AWSCURRENT) | SM |
| `my-secret#abc123` | Specific version by ID | SM |
| `my-secret:AWSCURRENT` | Current staging label | SM |
| `my-secret:AWSPREVIOUS` | Previous staging label | SM |
| `my-secret~1` | 1 version ago (by creation date) | SM |
| `my-secret:AWSCURRENT~1` | 1 before AWSCURRENT | SM |

## Command Reference

| Service | Aliases | Documentation |
|---------|---------|---------------|
| SSM Parameter Store | `ssm`, `ps`, `param` | [docs/ssm.md](docs/ssm.md) |
| Secrets Manager | `sm`, `secret` | [docs/sm.md](docs/sm.md) |

### SSM Parameter Store Commands

| Command | Description |
|---------|-------------|
| [`suve ssm show`](docs/ssm.md#suve-ssm-show) | Display parameter value with metadata |
| [`suve ssm cat`](docs/ssm.md#suve-ssm-cat) | Output raw parameter value (for piping) |
| [`suve ssm log`](docs/ssm.md#suve-ssm-log) | Show parameter version history |
| [`suve ssm diff`](docs/ssm.md#suve-ssm-diff) | Show differences between versions |
| [`suve ssm ls`](docs/ssm.md#suve-ssm-ls) | List parameters |
| [`suve ssm set`](docs/ssm.md#suve-ssm-set) | Create or update a parameter |
| [`suve ssm rm`](docs/ssm.md#suve-ssm-rm) | Delete a parameter |

### Secrets Manager Commands

| Command | Description |
|---------|-------------|
| [`suve sm show`](docs/sm.md#suve-sm-show) | Display secret value with metadata |
| [`suve sm cat`](docs/sm.md#suve-sm-cat) | Output raw secret value (for piping) |
| [`suve sm log`](docs/sm.md#suve-sm-log) | Show secret version history |
| [`suve sm diff`](docs/sm.md#suve-sm-diff) | Show differences between versions |
| [`suve sm ls`](docs/sm.md#suve-sm-ls) | List secrets |
| [`suve sm create`](docs/sm.md#suve-sm-create) | Create a new secret |
| [`suve sm set`](docs/sm.md#suve-sm-set) | Update an existing secret |
| [`suve sm rm`](docs/sm.md#suve-sm-rm) | Delete a secret |
| [`suve sm restore`](docs/sm.md#suve-sm-restore) | Restore a deleted secret |

## AWS Configuration

suve uses the standard AWS SDK configuration. Authentication is resolved in the following order:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (EC2, ECS, Lambda)

Set the region using:
- `AWS_REGION` environment variable
- `AWS_DEFAULT_REGION` environment variable
- `~/.aws/config` file

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
