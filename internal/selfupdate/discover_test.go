package selfupdate

import (
	"errors"
	"testing"
)

// checkURL is what makes the checksum meaningful. If an attacker can choose
// the host, they supply both the archive and the digest it is compared
// against, and verification proves nothing.
func TestCheckURL(t *testing.T) {
	allowed := []string{
		"https://github.com/vindhyadatascience/vmup/releases/latest",
		"https://api.github.com/repos/vindhyadatascience/vmup/releases/latest",
		"https://objects.githubusercontent.com/some/path",
		"https://release-assets.githubusercontent.com/some/path",
	}
	for _, u := range allowed {
		if err := checkURL(u); err != nil {
			t.Errorf("checkURL(%q) = %v, want nil", u, err)
		}
	}

	rejected := []string{
		"http://github.com/x/y",                 // downgraded scheme
		"https://evil.example.com/x",            // wrong host
		"https://github.com.evil.example.com/x", // suffix confusion
		"https://githubusercontent.com/x",       // not one of the asset hosts
		"ftp://github.com/x",                    // wrong scheme entirely
		"://not a url",                          // unparseable
	}
	for _, u := range rejected {
		if err := checkURL(u); !errors.Is(err, ErrUnsupportedHost) {
			t.Errorf("checkURL(%q) = %v, want ErrUnsupportedHost", u, err)
		}
	}
}

func TestTagFromLocation(t *testing.T) {
	const req = "https://github.com/vindhyadatascience/vmup/releases/latest"

	t.Run("absolute location", func(t *testing.T) {
		got, err := tagFromLocation(req, "https://github.com/vindhyadatascience/vmup/releases/tag/v1.9.0")
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if got != "v1.9.0" {
			t.Errorf("got %q, want v1.9.0", got)
		}
	})

	// GitHub currently sends an absolute Location, but a relative one is legal
	// and must resolve rather than be rejected.
	t.Run("relative location", func(t *testing.T) {
		got, err := tagFromLocation(req, "/vindhyadatascience/vmup/releases/tag/v1.9.1")
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if got != "v1.9.1" {
			t.Errorf("got %q, want v1.9.1", got)
		}
	})

	// A redirect to another host must not be able to name our version, or the
	// whole update channel can be pointed elsewhere.
	t.Run("redirect off github is refused", func(t *testing.T) {
		_, err := tagFromLocation(req, "https://evil.example.com/vmup/releases/tag/v9.9.9")
		if !errors.Is(err, ErrUnsupportedHost) {
			t.Errorf("got %v, want ErrUnsupportedHost", err)
		}
	})

	t.Run("scheme downgrade is refused", func(t *testing.T) {
		_, err := tagFromLocation(req, "http://github.com/vindhyadatascience/vmup/releases/tag/v9.9.9")
		if !errors.Is(err, ErrUnsupportedHost) {
			t.Errorf("got %v, want ErrUnsupportedHost", err)
		}
	})

	// The tag is interpolated into a URL and printed to a terminal, so a value
	// that is not a plain version is refused outright rather than sanitized
	// and used.
	t.Run("non-version tags are refused", func(t *testing.T) {
		for _, tag := range []string{
			"../../../other/repo",
			"v1.9.0%0d%0a",
			"latest",
			"",
		} {
			if _, err := tagFromLocation(req, "https://github.com/vindhyadatascience/vmup/releases/tag/"+tag); err == nil {
				t.Errorf("tag %q was accepted", tag)
			}
		}
	})

	t.Run("unexpected redirect target", func(t *testing.T) {
		if _, err := tagFromLocation(req, "https://github.com/login"); err == nil {
			t.Error("want an error when the path has no tag marker")
		}
	})
}
