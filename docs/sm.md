# Secrets Manager Commands

[← Back to README](../README.md) | [← SSM Parameter Store Commands](ssm.md)

Service aliases: `sm`, `secret`

## suve sm show

Display secret value with metadata.

```
suve sm show [options] <name[#VERSION | :LABEL][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Pretty-print JSON values with indentation |
| `--no-pager` | - | `false` | Disable pager output |

**Examples:**

```ShellSession
user@host:~$ suve sm show my-database-credentials
Name: my-database-credentials
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-database-credentials-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
Created: 2024-01-15T10:30:45Z

  {"username":"admin","password":"secret123"}
```

With `--json` for JSON values:

```ShellSession
user@host:~$ suve sm show -j my-database-credentials
Name: my-database-credentials
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-database-credentials-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
Created: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
```

```bash
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
suve sm cat [options] <name[#VERSION | :LABEL][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Pretty-print JSON values with indentation |

**Examples:**

```ShellSession
user@host:~$ suve sm cat my-database-credentials
{"username":"admin","password":"secret123"}
```

> [!TIP]
> Use `cat` for scripting and piping. The output has no trailing newline.

Extract JSON fields with `jq`:

```ShellSession
user@host:~$ suve sm cat my-database-credentials | jq -r '.password'
secret123
```

```bash
# Use in scripts
CREDS=$(suve sm cat my-database-credentials)

# Pipe to file
suve sm cat my-ssl-certificate > cert.pem

# Pretty print JSON
suve sm cat -j my-database-credentials
```

---

## suve sm log

Show secret version history, similar to `git log`.

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
| `--patch` | `-p` | `false` | Show diff between consecutive versions |
| `--json` | `-j` | `false` | Format JSON values before diffing (use with `-p`) |
| `--oneline` | - | `false` | Compact one-line-per-version format |
| `--reverse` | - | `false` | Show oldest versions first |
| `--no-pager` | - | `false` | Disable pager output |

**Examples:**

Basic version history:

```ShellSession
user@host:~$ suve sm log my-database-credentials
Version abc12345 [AWSCURRENT]
Date: 2024-01-15T10:30:45Z

Version def67890 [AWSPREVIOUS]
Date: 2024-01-14T09:20:30Z

Version ghi11111
Date: 2024-01-13T08:10:00Z
```

> [!NOTE]
> Version IDs are truncated to 8 characters for display.

With `--patch` to see what changed:

```ShellSession
user@host:~$ suve sm log -p my-database-credentials
Version abc12345 [AWSCURRENT]
Date: 2024-01-15T10:30:45Z

--- my-database-credentials#def67890
+++ my-database-credentials#abc12345
@@ -1 +1 @@
-{"password":"old-password"}
+{"password":"new-password"}

Version def67890 [AWSPREVIOUS]
Date: 2024-01-14T09:20:30Z

--- my-database-credentials#ghi11111
+++ my-database-credentials#def67890
@@ -1 +1 @@
-{"password":"initial"}
+{"password":"old-password"}
```

> [!TIP]
> Use `-p` to review what changed in each secret rotation, similar to `git log -p`.

> [!NOTE]
> When using `--patch`, the command fetches the actual secret values for each version to compute diffs.

```bash
# Show last 5 versions
suve sm log -n 5 my-database-credentials

# Show diffs with JSON formatting
suve sm log -p -j my-database-credentials

# Show oldest versions first
suve sm log --reverse my-database-credentials
```

---

## suve sm diff

Show differences between two secret versions in unified diff format.

```
suve sm diff <spec1> [spec2] | <name> <version1> [version2]
```

### Argument Formats

The diff command supports multiple argument formats for flexibility:

| Format | Args | Example | Description |
|--------|------|---------|-------------|
| full spec | 2 | `secret:AWSPREVIOUS secret:AWSCURRENT` | Both args include name and version |
| full spec | 1 | `secret:AWSPREVIOUS` | Compare specified version with AWSCURRENT |
| mixed | 2 | `secret:AWSPREVIOUS ':AWSCURRENT'` | First with version, second specifier only |
| partial spec | 2 | `secret ':AWSPREVIOUS'` | Name + specifier → compare with AWSCURRENT |
| partial spec | 3 | `secret ':AWSPREVIOUS' ':AWSCURRENT'` | Name + two specifiers |

> [!TIP]
> **Use full spec format** to avoid shell quoting issues. When `:` or `~` appear at the start of an argument, special shell handling may occur. Full spec format embeds the specifier within the name, so no quoting is needed.

> [!NOTE]
> When only one version is specified, it is compared against **AWSCURRENT** (the current version).

### Version Specifiers

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version by VersionId | `#abc12345-...` |
| `:LABEL` | Staging label | `:AWSCURRENT`, `:AWSPREVIOUS` |
| `~` | One version ago | `~` = current - 1 |
| `~N` | N versions ago | `~2` = current - 2 |

Specifiers can be combined: `secret:AWSCURRENT~1` means "1 version before AWSCURRENT".

> [!NOTE]
> **Labels vs Shift**: Labels (`:AWSCURRENT`, `:AWSPREVIOUS`) point to specific tagged versions. Shift (`~N`) navigates by creation date order. After a secret update:
> - `:AWSCURRENT` = new value
> - `:AWSPREVIOUS` = old value
> - `~1` = 1 version ago (same as `:AWSPREVIOUS` after a single update)

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |

### Examples

Compare AWSPREVIOUS with AWSCURRENT:

```ShellSession
user@host:~$ suve sm diff my-database-credentials:AWSPREVIOUS
--- my-database-credentials#def67890
+++ my-database-credentials#abc12345
@@ -1 +1 @@
-{"password":"old-password"}
+{"password":"new-password"}
```

Compare using shift syntax:

```ShellSession
user@host:~$ suve sm diff my-database-credentials~1
--- my-database-credentials#def67890
+++ my-database-credentials#abc12345
@@ -1 +1 @@
-{"password":"old-password"}
+{"password":"new-password"}
```

> [!NOTE]
> Output is colorized when stdout is a TTY:
> - **Red**: Deleted lines (`-`)
> - **Green**: Added lines (`+`)
> - **Cyan**: Headers (`---`, `+++`, `@@`)
>
> Version IDs in the diff header are truncated to 8 characters for readability.

### Identical Version Warning

> [!WARNING]
> When comparing versions with **identical content**, no diff is produced:
> ```
> Warning: comparing identical versions
> Hint: To compare with previous version, use: suve sm diff my-secret~1
> Hint: or: suve sm diff my-secret:AWSPREVIOUS
> ```

### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting specifiers to prevent potential shell interpretation:
> - `~` alone expands to `$HOME` in bash/zsh
> - `:` may have special meaning in some contexts

```bash
# Compare AWSPREVIOUS with AWSCURRENT
suve sm diff my-database-credentials ':AWSPREVIOUS' ':AWSCURRENT'

# Compare previous with current
suve sm diff my-database-credentials '~'

# Compare specific version IDs
suve sm diff my-database-credentials#abc12345 my-database-credentials#def67890

# Pipe to a file for review
suve sm diff my-database-credentials:AWSPREVIOUS > changes.diff
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

**Examples:**

```ShellSession
user@host:~$ suve sm ls
my-database-credentials
my-api-key
production/database-credentials
staging/database-credentials
```

With prefix filter:

```ShellSession
user@host:~$ suve sm ls production/
production/database-credentials
production/api-key
production/ssl-cert
```

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
| `--description` | - | - | Description for the secret |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times) |

**Examples:**

```ShellSession
user@host:~$ suve sm create my-api-key "sk-1234567890"
✓ Created secret my-api-key (version: abc12345-1234-1234-1234-123456789012)
```

With JSON value and description:

```ShellSession
user@host:~$ suve sm create -d "Production database credentials" my-database-credentials '{"username":"admin","password":"secret"}'
✓ Created secret my-database-credentials (version: def67890-1234-1234-1234-123456789012)
```

> [!NOTE]
> `create` fails if the secret already exists. Use `suve sm update` to update an existing secret.

---

## suve sm update

Update an existing secret's value.

```
suve sm update [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | New secret value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | - | - | Update secret description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times, additive) |
| `--untag` | - | - | Tag key to remove (can be specified multiple times) |
| `--yes` | `-y` | `false` | Skip confirmation prompt |

**Examples:**

```ShellSession
user@host:~$ suve sm update my-database-credentials '{"username":"admin","password":"newpassword"}'
? Update secret my-database-credentials? [y/N] y
✓ Updated secret my-database-credentials (version: ghi11111-1234-1234-1234-123456789012)
```

```bash
# Update with tags
suve sm update --tag env=prod my-api-key "new-key-value"

# Update without confirmation
suve sm update -y my-api-key "new-key-value"
```

> [!NOTE]
> - `update` fails if the secret doesn't exist. Use `suve sm create` to create a new secret.
> - The previous version automatically becomes AWSPREVIOUS.

---

## suve sm delete

Delete a secret.

```
suve sm delete [options] <name>
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
| `--yes` | `-y` | `false` | Skip confirmation prompt |

**Examples:**

With recovery window (default):

```ShellSession
user@host:~$ suve sm delete my-old-secret
? Delete secret my-old-secret? [y/N] y
! Scheduled deletion of secret my-old-secret (deletion date: 2024-02-14)
```

Immediate deletion:

```ShellSession
user@host:~$ suve sm delete --force my-old-secret
! Permanently deleted secret my-old-secret
```

> [!WARNING]
> Without `--force`, the secret can be restored using `suve sm restore` until the deletion date.

> [!CAUTION]
> With `--force`, deletion is **immediate and irreversible**. The secret cannot be recovered.

```bash
# Delete with 30-day recovery window (default)
suve sm delete my-old-secret

# Delete with 7-day recovery window
suve sm delete --recovery-window 7 my-old-secret

# Delete immediately (no recovery possible)
suve sm delete --force my-old-secret
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

**Examples:**

```ShellSession
user@host:~$ suve sm restore my-accidentally-deleted-secret
✓ Restored secret my-accidentally-deleted-secret
```

> [!NOTE]
> - Only works for secrets deleted without `--force`
> - Must be done before the scheduled deletion date
> - Cannot restore secrets that have been permanently deleted

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

## suve stage sm add

Stage a new secret for creation.

```
suve stage sm add [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Secret name |
| `value` | No | Secret value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | - | - | Secret description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times) |

**Examples:**

```ShellSession
user@host:~$ suve stage sm add my-new-secret '{"key":"value"}'
✓ Staged for creation: my-new-secret
```

```bash
# Stage with inline value
suve stage sm add my-new-secret '{"key":"value"}'

# Stage via editor
suve stage sm add my-new-secret

# Stage with description and tags
suve stage sm add --description "API key for production" --tag env=prod my-api-key "sk-1234567890"
```

---

## suve stage sm edit

Edit an existing secret and stage the changes.

```
suve stage sm edit [options] <name> [value]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Secret name |
| `value` | No | New value (opens editor if not provided) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--description` | - | - | Secret description |
| `--tag` | - | - | Tag in key=value format (can be specified multiple times) |

**Behavior:**

1. If the secret is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`) if value not provided
4. If the value changed, stages the new value

