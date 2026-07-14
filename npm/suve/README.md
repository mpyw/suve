# suve

**S**ecret **U**nified **V**ersioning **E**xplorer — a Git-like CLI for multi-cloud secret and parameter management across AWS (Parameter Store + Secrets Manager), Google Cloud (Secret Manager), and Azure (Key Vault + App Configuration).

## Install

```bash
npm install -g suve
# or run without installing
npx suve --version
```

The right prebuilt binary for your platform is installed automatically as an
optional dependency (`@mpyw/suve-<os>-<cpu>`). No build step, no network fetch
at install time beyond the npm registry.

### Supported platforms

| OS | Architectures | Build |
| --- | --- | --- |
| macOS | x64, arm64 | CLI/TUI **+ desktop GUI** (self-contained) |
| Windows | x64, arm64 | CLI/TUI **+ desktop GUI** (self-contained) |
| Linux | x64, arm64 | CLI/TUI-only (static, no GTK/WebKit dependency) |

> **Note:** The npm package for Linux ships the dependency-free CLI/TUI-only
> build, so `--gui` (the desktop GUI) is **not** available there — use a
> [package manager or the `.deb`/`.rpm`](https://github.com/mpyw/suve#installation)
> for the GUI on Linux. The keyboard-driven `--tui` works on every platform.

## Usage

```bash
suve --help
```

See the [full documentation](https://github.com/mpyw/suve#readme) for commands,
revision syntax, and the staging workflow.

## License

[MIT](https://github.com/mpyw/suve/blob/main/LICENSE) © mpyw
