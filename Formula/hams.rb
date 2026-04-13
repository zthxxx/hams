# Homebrew formula for hams
# To install: brew install zthxxx/tap/hams
#
# This file is just a example, like https://github.com/go-task/homebrew-tap/blob/main/Formula/go-task.rb
#
# The Formula/ will be extracted to other repo `zthxxx/homebrew-tap`, and then users can install hams via brew install zthxxx/tap/hams
#  and the file update will be trigger by GitHub Actions in zthxxx/hams repo, which will update the sha256 and version in zthxxx/homebrew-tap repo with PR
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
