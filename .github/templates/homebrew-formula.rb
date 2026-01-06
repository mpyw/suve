class Suve < Formula
  desc "Git-like CLI for AWS Parameter Store and Secrets Manager"
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
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve_${VERSION}_linux_arm64.tar.gz"
      sha256 "${SHA256_LINUX_ARM64}"
    end
    on_intel do
      url "https://github.com/mpyw/suve/releases/download/v${VERSION}/suve_${VERSION}_linux_amd64.tar.gz"
      sha256 "${SHA256_LINUX_AMD64}"
    end
  end

  def install
    bin.install "suve"
  end

  test do
    system bin/"suve", "--version"
  end
end
