# SSM Parameter Store Commands

[← Back to README](../README.md) | [Secrets Manager Commands →](sm.md)

Service aliases: `ssm`, `ps`, `param`

## suve ssm show

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

## suve ssm cat

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

## suve ssm log

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

## suve ssm diff

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

## suve ssm ls

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

## suve ssm set

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

## suve ssm rm

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
