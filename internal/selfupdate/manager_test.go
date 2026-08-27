package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerFor(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		manager string
	}{
		// Homebrew reaches the binary through a symlink in <prefix>/bin that
		// points into the Cellar, which is why the caller resolves symlinks
		// before classifying.
		{"homebrew cellar", "/opt/homebrew/Cellar/vmup/1.9.0/bin/vmup", "Homebrew"},
		{"homebrew intel prefix", "/usr/local/Cellar/vmup/1.9.0/bin/vmup", "Homebrew"},
		{"homebrew cask", "/opt/homebrew/Caskroom/vmup/1.9.0/vmup", "Homebrew"},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/vmup/1.9.0/bin/vmup", "Homebrew"},

		// These are Windows paths, and they are asserted here on the Linux CI
		// runner. filepath.ToSlash is a no-op on unix, so splitting on '/'
		// alone would leave each of these as a single segment and no rule
		// would ever match.
		{"scoop app", `C:\Users\me\scoop\apps\vmup\current\vmup.exe`, "Scoop"},
		{"scoop shim", `C:\Users\me\scoop\shims\vmup.exe`, "Scoop"},
		{"winget package", `C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\vmup_x\vmup.exe`, "winget"},
		{"winget shim", `C:\Users\me\AppData\Local\Microsoft\WinGet\Links\vmup.exe`, "winget"},
		{"chocolatey", `C:\ProgramData\chocolatey\lib\vmup\tools\vmup.exe`, "Chocolatey"},

		{"plain unix install", "/home/me/.local/bin/vmup", ""},
		{"system install", "/usr/local/bin/vmup", ""},
		// "scoop" alone, not followed by apps/shims, is somebody's directory.
		{"unrelated directory named scoop", "/home/me/scoop/vmup", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, cmd, ok := managerFor(c.path)
			if c.manager == "" {
				if ok {
					t.Fatalf("managerFor(%q) claimed %q, want unmanaged", c.path, got)
				}
				return
			}
			if !ok {
				t.Fatalf("managerFor(%q) found no manager, want %q", c.path, c.manager)
			}
			if got != c.manager {
				t.Errorf("managerFor(%q) = %q, want %q", c.path, got, c.manager)
			}
			if cmd == "" {
				t.Errorf("managerFor(%q) returned an empty upgrade command", c.path)
			}
		})
	}
}

// A conda environment is an ordinary user-writable directory. Whether vmup may
// replace itself there depends on whether conda OWNS the file, not on where it
// sits — copying a binary into an environment's bin is a normal thing to do,
// and refusing to update it would break that workflow for no benefit.
func TestCondaOwnership(t *testing.T) {
	newEnv := func(t *testing.T, owned bool) string {
		t.Helper()
		prefix := t.TempDir()
		if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(prefix, "conda-meta"), 0o755); err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(prefix, "bin", "vmup")
		if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
		files := `["bin/python", "lib/libpython.so"]`
		if owned {
			files = `["bin/vmup", "share/doc/vmup/README"]`
		}
		manifest := filepath.Join(prefix, "conda-meta", "somepkg-1.0-0.json")
		if err := os.WriteFile(manifest, []byte(`{"files": `+files+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		return exe
	}

	t.Run("conda owns the binary", func(t *testing.T) {
		exe := newEnv(t, true)
		name, cmd, ok := managerFor(exe)
		if !ok || name != "conda" {
			t.Fatalf("managerFor = (%q, %v), want conda", name, ok)
		}
		if cmd == "" {
			t.Error("expected an upgrade command")
		}
	})

	// The real-world case: vmup copied by hand into an environment whose other
	// packages are conda-managed. This must remain self-updatable.
	t.Run("copied in by hand alongside conda packages", func(t *testing.T) {
		exe := newEnv(t, false)
		if name, _, ok := managerFor(exe); ok {
			t.Errorf("managerFor claimed %q, want unmanaged", name)
		}
	})
}

func TestProbeWritable(t *testing.T) {
	dir := t.TempDir()
	if err := probeWritable(dir); err != nil {
		t.Errorf("probeWritable(%q) = %v, want nil", dir, err)
	}
	if err := probeWritable(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("probeWritable on a missing directory should fail")
	}

	// Leave nothing behind: the probe runs in the user's bin directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("probeWritable left %d files behind", len(entries))
	}
}