**Examples:**

```ShellSession
user@host:~$ suve stage sm edit my-database-credentials
✓ Staged: my-database-credentials
```

```bash
# Edit via editor
suve stage sm edit my-database-credentials

# Edit with inline value
suve stage sm edit my-database-credentials '{"username":"admin","password":"newpassword"}'

# Edit with tags
suve stage sm edit --tag env=prod my-database-credentials '{"password":"new"}'
```

---

## suve stage sm delete

Stage a secret for deletion.

```
suve stage sm delete [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to stage for deletion |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--force` | - | `false` | Force immediate deletion without recovery window |
| `--recovery-window` | - | `30` | Days before permanent deletion (7-30) |

**Examples:**

```ShellSession
user@host:~$ suve stage sm delete my-old-secret
✓ Staged for deletion: my-old-secret
```

```bash
# Stage for deletion with default 30-day recovery
suve stage sm delete my-old-secret

# Stage for deletion with 7-day recovery
suve stage sm delete --recovery-window 7 my-old-secret

# Stage for immediate deletion
suve stage sm delete --force my-old-secret
```

---

## suve stage sm status

Show staged changes for Secrets Manager secrets.

```
suve stage sm status [options] [name]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Specific secret name to show |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--verbose` | `-v` | `false` | Show detailed information including values |

