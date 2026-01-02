# SSM Parameter Store Commands

[<- Back to README](../README.md) | [Secrets Manager Commands ->](secret.md)

Primary command: `param`
Aliases: `ssm`, `ps`

## suve param show

Display parameter value with metadata.

```
suve param show [options] <name[#VERSION][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--decrypt` | - | `true` | Decrypt SecureString values. Use `--decrypt=false` to disable. |
| `--parse-json` | `-j` | `false` | Pretty-print JSON values with indentation |
| `--no-pager` | - | `false` | Disable pager output |
| `--raw` | - | `false` | Output raw value only without metadata (for piping) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```ShellSession
user@host:~$ suve param show /app/config/database-url
Name: /app/config/database-url
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  postgres://db.example.com:5432/myapp
```

With `--parse-json` for JSON values:

```ShellSession
user@host:~$ suve param show --parse-json /app/config/credentials
Name: /app/config/credentials
Version: 2
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
```

With `--raw` for scripting (outputs value only, no trailing newline):

```ShellSession
user@host:~$ suve param show --raw /app/config/database-url
postgres://db.example.com:5432/myapp
```

> [!TIP]
> Use `--raw` for scripting and piping. The output has no trailing newline.

```bash
# Show specific version
suve param show /app/config/database-url#3

# Show previous version
suve param show /app/config/database-url~1

# Show without decryption (displays encrypted value)
suve param show --decrypt=false /app/config/database-url

# Use in scripts
DB_URL=$(suve param show --raw /app/config/database-url)

# Pipe to file
suve param show --raw /app/config/ssl-cert > cert.pem

# Pretty print JSON with raw output
suve param show --raw --parse-json /app/config/database-credentials
```

---

## suve param log

Show parameter version history, similar to `git log`.

Command aliases: `history`

```
suve param log [options] <name>
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
| `--parse-json` | `-j` | `false` | Format JSON values before diffing (use with `--patch`) |
| `--oneline` | - | `false` | Compact one-line-per-version format |
| `--reverse` | - | `false` | Show oldest versions first |
| `--no-pager` | - | `false` | Disable pager output |
| `--from` | - | - | Start version (e.g., `#3`, `~2`) |
| `--to` | - | - | End version (e.g., `#5`, `~0`) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

Basic version history:

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

> [!NOTE]
> Values are truncated at 50 characters with `...` suffix.

With `--patch` to see what changed:

```ShellSession
user@host:~$ suve param log --patch /app/config/database-url
```

Output will look like:

```diff
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
> Use `--patch` to review what changed in each version, similar to `git log -p`.

```bash
# Show last 5 versions
suve param log --number 5 /app/config/database-url

# Show diffs with JSON formatting for JSON values
suve param log --patch --parse-json /app/config/database-credentials

# Show oldest versions first
suve param log --reverse /app/config/database-url
```

---

## suve param diff

Show differences between two parameter versions in unified diff format.

```
suve param diff <spec1> [spec2] | <name> <version1> [version2]
```

### Argument Formats

The diff command supports multiple argument formats for flexibility:

| Format | Args | Example | Description |
|--------|------|---------|-------------|
| full spec | 2 | `/param#1 /param#2` | Both args include name and version |
| full spec | 1 | `/param#3` | Compare specified version with latest |
| mixed | 2 | `/param#1 '#2'` | First with version, second specifier only |
| partial spec | 2 | `/param '#3'` | Name + specifier -> compare with latest |
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

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

### Examples

Compare two versions:

```ShellSession
user@host:~$ suve param diff /app/config/database-url#1 /app/config/database-url#3
```

Output will look like:

```diff
--- /app/config/database-url#1
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://localhost:5432/myapp
+postgres://db.example.com:5432/myapp
```

Compare previous with latest (shorthand):

```ShellSession
user@host:~$ suve param diff /app/config/database-url~1
```

Output will look like:

```diff
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
> Hint: To compare with previous version, use: suve param diff /param~1
> ```

### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting `#` and `~` specifiers to prevent shell interpretation:
> - `#` at argument start is treated as a comment in most shells
> - `~` alone expands to `$HOME` in bash/zsh

