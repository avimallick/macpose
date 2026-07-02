class Macpose < Formula
  desc "Compose-style runner for Apple container on macOS"
  homepage "https://github.com/avimallick/macpose"
  license "Apache-2.0"
  head "https://github.com/avimallick/macpose.git", branch: "main"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/avimallick/macpose/releases/download/v0.1.0/macpose-v0.1.0-darwin-arm64.tar.gz"
      sha256 "REPLACE_WITH_RELEASE_SHA256"
    end
  end

  def install
    bin.install "macpose"
    generate_completions_from_executable(bin/"macpose", "completion")
  end

  test do
    assert_match "macpose version", shell_output("#{bin}/macpose --version")
  end
end
