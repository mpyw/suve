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
    "password": "secret123",
    "username": "admin"
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

# Pretty print JSON
suve sm cat -j my-database-credentials

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
| `--patch` | `-p` | `false` | Show diff between consecutive versions |
| `--json` | `-j` | `false` | Format JSON values before diffing (use with `-p`) |
| `--reverse` | `-r` | `false` | Show oldest versions first |

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

**With `--patch`:**

```diff
Version abc12345 [AWSCURRENT]
Date: 2024-01-15T10:30:45Z

--- my-secret#def67890
+++ my-secret#abc12345
@@ -1 +1 @@
-{"password":"old-password"}
+{"password":"new-password"}

Version def67890 [AWSPREVIOUS]
Date: 2024-01-14T09:20:30Z

--- my-secret#ghi11111
+++ my-secret#def67890
...
```

> [!TIP]
> Use `-p` to review what changed in each secret rotation, similar to `git log -p`.

> [!NOTE]
> When using `--patch`, the command fetches the actual secret values for each version to compute diffs.

**Examples:**

```bash
# Show version history
suve sm log my-database-credentials

# Show last 5 versions
suve sm log -n 5 my-database-credentials

# Show versions with diffs
suve sm log -p my-database-credentials

# Show diffs with JSON formatting
suve sm log -p -j my-database-credentials

# Show last 3 versions with diffs
suve sm log -n 3 -p my-database-credentials

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

### Output

```diff
--- my-secret#abc12345
+++ my-secret#def67890
@@ -1 +1 @@
-{"password":"old"}
+{"password":"new"}
```

Output is colorized when stdout is a TTY:
- **Red**: Deleted lines (`-`)
- **Green**: Added lines (`+`)
- **Cyan**: Headers (`---`, `+++`, `@@`)

> [!NOTE]
> Version IDs in the diff header are truncated to 8 characters for readability.

### Identical Version Warning

> [!WARNING]
> When comparing versions with **identical content**, no diff is produced. Instead, warnings and hints are displayed:
> ```
> Warning: comparing identical versions
> Hint: To compare with previous version, use: suve sm diff my-secret~1
> Hint: or: suve sm diff my-secret:AWSPREVIOUS
> ```
> This typically happens when you compare AWSCURRENT with itself (e.g., `my-secret` vs `my-secret`).

### Examples

#### Full Spec Format (Recommended)

```bash
# Compare AWSPREVIOUS with AWSCURRENT
suve sm diff my-database-credentials:AWSPREVIOUS my-database-credentials:AWSCURRENT

# Compare AWSPREVIOUS with AWSCURRENT (single arg)
suve sm diff my-database-credentials:AWSPREVIOUS

# Compare previous with current using shift
suve sm diff my-database-credentials~1
```

#### Mixed Format

```bash
# Compare AWSPREVIOUS with AWSCURRENT (name from first arg)
suve sm diff my-database-credentials:AWSPREVIOUS ':AWSCURRENT'
```

#### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting specifiers to prevent potential shell interpretation:
> - `~` alone expands to `$HOME` in bash/zsh
> - `:` may have special meaning in some contexts

```bash
# Compare AWSPREVIOUS with AWSCURRENT
suve sm diff my-database-credentials ':AWSPREVIOUS' ':AWSCURRENT'

# Compare AWSPREVIOUS with AWSCURRENT (shorthand)
suve sm diff my-database-credentials ':AWSPREVIOUS'

# Compare using relative versions
suve sm diff my-database-credentials '~2' '~1'

# Compare previous with current
suve sm diff my-database-credentials '~'
```

#### Practical Use Cases

```bash
# Review what changed in the last secret rotation
suve sm diff my-database-credentials:AWSPREVIOUS

# Compare current with 2 versions ago
suve sm diff my-database-credentials~2 my-database-credentials:AWSCURRENT

# Compare specific version IDs
suve sm diff my-database-credentials#abc12345 my-database-credentials#def67890

# Pipe to a file for review
suve sm diff my-database-credentials:AWSPREVIOUS > changes.diff

# Use with jq for JSON secrets (compare formatted)
suve sm cat my-database-credentials:AWSPREVIOUS | jq . > old.json
suve sm cat my-database-credentials:AWSCURRENT | jq . > new.json
diff old.json new.json
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

## suve sm edit

Edit a secret value in your editor and stage the changes.

```
suve sm edit [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--delete` | `-d` | `false` | Stage secret for deletion |

**Behavior:**

1. If the secret is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`)
4. If the value changed, stages the new value

**Output:**

```
Staged my-database-credentials
```

If no changes were made:
```
No changes made
```

**Examples:**

```bash
# Edit a secret
suve sm edit my-database-credentials

# Stage a secret for deletion
suve sm edit --delete my-old-secret
```

---

## suve sm status

Show staged changes for Secrets Manager secrets.

```
suve sm status
```

**Output:**

```
Secrets Manager:
  set    my-database-credentials
  delete my-old-secret
```

If no changes are staged:
```
Secrets Manager:
  (no staged changes)
```

**Examples:**

```bash
# Show SM staged changes
suve sm status

# Show all staged changes (SSM + SM)
suve status
```

---

## suve sm push

Apply staged Secrets Manager changes to AWS.

```
suve sm push
```

**Behavior:**

1. Reads all staged SM changes
2. For each `set` operation: calls UpdateSecret (or CreateSecret if new)
3. For each `delete` operation: calls DeleteSecret with force
4. Removes successfully applied changes from stage
5. Keeps failed changes in stage for retry

**Output:**

```
Pushing Secrets Manager secrets...
✓ my-database-credentials
✓ my-old-secret (deleted)
```

If nothing is staged:
```
SM: nothing to push
```

**Examples:**

```bash
# Push SM changes only
suve sm push

# Push all changes (SSM + SM)
suve push
```

---

## suve sm reset

Unstage Secrets Manager changes.

```
suve sm reset [name]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Specific secret to unstage (if omitted, unstages all) |

**Output:**

Specific secret:
```
Unstaged my-database-credentials
```

All secrets:
```
Unstaged all SM changes
```

**Examples:**

```bash
# Unstage a specific secret
suve sm reset my-database-credentials

# Unstage all SM secrets
suve sm reset

# Unstage everything (SSM + SM)
suve reset
```

---

## suve sm diff --staged

Compare staged value with current AWS value.

```
suve sm diff --staged <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (without version specifier) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--staged` | - | `false` | Compare staged value with AWS current |
| `--json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Output:**

```diff
--- my-database-credentials (AWS)
+++ my-database-credentials (staged)
@@ -1 +1 @@
-{"password":"old"}
+{"password":"new"}
```

If secret is staged for deletion:
```diff
--- my-old-secret (AWS)
+++ my-old-secret (staged for deletion)
@@ -1 +0,0 @@
-{"password":"deleted"}
```

**Examples:**

```bash
# Compare staged vs AWS
suve sm diff --staged my-database-credentials

# Compare with JSON formatting
suve sm diff --staged --json my-database-credentials
```
