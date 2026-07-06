# AWS Commands (Parameter Store + Secrets Manager)

[<- Back to README](../README.md) | [Google Cloud Commands](gcloud.md) | [Azure Commands](azure.md)

> [!TIP]
> AWS is invoked as `suve aws param` (`ssm`, `ps`), `suve aws secret` (`sm`), and `suve aws stage` (`stg`).
> You can drop the `aws` prefix — `suve param`, `suve secret`, `suve stage` — when AWS is the only provider active in your environment. The exact rules are in [Provider selection](../README.md#provider-selection).

## suve aws param show

Display parameter value with metadata.

```
suve aws param show [options] <name[#VERSION][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Pretty-print JSON values with indentation |
| `--no-pager` | - | `false` | Disable pager output |
| `--raw` | - | `false` | Output raw value only without metadata (for piping) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```ShellSession
user@host:~$ suve aws param show /app/config/database-url
Name: /app/config/database-url
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  postgres://db.example.com:5432/myapp
```

With `--parse-json` for JSON values:

```ShellSession
user@host:~$ suve aws param show --parse-json /app/config/credentials
Name: /app/config/credentials
Version: 2
Type: SecureString
JsonParsed: true
Modified: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
```

> [!NOTE]
> `JsonParsed: true` appears only when `--parse-json` is used and the value is valid JSON. Keys are sorted alphabetically.

With `--raw` for scripting (outputs value only, no trailing newline):

```ShellSession
user@host:~$ suve aws param show --raw /app/config/database-url
postgres://db.example.com:5432/myapp
```

> [!TIP]
> Use `--raw` for scripting and piping. The output has no trailing newline.

> [!NOTE]
> Timestamps respect the `TZ` environment variable. Use `TZ=UTC` or `TZ=Asia/Tokyo` to change the displayed timezone.

With `--output=json` for structured output:

```ShellSession
user@host:~$ suve aws param show --output=json /app/config/database-url
{
  "name": "/app/config/database-url",
  "version": 3,
  "type": "SecureString",
  "modified": "2024-01-15T10:30:45Z",
  "value": "postgres://db.example.com:5432/myapp"
}
```

Extract fields with `jq`:

```ShellSession
user@host:~$ suve aws param show --output=json /app/config/database-url | jq -r '.value'
postgres://db.example.com:5432/myapp
```

```bash
# Show specific version
suve aws param show /app/config/database-url#3

# Show previous version
suve aws param show /app/config/database-url~1

# Use in scripts
DB_URL=$(suve aws param show --raw /app/config/database-url)

# Pipe to file
suve aws param show --raw /app/config/ssl-cert > cert.pem

# Pretty print JSON with raw output
suve aws param show --raw --parse-json /app/config/database-credentials
```

---

## suve aws param log

Show parameter version history, similar to `git log`.

Command aliases: `history`

```
suve aws param log [options] <name>
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
| `--since` | - | - | Show versions modified after this date (RFC3339 format) |
| `--until` | - | - | Show versions modified before this date (RFC3339 format) |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |
| `--max-value-length` | - | `0` | Maximum value preview length (0 = auto) |

**Examples:**

Basic version history:

```ShellSession
user@host:~$ suve aws param log /app/config/database-url
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
> In normal mode, full values are shown. In `--oneline` mode, values are truncated to fit terminal width (default 50 if unavailable). Use `--max-value-length` to override.

With `--patch` to see what changed:

```ShellSession
user@host:~$ suve aws param log --patch /app/config/database-url
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
suve aws param log --number 5 /app/config/database-url

# Show diffs with JSON formatting for JSON values
suve aws param log --patch --parse-json /app/config/database-credentials

# Show oldest versions first
suve aws param log --reverse /app/config/database-url

# Output as JSON for scripting
suve aws param log --output=json /app/config/database-url
```

---

## suve aws param diff

Show differences between two parameter versions in unified diff format.

```
suve aws param diff <spec1> [spec2] | <name> <version1> [version2]
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

> [!TIP]
> `~` without a number means `~1`. You can chain shifts: `~~` = `~1~1` = `~2`.

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

### Examples

Compare two versions:

```ShellSession
user@host:~$ suve aws param diff /app/config/database-url#1 /app/config/database-url#3
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
user@host:~$ suve aws param diff /app/config/database-url~1
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
> Hint: To compare with previous version, use: suve aws param diff /param~1
> ```

### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting `#` and `~` specifiers to prevent shell interpretation:
> - `#` at argument start is treated as a comment in most shells
> - `~` alone expands to `$HOME` in bash/zsh

```bash
# Compare version 1 with version 2
suve aws param diff /app/config/database-url '#1' '#2'

# Compare previous with latest
suve aws param diff /app/config/database-url '~'

# Pipe to a file for review
suve aws param diff /app/config/database-url~1 > changes.diff
```

---

## suve aws param list

List parameters.

Command aliases: `ls`

```
suve aws param list [options] [path-prefix]
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
user@host:~$ suve aws param list /app/config/
/app/config/database-url
/app/config/api-key
/app/config/redis-host
```

Recursive listing:

```ShellSession
user@host:~$ suve aws param list --recursive /app/
/app/config/database-url
/app/config/api-key
/app/config/nested/param
/app/secrets/api-token
```

> [!NOTE]
> Without `--recursive`, only lists parameters at the specified path level (OneLevel). With `--recursive`, lists all parameters under the path including nested paths.

```bash
# List all parameters (no filter)
suve aws param list

# List parameters under /app/config/
suve aws param list /app/config/

# List recursively
suve aws param list --recursive /app/

# Filter by regex pattern
suve aws param list --filter '\.prod\.'

# List with values
suve aws param list --show /app/

# Output as JSON for scripting
suve aws param list --output=json /app/config/
```

---

## suve aws param create

Create a new parameter.

```
suve aws param create [options] <name> <value>
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
| `--tier` | - | - | Parameter tier: `Standard`, `Advanced`, or `Intelligent-Tiering` |
| `--data-type` | - | - | Parameter data type (e.g. `text`, `aws:ec2:image`) |
| `--allowed-pattern` | - | - | Regex the value must match |
| `--policies` | - | - | Parameter policies as a JSON document |

> [!NOTE]
> `--secure` and `--type` cannot be used together.

**Examples:**

```ShellSession
user@host:~$ suve aws param create /app/config/log-level "debug"
✓ Created parameter /app/config/log-level (version: 1)
```

```bash
# Create as String (default)
suve aws param create /app/config/log-level "debug"

# Create as SecureString
suve aws param create --secure /app/config/api-key "sk-1234567890"

# Create with description
suve aws param create --description "Database connection string" --secure /app/config/database-url "postgres://..."

# StringList (comma-separated values)
suve aws param create --type StringList /app/config/allowed-hosts "host1,host2,host3"
```

> [!NOTE]
> `create` fails if the parameter already exists. Use `suve aws param update` to update an existing parameter.

> [!IMPORTANT]
> SecureString is encrypted using the default AWS KMS key. Ensure your IAM role has the necessary KMS permissions.

---

## suve aws param update

Update an existing parameter's value.

```
suve aws param update [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `value` | New parameter value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--type` | - | `String` | Parameter type: `String`, `StringList`, or `SecureString` |
| `--secure` | - | `false` | Shorthand for `--type SecureString` |
| `--description` | - | - | Parameter description |
| `--tier` | - | - | Parameter tier: `Standard`, `Advanced`, or `Intelligent-Tiering` |
| `--data-type` | - | - | Parameter data type (e.g. `text`, `aws:ec2:image`) |
| `--allowed-pattern` | - | - | Regex the value must match |
| `--policies` | - | - | Parameter policies as a JSON document |
| `--yes` | - | `false` | Skip confirmation prompt |

> [!NOTE]
> `--secure` and `--type` cannot be used together.

**Examples:**

```ShellSession
user@host:~$ suve aws param update --secure /app/config/database-url "postgres://new-db.example.com:5432/myapp"
--- /app/config/database-url (AWS)
+++ /app/config/database-url (new)
@@ -1 +1 @@
-postgres://db.example.com:5432/myapp
+postgres://new-db.example.com:5432/myapp

? Update parameter /app/config/database-url? [y/N] y
✓ Updated parameter /app/config/database-url (version: 2)
```

```bash
# Update parameter
suve aws param update /app/config/log-level "info"

# Update without confirmation
suve aws param update --yes /app/config/log-level "debug"
```

> [!TIP]
> When updating a parameter, `suve aws param update` shows a diff of the changes and prompts for confirmation. Use `--yes` to skip this review step.

> [!NOTE]
> `update` fails if the parameter doesn't exist. Use `suve aws param create` to create a new parameter.

---

## suve aws param delete

Delete a parameter.

Command aliases: `rm`

```
suve aws param delete [options] <name>
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
user@host:~$ suve aws param delete /app/config/old-param
! Current value of /app/config/old-param:

  postgres://db.example.com:5432/myapp

! This will permanently delete: /app/config/old-param
? Continue? [y/N] y
Deleted /app/config/old-param
```

```bash
# Delete without confirmation (skips value display)
suve aws param delete --yes /app/config/old-param
```

> [!CAUTION]
> Deletion is immediate and permanent. There is no recovery option.

---

## suve aws param tag

Add or update tags on an existing parameter.

```
suve aws param tag <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `key=value` | Tag in key=value format (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws param tag /app/config/database-url env=prod team=platform
✓ Tagged parameter /app/config/database-url (2 tag(s))
```

```bash
# Add single tag
suve aws param tag /app/config/key env=prod

# Add multiple tags
suve aws param tag /app/config/key env=prod team=platform
```

---

## suve aws param untag

Remove tags from an existing parameter.

```
suve aws param untag <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `key` | Tag key to remove (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws param untag /app/config/database-url deprecated old-tag
✓ Untagged parameter /app/config/database-url (2 key(s))
```

```bash
# Remove single tag
suve aws param untag /app/config/key deprecated

# Remove multiple tags
suve aws param untag /app/config/key deprecated old-tag
```

---

## Staging Workflow

The staging workflow allows you to prepare changes locally before applying them to AWS.

> [!IMPORTANT]
> The staging workflow lets you prepare changes locally, review them, and apply when ready--just like `git add` -> `git diff --staged` -> `git commit`.

Working state is split per service and namespaced by provider scope: `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/param.json` and `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/secret.json`. The stash is stored alongside them at `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/stash.json`. There is no longer any single `stage.json`.

> [!NOTE]
> The working-state files are encrypted at rest. The encryption key is resolved from the `SUVE_STAGING_KEY` environment variable (base64-encoded 32 bytes) if set, otherwise from an OS keychain (created on first use), otherwise the state is stored as plaintext with a warning.

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

## suve aws stage param add

Stage a new parameter for creation.

```
suve aws stage param add [options] <name> [value]
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

**Examples:**

```ShellSession
user@host:~$ suve aws stage param add /app/config/new-param "my-value"
Staged for creation: /app/config/new-param
```

```bash
# Stage with inline value
suve aws stage param add /app/config/new-param "my-value"

# Stage via editor
suve aws stage param add /app/config/new-param

# Stage with description
suve aws stage param add --description "API key" /app/config/api-key "sk-1234567890"
```

---

## suve aws stage param edit

Edit an existing parameter and stage the changes.

```
suve aws stage param edit [options] <name> [value]
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

**Behavior:**

1. If the parameter is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`) if value not provided
4. If the value changed, stages the new value

**Examples:**

```ShellSession
user@host:~$ suve aws stage param edit /app/config/database-url
Staged: /app/config/database-url
```

```bash
# Edit via editor
suve aws stage param edit /app/config/database-url

# Edit with inline value
suve aws stage param edit /app/config/database-url "new-value"
```

---

## suve aws stage param delete

Stage a parameter for deletion.

```
suve aws stage param delete <name>
```

**Examples:**

```ShellSession
user@host:~$ suve aws stage param delete /app/config/old-param
Staged for deletion: /app/config/old-param
```

---

## suve aws stage param status

Show staged changes for SSM Parameter Store parameters.

```
suve aws stage param status [options] [name]
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
user@host:~$ suve aws stage param status
Staged SSM Parameter Store changes (3):
  A /app/config/new-param
  M /app/config/database-url
  D /app/config/old-param
```

If no changes are staged:

```ShellSession
user@host:~$ suve aws stage param status
SSM Parameter Store:
  (no staged changes)
```

> [!TIP]
> Use `suve aws stage status` to show all staged changes (SSM Parameter Store + Secrets Manager combined).

---

## suve aws stage param diff

Compare staged values with current AWS values.

```
suve aws stage param diff [options] [name]
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
user@host:~$ suve aws stage param diff
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

> [!TIP]
> Use `--parse-json` to normalize JSON formatting before comparing, making it easier to spot actual content changes vs formatting differences.

---

## suve aws stage param apply

Apply staged SSM Parameter Store parameter changes to AWS.

Command aliases: `push`

```
suve aws stage param apply [options] [name]
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

**Behavior:**

1. Reads all staged SSM Parameter Store changes
2. For each `create` operation: calls PutParameter (with Overwrite=false)
3. For each `update` operation: calls PutParameter (with Overwrite=true)
4. For each `delete` operation: calls DeleteParameter
5. Removes successfully applied changes from stage
6. Keeps failed changes in stage for retry

**Examples:**

```ShellSession
user@host:~$ suve aws stage param apply
Applying SSM Parameter Store parameters...
Created /app/config/new-param (version: 1)
Updated /app/config/database-url (version: 4)
Deleted /app/config/old-param
```

> [!WARNING]
> `suve aws stage param apply` applies changes to AWS immediately. Always review with `suve aws stage param diff` first!

---

## suve aws stage param reset

Unstage SSM Parameter Store parameter changes or restore to a specific version.

```
suve aws stage param reset [options] [spec]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `spec` | No | Parameter name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--all` | - | `false` | Unstage all SSM Parameter Store parameters |

**Version Specifiers:**

| Specifier | Description |
|-----------|-------------|
| `<name>` | Unstage parameter (remove from staging) |
| `<name>#<ver>` | Restore to specific version |
| `<name>~1` | Restore to 1 version ago |

**Examples:**

```ShellSession
user@host:~$ suve aws stage param reset /app/config/database-url
Unstaged /app/config/database-url

user@host:~$ suve aws stage param reset --all
Unstaged all SSM Parameter Store changes
```

```bash
# Unstage specific parameter
suve aws stage param reset /app/config/database-url

# Restore to specific version and stage
suve aws stage param reset /app/config/database-url#3

# Restore to previous version and stage
suve aws stage param reset /app/config/database-url~1

# Unstage all SSM Parameter Store parameters
suve aws stage param reset --all
```

> [!TIP]
> Use `suve aws stage reset` to unstage all changes (SSM Parameter Store + Secrets Manager combined).

---

## suve aws stage param tag

Stage tag additions for a parameter.

```
suve aws stage param tag <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `key=value` | Tag in key=value format (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws stage param tag /app/config/database-url env=prod team=platform
✓ Staged tags for: /app/config/database-url
```

```bash
# Stage single tag
suve aws stage param tag /app/config/key env=prod

# Stage multiple tags
suve aws stage param tag /app/config/key env=prod team=platform
```

> [!NOTE]
> If the parameter is not already staged, a tag-only change is created.

---

## suve aws stage param untag

Stage tag removals for a parameter.

```
suve aws stage param untag <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Parameter name |
| `key` | Tag key to remove (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws stage param untag /app/config/database-url deprecated old-tag
✓ Staged tag removal for: /app/config/database-url
```

```bash
# Stage single tag removal
suve aws stage param untag /app/config/key deprecated

# Stage multiple tag removals
suve aws stage param untag /app/config/key deprecated old-tag
```

> [!NOTE]
> If the parameter is not already staged, a tag-only change is created.

## suve aws secret show

Display secret value with metadata.

```
suve aws secret show [options] <name[#VERSION | :LABEL][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Pretty-print JSON values with indentation |
| `--no-pager` | - | `false` | Disable pager output |
| `--raw` | - | `false` | Output raw value only without metadata (for piping) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```ShellSession
user@host:~$ suve aws secret show my-database-credentials
Name: my-database-credentials
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-database-credentials-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
Created: 2024-01-15T10:30:45Z

  {"username":"admin","password":"secret123"}
```

With `--parse-json` for JSON values:

```ShellSession
user@host:~$ suve aws secret show --parse-json my-database-credentials
Name: my-database-credentials
ARN: arn:aws:secretsmanager:us-east-1:123456789012:secret:my-database-credentials-AbCdEf
VersionId: abc12345-1234-1234-1234-123456789012
Stages: [AWSCURRENT]
JsonParsed: true
Created: 2024-01-15T10:30:45Z

  {
    "password": "secret123",
    "username": "admin"
  }
```

> [!NOTE]
> `JsonParsed: true` appears only when `--parse-json` is used and the value is valid JSON. Keys are sorted alphabetically.

With `--raw` for scripting (outputs value only, no trailing newline):

```ShellSession
user@host:~$ suve aws secret show --raw my-database-credentials
{"username":"admin","password":"secret123"}
```

> [!TIP]
> Use `--raw` for scripting and piping. The output has no trailing newline.

> [!NOTE]
> Timestamps respect the `TZ` environment variable. Use `TZ=UTC` or `TZ=Asia/Tokyo` to change the displayed timezone.

Extract JSON fields with `jq`:

```ShellSession
user@host:~$ suve aws secret show --raw my-database-credentials | jq -r '.password'
secret123
```

With `--output=json` for structured output:

```ShellSession
user@host:~$ suve aws secret show --output=json my-database-credentials
{
  "name": "my-database-credentials",
  "arn": "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-database-credentials-AbCdEf",
  "versionId": "abc12345-1234-1234-1234-123456789012",
  "stages": ["AWSCURRENT"],
  "created": "2024-01-15T10:30:45Z",
  "value": "{\"username\":\"admin\",\"password\":\"secret123\"}"
}
```

```bash
# Show previous version by label
suve aws secret show my-database-credentials:AWSPREVIOUS

# Show specific version by ID
suve aws secret show my-database-credentials#abc12345-1234-1234-1234-123456789012

# Show 1 version ago
suve aws secret show my-database-credentials~1

# Use in scripts
CREDS=$(suve aws secret show --raw my-database-credentials)

# Pipe to file
suve aws secret show --raw my-ssl-certificate > cert.pem

# Pretty print JSON with raw output
suve aws secret show --raw --parse-json my-database-credentials
```

---

## suve aws secret log

Show secret version history, similar to `git log`.

Command aliases: `history`

```
suve aws secret log [options] <name>
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
| `--parse-json` | `-j` | `false` | Format JSON values before diffing (use with `--patch`) |
| `--oneline` | - | `false` | Compact one-line-per-version format |
| `--reverse` | - | `false` | Show oldest versions first |
| `--since` | - | - | Show versions created after this date (RFC3339 format) |
| `--until` | - | - | Show versions created before this date (RFC3339 format) |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

Basic version history:

```ShellSession
user@host:~$ suve aws secret log my-database-credentials
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
user@host:~$ suve aws secret log --patch my-database-credentials
```

Output will look like:

```diff
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
> Use `--patch` to review what changed in each secret rotation, similar to `git log -p`.

> [!NOTE]
> When using `--patch`, the command fetches the actual secret values for each version to compute diffs.

```bash
# Show last 5 versions
suve aws secret log --number 5 my-database-credentials

# Show diffs with JSON formatting
suve aws secret log --patch --parse-json my-database-credentials

# Show oldest versions first
suve aws secret log --reverse my-database-credentials

# Output as JSON for scripting
suve aws secret log --output=json my-database-credentials
```

---

## suve aws secret diff

Show differences between two secret versions in unified diff format.

```
suve aws secret diff <spec1> [spec2] | <name> <version1> [version2]
```

### Argument Formats

The diff command supports multiple argument formats for flexibility:

| Format | Args | Example | Description |
|--------|------|---------|-------------|
| full spec | 2 | `secret:AWSPREVIOUS secret:AWSCURRENT` | Both args include name and version |
| full spec | 1 | `secret:AWSPREVIOUS` | Compare specified version with AWSCURRENT |
| mixed | 2 | `secret:AWSPREVIOUS ':AWSCURRENT'` | First with version, second specifier only |
| partial spec | 2 | `secret ':AWSPREVIOUS'` | Name + specifier -> compare with AWSCURRENT |
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

> [!TIP]
> `~` without a number means `~1`. You can chain shifts: `~~` = `~1~1` = `~2`.

> [!NOTE]
> **Labels vs Shift**: Labels (`:AWSCURRENT`, `:AWSPREVIOUS`) point to specific tagged versions. Shift (`~N`) navigates by creation date order. After a secret update:
> - `:AWSCURRENT` = new value
> - `:AWSPREVIOUS` = old value
> - `~1` = 1 version ago (same as `:AWSPREVIOUS` after a single update)

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

### Examples

Compare AWSPREVIOUS with AWSCURRENT:

```ShellSession
user@host:~$ suve aws secret diff my-database-credentials:AWSPREVIOUS
```

Output will look like:

```diff
--- my-database-credentials#def67890
+++ my-database-credentials#abc12345
@@ -1 +1 @@
-{"password":"old-password"}
+{"password":"new-password"}
```

Compare using shift syntax:

```ShellSession
user@host:~$ suve aws secret diff my-database-credentials~1
```

Output will look like:

```diff
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
> Hint: To compare with previous version, use: suve aws secret diff my-secret~1
> Hint: or: suve aws secret diff my-secret:AWSPREVIOUS
> ```

### Partial Spec Format

> [!IMPORTANT]
> Partial spec format requires quoting specifiers to prevent potential shell interpretation:
> - `~` alone expands to `$HOME` in bash/zsh
> - `:` may have special meaning in some contexts

```bash
# Compare AWSPREVIOUS with AWSCURRENT
suve aws secret diff my-database-credentials ':AWSPREVIOUS' ':AWSCURRENT'

# Compare previous with current
suve aws secret diff my-database-credentials '~'

# Compare specific version IDs
suve aws secret diff my-database-credentials#abc12345 my-database-credentials#def67890

# Pipe to a file for review
suve aws secret diff my-database-credentials:AWSPREVIOUS > changes.diff
```

---

## suve aws secret list

List secrets.

Command aliases: `ls`

```
suve aws secret list [filter-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `filter-prefix` | No | Filter secrets by name prefix |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--filter` | - | - | Filter by regex pattern |
| `--show` | - | `false` | Show secret values |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```ShellSession
user@host:~$ suve aws secret list
my-database-credentials
my-api-key
production/database-credentials
staging/database-credentials
```

With prefix filter:

```ShellSession
user@host:~$ suve aws secret list production/
production/database-credentials
production/api-key
production/ssl-cert
```

```bash
# List all secrets
suve aws secret list

# List secrets with prefix
suve aws secret list production/

# Filter by regex pattern
suve aws secret list --filter '\.prod$'

# List with values
suve aws secret list --show production/

# Output as JSON for scripting
suve aws secret list --output=json production/
```

---

## suve aws secret create

Create a new secret.

```
suve aws secret create [options] <name> <value>
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

**Examples:**

```ShellSession
user@host:~$ suve aws secret create my-api-key "sk-1234567890"
Created secret my-api-key (version: abc12345-1234-1234-1234-123456789012)
```

With JSON value and description:

```ShellSession
user@host:~$ suve aws secret create -d "Production database credentials" my-database-credentials '{"username":"admin","password":"secret"}'
Created secret my-database-credentials (version: def67890-1234-1234-1234-123456789012)
```

> [!NOTE]
> `create` fails if the secret already exists. Use `suve aws secret update` to update an existing secret.

---

## suve aws secret update

Update an existing secret's value.

```
suve aws secret update [options] <name> <value>
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
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```ShellSession
user@host:~$ suve aws secret update my-database-credentials '{"username":"admin","password":"newpassword"}'
--- my-database-credentials (AWS)
+++ my-database-credentials (new)
@@ -1 +1 @@
-{"username":"admin","password":"oldpassword"}
+{"username":"admin","password":"newpassword"}

? Update secret my-database-credentials? [y/N] y
✓ Updated secret my-database-credentials (version: ghi11111-1234-1234-1234-123456789012)
```

```bash
# Update without confirmation (skips diff display)
suve aws secret update --yes my-api-key "new-key-value"
```

> [!TIP]
> When updating a secret, `suve aws secret update` shows a diff of the changes and prompts for confirmation. Use `--yes` to skip this review step.

> [!NOTE]
> - `update` fails if the secret doesn't exist. Use `suve aws secret create` to create a new secret.
> - The previous version automatically becomes AWSPREVIOUS.

---

## suve aws secret delete

Delete a secret.

Command aliases: `rm`

```
suve aws secret delete [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to delete |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--force` | - | `false` | Delete immediately without recovery window |
| `--recovery-window` | - | `30` | Days before permanent deletion (7-30). Ignored if `--force` is set. |
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

With recovery window (default):

```ShellSession
user@host:~$ suve aws secret delete my-old-secret
! Current value of my-old-secret:

  {"username":"admin","password":"oldpassword"}

! This will permanently delete: my-old-secret
? Continue? [y/N] y
! Scheduled deletion of secret my-old-secret (deletion date: 2024-02-14)
```

Immediate deletion:

```ShellSession
user@host:~$ suve aws secret delete --force my-old-secret
! Current value of my-old-secret:

  {"username":"admin","password":"oldpassword"}

! This will permanently delete: my-old-secret
? Continue? [y/N] y
! Permanently deleted secret my-old-secret
```

> [!WARNING]
> Without `--force`, the secret can be restored using `suve aws secret restore` until the deletion date.

> [!CAUTION]
> With `--force`, deletion is **immediate and irreversible**. The secret cannot be recovered.

```bash
# Delete with 30-day recovery window (default)
suve aws secret delete my-old-secret

# Delete with 7-day recovery window
suve aws secret delete --recovery-window 7 my-old-secret

# Delete immediately (no recovery possible)
suve aws secret delete --force my-old-secret
```

---

## suve aws secret restore

Restore a deleted secret that is pending deletion.

```
suve aws secret restore <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to restore |

**Examples:**

```ShellSession
user@host:~$ suve aws secret restore my-accidentally-deleted-secret
Restored secret my-accidentally-deleted-secret
```

> [!NOTE]
> - Only works for secrets deleted without `--force`
> - Must be done before the scheduled deletion date
> - Cannot restore secrets that have been permanently deleted

---

## suve aws secret tag

Add or update tags on an existing secret.

```
suve aws secret tag <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key=value` | Tag in key=value format (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws secret tag my-database-credentials env=prod team=platform
✓ Tagged secret my-database-credentials (2 tag(s))
```

```bash
# Add single tag
suve aws secret tag my-api-key env=prod

# Add multiple tags
suve aws secret tag my-api-key env=prod team=platform
```

---

## suve aws secret untag

Remove tags from an existing secret.

```
suve aws secret untag <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key` | Tag key to remove (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws secret untag my-database-credentials deprecated old-tag
✓ Untagged secret my-database-credentials (2 key(s))
```

```bash
# Remove single tag
suve aws secret untag my-api-key deprecated

# Remove multiple tags
suve aws secret untag my-api-key deprecated old-tag
```

---

## Staging Workflow

The staging workflow allows you to prepare changes locally before applying them to AWS.

> [!IMPORTANT]
> The staging workflow lets you prepare changes locally, review them, and apply when ready--just like `git add` -> `git diff --staged` -> `git commit`.

Working state is split per service and namespaced by provider scope: `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/param.json` and `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/secret.json`. The stash is stored alongside them at `~/.suve/staging/aws/<ACCOUNT_ID>/<REGION>/stash.json`. There is no longer any single `stage.json`.

> [!NOTE]
> The working-state files are encrypted at rest. The encryption key is resolved from the `SUVE_STAGING_KEY` environment variable (base64-encoded 32 bytes) if set, otherwise from an OS keychain (created on first use), otherwise the state is stored as plaintext with a warning.

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

## suve aws stage secret add

Stage a new secret for creation.

```
suve aws stage secret add [options] <name> [value]
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

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret add my-new-secret '{"key":"value"}'
Staged for creation: my-new-secret
```

```bash
# Stage with inline value
suve aws stage secret add my-new-secret '{"key":"value"}'

# Stage via editor
suve aws stage secret add my-new-secret

# Stage with description
suve aws stage secret add --description "API key for production" my-api-key "sk-1234567890"
```

---

## suve aws stage secret edit

Edit an existing secret and stage the changes.

```
suve aws stage secret edit [options] <name> [value]
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

**Behavior:**

1. If the secret is already staged, uses the staged value
2. Otherwise, fetches the current value from AWS
3. Opens your editor (`$EDITOR`, defaults to `vim`) if value not provided
4. If the value changed, stages the new value

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret edit my-database-credentials
Staged: my-database-credentials
```

```bash
# Edit via editor
suve aws stage secret edit my-database-credentials

# Edit with inline value
suve aws stage secret edit my-database-credentials '{"username":"admin","password":"newpassword"}'
```

---

## suve aws stage secret delete

Stage a secret for deletion.

```
suve aws stage secret delete [options] <name>
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
user@host:~$ suve aws stage secret delete my-old-secret
Staged for deletion: my-old-secret
```

```bash
# Stage for deletion with default 30-day recovery
suve aws stage secret delete my-old-secret

# Stage for deletion with 7-day recovery
suve aws stage secret delete --recovery-window 7 my-old-secret

# Stage for immediate deletion
suve aws stage secret delete --force my-old-secret
```

---

## suve aws stage secret status

Show staged changes for Secrets Manager secrets.

```
suve aws stage secret status [options] [name]
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
user@host:~$ suve aws stage secret status
Staged Secrets Manager changes (3):
  A my-new-secret
  M my-database-credentials
  D my-old-secret
```

If no changes are staged:

```ShellSession
user@host:~$ suve aws stage secret status
Secrets Manager:
  (no staged changes)
```

> [!TIP]
> Use `suve aws stage status` to show all staged changes (SSM Parameter Store + Secrets Manager combined).

---

## suve aws stage secret diff

Compare staged values with current AWS values.

```
suve aws stage secret diff [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (optional, shows all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager |

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret diff
```

Output will look like:

```diff
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
user@host:~$ suve aws stage secret diff my-old-secret
```

Output will look like:

```diff
--- my-old-secret (AWS)
+++ my-old-secret (staged for deletion)
@@ -1 +0,0 @@
-{"password":"deleted"}
```

> [!TIP]
> Use `--parse-json` to normalize JSON formatting before comparing, making it easier to spot actual content changes vs formatting differences.

---

## suve aws stage secret apply

Apply staged Secrets Manager changes to AWS.

Command aliases: `push`

```
suve aws stage secret apply [options] [name]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name (optional, applies all if not specified) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |
| `--ignore-conflicts` | - | `false` | Apply even if AWS was modified after staging |

> [!NOTE]
> Before applying, suve checks if the AWS resource was modified after staging. If a conflict is detected, the apply is rejected to prevent lost updates. Use `--ignore-conflicts` to force apply despite conflicts.

**Behavior:**

1. Reads all staged Secrets Manager changes
2. For each `create` operation: calls CreateSecret
3. For each `update` operation: calls PutSecretValue
4. For each `delete` operation: calls DeleteSecret
5. Removes successfully applied changes from stage
6. Keeps failed changes in stage for retry

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret apply
Applying Secrets Manager secrets...
Created my-new-secret (version: abc12345)
Updated my-database-credentials (version: def67890)
Deleted my-old-secret
```

> [!WARNING]
> `suve aws stage secret apply` applies changes to AWS immediately. Always review with `suve aws stage secret diff` first!

---

## suve aws stage secret reset

Unstage Secrets Manager changes or restore to a specific version.

```
suve aws stage secret reset [options] [spec]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `spec` | No | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--all` | - | `false` | Unstage all Secrets Manager secrets |

**Version Specifiers:**

| Specifier | Description |
|-----------|-------------|
| `<name>` | Unstage secret (remove from staging) |
| `<name>#<ver>` | Restore to specific version |
| `<name>~1` | Restore to 1 version ago |

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret reset my-database-credentials
Unstaged my-database-credentials

user@host:~$ suve aws stage secret reset --all
Unstaged all Secrets Manager changes
```

```bash
# Unstage specific secret
suve aws stage secret reset my-database-credentials

# Restore to specific version and stage
suve aws stage secret reset my-database-credentials#abc12345

# Restore to previous version and stage
suve aws stage secret reset my-database-credentials~1

# Unstage all Secrets Manager secrets
suve aws stage secret reset --all
```

> [!TIP]
> Use `suve aws stage reset` to unstage all changes (SSM Parameter Store + Secrets Manager combined).

---

## suve aws stage secret tag

Stage tag additions for a secret.

```
suve aws stage secret tag <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key=value` | Tag in key=value format (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret tag my-database-credentials env=prod team=platform
✓ Staged tags for: my-database-credentials
```

```bash
# Stage single tag
suve aws stage secret tag my-secret env=prod

# Stage multiple tags
suve aws stage secret tag my-secret env=prod team=platform
```

> [!NOTE]
> If the secret is not already staged, a tag-only change is created.

---

## suve aws stage secret untag

Stage tag removals for a secret.

```
suve aws stage secret untag <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key` | Tag key to remove (one or more) |

**Examples:**

```ShellSession
user@host:~$ suve aws stage secret untag my-database-credentials deprecated old-tag
✓ Staged tag removal for: my-database-credentials
```

```bash
# Stage single tag removal
suve aws stage secret untag my-secret deprecated

# Stage multiple tag removals
suve aws stage secret untag my-secret deprecated old-tag
```

> [!NOTE]
> If the secret is not already staged, a tag-only change is created.

---

## Staging (`suve aws stage`)

Staging (`stage add`/`edit`/`delete`/`status`/`diff`/`apply`/`reset`/`stash`) is currently AWS-only. See [Staging State Transitions](staging-state-transitions.md) for the underlying state model; the per-command staging docs live in the README staging section. Full multi-provider staging support is tracked in [#247](https://github.com/mpyw/suve/issues/247).