```bash
# Compare version 1 with version 2
suve param diff /app/config/database-url '#1' '#2'

# Compare previous with latest
suve param diff /app/config/database-url '~'

# Pipe to a file for review
suve param diff /app/config/database-url~1 > changes.diff
```

---

## suve param list

List parameters.

Command aliases: `ls`

```
suve param list [options] [path-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `path-prefix` | No | Filter by path prefix (e.g., `/app/config/`) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--recursive` | `-R` | `false` | List parameters recursively under the path |
| `--filter` | - | - | Filter by regex pattern |
| `--show` | - | `false` | Show parameter values |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```ShellSession
user@host:~$ suve param list /app/config/
/app/config/database-url
/app/config/api-key
/app/config/redis-host
```

Recursive listing:

```ShellSession
user@host:~$ suve param list --recursive /app/
/app/config/database-url
/app/config/api-key
/app/config/nested/param
/app/secrets/api-token
```

> [!NOTE]
> Without `--recursive`, only lists parameters at the specified path level (OneLevel). With `--recursive`, lists all parameters under the path including nested paths.

```bash
# List all parameters (no filter)
suve param list

# List parameters under /app/config/
suve param list /app/config/

# List recursively
suve param list --recursive /app/

# Filter by regex pattern
suve param list --filter '\.prod\.'

# List with values
suve param list --show /app/
```

---

## suve param set

Create or update a parameter value.

```
suve param set [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `value` | Parameter value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--type` | - | `String` | Parameter type: `String`, `StringList`, or `SecureString` |
| `--secure` | - | `false` | Shorthand for `--type SecureString` |
| `--description` | - | - | Parameter description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times, additive) |
| `--untag` | - | - | Tag key to remove (can be specified multiple times) |
| `--yes` | - | `false` | Skip confirmation prompt |

> [!NOTE]
> `--secure` and `--type` cannot be used together.

**Examples:**

```ShellSession
user@host:~$ suve param set --secure /app/config/database-url "postgres://db.example.com:5432/myapp"
? Set parameter /app/config/database-url? [y/N] y
Set parameter /app/config/database-url (version: 1)
```

```bash
# Create/update as String (default)
suve param set /app/config/log-level "debug"

# Create as SecureString
suve param set --secure /app/config/api-key "sk-1234567890"

# Create with description
suve param set --description "Database connection string" --secure /app/config/database-url "postgres://..."

# StringList (comma-separated values)
suve param set --type StringList /app/config/allowed-hosts "host1,host2,host3"

# Set with tags
suve param set --tag env=prod --tag team=platform /app/config/key "value"

# Skip confirmation
suve param set --yes /app/config/log-level "debug"
```

> [!IMPORTANT]
> SecureString is encrypted using the default AWS KMS key. Ensure your IAM role has the necessary KMS permissions.

---

## suve param delete

Delete a parameter.

Command aliases: `rm`

```
suve param delete [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name to delete |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```ShellSession
user@host:~$ suve param delete /app/config/old-param
? Delete parameter /app/config/old-param? [y/N] y
Deleted /app/config/old-param
```

```bash
# Delete without confirmation
suve param delete --yes /app/config/old-param
```

> [!CAUTION]
> Deletion is immediate and permanent. There is no recovery option.

---

## Staging Workflow

The staging workflow allows you to prepare changes locally before applying them to AWS.

> [!IMPORTANT]
> The staging workflow lets you prepare changes locally, review them, and apply when ready--just like `git add` -> `git diff --staged` -> `git commit`.

The stage file is stored at `~/.suve/stage.json`.

### Workflow Overview

```
+---------+    +---------+    +---------+
|  edit   |--->|  stage  |--->|  apply  |
+---------+    +---------+    +---------+
     |              |              |
     |              |              v
     |              |         Applied to AWS
     |              |
     |              v
     |         status (view)
     |         diff (compare)
     |         reset (unstage)
     |              |
     |              v
     |         Discarded
     +-------------+
```

---

## suve stage param add

Stage a new parameter for creation.

```
suve stage param add [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Parameter name |
| `value` | No | Parameter value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | - | - | Parameter description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times) |

