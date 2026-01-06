class SuveCli < Formula
  desc "Git-like CLI for AWS Parameter Store and Secrets Manager (CLI-only)"
  homepage "https://github.com/mpyw/suve"
  license "MIT"
  version "${VERSION}"

  on_macos do
    on_arm do
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve_${VERSION}_darwin_arm64.tar.gz"
      sha256 "${SHA256_DARWIN_ARM64}"
    end
    on_intel do
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve_${VERSION}_darwin_amd64.tar.gz"
      sha256 "${SHA256_DARWIN_AMD64}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve-cli_${VERSION}_linux_arm64.tar.gz"
      sha256 "${SHA256_LINUX_CLI_ARM64}"
    end
    on_intel do
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve-cli_${VERSION}_linux_amd64.tar.gz"
      sha256 "${SHA256_LINUX_CLI_AMD64}"
    end
  end

  conflicts_with "suve", because: "both install a `suve` binary"

  def install
    # Linux uses CLI-only binary (named suve-cli), macOS uses full binary (named suve)
    if File.exist?("suve-cli")
      bin.install "suve-cli" => "suve"
    else
      bin.install "suve"
    end
  end

  test do
    system bin/"suve", "--version"
  end
end
