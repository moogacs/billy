class Billy < Formula
  desc "Privacy-first CLI for estimating Claude, Codex, and Cursor usage cost"
  homepage "https://github.com/moogacs/billy"
  head "https://github.com/moogacs/billy.git", branch: "main"
  license "Apache-2.0"

  depends_on "go" => :build

  def install
    ldflags = %W[
      -s -w
      -X main.version=#{version}
      -X main.commit=homebrew
      -X main.date=#{time.iso8601}
    ]

    system "go", "build", *std_go_args(ldflags:), "./cmd/billy"
  end

  test do
    assert_match "billy", shell_output("#{bin}/billy --help")
  end
end
