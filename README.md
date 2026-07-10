<div align="center">
  <img src="gui/build/appicon.png" alt="suve" width="128" height="128">
  <h1>suve</h1>
  <p><strong>S</strong>ecret <strong>U</strong>nified <strong>V</strong>ersioning <strong>E</strong>xplorer</p>
  <p>for &nbsp; <a href="https://aws.amazon.com/"><img width="60" alt="AWS" src="https://github.com/user-attachments/assets/03a2fde5-bf10-45f3-8bf0-722b10b6c97f" /></a>　<a href="https://cloud.google.com/"><img width="60" alt="Google Cloud" src="https://github.com/user-attachments/assets/d6e64422-dd06-482b-90a9-e2eb1e8c3de5" /></a>　<a href="https://azure.microsoft.com/"><img width="60" alt="Azure" src="https://github.com/user-attachments/assets/5095c477-6f77-4cea-84b6-50eff8e61df4" /></a></p>

  [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/suve.svg)](https://pkg.go.dev/github.com/mpyw/suve)
  [![Test](https://github.com/mpyw/suve/actions/workflows/test.yml/badge.svg)](https://github.com/mpyw/suve/actions/workflows/test.yml)
  [![Codecov](https://codecov.io/gh/mpyw/suve/graph/badge.svg)](https://codecov.io/gh/mpyw/suve)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
</div>

> [!NOTE]
> This project was written by AI (Claude Code).

A **Git-like CLI/GUI** for AWS Parameter Store / Secrets Manager, Google Cloud Secret Manager, and Azure Key Vault / App Configuration. Familiar commands like `show`, `log`, `diff`, and a **staging workflow** for safe, reviewable changes.

<p align="center">
  <img src="demo/cli-demo.gif" alt="CLI Demo" width="800">
</p>

<p align="center">
  <img src="demo/gui-demo.gif" alt="GUI Demo" width="800">
</p>

## Features

- **Git-like commands**: `show`, `log`, `diff`, `ls`, `tag`
- **Staging workflow**: `edit` → `status` → `diff` → `apply` (review changes before applying), plus `export` / `import` for portable, per-service snapshots
- **Version navigation**: `#VERSION`, `~SHIFT`, `:LABEL` syntax
- **Colored diff output**: Easy-to-read unified diff format
- **Multi-cloud**: [AWS SSM Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html) / [Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html), [Google Cloud Secret Manager](https://cloud.google.com/secret-manager/docs), and [Azure Key Vault](https://learn.microsoft.com/en-us/azure/key-vault/) / [App Configuration](https://learn.microsoft.com/en-us/azure/azure-app-configuration/)
- **Secure staging**: Working staging state is encrypted at rest with a data key stored in the OS keychain (override with `SUVE_STAGING_KEY`; plaintext fallback with a warning if unavailable). Exported snapshot files carry a separately passphrase-encrypted payload ([Argon2](https://en.wikipedia.org/wiki/Argon2) + [AES-GCM](https://en.wikipedia.org/wiki/Galois/Counter_Mode); an empty passphrase writes plaintext).
- **GUI mode**: Desktop application via `--gui` flag (built with [Wails](https://wails.io/))

## Installation

> [!NOTE]
> On Linux, `suve` requires GTK3 and WebKit2GTK for GUI support. Use the CLI-only version if you only need CLI functionality.

### Using [mise](https://mise.jdx.dev/) (macOS/Linux/Windows)

suve is installable directly from GitHub Releases via mise's `github` backend — no extra registry required:

```bash
# Full version (CLI + GUI)
mise use -g "github:mpyw/suve"

# CLI-only version (no GUI dependencies, recommended for Linux. Not available on macOS/Windows)
mise use -g "github:mpyw/suve[matching=cli]"
```

> [!TIP]
> Committing to a shared `mise.toml` used across OSes? Use a single cross-platform rule instead:
> ```toml
> [tools]
> "github:mpyw/suve" = { version = "latest", matching_regex = "(darwin|windows|cli_[0-9.]+_linux)" }
> ```

### Using [aqua](https://aquaproj.github.io/) (macOS/Linux/Windows)

suve is available in the [standard aqua registry](https://github.com/aquaproj/aqua-registry):

```bash
aqua g -i mpyw/suve
```

The registry picks the right asset per platform automatically: the self-contained GUI build on macOS/Windows, and the dependency-free CLI-only static build on Linux (supported from v1.3.0).

### Using [Homebrew](https://brew.sh/) (macOS/Linux)

```bash
# Full version (CLI + GUI)
brew install mpyw/tap/suve

# CLI-only version (no GUI dependencies, recommended for Linux)
brew install mpyw/tap/suve-cli
```

### Using [Scoop](https://scoop.sh/) (Windows)

```powershell
scoop bucket add mpyw https://github.com/mpyw/scoop-bucket.git
scoop install suve
```

<details>
<summary>Linux (.deb / .rpm)</summary>

Download packages from [GitHub Releases](https://github.com/mpyw/suve/releases):

**Debian/Ubuntu (.deb):**

```bash
export VERSION=0.0.0
export ARCH=amd64  # or arm64
export WEBKIT_SUFFIX=""  # use "_webkit2_41" for Ubuntu 24.04+

# CLI-only (recommended, no GUI dependencies)
curl -LO "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve-cli_${VERSION}-1_${ARCH}.deb"
sudo dpkg -i "suve-cli_${VERSION}-1_${ARCH}.deb"

# Full version (CLI + GUI, requires GTK3 and WebKit2GTK)
curl -LO "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve${WEBKIT_SUFFIX}_${VERSION}-1_${ARCH}.deb"
sudo dpkg -i "suve${WEBKIT_SUFFIX}_${VERSION}-1_${ARCH}.deb"
```

**Note:** Ubuntu 22.04/Debian uses webkit2gtk-4.0 (default). Ubuntu 24.04+ uses webkit2gtk-4.1 (set `WEBKIT_SUFFIX="_webkit2_41"`).

**Red Hat/Fedora (.rpm):**

```bash
export VERSION=0.0.0
export ARCH=x86_64  # or aarch64
export WEBKIT_SUFFIX=""  # use "_webkit2_41" for Fedora 40+

# CLI-only (recommended, no GUI dependencies)
curl -LO "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve-cli-${VERSION}-1.${ARCH}.rpm"
sudo rpm -i "suve-cli-${VERSION}-1.${ARCH}.rpm"

# Full version (CLI + GUI, requires GTK3 and WebKit2GTK)
curl -LO "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve${WEBKIT_SUFFIX}-${VERSION}-1.${ARCH}.rpm"
sudo rpm -i "suve${WEBKIT_SUFFIX}-${VERSION}-1.${ARCH}.rpm"
```

**Note:** Fedora 39 and earlier uses webkit2gtk-4.0 (default). Fedora 40+ uses webkit2gtk-4.1 (set `WEBKIT_SUFFIX="_webkit2_41"`).

</details>

<details>
<summary>Using <code>go install</code> (CLI only)</summary>

```bash
go install github.com/mpyw/suve/cmd/suve@latest
```

**Note:** `go install` builds CLI only. GUI requires pre-built assets that are not included in the Go module. For GUI support, use a [package manager](#installation) or [build from source](#building-from-source).

</details>

<details>
<summary>Using <code>go tool</code> (CLI only, Go 1.25+)</summary>

```bash
# Add to go.mod as a tool dependency
go get -tool github.com/mpyw/suve/cmd/suve@latest

# Run via go tool
go tool suve param show /my/param
```

</details>

<details>
<summary>Building from Source</summary>

For platforms without pre-built packages (e.g., Arch Linux) or if you need the latest development version with GUI:

Requires Go 1.25+.

```bash
git clone https://github.com/mpyw/suve.git
cd suve
```

**CLI only:**

```bash
mise build-cli
# Binary: bin/suve
```

**CLI + GUI** (requires [Wails CLI](https://wails.io/) and [Node.js](https://nodejs.org/)):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd gui && wails build -tags production -skipbindings
# Binary: gui/build/bin/gui
```

**Build dependencies for GUI:**

| Platform | Dependencies |
|----------|-------------|
| All | [Node.js](https://nodejs.org/) (for frontend build) |
| macOS | Xcode Command Line Tools |
| Windows | [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) (usually pre-installed) |

<details>
<summary>Linux build dependencies and webkit2gtk-4.1 support</summary>

Linux requires GTK3 and WebKit2GTK:

| Platform | Dependencies |
|----------|-------------|
| Ubuntu 22.04/Debian | `sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev` |
| Ubuntu 24.04+ | `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev` |
| Fedora 39 | `sudo dnf install gtk3-devel webkit2gtk4.0-devel` |
| Fedora 40+ | `sudo dnf install gtk3-devel webkit2gtk4.1-devel` |
| Arch Linux | `sudo pacman -S gtk3 webkit2gtk-4.1` |

For **webkit2gtk-4.1** (Ubuntu 24.04+, Fedora 40+, Arch Linux), use the `webkit2_41` build tag:

```bash
cd gui && wails build -tags production,webkit2_41 -skipbindings
```

</details>

</details>

## Shell Completion

suve can generate completion scripts for **bash**, **zsh**, **fish**, and **PowerShell**. Source the output to enable completion for commands, subcommands, and flags.

```bash
# bash — add to ~/.bashrc
source <(suve completion bash)

# zsh — add to ~/.zshrc
source <(suve completion zsh)

# fish
suve completion fish > ~/.config/fish/completions/suve.fish

# PowerShell — add to $PROFILE
suve completion pwsh | Out-String | Invoke-Expression
```

## Authentication

suve talks to each cloud's **data plane** using that cloud's own SDK credential chain — the same one the native CLI (`aws` / `gcloud` / `az`) uses. There is nothing suve-specific to configure: sign in the normal way, then point suve at the resource with an environment variable (or the equivalent flag).

<table>
<thead>
<tr><th>Provider</th><th>Sign in (identity)</th><th>Point at the resource</th></tr>
</thead>
<tbody>
<tr>
<td><b>AWS</b></td>
<td>

```bash
aws sso login \
  --profile prod
```

</td>
<td>

```bash
export AWS_PROFILE=prod
export AWS_REGION=us-east-1
```

</td>
</tr>
<tr>
<td><b>Google<br>Cloud</b></td>
<td>

```bash
gcloud auth \
  application-default \
  login
```

</td>
<td>

```bash
export GOOGLE_CLOUD_PROJECT=my-project
```

</td>
</tr>
<tr>
<td><b>Azure</b></td>
<td>

```bash
az login
```

</td>
<td>

```bash
# secret
export AZURE_KEYVAULT_NAME=my-vault
# param
export AZURE_APPCONFIG_NAME=my-store
```

</td>
</tr>
</tbody>
</table>

> [!NOTE]
> - Every value has a flag equivalent: `--profile`/`--region`, `--project`, `--vault-name`/`--store-name`.
> - **AWS** — the standard [credential chain](https://docs.aws.amazon.com/sdkref/latest/guide/standardized-credentials.html): SSO, static keys, `~/.aws/credentials`, or an IAM role. With [aws-vault](https://github.com/99designs/aws-vault): `aws-vault exec prod -- suve param show /my/param`.
> - **Google Cloud** — [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials); or a service-account key via `GOOGLE_APPLICATION_CREDENTIALS`.
> - **Azure** — the [DefaultAzureCredential](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication) chain; or a service principal via `AZURE_CLIENT_ID` / `AZURE_CLIENT_SECRET` / `AZURE_TENANT_ID`. `az login` sets no environment variables — it caches credentials under `~/.azure`, which suve reuses. The Key Vault / App Configuration name is a globally-unique endpoint, so **no subscription or resource group is needed**.

## Getting Started

### Basic Commands

```ShellSession
user@host:~$ suve param show /app/config/database-url
Name: /app/config/database-url
Version: 3
Type: SecureString
Modified: 2024-01-15T10:30:45Z

  postgres://db.example.com:5432/myapp

user@host:~$ suve param show --raw /app/config/database-url
postgres://db.example.com:5432/myapp
```

The `show` command displays value with metadata; `--raw` outputs raw value for piping:

```bash
# Use in scripts
DB_URL=$(suve param show --raw /app/config/database-url)

# Pipe to file
suve param show --raw /app/config/ssl-cert > cert.pem
```

### Version History with `log`

View version history, just like `git log`:

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

Use `--patch` to see what changed in each version:

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
> Add `--parse-json` to pretty-print JSON values before diffing. This normalizes formatting and sorts keys alphabetically, so you can focus on the actual content changes rather than formatting differences:
> ```bash
> suve param log --patch --parse-json /app/config/credentials
> ```

### Comparing Versions with `diff`

Compare previous version with latest (most common use case):

```ShellSession
user@host:~$ suve param diff /app/config/database-url~
```

Output will look like:

```diff
--- /app/config/database-url#2
+++ /app/config/database-url#3
@@ -1 +1 @@
-postgres://old-db.example.com:5432/myapp
+postgres://db.example.com:5432/myapp
```

Compare any two specific versions:

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

### Staging Workflow

> [!NOTE]
> The staging workflow lets you prepare changes locally, review them, and apply when ready—just like `git add` → `git diff --staged` → `git commit`.
> For detailed documentation, see [Staging State Transitions](docs/staging-state-transitions.md).

> [!TIP]
> Staged values live in encrypted files under `~/.suve/staging/`. Use `suve stage export <dir>` to write them to portable snapshot files and `suve stage import <dir>` to restore them later.

**1. Stage changes** (opens editor or accepts value directly):

> [!TIP]
> To use VSCode or Cursor as your editor, set `export VISUAL='code --wait'` or `export VISUAL='cursor --wait'` in your shell profile.

```ShellSession
user@host:~$ suve stage param add /app/config/new-param "my-value"
✓ Staged for creation: /app/config/new-param

user@host:~$ suve stage param edit /app/config/database-url
✓ Staged: /app/config/database-url

user@host:~$ suve stage param delete /app/config/old-param
✓ Staged for deletion: /app/config/old-param
```

**2. Review staged changes**:

```ShellSession
user@host:~$ suve stage status
Staged SSM Parameter Store changes (3):
  A /app/config/new-param
  M /app/config/database-url
  D /app/config/old-param

user@host:~$ suve stage diff
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

--- /app/config/old-param#2 (AWS)
+++ /app/config/old-param (staged for deletion)
@@ -1 +0,0 @@
-deprecated-value
```

**3. Apply changes**:

```ShellSession
user@host:~$ suve stage apply
Applying SSM Parameter Store parameters...
✓ Created /app/config/new-param
✓ Updated /app/config/database-url
✓ Deleted /app/config/old-param
```

**Reset if needed**:

```bash
# Unstage specific parameter
suve stage param reset /app/config/database-url

# Unstage all
suve stage reset --all
```

> [!TIP]
> `suve stage apply` prompts for confirmation before applying. Use `--yes` to skip the prompt.

**Save changes for later** (export / import):

```bash
# Export staged changes to a directory as one file per service
# (param.json / secret.json); prompts for a passphrase.
# By default the working staging area is cleared; use --keep to retain it.
suve stage export ./backup

# Restore them into the working staging area later
suve stage import ./backup

# Export a single service to a specific file
suve stage param export ./param-backup.json

# Import a single service from a specific file
suve stage param import ./param-backup.json
```

> [!NOTE]
> `export` writes the working area out wholesale (no merge). `import` prompts to Merge or Overwrite only when the working area already holds changes; pass `--merge` / `--overwrite` to choose non-interactively.

> [!NOTE]
> See [Staging State Transitions](docs/staging-state-transitions.md) for detailed staging documentation.

## Version Specification

Navigate versions with Git-like syntax.

### AWS SSM Parameter Store

> [!NOTE]
> SSM Parameter Store uses numeric version numbers (1, 2, 3, ...) that auto-increment on each update.

```
<name>[#VERSION][~SHIFT]*
where ~SHIFT = ~ | ~N  (repeatable, cumulative)
```

| Syntax | Description |
|--------|-------------|
| `/my/param` | Latest version |
| `/my/param#3` | Version 3 |
| `/my/param~1` | 1 version ago |
| `/my/param#5~2` | Version 5 minus 2 = Version 3 |
| `/my/param~~` | 2 versions ago (`~1~1`) |

### AWS Secrets Manager

> [!NOTE]
> Secrets Manager uses UUID version IDs and staging labels instead of numeric versions.
> `AWSCURRENT` and `AWSPREVIOUS` are special labels automatically managed by AWS—`AWSCURRENT` always points to the latest version.

```
<name>[#VERSION | :LABEL][~SHIFT]*
where ~SHIFT = ~ | ~N  (repeatable, cumulative)
```

| Syntax | Description |
|--------|-------------|
| `my-secret` | Current (AWSCURRENT) |
| `my-secret:AWSPREVIOUS` | Previous staging label |
| `my-secret#abc123` | Specific version ID |
| `my-secret~1` | 1 version ago |

> [!IMPORTANT]
> When specifying version-only syntax like `'#3'` or `':AWSPREVIOUS'`, you must use quotes to prevent shell interpretation of the `#` (comment) or `:` characters.

> [!TIP]
> `~` without a number means `~1`. You can chain them: `~~` = `~1~1` = `~2`

### Google Cloud Secret Manager

> [!NOTE]
> Google Cloud Secret Manager uses integer version numbers (1, 2, 3, ...) plus the `latest` alias. There are no staging labels, so `:LABEL` syntax does not apply.

| Syntax | Description |
|--------|-------------|
| `my-secret` | Latest version |
| `my-secret#3` | Version 3 |
| `my-secret~1` | 1 version ago |

### Azure Key Vault

> [!NOTE]
> Azure Key Vault versions are opaque version IDs. There are no staging labels, so `:LABEL` syntax does not apply.

| Syntax | Description |
|--------|-------------|
| `my-secret` | Current version |
| `my-secret#<id>` | Specific version ID |
| `my-secret~1` | 1 version ago |

### Azure App Configuration

> [!NOTE]
> Azure App Configuration is unversioned. `#`, `~`, and `:` are valid key characters — the whole argument is the literal key name, not a version spec — and `log` reports that history is unsupported.

## Providers

### Feature support

| Backend | Command | Versioning | Labels / Tags | Staging | GUI | Auth |
|---------|---------|------------|---------------|---------|------|------|
| [AWS Parameter Store](docs/aws.md) | `aws param` | ✅ numeric | ✅ tags | ✅ | ✅ | shared config/env/role |
| [AWS Secrets Manager](docs/aws.md) | `aws secret` | ✅ UUID + staging labels | ✅ tags | ✅ | ✅ | shared config/env/role |
| [Google Cloud Secret Manager](docs/gcloud.md) | `gcloud secret` | ✅ integer (`latest`) | ✅ labels | ✅ | ✅ | Application Default Credentials |
| [Azure Key Vault](docs/azure.md) | `azure secret` | ✅ opaque id | ✅ tags | ✅ | ✅ | DefaultAzureCredential |
| [Azure App Configuration](docs/azure.md) | `azure param` | ❌ unversioned | ✅ tags¹ | ✅² | ✅ | DefaultAzureCredential |

Read/write operations (`show`, `log`, `diff`, `list`, `create`, `update`, `delete`, `tag`, `untag`) are available on every backend, with these caveats: `restore` is available on AWS Secrets Manager and Azure Key Vault (soft-delete recovery); on Azure App Configuration `log` reports history unsupported and `#`/`~`/`:` are treated as literal key characters (not version specifiers). Only AWS Secrets Manager has staging labels (`:AWSCURRENT` etc.).

¹ App Configuration's PUT replaces the whole key-value, so tag writes are a **GET-merge-PUT** with an ETag precondition (`azappconfig/v2`): `tag`/`untag` preserve the value and other tags, and a value write (`update`) preserves existing tags.

² App Configuration is unversioned, so staging uses **last-write-wins** (no modified-after conflict check); `tag`/`untag` are available.

### Metadata terminology

suve normalizes every provider's key=value metadata to a single term — **tags** (`suve … tag` / `untag`). Each provider's native word is a documented mapping; suve does **not** add per-provider command or flag aliases.

| Cloud | metadata term (native) | suve term | identity axis (native) |
|---|---|---|---|
| AWS SSM / Secrets Manager | tags | **tags** | version |
| Azure Key Vault | tags | **tags** | version |
| Azure App Configuration | tags | **tags** | **label** (`(key, label)` — a *separate* concept, surfaced as suve's [namespace](#namespaces)) |
| Google Cloud Secret Manager | **labels** | **tags** | version |

> [!WARNING]
> Naming trap: Google Cloud "labels" are key=value **metadata** — suve surfaces them as **tags** (`suve gcloud secret tag …`). Azure App Configuration's "label" is an **identity** dimension (part of a setting's address), which suve exposes as its own **namespace** axis (`--namespace`/`--ns`; see [Namespaces](#namespaces)). These are different concepts that happen to share the vendor word "label".

### Provider selection

Every backend has an **explicit command group that is always available**, regardless of environment:

```bash
suve aws param    ...  # AWS Parameter Store
suve aws secret   ...  # AWS Secrets Manager
suve aws stage    ...  # AWS staging
suve gcloud secret ... # Google Cloud Secret Manager
suve gcloud stage  ... # Google Cloud staging
suve azure secret  ... # Azure Key Vault
suve azure param   ... # Azure App Configuration
suve azure stage   ... # Azure staging (secret = Key Vault, param = App Configuration)
```

For convenience, suve also exposes **bare top-level aliases** — `suve param`, `suve secret`, `suve stage` — but only when the environment makes the target unambiguous. `param`, `secret`, and `stage` are each resolved independently. All backends support staging, so `stage` follows the same "exactly one active backend" rule (Azure is staging-active when either `AZURE_KEYVAULT_NAME` or `AZURE_APPCONFIG_NAME` is set):

1. A backend is **active** when its identifying environment variable is set:

   | Backend | Active when set |
   |---------|-----------------|
   | AWS | `AWS_ACCESS_KEY_ID`, `AWS_VAULT`, or `AWS_PROFILE` |
   | Google Cloud | `GOOGLE_CLOUD_PROJECT` |
   | Azure Key Vault (secret) | `AZURE_KEYVAULT_NAME` |
   | Azure App Configuration (param) | `AZURE_APPCONFIG_NAME` |

2. The bare alias for a service appears **only when exactly one backend is active** for it. Zero or two-plus active → no alias, use the explicit group. **There is no priority order** — ambiguity is never resolved silently.
3. **AWS fallback:** if no backend is active via env at all, AWS is accepted via `~/.aws/credentials` (or `$AWS_SHARED_CREDENTIALS_FILE`). If that is also absent, there are no bare aliases.

Examples (`—` = alias not exposed):

| Environment | `param` → | `secret` → | `stage` → |
|-------------|-----------|------------|-----------|
| nothing set, `~/.aws/credentials` present | `aws` | `aws` | `aws` |
| `AWS_PROFILE` | `aws` | `aws` | `aws` |
| `GOOGLE_CLOUD_PROJECT` | — | `gcloud` | `gcloud` |
| `AZURE_KEYVAULT_NAME` | — | `azure` | `azure` |
| `AZURE_APPCONFIG_NAME` | `azure` | — | `azure` |
| `AWS_PROFILE` + `GOOGLE_CLOUD_PROJECT` | `aws` | — (ambiguous) | — (ambiguous) |
| nothing set, no credentials file | — | — | — |

`suve --help` lists which aliases are active in the current environment.

## Command Reference

### Services

Explicit command groups (always available) and their bare aliases (exposed per the [provider selection](#provider-selection) rules above):

| Backend | Explicit command | Bare alias (conditional) |
|---------|------------------|--------------------------|
| [AWS SSM Parameter Store](docs/aws.md) | `aws param` (`ssm`, `ps`) | `param` |
| [AWS Secrets Manager](docs/aws.md) | `aws secret` (`sm`, `secretsmanager`) | `secret` |
| AWS Staging | `aws stage` (`stg`) | `stage` |
| [Google Cloud Secret Manager](docs/gcloud.md) | `gcloud secret` (`secrets`, `sm`) | `secret` |
| Google Cloud Staging | `gcloud stage` (`stg`) | `stage` |
| [Azure Key Vault](docs/azure.md) | `azure secret` (`kv`, `keyvault`) | `secret` |
| [Azure App Configuration](docs/azure.md) | `azure param` (`appconfig`, `ac`, `appcfg`) | `param` |
| Azure Staging | `azure stage` (`stg`) | `stage` |

The command **groups** themselves also take aliases: `gcloud` → `gcp` / `google`, and `azure` → `az` (e.g. `suve gcp secrets show`, `suve az kv show`). <!-- naming-allow-gcp --> Under `azure stage`, the `secret` / `param` subgroups accept the same aliases as their read/write counterparts (`kv` / `keyvault`, `appconfig` / `ac` / `appcfg`).

### AWS SSM Parameter Store

| Command | Options | Description |
|---------|---------|-------------|
| [`suve aws param show`](docs/aws.md#suve-aws-param-show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display parameter with metadata |
| [`suve aws param log`](docs/aws.md#suve-aws-param-log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve aws param diff`](docs/aws.md#suve-aws-param-diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve aws param list`](docs/aws.md#suve-aws-param-list) | `--recursive` (`-R`)<br>`--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List parameters |
| [`suve aws param create`](docs/aws.md#suve-aws-param-create) | `--type=<TYPE>`<br>`--secure`<br>`--description=<TEXT>`<br>`--tier=<TIER>`<br>`--data-type=<TYPE>`<br>`--allowed-pattern=<REGEX>`<br>`--policies=<JSON>` | Create a new parameter |
| [`suve aws param update`](docs/aws.md#suve-aws-param-update) | `--type=<TYPE>`<br>`--secure`<br>`--description=<TEXT>`<br>`--tier=<TIER>`<br>`--data-type=<TYPE>`<br>`--allowed-pattern=<REGEX>`<br>`--policies=<JSON>`<br>`--yes` | Update an existing parameter |
| [`suve aws param delete`](docs/aws.md#suve-aws-param-delete) | `--yes` | Delete parameter |
| [`suve aws param tag`](docs/aws.md#suve-aws-param-tag) | `<KEY>=<VALUE>...` | Add or update tags |
| [`suve aws param untag`](docs/aws.md#suve-aws-param-untag) | `<KEY>...` | Remove tags |

**Staging commands** (under `suve stage param`):

| Command | Options | Description |
|---------|---------|-------------|
| `suve stage param add` | `--description=<TEXT>` | Stage new parameter |
| `suve stage param edit` | `--description=<TEXT>` | Stage modification |
| `suve stage param delete` | | Stage deletion |
| `suve stage param status` | `--verbose` (`-v`) | Show staged changes |
| `suve stage param diff` | `--parse-json` (`-j`)<br>`--no-pager` | Compare staged vs AWS |
| `suve stage param apply` | `--yes`<br>`--ignore-conflicts` | Apply staged changes |
| `suve stage param reset` | `--all` | Unstage changes |
| `suve stage param tag` | `<KEY>=<VALUE>...` | Stage tag additions |
| `suve stage param untag` | `<KEY>...` | Stage tag removals |

### AWS Secrets Manager

| Command | Options | Description |
|---------|---------|-------------|
| [`suve aws secret show`](docs/aws.md#suve-aws-secret-show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display secret with metadata |
| [`suve aws secret log`](docs/aws.md#suve-aws-secret-log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve aws secret diff`](docs/aws.md#suve-aws-secret-diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve aws secret list`](docs/aws.md#suve-aws-secret-list) | `--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List secrets |
| [`suve aws secret create`](docs/aws.md#suve-aws-secret-create) | `--description=<TEXT>` | Create new secret |
| [`suve aws secret update`](docs/aws.md#suve-aws-secret-update) | `--description=<TEXT>`<br>`--yes` | Update existing secret |
| [`suve aws secret delete`](docs/aws.md#suve-aws-secret-delete) | `--force`<br>`--recovery-window=<DAYS>`<br>`--yes` | Delete secret |
| [`suve aws secret restore`](docs/aws.md#suve-aws-secret-restore) | | Restore deleted secret |
| [`suve aws secret tag`](docs/aws.md#suve-aws-secret-tag) | `<KEY>=<VALUE>...` | Add or update tags |
| [`suve aws secret untag`](docs/aws.md#suve-aws-secret-untag) | `<KEY>...` | Remove tags |

**Staging commands** (under `suve stage secret`):

| Command | Options | Description |
|---------|---------|-------------|
| `suve stage secret add` | `--description=<TEXT>` | Stage new secret |
| `suve stage secret edit` | `--description=<TEXT>` | Stage modification |
| `suve stage secret delete` | `--force`<br>`--recovery-window=<DAYS>` | Stage deletion |
| `suve stage secret status` | `--verbose` (`-v`) | Show staged changes |
| `suve stage secret diff` | `--parse-json` (`-j`)<br>`--no-pager` | Compare staged vs AWS |
| `suve stage secret apply` | `--yes`<br>`--ignore-conflicts` | Apply staged changes |
| `suve stage secret reset` | `--all` | Unstage changes |
| `suve stage secret tag` | `<KEY>=<VALUE>...` | Stage tag additions |
| `suve stage secret untag` | `<KEY>...` | Stage tag removals |

### Google Cloud Secret Manager

Uses integer version numbers (with the `latest` alias) and has no staging labels. Select the project with `--project` or the `GOOGLE_CLOUD_PROJECT` environment variable. Authentication uses [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/application-default-credentials). See [docs/gcloud.md](docs/gcloud.md) for details.

> [!NOTE]
> Google Cloud Secret Manager calls key=value metadata **"labels"**; suve surfaces them under its cross-provider term **tags** (`tag` / `untag`). See [Metadata terminology](#metadata-terminology).

| Command | Options | Description |
|---------|---------|-------------|
| [`suve gcloud secret show`](docs/gcloud.md#suve-gcloud-secret-show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display secret with metadata |
| [`suve gcloud secret log`](docs/gcloud.md#suve-gcloud-secret-log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve gcloud secret diff`](docs/gcloud.md#suve-gcloud-secret-diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve gcloud secret list`](docs/gcloud.md#suve-gcloud-secret-list) | `--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List secrets |
| [`suve gcloud secret create`](docs/gcloud.md#suve-gcloud-secret-create) | | Create new secret |
| [`suve gcloud secret update`](docs/gcloud.md#suve-gcloud-secret-update) | `--yes` | Update existing secret |
| [`suve gcloud secret delete`](docs/gcloud.md#suve-gcloud-secret-delete) | `--yes` | Delete secret |
| [`suve gcloud secret tag`](docs/gcloud.md#suve-gcloud-secret-tag) | `<KEY>=<VALUE>...` | Add or update tags (Google Cloud "labels") |
| [`suve gcloud secret untag`](docs/gcloud.md#suve-gcloud-secret-untag) | `<KEY>...` | Remove tags (Google Cloud "labels") |

**Staging commands** (under `suve gcloud stage`; Google Cloud is secret-only, so staging operates on secrets directly):

| Command | Options | Description |
|---------|---------|-------------|
| `suve gcloud stage add` | `--description=<TEXT>` | Stage a new secret |
| `suve gcloud stage edit` | `--description=<TEXT>` | Stage a modification (applies as a new version) |
| `suve gcloud stage delete` | | Stage a deletion |
| `suve gcloud stage status` | `--verbose` (`-v`) | Show staged changes |
| `suve gcloud stage diff` | `--parse-json` (`-j`)<br>`--no-pager` | Show staged vs Google Cloud |
| `suve gcloud stage apply` | `--yes`<br>`--ignore-conflicts` | Apply staged changes |
| `suve gcloud stage reset` | `--all` | Unstage changes |
| `suve gcloud stage tag` | `<KEY>=<VALUE>...` | Stage tag additions (Google Cloud "labels") |
| `suve gcloud stage untag` | `<KEY>...` | Stage tag removals (Google Cloud "labels") |
| `suve gcloud stage export` | `<file>` | Export staged changes to a snapshot file |
| `suve gcloud stage import` | `<file>` | Import staged changes from a snapshot file |

### Azure Key Vault

Secrets are versioned by opaque IDs and have no staging labels. Select the vault with `--vault-name` or the `AZURE_KEYVAULT_NAME` environment variable — the vault name is a globally-unique endpoint, so no subscription or resource group is needed. Authentication uses the [DefaultAzureCredential](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication) chain (environment, managed identity, Azure CLI, ...). See [docs/azure.md](docs/azure.md) for details.

| Command | Options | Description |
|---------|---------|-------------|
| [`suve azure secret show`](docs/azure.md#suve-azure-secret-show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display secret with metadata |
| [`suve azure secret log`](docs/azure.md#suve-azure-secret-log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve azure secret diff`](docs/azure.md#suve-azure-secret-diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve azure secret list`](docs/azure.md#suve-azure-secret-list) | `--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List secrets |
| [`suve azure secret create`](docs/azure.md#suve-azure-secret-create) | | Create new secret |
| [`suve azure secret update`](docs/azure.md#suve-azure-secret-update) | `--yes` | Update existing secret |
| [`suve azure secret delete`](docs/azure.md#suve-azure-secret-delete) | `--yes` | Delete secret (soft-delete) |
| [`suve azure secret restore`](docs/azure.md#suve-azure-secret-restore) | | Recover a soft-deleted secret |
| [`suve azure secret tag`](docs/azure.md#suve-azure-secret-tag) | `<KEY>=<VALUE>...` | Add or update tags |
| [`suve azure secret untag`](docs/azure.md#suve-azure-secret-untag) | `<KEY>...` | Remove tags |

**Staging commands** (under `suve azure stage secret`):

| Command | Options | Description |
|---------|---------|-------------|
| `suve azure stage secret add` | `--description=<TEXT>` | Stage a new secret |
| `suve azure stage secret edit` | `--description=<TEXT>` | Stage a modification (new version) |
| `suve azure stage secret delete` | | Stage a deletion |
| `suve azure stage secret status` | `--verbose` (`-v`) | Show staged changes |
| `suve azure stage secret diff` | `--parse-json` (`-j`)<br>`--no-pager` | Show staged vs Key Vault |
| `suve azure stage secret apply` | `--yes`<br>`--ignore-conflicts` | Apply staged changes |
| `suve azure stage secret reset` | `--all` | Unstage changes |
| `suve azure stage secret tag` | `<KEY>=<VALUE>...` | Stage tag additions |
| `suve azure stage secret untag` | `<KEY>...` | Stage tag removals |
| `suve azure stage secret export` | `<file>` | Export staged changes to a snapshot file |
| `suve azure stage secret import` | `<file>` | Import staged changes from a snapshot file |

### Azure App Configuration

Unversioned key-value store. `#`, `~`, and `:` are valid key characters — the whole argument is the literal key name, not a version spec — and `log` reports that history is unsupported. `tag` / `untag` are supported via a GET-merge-PUT (see footnote ¹). Select the store with `--store-name` or the `AZURE_APPCONFIG_NAME` environment variable — the store name is a globally-unique endpoint, so no subscription or resource group is needed. See [docs/azure.md](docs/azure.md) for details.

| Command | Options | Description |
|---------|---------|-------------|
| [`suve azure param show`](docs/azure.md#suve-azure-param-show) | `--namespace`/`--ns`<br>`--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display value with metadata |
| [`suve azure param list`](docs/azure.md#suve-azure-param-list) | `--namespace`/`--ns`<br>`--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List keys |
| [`suve azure param create`](docs/azure.md#suve-azure-param-create) | `--namespace`/`--ns` | Create a new key |
| [`suve azure param update`](docs/azure.md#suve-azure-param-update) | `--namespace`/`--ns`<br>`--yes` | Update an existing key |
| [`suve azure param delete`](docs/azure.md#suve-azure-param-delete) | `--namespace`/`--ns`<br>`--yes` | Delete a key |
| [`suve azure param tag`](docs/azure.md#suve-azure-param-tag) | `--namespace`/`--ns` | Add or update tags (GET-merge-PUT) |
| [`suve azure param untag`](docs/azure.md#suve-azure-param-untag) | `--namespace`/`--ns` | Remove tags |

#### Namespaces

App Configuration addresses a setting by `(key, label)`. This **label axis is an identity dimension** — a flat partition of the key space, like a Kubernetes namespace — not key=value metadata. Because "label" almost everywhere else means metadata (which suve unifies as [tags](#metadata-terminology)), **suve calls this axis a namespace**; Azure App Configuration calls it a "label".

Select the namespace with `--namespace` (alias `--ns`) or the `AZURE_APPCONFIG_NAMESPACE` environment variable (precedence: flag > env > default). The default is the **null namespace** (App Configuration's unlabeled/default settings); a `dev` namespace never surfaces `prod` settings in `list`/`show`.

The value is interpreted by context:

| Value | Meaning | Where |
|---|---|---|
| unset or `""` | the **null namespace** (default; `""` also overrides an env default back to null) | all commands |
| `"*"` | **all** namespaces (wildcard) | list/read only |
| `"dev,prod"` | **dev OR prod** (`,` = OR-list) | list/read only |
| `"dev*"` | prefix wildcard | list/read only |
| `"dev"` | the literal namespace `dev` | all commands |
| `"\*"`, `"foo\,bar"` | a **literal** namespace containing a reserved char (`\` escapes `*`, `,`, `\`) | all commands |

- **List/read** (`list`, `show`) forward the value to App Configuration's label filter, so `*` (all), `dev*` (prefix), and `dev,prod` (OR-list) work natively; an empty value maps to the null-namespace filter.
- **Single-item ops** (`show`, `create`, `update`, `delete`, staging) need exactly one namespace: `\` escapes are decoded, and any **unescaped** `*` or `,` is a usage error (it names all/multiple namespaces). This is also how you address a namespace literally named `*` / `,` / `\` — e.g. `--namespace "\*"`.
- The namespace is a **separate flag/env channel**; the positional argument stays the whole key, so colon keys like `Logging:LogLevel:Default` are unaffected. The filter grammar (`*` `,` `\`) lives only inside the `--namespace` value.

**Staging commands** (under `suve azure stage param`; unversioned → last-write-wins, no `tag`/`untag`):

| Command | Options | Description |
|---------|---------|-------------|
| `suve azure stage param add` | `--description=<TEXT>` | Stage a new setting |
| `suve azure stage param edit` | `--description=<TEXT>` | Stage a modification |
| `suve azure stage param delete` | | Stage a deletion |
| `suve azure stage param status` | `--verbose` (`-v`) | Show staged changes |
| `suve azure stage param diff` | `--parse-json` (`-j`)<br>`--no-pager` | Show staged vs App Configuration |
| `suve azure stage param apply` | `--yes` | Apply staged changes |
| `suve azure stage param reset` | `--all` | Unstage changes |
| `suve azure stage param export` | `<file>` | Export staged changes to a snapshot file |
| `suve azure stage param import` | `<file>` | Import staged changes from a snapshot file |

> [!NOTE]
> Azure staging is **per-service** under the hood: `suve azure stage secret` (Key Vault) and `suve azure stage param` (App Configuration) keep separate staging state. Provider-wide `azure stage status`/`diff`/`apply`/`reset` span both services, skipping whichever one is not configured.

### Global Stage Commands (AWS)

| Command | Options | Description |
|---------|---------|-------------|
| `suve stage status` | `--verbose` (`-v`) | Show all staged changes |
| `suve stage diff` | `--parse-json` (`-j`)<br>`--no-pager` | Compare all staged vs AWS |
| `suve stage apply` | `--yes`<br>`--ignore-conflicts` | Apply all staged changes |
| `suve stage reset` | `--all` | Unstage all changes |

### Export / Import Commands

Export writes the working staging area out to portable snapshot files (one per service) and import reads them back. Each file is a plaintext JSON envelope (`{version, provider, scope, service, payload}`) whose `payload` is passphrase-encrypted (Argon2id) or plaintext when the passphrase is empty. The full scope is embedded and validated on import.

| Command | Argument | Options | Description |
|---------|----------|---------|-------------|
| `suve stage export` | `<dir>` | `--keep`<br>`--yes` (`--force`)<br>`--passphrase-stdin` | Export every service with staged changes to `<dir>/param.json` + `<dir>/secret.json` |
| `suve stage {param,secret} export` | `<file>` | `--keep`<br>`--yes` (`--force`)<br>`--passphrase-stdin` | Export a single service to `<file>` |
| `suve stage import` | `<dir>` | `--merge`<br>`--overwrite`<br>`--yes`<br>`--passphrase-stdin`<br>`--force` | Import `param.json` / `secret.json` from `<dir>` (missing files skipped; nothing imported if both absent) |
| `suve stage {param,secret} import` | `<file>` | `--merge`<br>`--overwrite`<br>`--yes`<br>`--passphrase-stdin`<br>`--force` | Import a single service from `<file>` (missing file or service mismatch is a hard error) |

- **`export`** writes the working area out wholesale; there is no `--merge` / `--overwrite`. By default it clears the working staging area; `--keep` retains it. `--yes` / `--force` skip the overwrite confirmation.
- **`import`** has no `--keep` (it is read-only on the file). `--merge` / `--overwrite` are mutually exclusive and only matter when the working area already holds changes; otherwise the file is applied directly. `--force` imports even when the file's embedded scope differs from the current scope.

## Environment Variables

### Timezone

suve respects the `TZ` environment variable for date/time formatting:

```bash
# Show times in UTC
TZ=UTC suve param show /app/config

# Show times in Japan Standard Time
TZ=Asia/Tokyo suve param show /app/config
```

All timestamps are formatted in RFC3339 format with the local timezone offset applied. If `TZ` is not set, the system's local timezone is used. Invalid timezone values fall back to UTC.

### General

| Variable | Description |
|----------|-------------|
| `TZ` | Timezone for date/time formatting (see above) |
| `SUVE_NO_UPDATE_CHECK` | Opt out of the update-check notification |
| `SUVE_DEBUG` | Enable verbose debug logging (same as the global `--debug` flag); any non-empty value except `0`/`false` enables it |
| `SUVE_NO_REDACTION` | With debug enabled, log full request/response bodies and unmasked headers — **including secret values and credentials** (same as the global `--no-redaction` flag); parsed like `SUVE_DEBUG` |

### Debugging

Pass the global `--debug` flag (or set `SUVE_DEBUG=1`) to trace what suve is
doing on stderr. This is designed for the "command produces empty or unexpected
output" case: it shows the decisions suve made *before* calling any API, the
effective cloud configuration, each SDK request/response, and how many results
each step produced:

```bash
suve secret ls --debug          # flag works in any position
SUVE_DEBUG=1 suve secret ls      # or via environment
```

Each entry starts with a `[suve debug <time>]` prefix (multi-line HTTP dumps
are prefixed on their first line). The output includes:

- **CLI decisions** — the suve version and which provider each flat alias
  (`param` / `secret` / `stage`) resolved to.
- **Effective configuration** — for AWS, the resolved region, profile, and
  credentials source (the usual suspects when a listing is unexpectedly empty);
  for Google Cloud, the queried `projects/...` parent; for Azure, the
  credential the `DefaultAzureCredential` chain selected.
- **SDK requests/responses** — AWS HTTP request/response dumps (bodyless) with
  retries, gRPC calls with resource paths and durations for Google Cloud, and
  azcore request/response/retry/authentication events for Azure.
- **Result counts** — items returned per API page, and how many names survived
  the client-side prefix/regex filters, so "the API returned nothing" and "my
  filter dropped everything" are distinguishable.

Only request/response **metadata** is printed — secret values are never logged:
AWS uses the bodyless log modes, and because the dump is taken after request
signing, suve shows only an **allowlist** of non-sensitive headers (`Host`,
`X-Amz-Target`, request IDs, …) and redacts every other header value — so the
signing `Authorization` header, the session token, and any future
credential-bearing header fail closed rather than leaking. The gRPC interceptor
never prints request/reply messages; azcore redacts headers outside its own
allowlist and never logs bodies (error events may include the service's error
document, the same text normal error output already shows). Debug output goes to
stderr, so it never contaminates piped stdout.

#### Unredacted output (`--no-redaction`)

When the redacted default hides too much — e.g. you need to see the exact secret
value a request returned, or the raw payload of a failing call — add
`--no-redaction` (or set `SUVE_NO_REDACTION=1`) alongside `--debug`:

```bash
suve secret show my-secret --debug --no-redaction
```

This switches AWS to the with-body log modes and stops masking headers, and turns
on azcore body logging for Azure, so **full request/response bodies and
credentials — including secret values and the signing `Authorization` header —
are written to stderr**. Use it only for deliberate, local debugging and never
where the log could be captured or shared; a one-line warning is printed to
stderr whenever it is active. It has no effect without `--debug` (you'll get a
hint saying so), and no effect on Google Cloud, whose gRPC interceptor logs no
message bodies at all.

### Staging

| Variable | Description |
|----------|-------------|
| `SUVE_STAGING_KEY` | Base64-encoded 32-byte key that overrides the OS keychain for encrypting the working staging state |

### AWS

| Variable | Description |
|----------|-------------|
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` | Static credentials |
| `AWS_PROFILE` | Shared-config profile to load |
| `AWS_REGION` / `AWS_DEFAULT_REGION` | Region |

### Google Cloud

| Variable | Description |
|----------|-------------|
| `GOOGLE_CLOUD_PROJECT` | Project for Secret Manager (or use `--project`) |

### Azure

| Variable | Description |
|----------|-------------|
| `AZURE_KEYVAULT_NAME` | Key Vault name for `azure secret` (or use `--vault-name`) |
| `AZURE_APPCONFIG_NAME` | App Configuration store for `azure param` (or use `--store-name`) |
| `AZURE_APPCONFIG_NAMESPACE` | Default [namespace](#namespaces) for `azure param` — Azure calls this axis a "label" (or use `--namespace`/`--ns`) |

Authentication uses the DefaultAzureCredential chain (`az login`, environment, managed identity, ...). The Key Vault / App Configuration name is a globally-unique endpoint, so no subscription or resource group is needed.

## AWS Configuration

suve uses standard AWS SDK configuration:

**Authentication** (in order of precedence):
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared credentials file (`~/.aws/credentials`) and config (`~/.aws/config`)
   - Use `AWS_PROFILE` to specify which profile to load (default: `default`)
3. IAM role (EC2, ECS, Lambda)

**Region**:
- `AWS_REGION` or `AWS_DEFAULT_REGION` environment variable
- `~/.aws/config` file

> [!WARNING]
> Ensure your IAM role/user has appropriate permissions:
> - SSM: `ssm:GetParameter`, `ssm:GetParameters`, `ssm:GetParameterHistory`, `ssm:PutParameter`, `ssm:DeleteParameter`, `ssm:DescribeParameters`, `ssm:AddTagsToResource`, `ssm:RemoveTagsFromResource`
> - SM: `secretsmanager:GetSecretValue`, `secretsmanager:ListSecretVersionIds`, `secretsmanager:ListSecrets`, `secretsmanager:CreateSecret`, `secretsmanager:PutSecretValue`, `secretsmanager:UpdateSecret`, `secretsmanager:DeleteSecret`, `secretsmanager:RestoreSecret`, `secretsmanager:TagResource`, `secretsmanager:UntagResource`

> [!NOTE]
> The `gcloud` and `azure` commands use their own credential chains: Google Cloud uses [Application Default Credentials (ADC)](https://cloud.google.com/docs/authentication/application-default-credentials), and Azure uses [DefaultAzureCredential](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication) (environment, managed identity, Azure CLI, ...). See [docs/gcloud.md](docs/gcloud.md) and [docs/azure.md](docs/azure.md) for details.

## Development

```bash
# Run tests
mise test

# Run linter
mise lint

# Build CLI (without GUI)
mise build-cli

# Build with GUI support — builds the frontend first, then embeds it into
# bin/suve. (A bare `go build -tags production` skips the frontend build, so the
# binary aborts at `suve --gui` with "no index.html could be found".)
mise build-gui

# Run E2E tests (requires Docker)
mise e2e

# Coverage (unit + E2E combined)
mise coverage-all
```

### Local Development with Emulators

`mise run bash` starts the selected cloud's emulator(s) and opens a shell with
the right environment injected, so `suve` (and `suve --gui`) talk to the local
emulators and auto-detect the active provider. Flags combine freely (0–4):

```bash
mise run bash --aws               # AWS (localstack: SSM + Secrets Manager)
mise run bash --gcloud            # Google Cloud Secret Manager
mise run bash --azure-appconfig   # Azure App Configuration
mise run bash --azure-keyvault    # Azure Key Vault
mise run bash --azure             # both Azure services
mise run bash --aws --gcloud --azure   # everything at once

# inside the shell:
suve --gui        # auto-detects the active provider
suve param ls
suve secret list
```

Containers keep running after you exit the shell; stop them with
`docker compose down` (or `mise run clean`).

Every cloud is behind a docker compose profile (`aws` / `gcloud` / `azure`), so
nothing starts by default. To drive the AWS emulator manually instead of the
shell above:

```bash
SUVE_LOCALSTACK_EXTERNAL_PORT=4566 docker compose --profile aws up -d
AWS_ENDPOINT_URL=http://127.0.0.1:4566 \
AWS_ACCESS_KEY_ID=dummy \
AWS_SECRET_ACCESS_KEY=dummy \
AWS_DEFAULT_REGION=us-east-1 \
suve param ls
docker compose down
```

> [!IMPORTANT]
> Dummy credentials (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) are required to prevent the SDK from attempting IAM role credential fetching. The `mise run bash --aws` shell sets these for you.

## License

MIT License
