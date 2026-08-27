package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

func assets(names ...string) []Asset {
	out := make([]Asset, 0, len(names))
	for _, n := range names {
		out = append(out, Asset{Name: n, URL: "https://github.com/x/y/releases/download/v1/" + n})
	}
	return out
}

func TestArchiveAsset(t *testing.T) {
	live := assets(
		"vmup_darwin_amd64.tar.gz",
		"vmup_darwin_arm64.tar.gz",
		"vmup_linux_amd64.tar.gz",
		"vmup_linux_arm64.tar.gz",
		"vmup_windows_amd64.tar.gz",
		"vmup_1.9.0_checksums.txt",
	)

	cases := []struct {
		target Target
		want   string
	}{
		{Target{"darwin", "arm64"}, "vmup_darwin_arm64.tar.gz"},
		{Target{"darwin", "amd64"}, "vmup_darwin_amd64.tar.gz"},
		{Target{"linux", "arm64"}, "vmup_linux_arm64.tar.gz"},
		{Target{"windows", "amd64"}, "vmup_windows_amd64.tar.gz"},
	}
	for _, c := range cases {
		t.Run(c.target.String(), func(t *testing.T) {
			got, err := ArchiveAsset(live, c.target)
			if err != nil {
				t.Fatalf("ArchiveAsset(%s) returned %v", c.target, err)
			}
			if got.Name != c.want {
				t.Errorf("ArchiveAsset(%s) = %q, want %q", c.target, got.Name, c.want)
			}
		})
	}

	t.Run("no build for this platform", func(t *testing.T) {
		if _, err := ArchiveAsset(live, Target{"plan9", "riscv64"}); !errors.Is(err, ErrAssetNotFound) {
			t.Errorf("want ErrAssetNotFound, got %v", err)
		}
	})

	// A future release could add a signature or SBOM beside each archive.
	// Those must never be mistaken for the binary.
	t.Run("ignores sidecar files", func(t *testing.T) {
		withSidecars := append(assets("vmup_linux_amd64.tar.gz.sig", "vmup_linux_amd64.tar.gz.sbom.json"), live...)
		got, err := ArchiveAsset(withSidecars, Target{"linux", "amd64"})
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if got.Name != "vmup_linux_amd64.tar.gz" {
			t.Errorf("got %q", got.Name)
		}
	})

	// Matching a pattern is what lets a future packaging change reach clients
	// that were compiled before it existed.
	t.Run("tolerates a version segment and a format change", func(t *testing.T) {
		future := assets("vmup_2.0.0_linux_amd64.zip", "vmup_2.0.0_checksums.txt")
		got, err := ArchiveAsset(future, Target{"linux", "amd64"})
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if got.Name != "vmup_2.0.0_linux_amd64.zip" {
			t.Errorf("got %q", got.Name)
		}
	})

	// Installing the wrong architecture is worse than refusing, so an
	// ambiguous match must fail rather than guess — unless one candidate is
	// the canonical name.
	t.Run("ambiguity without a canonical name is an error", func(t *testing.T) {
		ambiguous := assets("vmup_a_linux_amd64.zip", "vmup_b_linux_amd64.tgz")
		if _, err := ArchiveAsset(ambiguous, Target{"linux", "amd64"}); !errors.Is(err, ErrAssetAmbiguous) {
			t.Errorf("want ErrAssetAmbiguous, got %v", err)
		}
	})

	t.Run("prefers the canonical name when ambiguous", func(t *testing.T) {
		ambiguous := assets("vmup_extra_linux_amd64.zip", "vmup_linux_amd64.tar.gz")
		got, err := ArchiveAsset(ambiguous, Target{"linux", "amd64"})
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if got.Name != "vmup_linux_amd64.tar.gz" {
			t.Errorf("got %q", got.Name)
		}
	})
}

func TestChecksumsAsset(t *testing.T) {
	got, err := ChecksumsAsset(assets("vmup_linux_amd64.tar.gz", "vmup_1.9.0_checksums.txt"))
	if err != nil {
		t.Fatalf("returned %v", err)
	}
	if got.Name != "vmup_1.9.0_checksums.txt" {
		t.Errorf("got %q", got.Name)
	}
	if _, err := ChecksumsAsset(assets("vmup_linux_amd64.tar.gz")); !errors.Is(err, ErrAssetNotFound) {
		t.Errorf("want ErrAssetNotFound, got %v", err)
	}
}

func TestParseChecksums(t *testing.T) {
	// The real format GoReleaser emits: 64 hex, two spaces, bare filename.
	const real = "048574274fdb2bf011bc69d53ecbdac8389e6dba147c4b4760aa4481283fa24b  vmup_windows_amd64.tar.gz\n" +
		"ad254761f51c2de69dce93416369ad89ae709e63e3e0aea3b4844f42cee5897b  vmup_darwin_arm64.tar.gz\n"

	table, err := ParseChecksums(strings.NewReader(real))
	if err != nil {
		t.Fatalf("returned %v", err)
	}
	if got := table["vmup_darwin_arm64.tar.gz"]; got != "ad254761f51c2de69dce93416369ad89ae709e63e3e0aea3b4844f42cee5897b" {
		t.Errorf("darwin/arm64 digest = %q", got)
	}

	// Binary mode prefixes the name with an asterisk. Splitting on whitespace
	// rather than slicing at a fixed offset means this still parses.
	t.Run("binary mode", func(t *testing.T) {
		bin := "ad254761f51c2de69dce93416369ad89ae709e63e3e0aea3b4844f42cee5897b *vmup_darwin_arm64.tar.gz\n"
		table, err := ParseChecksums(strings.NewReader(bin))
		if err != nil {
			t.Fatalf("returned %v", err)
		}
		if _, ok := table["vmup_darwin_arm64.tar.gz"]; !ok {
			t.Errorf("binary-mode entry not parsed: %v", table)
		}
	})

	t.Run("skips junk and rejects an empty result", func(t *testing.T) {
		if _, err := ParseChecksums(strings.NewReader("not a checksum line\n\n")); err == nil {
			t.Error("want an error for a file with no usable entries")
		}
	})
}
