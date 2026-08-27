package selfupdate

import "testing"

func TestIsReleaseVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"released, stamped by goreleaser", "1.9.0", true},
		{"released, with tag prefix", "v1.9.0", true},
		{"default for a plain go build", "dev", false},
		{"empty", "", false},
		// The Makefile appends +local so a clean checkout at a tag is
		// distinguishable from a release. go-version ignores build metadata, so
		// without this test a local build compares EQUAL to the release and
		// would silently be treated as up to date.
		{"local build at a tag", "1.9.0+local", false},
		{"local build with dirty tree", "v1.9.0-dirty+local", false},
		// git describe off-tag parses as a PRERELEASE below 1.9.0, so a
		// comparison-based check would offer a developer a "newer" release that
		// actually overwrites their own build.
		{"git describe off tag", "v1.9.0-3-gac69ae3", false},
		{"git describe off tag, dirty", "v1.9.0-3-gac69ae3-dirty", false},
		{"not a version at all", "banana", false},
		{"prerelease tag", "v2.0.0-rc1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsReleaseVersion(c.in); got != c.want {
				t.Errorf("IsReleaseVersion(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.9.0":   "1.9.0",
		"1.9.0":    "1.9.0",
		" v1.9.0 ": "1.9.0",
		"":         "",
	} {
		if got := NormalizeVersion(in); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		want            bool
	}{
		// The case that matters most: a released binary reports "1.9.0" while
		// the tag reads "v1.9.0". Without normalization on both sides these
		// look unequal and every release appears to be an update.
		{"same version, mixed prefixes", "1.9.0", "v1.9.0", false},
		{"newer patch", "1.9.0", "v1.9.1", true},
		{"newer minor", "v1.9.0", "1.10.0", true},
		{"older is not newer", "v1.9.1", "v1.9.0", false},
		{"double digit minor sorts numerically", "1.9.0", "1.10.0", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Newer(c.current, c.latest)
			if err != nil {
				t.Fatalf("Newer(%q, %q) returned %v", c.current, c.latest, err)
			}
			if got != c.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
			}
		})
	}

	if _, err := Newer("banana", "1.0.0"); err == nil {
		t.Error("Newer with an unparseable current version should return an error")
	}
}

func TestSanitizeStripsControlCharacters(t *testing.T) {
	// Tags and asset names come from a remote server and are printed to a
	// terminal, so an escape sequence in one must not be interpreted.
	got := sanitize("v1.9.0\x1b[31mHACKED\x1b[0m\n")
	want := "v1.9.0[31mHACKED[0m"
	if got != want {
		t.Errorf("sanitize() = %q, want %q", got, want)
	}
}
