class Runeverything < Formula
  desc "Open-source remote agent for Intent Computing (pair via QR, no GUI)"
  homepage "https://github.com/foqerhk/runeverything"
  version "0.1.0"
  license "MIT"

  # Prefer prebuilt binaries from GitHub Releases (fast one-liner install).
  # After tagging a release, run scripts/build-release.sh and
  # scripts/sync-homebrew-formula.sh to refresh URLs + sha256.
  on_macos do
    on_arm do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_darwin_arm64.tar.gz"
      sha256 "11447349566dff5b58d4872a86eb3223bcc21e2abf8ddfa74e8515d7288fbda8"
    end
    on_intel do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_darwin_amd64.tar.gz"
      sha256 "a0b3f8115107d56ac25f3432051edcbe68a89aeb96f8960615cd065dd0c1376b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_linux_arm64.tar.gz"
      sha256 "ac54c042e839e2ef5c7254dd917d024be0269a5d15890fc77293b8e327e5b866"
    end
    on_intel do
      url "https://github.com/foqerhk/runeverything/releases/download/v#{version}/runeverything_linux_amd64.tar.gz"
      sha256 "1d8f251ab3da5fccaa111964efe1cc30bf728eb0f1d9d1e743439169e352c6ff"
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
    else
      bin.install "runeverything"
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
