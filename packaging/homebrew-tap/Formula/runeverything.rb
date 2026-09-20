class Runeverything < Formula
  desc "Open-source remote agent for Intent Computing (pair via QR, no GUI)"
  homepage "https://github.com/foqerhk/runeverything"
  version "0.1.1"
  license "MIT"

  # Prefer prebuilt binaries from GitHub Releases (fast one-liner install).
  # After tagging a release, run scripts/build-release.sh and
  # scripts/sync-homebrew-formula.sh to refresh URLs + sha256.
  on_macos do
    on_arm do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_darwin_arm64.tar.gz"
      sha256 "93a9c7b2728aebb6cc111c4e77ec9d5a9435b620c5c97b586bef6212ce17e5f0"
    end
    on_intel do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_darwin_amd64.tar.gz"
      sha256 "a1e55282b95f5270c76ad9781f2e19c8420091c4414533d2e5e835ddb38c8bd8"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_linux_arm64.tar.gz"
      sha256 "4ca68381617d658132edb1a9997aec99cbae514e743d810b654c7276461d7092"
    end
    on_intel do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_linux_amd64.tar.gz"
      sha256 "4f2998ea96193e7eccc432986582009a2610fe3e6fb4b4521b53f9f3bdecb1cf"
    end
  end

  head do
    url "https://github.com/foqerhk/runeverything.git", branch: "main"
    depends_on "go" => :build
  end

  livecheck do
    url "https://github.com/foqerhk/runeverything/releases/latest"
    regex(%r{href=.*?/tag/v?(\d+(?:\.\d+)+)}i)
  end

  def install
    if build.head?
      system "go", "build",
             *std_go_args(ldflags: "-s -w -X main.version=HEAD", output: bin/"runeverything"),
             "./cmd/agent"
      system "go", "build",
             *std_go_args(ldflags: "-s -w -X main.version=HEAD", output: bin/"runeverything-relay"),
             "./cmd/relay"
    else
      bin.install "runeverything"
      bin.install "runeverything-relay" if File.exist?("runeverything-relay")
    end
  end

  service do
    run [opt_bin/"runeverything", "run", "-no-qr"]
    keep_alive true
    working_dir var/"runeverything"
    environment_variables RE_HOME: var/"runeverything"
    log_path var/"log/runeverything.log"
    error_log_path var/"log/runeverything.error.log"
  end

  def caveats
    <<~EOS
      Pair with KoKo (print QR):
        runeverything pair

      Background service (keeps agent alive; agent also prevents idle sleep):
        brew services start runeverything

      Desktop tip: leave the Mac plugged in. Lock screen is fine; avoid Sleep.
      Disable keep-awake with: export RE_KEEP_AWAKE=0
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/runeverything version")
  end
end
