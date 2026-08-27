package selfupdate

import (
	"fmt"
	"runtime"
)

// Target is the os/arch flavour of release asset this process needs.
type Target struct {
	GOOS   string
	GOARCH string
}

func (t Target) String() string { return t.GOOS + "/" + t.GOARCH }

// CurrentTarget resolves the asset flavour for the running process.
//
// Two corrections to runtime's view of the world:
//
//   - Under Rosetta on Apple Silicon, runtime.GOARCH reports "amd64". Taking
//     that at face value would pin a user who once installed the Intel build to
//     the Intel build forever, with every future update reinforcing it.
//   - .goreleaser.yml does not build windows/arm64, so that combination is
//     reported as unsupported here rather than 404ing mid-download.
func CurrentTarget() (Target, error) {
	t := Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if t.GOOS == "darwin" && t.GOARCH == "amd64" && isTranslated() {
		t.GOARCH = "arm64"
	}
	if t.GOOS == "windows" && t.GOARCH == "arm64" {
		return t, fmt.Errorf("%w: %s", ErrUnsupportedTarget, t)
	}
	return t, nil
}

// binaryMemberName is the archive entry to extract on this platform.
func binaryMemberName() string {
	if runtime.GOOS == "windows" {
		return BinaryName + ".exe"
	}
	return BinaryName
}

// InstallerHint is the official one-liner for reinstalling, used when vmup
// cannot replace itself.
func InstallerHint() string {
	if runtime.GOOS == "windows" {
		return "irm https://raw.githubusercontent.com/" + Repo + "/main/install.ps1 | iex"
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + Repo + "/main/install.sh | sh"
}