**Examples:**

```ShellSession
user@host:~$ suve stage param add /app/config/new-param "my-value"
Staged for creation: /app/config/new-param
```

```bash
# Stage with inline value
suve stage param add /app/config/new-param "my-value"

# Stage via editor
suve stage param add /app/config/new-param

# Stage with description and tags
suve stage param add --description "API key" --tag env=prod /app/config/api-key "sk-1234567890"
```

---

## suve stage param edit

Edit an existing parameter and stage the changes.

```
suve stage param edit [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Parameter name |
| `value` | No | New value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | - | - | Parameter description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times) |

**Behavior:**

1. If the parameter is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`) if value not provided
4. If the value changed, stages the new value

**Examples:**

```ShellSession
user@host:~$ suve stage param edit /app/config/database-url
Staged: /app/config/database-url
```

```bash
# Edit via editor
suve stage param edit /app/config/database-url

# Edit with inline value
suve stage param edit /app/config/database-url "new-value"

# Edit with tags
suve stage param edit --tag env=prod /app/config/database-url "new-value"
```

---

## suve stage param delete

Stage a parameter for deletion.

```
suve stage param delete <name>
```

**Examples:**

```ShellSession
user@host:~$ suve stage param delete /app/config/old-param
Staged for deletion: /app/config/old-param
```

---

## suve stage param status

Show staged changes for SSM parameters.

```
suve stage param status [options] [name]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Specific parameter name to show |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--verbose` | `-v` | `false` | Show detailed information including values |

**Examples:**

```ShellSession
user@host:~$ suve stage param status
Staged SSM changes (3):
  A /app/config/new-param
  M /app/config/database-url
  D /app/config/old-param
```

If no changes are staged:

```ShellSession
user@host:~$ suve stage param status
SSM Parameter Store:
  (no staged changes)
```

> [!TIP]
> Use `suve stage status` to show all staged changes (SSM + SM combined).

---

## suve stage param diff

Compare staged values with current AWS values.

```
suve stage param diff [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (optional, shows all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Examples:**

```ShellSession
user@host:~$ suve stage param diff
```

Output will look like:

```diff
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
> Always review the diff before applying to ensure you're applying the intended changes.

---

## suve stage param apply

Apply staged SSM parameter changes to AWS.

Command aliases: `push`

```
suve stage param apply [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name (optional, applies all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |
| `--ignore-conflicts` | - | `false` | Apply even if AWS was modified after staging |

> [!NOTE]
> Before applying, suve checks if the AWS resource was modified after staging. If a conflict is detected, the apply is rejected to prevent lost updates. Use `--ignore-conflicts` to force apply despite conflicts.

**Examples:**

```ShellSession
user@host:~$ suve stage param apply
Applying SSM parameters...
Set /app/config/new-param (version: 1)
Set /app/config/database-url (version: 4)
Deleted /app/config/old-param
```

> [!CAUTION]
> `suve stage param apply` applies changes to AWS immediately. Always review with `suve stage param diff` first!

---

## suve stage param reset

Unstage SSM parameter changes or restore to a specific version.

```
suve stage param reset [options] [spec]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `spec` | No | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--all` | - | `false` | Unstage all SSM parameters |

**Version Specifiers:**

| Specifier | Description |
|-----------|-------------|
| `<name>` | Unstage parameter (remove from staging) |
| `<name>#<ver>` | Restore to specific version |
| `<name>~1` | Restore to 1 version ago |

**Examples:**

```ShellSession
user@host:~$ suve stage param reset /app/config/database-url
Unstaged /app/config/database-url

user@host:~$ suve stage param reset --all
Unstaged all SSM changes
```

```bash
# Unstage specific parameter
suve stage param reset /app/config/database-url

# Restore to specific version and stage
suve stage param reset /app/config/database-url#3

# Restore to previous version and stage
suve stage param reset /app/config/database-url~1

# Unstage all SSM parameters
suve stage param reset --all
```

> [!TIP]
> Use `suve stage reset` to unstage all changes (SSM + SM combined).
