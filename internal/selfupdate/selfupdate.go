// Package selfupdate discovers newer vmup releases and replaces the running
// binary with one.
//
// The design is deliberately conservative:
//
//   - The update target is compiled in. There is no environment override for
//     the repository or the download host: an override would let anything that
//     can set an environment variable choose what code vmup installs.
//   - Every URL is checked against an allowlist of GitHub hosts before a
//     request is made, and redirects are re-checked. A download whose bytes and
//     whose checksums both come from an attacker verifies perfectly, so pinning
//     the host is what makes the checksum meaningful.
//   - The binary is replaced by renaming a new file over it, never by writing
//     into the existing file. Overwriting a running Mach-O in place leaves it
//     permanently unrunnable on macOS, because the kernel validates page hashes
//     recorded in its code signature and Go ad-hoc signs every darwin binary.
//   - Nothing is installed without the user asking for it.
package selfupdate

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Repo is the source of updates. Compile-time only, by design.
	Repo = "vindhyadatascience/vmup"

	// BinaryName is the executable's name, without any platform suffix.
	BinaryName = "vmup"

	// EnvDisable suppresses the background check and refuses to install.
	// One switch, so there is one thing to document and one thing to test.
	EnvDisable = "VMUP_NO_UPDATE_CHECK"

	// userAgent identifies vmup to GitHub. Requests without one are more
	// likely to be rate limited.
	userAgent = "vmup-selfupdate"

	// maxArchiveSize caps what will be downloaded and decompressed. Releases
	// are ~10 MB; this leaves generous headroom while bounding a hostile or
	// corrupt archive.
	maxArchiveSize = 128 << 20

	// maxArchiveMembers caps tar entries scanned while looking for the binary.
	maxArchiveMembers = 100
)

var (
	ErrDisabled          = errors.New("self-update is disabled")
	ErrNotARelease       = errors.New("not a released build")
	ErrUpToDate          = errors.New("already up to date")
	ErrUnsupportedHost   = errors.New("refusing to fetch from a non-GitHub host")
	ErrChecksumMismatch  = errors.New("downloaded archive does not match its published checksum")
	ErrAssetNotFound     = errors.New("no matching release asset")
	ErrAssetAmbiguous    = errors.New("more than one release asset matches")
	ErrMemberNotFound    = errors.New("archive does not contain the vmup binary")
	ErrUnsupportedTarget = errors.New("no vmup build is published for this platform")
)

// allowedHosts is every host a release download may legitimately come from.
// github.com issues the redirect; the *.githubusercontent.com hosts serve the
// bytes.
var allowedHosts = map[string]bool{
	"github.com":                           true,
	"api.github.com":                       true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// checkURL rejects any URL that is not HTTPS to a known GitHub host. It is
// applied to every request before it is made, and to every redirect hop.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: unparseable URL", ErrUnsupportedHost)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: %s is not https", ErrUnsupportedHost, u.Scheme)
	}
	if !allowedHosts[u.Host] {
		return fmt.Errorf("%w: %s", ErrUnsupportedHost, u.Host)
	}
	return nil
}

// redirectPolicy re-checks the host on every hop. Without it, one redirect off
// GitHub would let a single compromised URL supply both the archive and the
// checksums that "verify" it.
func redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return checkURL(req.URL.String())
}

// noRedirectClient reads the tag out of a 302 Location without following it.
// Using the HTML endpoint rather than the API keeps the background check off
// GitHub's 60-request-per-hour unauthenticated limit, which is applied per IP
// and therefore shared by everyone behind one office NAT.
var noRedirectClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// apiClient talks to api.github.com for release metadata.
var apiClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: redirectPolicy,
}

// downloadClient fetches release assets. It has no Timeout because the caller's
// context bounds it: a fixed timeout large enough for a slow link is useless as
// a stall detector, and one small enough to detect stalls breaks slow links.
var downloadClient = &http.Client{
	CheckRedirect: redirectPolicy,
}

// sanitize strips control characters from release-controlled text before it is
// printed. Asset names and tags come from a remote server and are written to a
// terminal, so an escape sequence in one would otherwise be interpreted.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
