# typed: false
# frozen_string_literal: true

class Ralphglasses < Formula
  desc "Command-and-control TUI for parallel multi-LLM agent fleets"
  homepage "https://github.com/hairglasses-studio/ralphglasses"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/hairglasses-studio/ralphglasses/releases/download/v#{version}/ralphglasses_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/hairglasses-studio/ralphglasses/releases/download/v#{version}/ralphglasses_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      raise "Linux arm64 is no longer supported for ralphglasses"
    else
      url "https://github.com/hairglasses-studio/ralphglasses/releases/download/v#{version}/ralphglasses_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install "ralphglasses"

    # Install shell completions if present
    bash_completion.install "completions/ralphglasses.bash" => "ralphglasses" if File.exist? "completions/ralphglasses.bash"
    zsh_completion.install "completions/_ralphglasses" if File.exist? "completions/_ralphglasses"
    fish_completion.install "completions/ralphglasses.fish" if File.exist? "completions/ralphglasses.fish"

    # Install man pages if present
    man1.install Dir["man/man1/*.1"] if Dir.exist? "man/man1"
  end

  test do
    assert_match "ralphglasses", shell_output("#{bin}/ralphglasses --version 2>&1", 0)
  end
end
