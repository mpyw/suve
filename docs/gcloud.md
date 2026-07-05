# Google Cloud Secret Manager Commands

[<- Back to README](../README.md) | [AWS Commands](aws.md) | [Azure Commands](azure.md)

> [!TIP]
> Invoke as `suve gcloud secret`. You can drop the `gcloud` prefix (`suve secret`) when Google Cloud is the only active secret provider — see [Provider selection](../README.md#provider-selection).

Primary command: `gcloud secret`

`suve gcloud secret` provides Git-style access to Google Cloud Secret Manager, mirroring the AWS `secret` commands where the service allows.

> [!NOTE]
> Google Cloud secrets are integer-versioned (`1`, `2`, `3`, ... or `latest`) and have **no staging labels**.

Google Cloud also supports the local **staging workflow** via `suve gcloud stage` (or the bare `suve stage` alias when Google Cloud is the only active staging backend). Because Google Cloud is secret-only, `gcloud stage` operates on secrets directly: `add`, `edit`, `delete`, `status`, `diff`, `apply`, `reset`, `tag`, `untag`, and `stash`. Since Secret Manager versions are immutable, a staged `edit` applies as a new version, and there are no force / recovery-window delete options. See the [staging workflow](../README.md#staging-workflow) overview for the general flow.

## Authentication and Configuration

| Setting | Flag | Environment variable |
|---------|------|----------------------|
| Project | `--project` | `GOOGLE_CLOUD_PROJECT` |

Authentication uses **Application Default Credentials (ADC)**. The simplest way to set this up locally is:

```bash
gcloud auth application-default login
```

The project id can be supplied per-invocation with `--project` or globally via the `GOOGLE_CLOUD_PROJECT` environment variable:

```bash
# Via flag
suve gcloud --project my-project secret list

# Via environment variable
export GOOGLE_CLOUD_PROJECT=my-project
suve gcloud secret list
```

## Version Specification

Google Cloud secrets are integer-versioned. The version spec is:

```
<name>[#VERSION][~SHIFT]*
```

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version by integer number | `#3` = version 3 |
| `~` | One enabled version ago | `~` = latest - 1 |
| `~N` | N enabled versions ago | `~2` = latest - 2 |

Specifiers can be combined: `my-secret#5~2` means "version 5, then 2 back".

> [!NOTE]
> Unlike AWS Secrets Manager, Google Cloud Secret Manager has **no `:LABEL` syntax**. Versions are addressed only by integer number, `latest`, or shift.

> [!TIP]
> `~` without a number means `~1`. You can chain shifts: `~~` = `~1~1` = `~2`. Shift counts only **enabled** versions; disabled and destroyed versions are skipped.

---

## suve gcloud secret show

Display a secret value with metadata.

```
suve gcloud secret show [options] <name[#VERSION][~SHIFT]*>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name with optional version specifier |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Pretty-print JSON values (keys sorted alphabetically) |
| `--no-pager` | - | `false` | Disable pager output |
| `--raw` | - | `false` | Output raw value only without metadata (for piping) |
| `--output` | - | `text` | Output format: `text` (default) or `json` (cannot be used with `--raw`) |

**Examples:**

```bash
# Show latest version
suve gcloud secret show my-secret

# Show version 3
suve gcloud secret show my-secret#3

# Show previous version
suve gcloud secret show my-secret~

# Output raw value for piping (no trailing newline)
suve gcloud secret show --raw my-secret | jq -r '.password'

# Structured JSON output
suve gcloud secret show --output=json my-secret
```

> [!NOTE]
> Timestamps respect the `TZ` environment variable. Use `TZ=UTC` or `TZ=Asia/Tokyo` to change the displayed timezone.

---

## suve gcloud secret log

Show secret version history, similar to `git log`.

Command aliases: `history`

```
suve gcloud secret log [options] <name>
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

Each version is listed with its integer number, state (`enabled`/`disabled`/`destroyed`), and creation date. Output is sorted with the most recent version first (use `--reverse` to flip).

> [!NOTE]
> Disabled and destroyed versions have no accessible value, so their `--patch` diffs are skipped.

**Examples:**

```bash
# Show last 10 versions
suve gcloud secret log my-secret

# Show last 5 versions
suve gcloud secret log --number 5 my-secret

# Show diffs between consecutive versions
suve gcloud secret log --patch my-secret

# Compact one-line format
suve gcloud secret log --oneline my-secret

# Output as JSON for scripting
suve gcloud secret log --output=json my-secret
```

---

## suve gcloud secret diff

Show differences between two secret versions in unified diff format.

```
suve gcloud secret diff [options] <spec1> [spec2] | <name> <version1> [version2]
```

If only one version/spec is specified, it is compared against the **latest** version.

### Version Specifiers

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version by integer number | `#3` |
| `~` | One version ago | `~` = latest - 1 |
| `~N` | N versions ago | `~2` = latest - 2 |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```bash
# Compare previous with latest
suve gcloud secret diff my-secret~

# Compare version 1 with version 2
suve gcloud secret diff my-secret#1 my-secret#2

# Format JSON values before diffing
suve gcloud secret diff --parse-json my-secret~

# Output comparison as JSON
suve gcloud secret diff --output=json my-secret~
```

> [!TIP]
> Use full spec format (embedding `#`/`~` within the name) to avoid shell quoting issues.

---

## suve gcloud secret list

List secrets in the project.

Command aliases: `ls`

```
suve gcloud secret list [options] [filter-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `filter-prefix` | No | Filter secrets by name prefix |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--filter` | - | - | Filter by regex pattern (client-side) |
| `--show` | - | `false` | Show secret values (format: `<name><TAB><value>`) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```bash
# List all secrets in the project
suve gcloud secret list

# List secrets starting with "prod"
suve gcloud secret list prod

# List with values
suve gcloud secret list --show prod

# Filter by regex pattern
suve gcloud secret list --filter '\-prod$'

# Output as JSON
suve gcloud secret list --output=json prod
```

---

## suve gcloud secret create

Create a new secret. The secret is created with automatic replication, and the given value becomes its first version.

```
suve gcloud secret create [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | Secret value |

**Examples:**

```bash
# Create a simple secret
suve gcloud secret create my-api-key "sk-12345"

# Create a JSON secret
suve gcloud secret create my-config '{"host":"db"}'
```

> [!NOTE]
> `create` is for new secrets only. To add a new version to an existing secret, use `suve gcloud secret update`. To add labels after creation, use `suve gcloud secret tag`.

---

## suve gcloud secret update

Update a secret's value by adding a new version. The new version becomes the latest; prior versions remain accessible by number.

```
suve gcloud secret update [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | New secret value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```bash
# Add a new version
suve gcloud secret update my-api-key "new-value"

# Update without confirmation
suve gcloud secret update --yes my-api-key "new-value"
```

> [!NOTE]
> `update` fails if the secret doesn't exist. Use `suve gcloud secret create` to create a new secret.

---

## suve gcloud secret delete

Permanently delete a secret and all its versions.

Command aliases: `rm`

```
suve gcloud secret delete [options] <name>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name to delete |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```bash
# Delete (with confirmation)
suve gcloud secret delete my-secret

# Delete without confirmation
suve gcloud secret delete --yes my-secret
```

> [!CAUTION]
> Unlike AWS Secrets Manager, Google Cloud has **no recovery window**. Deletion is immediate and permanent; there is no restore command.

---

## suve gcloud secret tag

Add or update **labels** on an existing secret. On Google Cloud, `tag`/`untag` manage secret labels.

```
suve gcloud secret tag [options] <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key=value` | Label in key=value format (one or more) |

**Examples:**

```bash
# Add single label
suve gcloud secret tag my-api-key env=prod

# Add multiple labels
suve gcloud secret tag my-api-key env=prod team=backend
```

> [!NOTE]
> If a label key already exists, its value is updated.

---

## suve gcloud secret untag

Remove **labels** from an existing secret.

```
suve gcloud secret untag [options] <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key` | Label key to remove (one or more) |

**Examples:**

```bash
# Remove single label
suve gcloud secret untag my-api-key deprecated

# Remove multiple labels
suve gcloud secret untag my-api-key env team
```

> [!NOTE]
> Non-existent keys are silently ignored.