**Examples:**

```ShellSession
user@host:~$ suve stage sm status
Staged SM changes (3):
  A my-new-secret
  M my-database-credentials
  D my-old-secret
```

If no changes are staged:

```ShellSession
user@host:~$ suve stage sm status
Secrets Manager:
  (no staged changes)
```

> [!TIP]
> Use `suve stage status` to show all staged changes (SSM + SM combined).

---

## suve stage sm diff

Compare staged values with current AWS values.

```
suve stage sm diff [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (optional, shows all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Examples:**

```ShellSession
user@host:~$ suve stage sm diff
--- my-database-credentials (AWS)
+++ my-database-credentials (staged)
@@ -1 +1 @@
-{"password":"old"}
+{"password":"new"}

--- my-new-secret (not in AWS)
+++ my-new-secret (staged for creation)
@@ -0,0 +1 @@
+{"key":"value"}
```

For secrets staged for deletion:

```ShellSession
user@host:~$ suve stage sm diff my-old-secret
--- my-old-secret (AWS)
+++ my-old-secret (staged for deletion)
@@ -1 +0,0 @@
-{"password":"deleted"}
```

> [!CAUTION]
> Always review the diff before pushing to ensure you're applying the intended changes.

---

## suve stage sm push

Apply staged Secrets Manager changes to AWS.

```
suve stage sm push [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (optional, pushes all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | `-y` | `false` | Skip confirmation prompt |
| `--ignore-conflicts` | - | `false` | Push even if AWS was modified after staging |

> [!NOTE]
> Before pushing, suve checks if the AWS resource was modified after staging. If a conflict is detected, the push is rejected to prevent lost updates. Use `--ignore-conflicts` to force push despite conflicts.

**Behavior:**

1. Reads all staged SM changes
2. For each `set` operation: calls UpdateSecret (or CreateSecret if new)
3. For each `delete` operation: calls DeleteSecret
4. Removes successfully applied changes from stage
5. Keeps failed changes in stage for retry

**Examples:**

```ShellSession
user@host:~$ suve stage sm push
Pushing Secrets Manager secrets...
✓ Created my-new-secret (version: abc12345)
✓ Updated my-database-credentials (version: def67890)
✓ Deleted my-old-secret
```

> [!CAUTION]
> `suve stage sm push` applies changes to AWS immediately. Always review with `suve stage sm diff` first!

---

## suve stage sm reset

Unstage Secrets Manager changes or restore to a specific version.

```
suve stage sm reset [options] [spec]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `spec` | No | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--all` | - | `false` | Unstage all SM secrets |

**Version Specifiers:**

| Specifier | Description |
|-----------|-------------|
| `<name>` | Unstage secret (remove from staging) |
| `<name>#<ver>` | Restore to specific version |
| `<name>~1` | Restore to 1 version ago |

**Examples:**

```ShellSession
user@host:~$ suve stage sm reset my-database-credentials
Unstaged my-database-credentials

user@host:~$ suve stage sm reset --all
Unstaged all SM changes
```

```bash
# Unstage specific secret
suve stage sm reset my-database-credentials

# Restore to specific version and stage
suve stage sm reset my-database-credentials#abc12345

# Restore to previous version and stage
suve stage sm reset my-database-credentials~1

# Unstage all SM secrets
suve stage sm reset --all
```

> [!TIP]
> Use `suve stage reset` to unstage all changes (SSM + SM combined).
