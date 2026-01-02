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

**Examples:**

```ShellSession
user@host:~$ suve ssm show /app/config/database-url
Name: /app/config/database-url
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  postgres://db.example.com:5432/myapp
```

With `--json` for JSON values:

```ShellSession
user@host:~$ suve ssm show -j /app/config/credentials
Name: /app/config/credentials
Version: 2
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
```

```bash
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

**Examples:**

```ShellSession
user@host:~$ suve ssm cat /app/config/database-url
postgres://db.example.com:5432/myapp
```

> [!TIP]
> Use `cat` for scripting and piping. The output has no trailing newline.

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

Show parameter version history, similar to `git log`.

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

**Examples:**

Basic version history:

```ShellSession
user@host:~$ suve ssm log /app/config/database-url
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

> [!NOTE]
> Values are truncated at 50 characters with `...` suffix.

With `--patch` to see what changed:

```ShellSession
user@host:~$ suve ssm log -p /app/config/database-url
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
> Use `-p` to review what changed in each version, similar to `git log -p`.

```bash
# Show last 5 versions
suve ssm log -n 5 /app/config/database-url

# Show diffs with JSON formatting for JSON values
suve ssm log -p -j /app/config/database-credentials

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

### Examples

Compare two versions:

```ShellSession
user@host:~$ suve ssm diff /app/config/database-url#1 /app/config/database-url#3
--- /app/config/database-url#1
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://localhost:5432/myapp
+postgres://db.example.com:5432/myapp
```

Compare previous with latest (shorthand):

```ShellSession
user@host:~$ suve ssm diff /app/config/database-url~1
--- /app/config/database-url#2
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://old-db.example.com:5432/myapp
+postgres://db.example.com:5432/myapp
```

> [!NOTE]
> Output is colorized when stdout is a TTY:
> - **Red**: Deleted lines (`-`)
> - **Green**: Added lines (`+`)
> - **Cyan**: Headers (`---`, `+++`, `@@`)

### Identical Version Warning

> [!WARNING]
> When comparing versions with **identical content**, no diff is produced:
> ```
> Warning: comparing identical versions
> Hint: To compare with previous version, use: suve ssm diff /param~1
> ```

### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting `#` and `~` specifiers to prevent shell interpretation:
> - `#` at argument start is treated as a comment in most shells
> - `~` alone expands to `$HOME` in bash/zsh

```bash
# Compare version 1 with version 2
suve ssm diff /app/config/database-url '#1' '#2'

# Compare previous with latest
suve ssm diff /app/config/database-url '~'

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

**Examples:**

```ShellSession
user@host:~$ suve ssm ls /app/config/
/app/config/database-url
/app/config/api-key
/app/config/redis-host
```

Recursive listing:

```ShellSession
user@host:~$ suve ssm ls -r /app/
/app/config/database-url
/app/config/api-key
/app/config/nested/param
/app/secrets/api-token
```

> [!NOTE]
> Without `--recursive`, only lists parameters at the specified path level (OneLevel). With `--recursive`, lists all parameters under the path including nested paths.

```bash
# List all parameters (no filter)
suve ssm ls

# List parameters under /app/config/
suve ssm ls /app/config/

# List recursively
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

> [!NOTE]
> `--secure` and `--type` cannot be used together.

**Examples:**

```ShellSession
user@host:~$ suve ssm set --secure /app/config/database-url "postgres://db.example.com:5432/myapp"
✓ Set parameter /app/config/database-url (version: 1)
```

```bash
# Create/update as String (default)
suve ssm set /app/config/log-level "debug"

# Create as SecureString (shorthand)
suve ssm set -s /app/config/api-key "sk-1234567890"

# Create with description
suve ssm set -d "Database connection string" -s /app/config/database-url "postgres://..."

# StringList (comma-separated values)
suve ssm set --type StringList /app/config/allowed-hosts "host1,host2,host3"
```

> [!IMPORTANT]
> SecureString is encrypted using the default AWS KMS key. Ensure your IAM role has the necessary KMS permissions.

---

## suve ssm delete

Delete a parameter.

