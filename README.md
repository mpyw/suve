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

---

## Parameter Store Commands (ssm)

Service aliases: `ssm`, `ps`, `param`

### suve ssm show

Display parameter value with metadata.

```
suve ssm show [options] <name[#N][~...]>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--decrypt` | `-d` | `true` | Decrypt SecureString values. Use `--decrypt=false` to disable. |

**Output:**

```
Name: /my/parameter
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  my-secret-value
```

**Examples:**

```bash
# Show latest version
suve ssm show /app/config/database-url

# Show specific version
suve ssm show /app/config/database-url#3

# Show previous version
suve ssm show /app/config/database-url~1

# Show without decryption (displays encrypted value)
suve ssm show --decrypt=false /app/config/database-url
```

---

### suve ssm cat

Output raw parameter value without metadata. Designed for piping and scripting.

```
suve ssm cat [options] <name[#N][~...]>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--decrypt` | `-d` | `true` | Decrypt SecureString values. Use `--decrypt=false` to disable. |

**Output:**

Raw value without trailing newline.

**Examples:**

```bash
# Use in scripts
DB_URL=$(suve ssm cat /app/config/database-url)

# Pipe to file
suve ssm cat /app/config/ssl-cert > cert.pem

# Pipe to another command
suve ssm cat /app/config/ssh-key | ssh-add -
```

---

### suve ssm log

Show parameter version history.

```
suve ssm log [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (without version specifier) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--number` | `-n` | `10` | Maximum number of versions to show |

**Output:**

```
Version 3 (current)
Date: 2024-01-15T10:30:45Z
my-secret-value...

Version 2
Date: 2024-01-14T09:20:30Z
previous-value...

Version 1
Date: 2024-01-13T08:10:00Z
initial-value...
```

Values are truncated at 50 characters with `...` suffix.

**Examples:**

```bash
# Show last 10 versions (default)
suve ssm log /app/config/database-url

# Show last 5 versions
suve ssm log -n 5 /app/config/database-url
```

---

### suve ssm diff

Show differences between two parameter versions.

```
suve ssm diff <name> <version1> [version2]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Parameter name |
| `version1` | Yes | First version specifier (e.g., `#1`, `~2`) |
| `version2` | No | Second version specifier. If omitted, compares `version1` with latest. |

**Behavior:**

- `suve ssm diff /param #1 #2` - Compare version 1 with version 2
- `suve ssm diff /param #2` - Compare latest with version 2

**Output:**

```diff
--- /my/param#2
+++ /my/param#3
@@ -1 +1 @@
-old-value
+new-value
```

Output is colorized: red for deletions, green for additions.

**Examples:**

```bash
# Compare two specific versions
suve ssm diff /app/config/database-url #1 #2

# Compare latest with version 2
suve ssm diff /app/config/database-url #2

# Compare using relative versions
suve ssm diff /app/config/database-url '~2' '~1'
```

---

### suve ssm ls

List parameters.

```
suve ssm ls [options] [path-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `path-prefix` | No | Filter by path prefix (e.g., `/app/config/`) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--recursive` | `-r` | `false` | List parameters recursively under the path |

**Output:**

One parameter name per line.

```
/app/config/database-url
/app/config/api-key
/app/config/redis-host
```

**Behavior:**

Without `--recursive`, only lists parameters at the specified path level (OneLevel). With `--recursive`, lists all parameters under the path including nested paths.

**Examples:**

```bash
# List all parameters (no filter)
suve ssm ls

# List parameters under /app/config/
suve ssm ls /app/config/

# List recursively (includes /app/config/nested/param)
suve ssm ls -r /app/
```

---

### suve ssm set

Create or update a parameter value.

```
suve ssm set [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `value` | Parameter value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--type` | `-t` | `String` | Parameter type: `String`, `StringList`, or `SecureString` |
| `--secure` | `-s` | `false` | Shorthand for `--type SecureString` |
| `--description` | `-d` | - | Parameter description |

> **Note:** `--secure` and `--type` cannot be used together.

**Output:**

```
✓ Set parameter /app/config/database-url (version: 1)
```

**Notes:**

- Always overwrites existing parameters (no confirmation)
- SecureString is encrypted using the default AWS KMS key (requires KMS permissions)

**Examples:**

```bash
# Create/update as String (default)
suve ssm set /app/config/log-level "debug"

# Create as SecureString (shorthand)
suve ssm set --secure /app/config/database-url "postgres://..."
suve ssm set -s /app/config/api-key "sk-1234567890"

# Create as SecureString (explicit)
suve ssm set --type SecureString /app/config/database-url "postgres://..."

# Create with description
suve ssm set -d "Database connection string" -s /app/config/database-url "postgres://..."

# StringList (comma-separated values)
suve ssm set --type StringList /app/config/allowed-hosts "host1,host2,host3"
```

---

### suve ssm rm

Delete a parameter.

