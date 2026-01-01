# SSM Parameter Store Commands

[← Back to README](../README.md) | [Secrets Manager Commands →](sm.md)

Service aliases: `ssm`, `ps`, `param`

## suve ssm show

Display parameter value with metadata.

```
suve ssm show [options] <name[#VERSION][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--decrypt` | `-d` | `true` | Decrypt SecureString values. Use `--decrypt=false` to disable. |
| `--json` | `-j` | `false` | Pretty-print JSON values with indentation |

**Output:**

```
Name: /my/parameter
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  my-secret-value
```

With `--json`:

```
Name: /my/parameter
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
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

# Show with JSON formatting
suve ssm show -j /app/config/database-credentials

```

---

## suve ssm cat

Output raw parameter value without metadata. Designed for piping and scripting.

```
suve ssm cat [options] <name[#VERSION][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--decrypt` | `-d` | `true` | Decrypt SecureString values. Use `--decrypt=false` to disable. |
| `--json` | `-j` | `false` | Pretty-print JSON values with indentation |

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

# Pretty print JSON
suve ssm cat -j /app/config/database-credentials

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
| `--patch` | `-p` | `false` | Show diff between consecutive versions |
| `--json` | `-j` | `false` | Format JSON values before diffing (use with `-p`) |
| `--reverse` | `-r` | `false` | Show oldest versions first |

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

**With `--patch`:**

```diff
Version 3 (current)
Date: 2024-01-15T10:30:45Z

--- /app/config/database-url#2
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://old-host:5432/db
+postgres://new-host:5432/db

Version 2
Date: 2024-01-14T09:20:30Z

--- /app/config/database-url#1
+++ /app/config/database-url#2
...
```

> [!TIP]
> Use `-p` to review what changed in each version, similar to `git log -p`.

**Examples:**

```bash
# Show last 10 versions (default)
suve ssm log /app/config/database-url

# Show last 5 versions
suve ssm log -n 5 /app/config/database-url

# Show versions with diffs
suve ssm log -p /app/config/database-url

# Show diffs with JSON formatting
suve ssm log -p -j /app/config/database-credentials

# Show last 3 versions with diffs
suve ssm log -n 3 -p /app/config/database-url

# Show oldest versions first
suve ssm log --reverse /app/config/database-url
```

---

## suve ssm diff

Show differences between two parameter versions in unified diff format.

```
suve ssm diff <spec1> [spec2] | <name> <version1> [version2]
```

### Argument Formats

The diff command supports multiple argument formats for flexibility:

| Format | Args | Example | Description |
|--------|------|---------|-------------|
| full spec | 2 | `/param#1 /param#2` | Both args include name and version |
| full spec | 1 | `/param#3` | Compare specified version with latest |
| mixed | 2 | `/param#1 '#2'` | First with version, second specifier only |
| partial spec | 2 | `/param '#3'` | Name + specifier → compare with latest |
| partial spec | 3 | `/param '#1' '#2'` | Name + two specifiers |

> [!TIP]
> **Use full spec format** to avoid shell quoting issues. When `#` appears at the start of an argument, shells interpret it as a comment. Full spec format embeds the specifier within the path, so no quoting is needed.

> [!NOTE]
> When only one version is specified, it is compared against the **latest** version.

### Version Specifiers

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version number | `#3` = version 3 |
| `~` | One version ago | `~` = latest - 1 |
| `~N` | N versions ago | `~2` = latest - 2 |

Specifiers can be combined: `/param#5~2` means "version 5, then 2 back" = version 3.

### Output

```diff
--- /my/param#2
+++ /my/param#3
@@ -1 +1 @@
-old-value
+new-value
```

Output is colorized when stdout is a TTY:
- **Red**: Deleted lines (`-`)
- **Green**: Added lines (`+`)
- **Cyan**: Headers (`---`, `+++`, `@@`)

### Identical Version Warning

> [!WARNING]
> When comparing versions with **identical content**, no diff is produced. Instead, a warning and hint are displayed:
> ```
> Warning: comparing identical versions
> Hint: To compare with previous version, use: suve ssm diff /param~1
> ```
> This typically happens when you compare the latest version with itself (e.g., `/param` vs `/param`).

### Examples

#### Full Spec Format (Recommended)

```bash
# Compare version 1 with version 2
suve ssm diff /app/config/database-url#1 /app/config/database-url#2

# Compare version 3 with latest
suve ssm diff /app/config/database-url#3

# Compare previous with latest
suve ssm diff /app/config/database-url~1
```

#### Mixed Format

