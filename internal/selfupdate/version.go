package selfupdate

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	goversion "github.com/hashicorp/go-version"
)

// describeSuffix matches the tail `git describe` appends when HEAD is not
// exactly on a tag, e.g. "v1.9.0-3-gac69ae3".
var describeSuffix = regexp.MustCompile(`-\d+-g[0-9a-f]{7,40}$`)

// NormalizeVersion trims whitespace and a single leading "v".
//
// This matters more than it looks: GoReleaser stamps the version without the
// "v" (a released binary reports "1.9.0"), while git tags and the GitHub
// redirect both carry it ("v1.9.0"). Comparing the two forms directly makes
// every release look different from itself.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// IsReleaseVersion reports whether v names a published release, as opposed to
// a local or development build.
//
// This is a test on the raw string and must run before any semver comparison,
// because comparison gives the wrong answer twice over:
//
//   - go-version ignores build metadata, so "1.9.0+local" compares EQUAL to
//     "1.9.0" and a local build would be reported as up to date.
//   - `git describe` off-tag yields "v1.9.0-3-gabc1234", which parses as a
//     PRERELEASE sorting BELOW 1.9.0 — so a developer would be offered an
//     "update" that is actually a downgrade onto their own work.
func IsReleaseVersion(v string) bool {
	v = strings.TrimSpace(v)
	switch {
	case v == "", v == "dev":
		return false
	case strings.Contains(v, "+"):
		return false
	case strings.Contains(v, "-dirty"):
		return false
	case describeSuffix.MatchString(v):
		return false
	}
	_, err := goversion.NewVersion(NormalizeVersion(v))
	return err == nil
}

// Newer reports whether latest is strictly newer than current.
func Newer(current, latest string) (bool, error) {
	c, err := goversion.NewVersion(NormalizeVersion(current))
	if err != nil {
		return false, fmt.Errorf("parsing current version %q: %w", sanitize(current), err)
	}
	l, err := goversion.NewVersion(NormalizeVersion(latest))
	if err != nil {
		return false, fmt.Errorf("parsing latest version %q: %w", sanitize(latest), err)
	}
	return l.GreaterThan(c), nil
}

// WhyNotUpdatable returns a user-facing reason self-update cannot run for this
// build, or "" when it can.
func WhyNotUpdatable(current string) string {
	if os.Getenv(EnvDisable) != "" {
		return "Update checks are disabled by " + EnvDisable + "."
	}
	if !IsReleaseVersion(current) {
		return fmt.Sprintf("vmup %s is a local build, so there is nothing to update to.", sanitize(current))
	}
	if _, err := CurrentTarget(); err != nil {
		return fmt.Sprintf("No vmup build is published for %s/%s.", runtime.GOOS, runtime.GOARCH)
	}
	return ""
}
