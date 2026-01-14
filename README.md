<div align="center">
  <img src="gui/build/appicon.png" alt="suve" width="128" height="128">
  <h1>suve</h1>
  <p><strong>S</strong>ecret <strong>U</strong>nified <strong>V</strong>ersioning <strong>E</strong>xplorer</p>

  [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/suve.svg)](https://pkg.go.dev/github.com/mpyw/suve)
  [![Test](https://github.com/mpyw/suve/actions/workflows/test.yml/badge.svg)](https://github.com/mpyw/suve/actions/workflows/test.yml)
  [![Codecov](https://codecov.io/gh/mpyw/suve/graph/badge.svg)](https://codecov.io/gh/mpyw/suve)
  [![Go Report Card](https://goreportcard.com/badge/github.com/mpyw/suve)](https://goreportcard.com/report/github.com/mpyw/suve)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
</div>

> [!NOTE]
> This project was written by AI (Claude Code).

A **Git-like CLI/GUI** for AWS Parameter Store and Secrets Manager. Familiar commands like `show`, `log`, `diff`, and a **staging workflow** for safe, reviewable changes.

<p align="center">
  <img src="demo/cli-demo.gif" alt="CLI Demo" width="800">
</p>

<p align="center">
  <img src="demo/gui-demo.gif" alt="GUI Demo" width="800">
</p>

## Features

- **Git-like commands**: `show`, `log`, `diff`, `ls`, `create`, `update`, `rm`
- **Staging workflow**: `edit` → `status` → `diff` → `apply` (review changes before applying)
- **Version navigation**: `#VERSION`, `~SHIFT`, `:LABEL` syntax
- **Colored diff output**: Easy-to-read unified diff format
- **Both services**: SSM Parameter Store and Secrets Manager
- **GUI mode**: Desktop application via `--gui` flag (built with [Wails](https://wails.io/))

## Installation

### Using [Homebrew](https://brew.sh/) (macOS/Linux)

```bash
# Full version (CLI + GUI)
brew install mpyw/tap/suve

# CLI-only version (no GUI dependencies, recommended for Linux)
brew install mpyw/tap/suve-cli
```

> [!NOTE]
> On Linux, `suve` requires GTK3 and WebKit2GTK for GUI support. Use `suve-cli` if you only need CLI functionality.

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
<summary>Using <code>go tool</code> (CLI only, Go 1.24+)</summary>

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

```bash
git clone https://github.com/mpyw/suve.git
cd suve
```

**CLI only:**

```bash
make build
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

> [!TIP]
> **Using with [aws-vault](https://github.com/99designs/aws-vault)**: Wrap commands with `aws-vault exec` for temporary credentials:
> ```bash
> aws-vault exec my-profile -- suve param show /my/param
> ```

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
> For detailed documentation, see [Staging State Transitions](docs/staging-state-transitions.md) and [Staging Agent/Daemon](docs/staging-agent.md).

> [!TIP]
> Staged values are stored in secure memory (daemon process). Use `suve stage stash` to save changes to an encrypted file for later restoration.

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

**Save changes for later** (stash):

```bash
# Save staged changes to file (prompts for passphrase)
suve stage stash

# Restore from file
suve stage stash pop

# Preview stashed changes
suve stage stash show

# Delete stash without restoring
suve stage stash drop
```

> [!NOTE]
> See [Staging Agent/Daemon](docs/staging-agent.md) for detailed stash command documentation.

## Version Specification

Navigate versions with Git-like syntax.

### SSM Parameter Store

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

### Secrets Manager

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

## Command Reference

### Services

| Service | Aliases |
|---------|---------|
| [SSM Parameter Store](docs/param.md) | `param`, `ssm`, `ps` |
| [Secrets Manager](docs/secret.md) | `secret`, `sm` |

### SSM Parameter Store

| Command | Options | Description |
|---------|---------|-------------|
| [`suve param show`](docs/param.md#show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display parameter with metadata |
| [`suve param log`](docs/param.md#log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve param diff`](docs/param.md#diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve param list`](docs/param.md#list) | `--recursive` (`-R`)<br>`--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List parameters |
| [`suve param create`](docs/param.md#create) | `--type=<TYPE>`<br>`--secure`<br>`--description=<TEXT>` | Create a new parameter |
| [`suve param update`](docs/param.md#update) | `--type=<TYPE>`<br>`--secure`<br>`--description=<TEXT>`<br>`--yes` | Update an existing parameter |
| [`suve param delete`](docs/param.md#delete) | `--yes` | Delete parameter |
| [`suve param tag`](docs/param.md#tag) | `<KEY>=<VALUE>...` | Add or update tags |
| [`suve param untag`](docs/param.md#untag) | `<KEY>...` | Remove tags |

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

### Secrets Manager

| Command | Options | Description |
|---------|---------|-------------|
| [`suve secret show`](docs/secret.md#show) | `--raw`<br>`--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Display secret with metadata |
| [`suve secret log`](docs/secret.md#log) | `--number=<N>` (`-n`)<br>`--patch` (`-p`)<br>`--parse-json` (`-j`)<br>`--oneline`<br>`--reverse`<br>`--since=<DATE>`<br>`--until=<DATE>`<br>`--no-pager`<br>`--output=<FORMAT>` | Show version history |
| [`suve secret diff`](docs/secret.md#diff) | `--parse-json` (`-j`)<br>`--no-pager`<br>`--output=<FORMAT>` | Compare versions |
| [`suve secret list`](docs/secret.md#list) | `--filter=<REGEX>`<br>`--show`<br>`--output=<FORMAT>` | List secrets |
| [`suve secret create`](docs/secret.md#create) | `--description=<TEXT>` | Create new secret |
| [`suve secret update`](docs/secret.md#update) | `--description=<TEXT>`<br>`--yes` | Update existing secret |
| [`suve secret delete`](docs/secret.md#delete) | `--force`<br>`--recovery-window=<DAYS>`<br>`--yes` | Delete secret |
| [`suve secret restore`](docs/secret.md#restore) | | Restore deleted secret |
| [`suve secret tag`](docs/secret.md#tag) | `<KEY>=<VALUE>...` | Add or update tags |
| [`suve secret untag`](docs/secret.md#untag) | `<KEY>...` | Remove tags |

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

### Global Stage Commands

| Command | Options | Description |
|---------|---------|-------------|
| `suve stage status` | `--verbose` (`-v`) | Show all staged changes |
| `suve stage diff` | `--parse-json` (`-j`)<br>`--no-pager` | Compare all staged vs AWS |
| `suve stage apply` | `--yes`<br>`--ignore-conflicts` | Apply all staged changes |
| `suve stage reset` | `--all` | Unstage all changes |

### Stash Commands

| Command | Options | Description |
|---------|---------|-------------|
| `suve stage stash` | | Save staged changes to file (alias for `push`) |
| `suve stage stash push` | `--keep`<br>`--force`<br>`--merge`<br>`--passphrase-stdin` | Save staged changes from memory to file |
| `suve stage stash pop` | `--keep`<br>`--force`<br>`--merge`<br>`--passphrase-stdin` | Restore staged changes from file |
| `suve stage stash show` | `--verbose` (`-v`)<br>`--passphrase-stdin` | Preview stashed changes |
| `suve stage stash drop` | `--force`<br>`--passphrase-stdin` | Delete stash file |

### Agent Commands

| Command | Description |
|---------|-------------|
| `suve stage agent start` | Start the staging daemon manually |
| `suve stage agent stop` | Stop the staging daemon |

> [!NOTE]
> See [Staging Agent/Daemon](docs/staging-agent.md) for detailed documentation on daemon architecture and stash commands.

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

## Development

```bash
# Run tests
make test

# Run linter
make lint

# Build CLI (without GUI)
make build

# Build with GUI support
go build -tags production -o bin/suve ./cmd/suve

# Run E2E tests (requires Docker)
make e2e

# Coverage (unit + E2E combined)
make coverage-all
```

### Local Development with Localstack

To test against [localstack](https://localstack.cloud/) instead of real AWS:

```bash
# Start localstack
SUVE_LOCALSTACK_EXTERNAL_PORT=4566 docker compose up -d

# Run commands with localstack
AWS_ENDPOINT_URL=http://127.0.0.1:4566 \
AWS_ACCESS_KEY_ID=dummy \
AWS_SECRET_ACCESS_KEY=dummy \
AWS_DEFAULT_REGION=us-east-1 \
suve param ls

# GUI with localstack
AWS_ENDPOINT_URL=http://127.0.0.1:4566 \
AWS_ACCESS_KEY_ID=dummy \
AWS_SECRET_ACCESS_KEY=dummy \
AWS_DEFAULT_REGION=us-east-1 \
suve --gui

# Stop localstack
docker compose down
```

> [!IMPORTANT]
> Dummy credentials (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) are required to prevent the SDK from attempting IAM role credential fetching.

## License

MIT License
