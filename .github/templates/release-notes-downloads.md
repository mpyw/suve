## Downloads

### macOS (GUI with CLI)

| Architecture | Download |
|--------------|----------|
| Apple Silicon (M1/M2/M3/M4) | [suve_${VERSION}_darwin_arm64.tar.gz](${BASE_URL}/suve_${VERSION}_darwin_arm64.tar.gz) |
| Intel | [suve_${VERSION}_darwin_amd64.tar.gz](${BASE_URL}/suve_${VERSION}_darwin_amd64.tar.gz) |

> [!TIP]
> If macOS shows "cannot be opened because the developer cannot be verified", run:
> ```bash
> xattr -d com.apple.quarantine /path/to/suve
> ```

### Windows (GUI with CLI)

| Architecture | Download |
|--------------|----------|
| x86_64 | [suve_${VERSION}_windows_amd64.zip](${BASE_URL}/suve_${VERSION}_windows_amd64.zip) |
| ARM64 | [suve_${VERSION}_windows_arm64.zip](${BASE_URL}/suve_${VERSION}_windows_arm64.zip) |

> [!TIP]
> On first run, if Windows SmartScreen shows "Windows protected your PC", click **More info** → **Run anyway**.

### Linux GUI (webkit2gtk-4.0: Ubuntu 22.04, Debian 11/12, etc.)

| Architecture | Tarball | Debian/Ubuntu | RHEL/Fedora |
|--------------|---------|---------------|-------------|
| x86_64 | [suve_${VERSION}_linux_amd64.tar.gz](${BASE_URL}/suve_${VERSION}_linux_amd64.tar.gz) | [suve_${VERSION}-1_amd64.deb](${BASE_URL}/suve_${VERSION}-1_amd64.deb) | [suve-${VERSION}-1.x86_64.rpm](${BASE_URL}/suve-${VERSION}-1.x86_64.rpm) |
| ARM64 | [suve_${VERSION}_linux_arm64.tar.gz](${BASE_URL}/suve_${VERSION}_linux_arm64.tar.gz) | [suve_${VERSION}-1_arm64.deb](${BASE_URL}/suve_${VERSION}-1_arm64.deb) | [suve-${VERSION}-1.aarch64.rpm](${BASE_URL}/suve-${VERSION}-1.aarch64.rpm) |

### Linux GUI (webkit2gtk-4.1: Ubuntu 24.04+, Fedora 40+, etc.)

| Architecture | Tarball | Debian/Ubuntu | RHEL/Fedora |
|--------------|---------|---------------|-------------|
| x86_64 | [suve_${VERSION}_linux_amd64_webkit2_41.tar.gz](${BASE_URL}/suve_${VERSION}_linux_amd64_webkit2_41.tar.gz) | [suve_webkit2_41_${VERSION}-1_amd64.deb](${BASE_URL}/suve_webkit2_41_${VERSION}-1_amd64.deb) | [suve_webkit2_41-${VERSION}-1.x86_64.rpm](${BASE_URL}/suve_webkit2_41-${VERSION}-1.x86_64.rpm) |
| ARM64 | [suve_${VERSION}_linux_arm64_webkit2_41.tar.gz](${BASE_URL}/suve_${VERSION}_linux_arm64_webkit2_41.tar.gz) | [suve_webkit2_41_${VERSION}-1_arm64.deb](${BASE_URL}/suve_webkit2_41_${VERSION}-1_arm64.deb) | [suve_webkit2_41-${VERSION}-1.aarch64.rpm](${BASE_URL}/suve_webkit2_41-${VERSION}-1.aarch64.rpm) |

### Linux CLI-only (no dependencies)

| Architecture | Tarball | Debian/Ubuntu | RHEL/Fedora |
|--------------|---------|---------------|-------------|
| x86_64 | [suve-cli_${VERSION}_linux_amd64.tar.gz](${BASE_URL}/suve-cli_${VERSION}_linux_amd64.tar.gz) | [suve-cli_${VERSION}-1_amd64.deb](${BASE_URL}/suve-cli_${VERSION}-1_amd64.deb) | [suve-cli-${VERSION}-1.x86_64.rpm](${BASE_URL}/suve-cli-${VERSION}-1.x86_64.rpm) |
| ARM64 | [suve-cli_${VERSION}_linux_arm64.tar.gz](${BASE_URL}/suve-cli_${VERSION}_linux_arm64.tar.gz) | [suve-cli_${VERSION}-1_arm64.deb](${BASE_URL}/suve-cli_${VERSION}-1_arm64.deb) | [suve-cli-${VERSION}-1.aarch64.rpm](${BASE_URL}/suve-cli-${VERSION}-1.aarch64.rpm) |

### Checksums

[checksums.txt](${BASE_URL}/checksums.txt)