```
suve ssm delete <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name to delete |

**Examples:**

```ShellSession
user@host:~$ suve ssm delete /app/config/old-param
Deleted /app/config/old-param
```

> [!CAUTION]
> Deletion is immediate and permanent. There is no confirmation prompt or recovery option.

---

## Staging Workflow

The staging workflow allows you to prepare changes locally before applying them to AWS.

> [!IMPORTANT]
> The staging workflow lets you prepare changes locally, review them, and apply when ready—just like `git add` → `git diff --staged` → `git commit`.

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
     │         diff (compare)
     │         reset (unstage)
     │              │
     │              v
     │         Discarded
     └──────────────┘
```

---

## suve stage ssm add

Stage a new parameter or modification.

```
suve stage ssm add [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Parameter name |
| `value` | No | Parameter value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--type` | `-t` | `String` | Parameter type: `String`, `StringList`, or `SecureString` |
| `--secure` | `-s` | `false` | Shorthand for `--type SecureString` |
| `--description` | `-d` | - | Parameter description |

**Examples:**

```ShellSession
user@host:~$ suve stage ssm add /app/config/new-param "my-value"
✓ Staged for creation: /app/config/new-param
```

```bash
# Stage with inline value
suve stage ssm add /app/config/new-param "my-value"

# Stage via editor
suve stage ssm add /app/config/new-param

# Stage as SecureString
suve stage ssm add -s /app/config/api-key "sk-1234567890"
```

---

## suve stage ssm edit

Edit an existing parameter and stage the changes.

```
suve stage ssm edit [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Parameter name |
| `value` | No | New value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--delete` | `-d` | `false` | Stage parameter for deletion |

**Behavior:**

1. If the parameter is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`) if value not provided
4. If the value changed, stages the new value

**Examples:**

```ShellSession
user@host:~$ suve stage ssm edit /app/config/database-url
✓ Staged: /app/config/database-url
```

```bash
# Edit via editor
suve stage ssm edit /app/config/database-url

# Edit with inline value
suve stage ssm edit /app/config/database-url "new-value"

# Stage for deletion
suve stage ssm edit --delete /app/config/old-param
```

---

## suve stage ssm delete

Stage a parameter for deletion.

```
suve stage ssm delete <name>
```

**Examples:**

```ShellSession
user@host:~$ suve stage ssm delete /app/config/old-param
✓ Staged for deletion: /app/config/old-param
```

---

## suve stage ssm status

Show staged changes for SSM parameters.

```
suve stage ssm status
```

**Examples:**

```ShellSession
user@host:~$ suve stage ssm status
Staged SSM changes (3):
  A /app/config/new-param
  M /app/config/database-url
  D /app/config/old-param
```

If no changes are staged:

```ShellSession
user@host:~$ suve stage ssm status
SSM Parameter Store:
  (no staged changes)
```

> [!TIP]
> Use `suve stage status` to show all staged changes (SSM + SM combined).

---

## suve stage ssm diff

Compare staged values with current AWS values.

```
suve stage ssm diff [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (optional, shows all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Examples:**

```ShellSession
user@host:~$ suve stage ssm diff
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

> [!CAUTION]
> Always review the diff before pushing to ensure you're applying the intended changes.

---

## suve stage ssm push

Apply staged SSM parameter changes to AWS.

```
suve stage ssm push [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (optional, pushes all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | `-y` | `false` | Skip confirmation prompt |

**Examples:**

```ShellSession
user@host:~$ suve stage ssm push
Pushing SSM parameters...
✓ Set /app/config/new-param (version: 1)
✓ Set /app/config/database-url (version: 4)
✓ Deleted /app/config/old-param
```

> [!CAUTION]
> `suve stage ssm push` applies changes to AWS immediately. Always review with `suve stage ssm diff` first!

---

## suve stage ssm reset

Unstage SSM parameter changes.

```
suve stage ssm reset [name]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Specific parameter to unstage (if omitted, unstages all) |

**Examples:**

```ShellSession
user@host:~$ suve stage ssm reset /app/config/database-url
Unstaged /app/config/database-url

user@host:~$ suve stage ssm reset
Unstaged all SSM changes
```

> [!TIP]
> Use `suve stage reset` to unstage all changes (SSM + SM combined).
