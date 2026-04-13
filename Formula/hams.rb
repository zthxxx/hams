# Homebrew formula for hams
# To install: brew install zthxxx/tap/hams
class Hams < Formula
  desc "Declarative IaC environment management for workstations"
  homepage "https://hams.zthxxx.me"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/zthxxx/hams/releases/download/v#{version}/hams-darwin-arm64"
      sha256 "PLACEHOLDER"

      def install
        bin.install "hams-darwin-arm64" => "hams"
      end
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/zthxxx/hams/releases/download/v#{version}/hams-linux-amd64"
      sha256 "PLACEHOLDER"

      def install
        bin.install "hams-linux-amd64" => "hams"
      end
    end

    on_arm do
      url "https://github.com/zthxxx/hams/releases/download/v#{version}/hams-linux-arm64"
      sha256 "PLACEHOLDER"

      def install
        bin.install "hams-linux-arm64" => "hams"
      end
    end
  end

  test do
    assert_match "hams", shell_output("#{bin}/hams --version")
  end
end
