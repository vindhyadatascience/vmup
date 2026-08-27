package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Package-level vars rather than consts so tests can point them at an
// httptest server. They are still derived from the compiled-in Repo and are
// never read from the environment.
// tagPattern is the only shape a release tag may take. Anything else is
// refused rather than cleaned up.
var tagPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$`)

var (
	releasesLatestURL = "https://github.com/" + Repo + "/releases/latest"
	apiLatestURL      = "https://api.github.com/repos/" + Repo + "/releases/latest"
)

// Release is the subset of a GitHub release that matters here.
type Release struct {
	Tag    string
	Assets []Asset
}

// LatestTag returns the newest release tag using the HTML endpoint's redirect.
//
// GitHub answers /releases/latest with a 302 whose Location ends in
// /releases/tag/<tag>. Reading the tag from that header costs nothing against
// the API's 60-per-hour unauthenticated budget, which is counted per IP and so
// is shared by every user behind the same NAT. A background check that burned
// that budget would fail for a whole office at once.
func LatestTag(ctx context.Context) (string, error) {
	if err := checkURL(releasesLatestURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesLatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("checking for updates: no redirect from %s (status %d)", releasesLatestURL, resp.StatusCode)
	}
	return tagFromLocation(releasesLatestURL, loc)
}

// apiRelease mirrors the fields we read from the releases API.
type apiRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// LatestRelease fetches the newest release including its asset list. This
// costs one API request and is only used when an install is actually being
// performed, never for the background check.
func LatestRelease(ctx context.Context) (Release, error) {
	if err := checkURL(apiLatestURL); err != nil {
		return Release{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiLatestURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := apiClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetching release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetching release metadata: GitHub returned %s", resp.Status)
	}

	var ar apiRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&ar); err != nil {
		return Release{}, fmt.Errorf("decoding release metadata: %w", err)
	}
	if ar.Draft || ar.Prerelease {
		return Release{}, fmt.Errorf("latest release is not a stable release")
	}
	if !IsReleaseVersion(ar.TagName) {
		return Release{}, fmt.Errorf("release tag %q is not a release version", sanitize(ar.TagName))
	}

	rel := Release{Tag: ar.TagName}
	for _, a := range ar.Assets {
		// An asset URL is server-controlled, so it is validated before it is
		// stored, not merely before it is fetched.
		if err := checkURL(a.URL); err != nil {
			continue
		}
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	return rel, nil
}

// tagFromLocation extracts a release tag from the Location header of
// /releases/latest.
//
// Split out from LatestTag so it can be tested directly: this is where a
// redirect to another host, or a crafted tag, would have to be caught, and
// exercising that through a live HTTP client would mean relaxing the host
// allowlist in order to test it.
func tagFromLocation(requestURL, location string) (string, error) {
	base, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("checking for updates: unparseable redirect")
	}

	// Resolve first, so a relative Location is interpreted against the request
	// rather than rejected, then re-check the host so an absolute redirect off
	// GitHub cannot name our tag.
	abs := base.ResolveReference(ref)
	if err := checkURL(abs.String()); err != nil {
		return "", err
	}

	const marker = "/releases/tag/"
	i := strings.LastIndex(abs.Path, marker)
	if i < 0 {
		return "", fmt.Errorf("checking for updates: unexpected redirect target")
	}
	tag := strings.Trim(abs.Path[i+len(marker):], "/")
	if tag == "" {
		return "", fmt.Errorf("checking for updates: empty tag in redirect")
	}

	// Validate the tag's exact shape rather than trusting a lenient parse.
	// url.Parse percent-decodes the path, so a Location ending in "%0d%0a"
	// yields a tag containing a real CRLF; IsReleaseVersion trims whitespace
	// internally and would accept it, and the raw value would then be
	// interpolated into a URL and printed to a terminal.
	if !tagPattern.MatchString(tag) {
		return "", fmt.Errorf("checking for updates: %q is not a release version", sanitize(tag))
	}
	if !IsReleaseVersion(tag) {
		return "", fmt.Errorf("checking for updates: %q is not a release version", sanitize(tag))
	}
	return tag, nil
}