```bash
# Compare version 1 with version 2 (name from first arg)
suve ssm diff /app/config/database-url#1 '#2'
```

#### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting `#` and `~` specifiers to prevent shell interpretation:
> - `#` at argument start is treated as a comment in most shells
> - `~` alone expands to `$HOME` in bash/zsh

```bash
# Compare version 1 with version 2
suve ssm diff /app/config/database-url '#1' '#2'

# Compare version 2 with latest
suve ssm diff /app/config/database-url '#2'

# Compare using relative versions
suve ssm diff /app/config/database-url '~2' '~1'

# Compare previous with latest
suve ssm diff /app/config/database-url '~'
```

#### Practical Use Cases

```bash
# Review what changed in the last update
suve ssm diff /app/config/database-url~1

# Compare any two versions
suve ssm diff /app/config/database-url#1 /app/config/database-url#5

# Pipe to a file for review
suve ssm diff /app/config/database-url~1 > changes.diff
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

---

## Staging Workflow

The staging workflow allows you to prepare changes locally before applying them to AWS. This is useful for:
- Reviewing changes before they go live
- Batch applying multiple changes together
- Avoiding accidental modifications

The stage file is stored at `~/.suve/stage.json`.

### Workflow Overview

```
┌─────────┐    ┌─────────┐    ┌─────────┐
│  edit   │───>│  stage  │───>│  push   │
└─────────┘    └─────────┘    └─────────┘
     │              │              │
     │              │              v
     │              │         Applied to AWS
     │              │
     │              v
     │         status (view)
     │         diff --staged (compare)
     │         reset (unstage)
     │              │
     │              v
     │         Discarded
     └──────────────┘
```

---

## suve ssm edit

Edit a parameter value in your editor and stage the changes.

```
suve ssm edit [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--delete` | `-d` | `false` | Stage parameter for deletion |

**Behavior:**

1. If the parameter is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`)
4. If the value changed, stages the new value

**Output:**

```
Staged /app/config/database-url
```

If no changes were made:
```
No changes made
```

**Examples:**

```bash
# Edit a parameter
suve ssm edit /app/config/database-url

# Stage a parameter for deletion
suve ssm edit --delete /app/config/old-param
```

---

## suve ssm status

Show staged changes for SSM parameters.

```
suve ssm status
```

**Output:**

```
SSM Parameter Store:
  set    /app/config/database-url
  delete /app/config/old-param
```

If no changes are staged:
```
SSM Parameter Store:
  (no staged changes)
```

**Examples:**

```bash
# Show SSM staged changes
suve ssm status

# Show all staged changes (SSM + SM)
suve status
```

---

## suve ssm push

Apply staged SSM parameter changes to AWS.

```
suve ssm push
```

**Behavior:**

1. Reads all staged SSM changes
2. For each `set` operation: calls PutParameter
3. For each `delete` operation: calls DeleteParameter
4. Removes successfully applied changes from stage
5. Keeps failed changes in stage for retry

**Output:**

```
Pushing SSM parameters...
✓ /app/config/database-url
✓ /app/config/old-param (deleted)
```

If nothing is staged:
```
SSM: nothing to push
```

**Examples:**

```bash
# Push SSM changes only
suve ssm push

# Push all changes (SSM + SM)
suve push
```

---

## suve ssm reset

Unstage SSM parameter changes.

```
suve ssm reset [name]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Specific parameter to unstage (if omitted, unstages all) |

**Output:**

Specific parameter:
```
Unstaged /app/config/database-url
```

All parameters:
```
Unstaged all SSM changes
```

**Examples:**

```bash
# Unstage a specific parameter
suve ssm reset /app/config/database-url

# Unstage all SSM parameters
suve ssm reset

# Unstage everything (SSM + SM)
suve reset
```

---

## suve ssm diff --staged

Compare staged value with current AWS value.

```
suve ssm diff --staged <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (without version specifier) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--staged` | - | `false` | Compare staged value with AWS current |
| `--json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Output:**

```diff
--- /app/config/database-url (AWS)
+++ /app/config/database-url (staged)
@@ -1 +1 @@
-postgres://old-host:5432/db
+postgres://new-host:5432/db
```

If parameter is staged for deletion:
```diff
--- /app/config/old-param (AWS)
+++ /app/config/old-param (staged for deletion)
@@ -1 +0,0 @@
-old-value
```

**Examples:**

```bash
# Compare staged vs AWS
suve ssm diff --staged /app/config/database-url

# Compare with JSON formatting
suve ssm diff --staged --json /app/config/database-credentials
```
