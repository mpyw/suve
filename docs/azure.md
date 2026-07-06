# Azure Commands (Key Vault + App Configuration)

[<- Back to README](../README.md) | [AWS Commands](aws.md) | [Google Cloud Commands](gcloud.md)

> [!TIP]
> Invoke as `suve azure secret` (Key Vault; aliases `kv`, `keyvault`) and `suve azure param` (App Configuration; aliases `appconfig`, `ac`, `appcfg`); the group also answers to `az`, and `stage` to `stg`. You can drop the `azure` prefix when Azure is the only active provider for that service — see [Provider selection](../README.md#provider-selection).

Azure splits into two services under `suve azure`:

| Command | Azure service | Versioning |
|---------|---------------|------------|
| `suve azure secret` | Key Vault | Versioned by opaque ids, no labels |
| `suve azure param` | App Configuration | **Unversioned** (single current value) |

Azure also supports the local **staging workflow** via `suve azure stage` (or the bare `suve stage` alias when Azure is the only active staging backend). It is **per-service**, because Key Vault and App Configuration keep separate staging state:

- `suve azure stage secret` — Key Vault secrets. Full workflow (`add`/`edit`/`delete`/`status`/`diff`/`apply`/`reset`/`tag`/`untag`/`stash`). Versions are immutable, so a staged `edit` applies as a new version.
- `suve azure stage param` — App Configuration settings. Because App Configuration is **unversioned**, staging uses **last-write-wins** (no modified-after conflict check), and `tag`/`untag` are unavailable (tags aren't writable). Workflow: `add`/`edit`/`delete`/`status`/`diff`/`apply`/`reset`/`stash`.

Unlike `aws stage`, there is no cross-service `azure stage status`/`apply` aggregate — the two services have distinct staging scopes. See the [staging workflow](../README.md#staging-workflow) overview for the general flow.

## Authentication and Configuration

| Setting | Flag | Environment variable |
|---------|------|----------------------|
| Subscription | `--subscription` | `AZURE_SUBSCRIPTION_ID` |
| Resource group | `--resource-group` | `AZURE_RESOURCE_GROUP` |
| Key Vault name (secret) | `--vault-name` | `AZURE_KEYVAULT_NAME` |
| App Config store (param) | `--store-name` | `AZURE_APPCONFIG_NAME` |

Authentication uses the **DefaultAzureCredential** chain (environment, managed identity, Azure CLI, ...). The simplest way to set this up locally is:

```bash
az login
```

The subscription and resource group can be supplied per-invocation or globally via environment variables:

```bash
# Via flags
suve azure --subscription <sub-id> --resource-group my-rg secret list --vault-name my-vault

# Via environment variables
export AZURE_SUBSCRIPTION_ID=<sub-id>
export AZURE_RESOURCE_GROUP=my-rg
export AZURE_KEYVAULT_NAME=my-vault
suve azure secret list
```

---

# suve azure secret (Key Vault)

Git-style access to Azure Key Vault secrets.

> [!NOTE]
> Key Vault secrets are versioned by **opaque ids** (e.g. a 32-character hex string) and have **no staging labels**. Set the vault with `--vault-name` or `AZURE_KEYVAULT_NAME`.

## Version Specification

```
<name>[#VERSION][~SHIFT]*
```

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version by opaque id | `#abc123...` |
| `~` | One version ago | `~` = current - 1 |
| `~N` | N versions ago | `~2` = current - 2 |

> [!NOTE]
> Key Vault has **no `:LABEL` syntax**. Versions are addressed only by opaque id or shift.

> [!TIP]
> `~` without a number means `~1`. You can chain shifts: `~~` = `~1~1` = `~2`.

---

## suve azure secret show

Display a secret value with metadata.

```
suve azure secret show [options] <name[#VERSION][~SHIFT]*>
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
# Show current version
suve azure secret show my-secret --vault-name my-vault

# Show a specific version id
suve azure secret show my-secret#abc123 --vault-name my-vault

# Show previous version
suve azure secret show my-secret~ --vault-name my-vault

# Output raw value for piping (no trailing newline)
suve azure secret show --raw my-secret --vault-name my-vault | jq -r '.password'

# Structured JSON output
suve azure secret show --output=json my-secret --vault-name my-vault
```

> [!NOTE]
> Timestamps respect the `TZ` environment variable. Use `TZ=UTC` or `TZ=Asia/Tokyo` to change the displayed timezone.

---

## suve azure secret log

Show secret version history, similar to `git log`.

Command aliases: `history`

```
suve azure secret log [options] <name>
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

Each version is listed with its opaque id, state (`enabled`/`disabled`), and creation date. Output is sorted with the most recent version first (use `--reverse` to flip).

> [!NOTE]
> Disabled versions have no accessible value, so their `--patch` diffs are skipped.

**Examples:**

```bash
# Show last 10 versions
suve azure secret log my-secret --vault-name my-vault

# Show diffs between consecutive versions
suve azure secret log --patch my-secret --vault-name my-vault

# Compact one-line format
suve azure secret log --oneline my-secret --vault-name my-vault

# Output as JSON
suve azure secret log --output=json my-secret --vault-name my-vault
```

---

## suve azure secret diff

Show differences between two secret versions in unified diff format.

```
suve azure secret diff [options] <spec1> [spec2] | <name> <version1> [version2]
```

If only one version/spec is specified, it is compared against the **current** version.

### Version Specifiers

| Specifier | Description | Example |
|-----------|-------------|---------|
| `#VERSION` | Specific version by opaque id | `#abc123...` |
| `~` | One version ago | `~` = current - 1 |
| `~N` | N versions ago | `~2` = current - 2 |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```bash
# Compare previous with current
suve azure secret diff my-secret~ --vault-name my-vault

# Compare two version ids
suve azure secret diff my-secret#abc my-secret#def --vault-name my-vault

# Format JSON values before diffing
suve azure secret diff --parse-json my-secret~ --vault-name my-vault

# Output comparison as JSON
suve azure secret diff --output=json my-secret~ --vault-name my-vault
```

---

## suve azure secret list

List secrets in the vault.

Command aliases: `ls`

```
suve azure secret list [options] [filter-prefix]
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
# List all secrets in the vault
suve azure secret list --vault-name my-vault

# List secrets starting with "prod"
suve azure secret list prod --vault-name my-vault

# List with values
suve azure secret list --show prod --vault-name my-vault

# Output as JSON
suve azure secret list --output=json prod --vault-name my-vault
```

---

## suve azure secret create

Create a new secret. The given value becomes the secret's first version.

```
suve azure secret create [options] <name> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `value` | Secret value |

**Examples:**

```bash
# Create a simple secret
suve azure secret create my-api-key "sk-12345" --vault-name my-vault

# Create a JSON secret
suve azure secret create my-config '{"host":"db"}' --vault-name my-vault
```

> [!NOTE]
> `create` is for new secrets only. To add a new version to an existing secret, use `suve azure secret update`. To add tags after creation, use `suve azure secret tag`.

---

## suve azure secret update

Update a secret's value by adding a new version. The new version becomes the current one; prior versions remain accessible by id.

```
suve azure secret update [options] <name> <value>
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
suve azure secret update my-api-key "new-value" --vault-name my-vault

# Update without confirmation
suve azure secret update --yes my-api-key "new-value" --vault-name my-vault
```

> [!NOTE]
> `update` fails if the secret doesn't exist. Use `suve azure secret create` to create a new secret.

---

## suve azure secret delete

Delete a secret and all its versions.

Command aliases: `rm`

```
suve azure secret delete [options] <name>
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
suve azure secret delete my-secret --vault-name my-vault

# Delete without confirmation
suve azure secret delete --yes my-secret --vault-name my-vault
```

> [!NOTE]
> When the vault has soft-delete enabled, the secret is recoverable within the vault's retention window via the Azure portal/CLI; otherwise deletion is permanent.

---

## suve azure secret tag

Add or update tags on an existing secret.

```
suve azure secret tag [options] <name> <key=value>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key=value` | Tag in key=value format (one or more) |

**Examples:**

```bash
# Add single tag
suve azure secret tag my-api-key env=prod --vault-name my-vault

# Add multiple tags
suve azure secret tag my-api-key env=prod team=backend --vault-name my-vault
```

> [!NOTE]
> If a tag key already exists, its value is updated.

---

## suve azure secret untag

Remove tags from an existing secret.

```
suve azure secret untag [options] <name> <key>...
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `name` | Secret name |
| `key` | Tag key to remove (one or more) |

**Examples:**

```bash
# Remove single tag
suve azure secret untag my-api-key deprecated --vault-name my-vault

# Remove multiple tags
suve azure secret untag my-api-key env team --vault-name my-vault
```

> [!NOTE]
> Non-existent keys are silently ignored.

---

# suve azure param (App Configuration)

Access to Azure App Configuration key-values.

> [!IMPORTANT]
> App Configuration is **UNVERSIONED**: each key (with the default label) holds a single current value with **no history**. Version specifiers (`#VERSION`, `~SHIFT`, `:LABEL`) are **rejected with a clear error**. Set the store with `--store-name` or `AZURE_APPCONFIG_NAME`.

Because App Configuration has no versions, several commands behave differently from their AWS / Key Vault / Google Cloud counterparts:

| Command | Status |
|---------|--------|
| `show`, `list`/`ls`, `create`, `update`, `delete`/`rm` | Supported |
| `diff` | Supported, but compares **two distinct keys** (not two versions) |
| `log`/`history` | **Unsupported** -- always reports that history is unsupported |
| `tag`, `untag` | **Unsupported** -- report an unsupported error (SDK limitation) |

---

## suve azure param show

Display an App Configuration setting value with metadata.

```
suve azure param show [options] <key>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | Setting key (no version specifier -- specifiers are rejected) |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Pretty-print JSON values (keys sorted alphabetically) |
| `--no-pager` | - | `false` | Disable pager output |
| `--raw` | - | `false` | Output raw value only without metadata (for piping) |
| `--output` | - | `text` | Output format: `text` (default) or `json` (cannot be used with `--raw`) |

**Examples:**

```bash
# Show the setting value
suve azure param show my-key --store-name my-store

# Output raw value for piping (no trailing newline)
suve azure param show --raw my-key --store-name my-store

# Structured JSON output
suve azure param show --output=json my-key --store-name my-store
```

> [!NOTE]
> Version specifiers (`#VERSION`, `~SHIFT`, `:LABEL`) are rejected with a clear error because App Configuration is unversioned.

---

## suve azure param log

App Configuration has no version history.

Command aliases: `history`

```
suve azure param log [options] <key>
```

> [!WARNING]
> This command exists only for parity with the other providers. It **always reports that version history is unsupported** (it never crashes). The `--number`, `--patch`, `--parse-json`, `--oneline`, `--reverse`, `--since`, and `--until` flags are accepted but have no effect.

**Example:**

```bash
suve azure param log my-key --store-name my-store    # Reports "history unsupported"
```

---

## suve azure param diff

Show differences between two settings in unified diff format.

```
suve azure param diff [options] <key1> [key2]
```

> [!NOTE]
> App Configuration is unversioned, so `diff` compares **two distinct keys** rather than two versions of one key. Version specifiers (`#VERSION`, `~SHIFT`, `:LABEL`) are rejected with a clear error.

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key1` | First setting key |
| `key2` | Second setting key |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--parse-json` | `-j` | `false` | Format JSON values before diffing |
| `--no-pager` | - | `false` | Disable pager output |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```bash
# Compare two settings
suve azure param diff key-a key-b --store-name my-store

# Output comparison as JSON
suve azure param diff --output=json key-a key-b --store-name my-store
```

---

## suve azure param list

List settings (key-values) in the store.

Command aliases: `ls`

```
suve azure param list [options] [filter-prefix]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `filter-prefix` | No | Filter keys by prefix |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--filter` | - | - | Filter by regex pattern (client-side) |
| `--show` | - | `false` | Show setting values (format: `<key><TAB><value>`) |
| `--output` | - | `text` | Output format: `text` (default) or `json` |

**Examples:**

```bash
# List all settings in the store
suve azure param list --store-name my-store

# List settings starting with "app/"
suve azure param list app/ --store-name my-store

# List with values
suve azure param list --show app/ --store-name my-store

# Output as JSON
suve azure param list --output=json app/ --store-name my-store
```

---

## suve azure param create

Create a new setting (key-value).

```
suve azure param create [options] <key> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | Setting key |
| `value` | Setting value |

**Examples:**

```bash
# Create a simple setting
suve azure param create app/timeout "30" --store-name my-store

# Create a JSON setting
suve azure param create app/config '{"host":"db"}' --store-name my-store
```

> [!NOTE]
> `create` is for new keys only. To change the value of an existing key, use `suve azure param update`.

---

## suve azure param update

Update a setting's value. App Configuration is unversioned, so the value is replaced in place.

```
suve azure param update [options] <key> <value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | Setting key |
| `value` | New value |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```bash
# Replace the value
suve azure param update app/timeout "60" --store-name my-store

# Update without confirmation
suve azure param update --yes app/timeout "60" --store-name my-store
```

> [!NOTE]
> Use `suve azure param create` to create a new setting. Because App Configuration is unversioned, there is no prior version to fall back to after an update.

---

## suve azure param delete

Delete a setting (key-value).

Command aliases: `rm`

```
suve azure param delete [options] <key>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | Setting key to delete |

**Options:**

| Option | Alias | Default | Description |
|--------|-------|---------|-------------|
| `--yes` | - | `false` | Skip confirmation prompt |

**Examples:**

```bash
# Delete (with confirmation)
suve azure param delete app/timeout --store-name my-store

# Delete without confirmation
suve azure param delete --yes app/timeout --store-name my-store
```

> [!CAUTION]
> Deletion removes the current value for the key (default label). App Configuration is unversioned, so there is no prior version to fall back to.

---

## suve azure param tag / untag (unsupported)

```
suve azure param tag [options] <key> <key=value>...
suve azure param untag [options] <key> <key>...
```

> [!WARNING]
> Tag mutation is **unsupported** for App Configuration. The `azappconfig` SDK cannot write setting tags without clearing them, so these commands report an unsupported error rather than losing data. Tags set out-of-band (e.g. via the Azure portal) are still shown by `suve azure param show`.

**Examples:**

```bash
suve azure param tag app/timeout env=prod --store-name my-store     # Reports "tags unsupported"
suve azure param untag app/timeout env --store-name my-store        # Reports "tags unsupported"
```
