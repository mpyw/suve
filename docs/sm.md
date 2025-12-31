# Secrets Manager Commands

[← Back to README](../README.md) | [← SSM Parameter Store Commands](ssm.md)

Service aliases: `sm`, `secret`

## suve sm show

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

## suve sm cat

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

## suve sm log

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

## suve sm diff

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

## suve sm ls

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

## suve sm create

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
- Use `suve sm update` to update an existing secret

**Examples:**

```bash
# Create a simple secret
suve sm create my-api-key "sk-1234567890"

# Create with description
suve sm create -d "Production database credentials" my-database-credentials '{"username":"admin","password":"secret"}'
```

---

## suve sm update

Update an existing secret's value.

```
suve sm update <name> <value>
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
suve sm update my-database-credentials '{"username":"admin","password":"newpassword"}'
```

---

## suve sm rm

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

## suve sm restore

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