```
suve ssm rm <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name to delete |

**Output:**

```
Deleted /app/config/database-url
```

**Notes:**

- Deletion is immediate and permanent
- No confirmation prompt
- Deleting non-existent parameter results in an error

**Examples:**

```bash
suve ssm rm /app/config/old-param
```

---

## Secrets Manager Commands (sm)

Service aliases: `sm`, `secret`

### suve sm show

Display secret value with metadata.

```
suve sm show [options] <name[#id | :label][~...]>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Pretty-print JSON values with indentation |

**Output:**

```
Name: my-secret
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
Created: 2024-01-15T10:30:45Z

  {"username":"admin","password":"secret123"}
```

With `--json`:

```
Name: my-secret
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
Created: 2024-01-15T10:30:45Z

  {
    "username": "admin",
    "password": "secret123"
  }
```

**Examples:**

```bash
# Show current version
suve sm show my-database-credentials

# Show with pretty JSON formatting
suve sm show --json my-database-credentials

# Show previous version by label
suve sm show my-database-credentials:AWSPREVIOUS

# Show specific version by ID
suve sm show my-database-credentials#abc12345-1234-1234-1234-123456789012

# Show 1 version ago
suve sm show my-database-credentials~1
```

---

### suve sm cat

Output raw secret value without metadata. Designed for piping and scripting.

```
suve sm cat <name[#id | :label][~...]>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Output:**

Raw value without trailing newline.

**Examples:**

```bash
# Use in scripts
CREDS=$(suve sm cat my-database-credentials)

# Extract JSON field with jq
suve sm cat my-database-credentials | jq -r '.password'

# Pipe to file
suve sm cat my-ssl-certificate > cert.pem
```

---

### suve sm log

Show secret version history.

```
suve sm log [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (without version specifier) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--number` | `-n` | `10` | Maximum number of versions to show |

**Output:**

```
Version abc12345 [AWSCURRENT]
Date: 2024-01-15T10:30:45Z

Version def67890 [AWSPREVIOUS]
Date: 2024-01-14T09:20:30Z

Version ghi11111
Date: 2024-01-13T08:10:00Z
```

Version IDs are truncated to 8 characters for display.

**Examples:**

```bash
# Show version history
suve sm log my-database-credentials

# Show last 5 versions
suve sm log -n 5 my-database-credentials
```

---

### suve sm diff

Show differences between two secret versions.

```
suve sm diff <name> <version1> [version2]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Secret name |
| `version1` | Yes | First version specifier (e.g., `:AWSPREVIOUS`, `~1`) |
| `version2` | No | Second version specifier. If omitted, compares AWSCURRENT with `version1`. |

**Behavior:**

- `suve sm diff secret :AWSPREVIOUS :AWSCURRENT` - Compare previous with current
- `suve sm diff secret :AWSPREVIOUS` - Compare AWSCURRENT with AWSPREVIOUS

**Output:**

```diff
--- my-secret#abc12345
+++ my-secret#def67890
@@ -1 +1 @@
-{"password":"old"}
+{"password":"new"}
```

**Examples:**

```bash
# Compare previous with current
suve sm diff my-database-credentials :AWSPREVIOUS :AWSCURRENT

# Compare current with previous (shorthand)
suve sm diff my-database-credentials :AWSPREVIOUS

# Compare using relative versions
suve sm diff my-database-credentials '~2' '~1'
```

---

### suve sm ls

List secrets.

```
suve sm ls [filter-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `filter-prefix` | No | Filter secrets by name prefix |

**Output:**

One secret name per line.

```
my-database-credentials
my-api-key
production/database-credentials
```

**Examples:**

```bash
# List all secrets
suve sm ls

# List secrets with prefix
suve sm ls production/
```

---

### suve sm create

Create a new secret.

```
suve sm create [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | Secret value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | `-d` | - | Description for the secret |

**Output:**

```
✓ Created secret my-database-credentials (version: abc12345-1234-1234-1234-123456789012)
```

**Notes:**

- Creates a new secret; fails if secret already exists
- Use `suve sm set` to update an existing secret

**Examples:**

```bash
# Create a simple secret
suve sm create my-api-key "sk-1234567890"

# Create with description
suve sm create -d "Production database credentials" my-database-credentials '{"username":"admin","password":"secret"}'
```

---

### suve sm set

Update an existing secret's value.

```
suve sm set <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | New secret value |

**Output:**

```
✓ Updated secret my-database-credentials (version: def67890-1234-1234-1234-123456789012)
```

**Notes:**

- Updates existing secret; fails if secret doesn't exist
- Use `suve sm create` to create a new secret
- Previous version becomes AWSPREVIOUS

**Examples:**

```bash
# Update secret value
suve sm set my-database-credentials '{"username":"admin","password":"newpassword"}'
```

---

### suve sm rm

Delete a secret.

```
suve sm rm [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to delete |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--force` | `-f` | `false` | Delete immediately without recovery window |
| `--recovery-window` | - | `30` | Days before permanent deletion (7-30). Ignored if `--force` is set. |

**Output:**

Without `--force`:
```
! Scheduled deletion of secret my-database-credentials (deletion date: 2024-02-14)
```

With `--force`:
```
! Permanently deleted secret my-database-credentials
```

**Notes:**

- Without `--force`, secret can be restored using `suve sm restore` until the deletion date
- With `--force`, deletion is immediate and irreversible

**Examples:**

```bash
# Delete with 30-day recovery window (default)
suve sm rm my-old-secret

# Delete with 7-day recovery window
suve sm rm --recovery-window 7 my-old-secret

# Delete immediately (no recovery possible)
suve sm rm --force my-old-secret
```

---

### suve sm restore

Restore a deleted secret that is pending deletion.

```
suve sm restore <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to restore |

**Output:**

```
✓ Restored secret my-database-credentials
```

**Notes:**

- Only works for secrets deleted without `--force`
- Must be done before the scheduled deletion date
- Cannot restore secrets that have been permanently deleted

**Examples:**

```bash
suve sm restore my-accidentally-deleted-secret
```

---

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
