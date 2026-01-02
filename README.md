# suve

[![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/suve.svg)](https://pkg.go.dev/github.com/mpyw/suve)
[![Test](https://github.com/mpyw/suve/actions/workflows/test.yml/badge.svg)](https://github.com/mpyw/suve/actions/workflows/test.yml)
[![Codecov](https://codecov.io/gh/mpyw/suve/graph/badge.svg)](https://codecov.io/gh/mpyw/suve)
[![Go Report Card](https://goreportcard.com/badge/github.com/mpyw/suve)](https://goreportcard.com/report/github.com/mpyw/suve)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> [!NOTE]
> This project was written by AI (Claude Code).

A **Git-like CLI** for AWS Parameter Store and Secrets Manager. Familiar commands like `show`, `log`, `diff`, and a **staging workflow** for safe, reviewable changes.

## Features

- **Git-like commands**: `show`, `log`, `diff`, `cat`, `ls`, `set`, `delete`
- **Staging workflow**: `edit` → `status` → `diff` → `apply` (review changes before applying)
- **Version navigation**: `#VERSION`, `~SHIFT`, `:LABEL` syntax
- **Colored diff output**: Easy-to-read unified diff format
- **Both services**: SSM Parameter Store and Secrets Manager

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
go tool suve param show /my/param
```

> [!TIP]
> **Using with aws-vault**: Wrap commands with `aws-vault exec` for temporary credentials:
> ```bash
> aws-vault exec my-profile -- suve param show /my/param
> ```

## Getting Started

### Basic Commands

```ShellSession
user@host:~$ suve param show /app/config/database-url
Name: /app/config/database-url
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  postgres://db.example.com:5432/myapp

user@host:~$ suve param show --raw /app/config/database-url
postgres://db.example.com:5432/myapp
```

The `show` command displays value with metadata; `--raw` outputs raw value for piping:

```bash
# Use in scripts
DB_URL=$(suve param show --raw /app/config/database-url)

# Pipe to file
suve param show --raw /app/config/ssl-cert > cert.pem
```

### Version History with `log`

View version history, just like `git log`:

```ShellSession
user@host:~$ suve param log /app/config/database-url
Version 3 (current)
Date: 2024-01-15T10:30:45Z
postgres://db.example.com:5432/myapp...

Version 2
Date: 2024-01-14T09:20:30Z
postgres://old-db.example.com:5432/myapp...

Version 1
Date: 2024-01-13T08:10:00Z
postgres://localhost:5432/myapp...
```

Use `--patch` to see what changed in each version:

```ShellSession
user@host:~$ suve param log --patch /app/config/database-url
Version 3 (current)
Date: 2024-01-15T10:30:45Z

--- /app/config/database-url#2
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://old-db.example.com:5432/myapp
+postgres://db.example.com:5432/myapp

Version 2
Date: 2024-01-14T09:20:30Z

--- /app/config/database-url#1
+++ /app/config/database-url#2
@@ -1 +1 @@
-postgres://localhost:5432/myapp
+postgres://old-db.example.com:5432/myapp
```

> [!TIP]
> Add `--parse-json` flag to pretty-print JSON values when viewing diffs:
> ```bash
> suve param log --patch --parse-json /app/config/credentials
> ```

### Comparing Versions with `diff`

Compare previous version with latest (most common use case):

```ShellSession
user@host:~$ suve param diff /app/config/database-url~
--- /app/config/database-url#2
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://old-db.example.com:5432/myapp
+postgres://db.example.com:5432/myapp
```

Compare any two specific versions:

```ShellSession
user@host:~$ suve param diff /app/config/database-url#1 /app/config/database-url#3
--- /app/config/database-url#1
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://localhost:5432/myapp
+postgres://db.example.com:5432/myapp
```

### Staging Workflow

> [!IMPORTANT]
> The staging workflow lets you prepare changes locally, review them, and apply when ready—just like `git add` → `git diff --staged` → `git commit`.

**1. Stage changes** (opens editor or accepts value directly):

```ShellSession
user@host:~$ suve stage param add /app/config/new-param "my-value"
✓ Staged for creation: /app/config/new-param

user@host:~$ suve stage param edit /app/config/database-url
✓ Staged: /app/config/database-url

user@host:~$ suve stage param delete /app/config/old-param
✓ Staged for deletion: /app/config/old-param
```

**2. Review staged changes**:

```ShellSession
user@host:~$ suve stage status
Staged SSM changes (3):
  A /app/config/new-param
  M /app/config/database-url
  D /app/config/old-param

user@host:~$ suve stage diff
--- /app/config/database-url#3 (AWS)
+++ /app/config/database-url (staged)
@@ -1 +1 @@
-postgres://db.example.com:5432/myapp
+postgres://new-db.example.com:5432/myapp

--- /app/config/new-param (not in AWS)
+++ /app/config/new-param (staged for creation)
@@ -0,0 +1 @@
+my-value
```

**3. Apply changes**:

```ShellSession
user@host:~$ suve stage apply
Applying SSM Parameter Store parameters...
✓ Created /app/config/new-param
✓ Updated /app/config/database-url
✓ Deleted /app/config/old-param
```

**Reset if needed**:

```bash
# Unstage specific parameter
suve stage param reset /app/config/database-url

# Unstage all
suve stage reset
```

> [!CAUTION]
> `suve stage apply` applies changes to AWS immediately. Always review with `suve stage diff` first!

## Version Specification

Navigate versions with Git-like syntax:

```
# SSM Parameter Store
<name>[#VERSION][~SHIFT]*

# Secrets Manager
<name>[#VERSION | :LABEL][~SHIFT]*

where ~SHIFT = ~ | ~N  (repeatable, cumulative)
```

| Syntax | Description | Service |
|--------|-------------|---------|
| `/my/param` | Latest version | SSM |
| `/my/param#3` | Version 3 | SSM |
| `/my/param~1` | 1 version ago | SSM |
| `/my/param#5~2` | Version 5 minus 2 = Version 3 | SSM |
| `/my/param~~` | 2 versions ago (`~1~1`) | SSM |
| `my-secret` | Current (AWSCURRENT) | SM |
| `my-secret:AWSPREVIOUS` | Previous staging label | SM |
| `my-secret#abc123` | Specific version ID | SM |
| `my-secret~1` | 1 version ago | SM |

> [!TIP]
> `~` without a number means `~1`. You can chain them: `~~` = `~1~1` = `~2`

## Command Reference

### Services

| Service | Aliases | Documentation |
|---------|---------|---------------|
| SSM Parameter Store | `param`, `ssm`, `ps` | [docs/ssm.md](docs/ssm.md) |
| Secrets Manager | `secret`, `sm` | [docs/sm.md](docs/sm.md) |

### SSM Parameter Store

| Command | Description |
|---------|-------------|
| `suve param show <name>` | Display parameter with metadata |
| `suve param show --raw <name>` | Output raw value (for piping) |
| `suve param log <name>` | Show version history |
| `suve param diff <spec1> [spec2]` | Compare versions |
| `suve param list [path]` | List parameters |
| `suve param set <name> <value>` | Create or update parameter |
| `suve param delete <name>` | Delete parameter |

**Staging commands** (under `suve stage param`):

| Command | Description |
|---------|-------------|
| `suve stage param add <name> [value]` | Stage new parameter |
| `suve stage param edit <name> [value]` | Stage modification |
| `suve stage param delete <name>` | Stage deletion |
| `suve stage param status` | Show staged changes |
| `suve stage param diff [name]` | Compare staged vs AWS |
| `suve stage param apply [name]` | Apply staged changes |
| `suve stage param reset [name]` | Unstage changes |

### Secrets Manager

| Command | Description |
|---------|-------------|
| `suve secret show <name>` | Display secret with metadata |
| `suve secret show --raw <name>` | Output raw value (for piping) |
| `suve secret log <name>` | Show version history |
| `suve secret diff <spec1> [spec2]` | Compare versions |
| `suve secret list [prefix]` | List secrets |
| `suve secret create <name> <value>` | Create new secret |
| `suve secret update <name> <value>` | Update existing secret |
| `suve secret delete <name>` | Delete secret |
| `suve secret restore <name>` | Restore deleted secret |

**Staging commands** (under `suve stage secret`):

| Command | Description |
|---------|-------------|
| `suve stage secret add <name> [value]` | Stage new secret |
| `suve stage secret edit <name> [value]` | Stage modification |
| `suve stage secret delete <name>` | Stage deletion |
| `suve stage secret status` | Show staged changes |
| `suve stage secret diff [name]` | Compare staged vs AWS |
| `suve stage secret apply [name]` | Apply staged changes |
| `suve stage secret reset [name]` | Unstage changes |

### Global Stage Commands

| Command | Description |
|---------|-------------|
| `suve stage status` | Show all staged changes |
| `suve stage diff` | Compare all staged vs AWS |
| `suve stage apply` | Apply all staged changes |
| `suve stage reset` | Unstage all changes |

## AWS Configuration

suve uses standard AWS SDK configuration:

**Authentication** (in order of precedence):
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (EC2, ECS, Lambda)

**Region**:
- `AWS_REGION` or `AWS_DEFAULT_REGION` environment variable
- `~/.aws/config` file

> [!WARNING]
> Ensure your IAM role/user has appropriate permissions:
> - SSM: `ssm:GetParameter`, `ssm:GetParameterHistory`, `ssm:PutParameter`, `ssm:DeleteParameter`, `ssm:DescribeParameters`
> - SM: `secretsmanager:GetSecretValue`, `secretsmanager:ListSecretVersionIds`, `secretsmanager:PutSecretValue`, `secretsmanager:CreateSecret`, `secretsmanager:DeleteSecret`, `secretsmanager:RestoreSecret`

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

# Coverage (unit + E2E combined)
make coverage-all
```

## License

MIT License
