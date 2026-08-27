package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind describes what, if anything, owns the installed binary.
type Kind int

const (
	// KindWritable means vmup can replace itself in place.
	KindWritable Kind = iota
	// KindManaged means a package manager owns the binary and should be used
	// instead, so an update does not fight the manager's own bookkeeping.
	KindManaged
	// KindNotWritable means the install directory cannot be written to.
	KindNotWritable
)

// Installation describes the installed binary and how it may be updated.
type Installation struct {
	// Path is the executable with symlinks resolved. Resolving matters: a
	// Homebrew install is reached through a symlink in <prefix>/bin that points
	// into the Cellar, and winget puts a shim on PATH. Testing the unresolved
	// path would classify both as ordinary writable installs and then overwrite
	// the shim rather than the binary.
	Path string
	Dir  string
	Kind Kind
	// Manager and UpgradeCmd are set when Kind is KindManaged.
	Manager    string
	UpgradeCmd string
}

// pathSegments splits a path on both separators.
//
// filepath.ToSlash is a no-op on unix, so a Windows-style path examined on
// linux or darwin — which is exactly what happens in tests, and in CI — stays
// one long segment and no rule ever matches.
func pathSegments(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
}

func hasSegment(segments []string, want string) bool {
	for _, s := range segments {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// hasAdjacentSegments reports whether a and b appear consecutively.
func hasAdjacentSegments(segments []string, a, b string) bool {
	for i := 0; i+1 < len(segments); i++ {
		if strings.EqualFold(segments[i], a) && strings.EqualFold(segments[i+1], b) {
			return true
		}
	}
	return false
}

// DetectInstall classifies the running binary's installation.
func DetectInstall() (Installation, error) {
	exe, err := os.Executable()
	if err != nil {
		return Installation{}, fmt.Errorf("locating the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}

	in := Installation{Path: exe, Dir: filepath.Dir(exe)}

	if name, cmd, ok := managerFor(exe); ok {
		in.Kind, in.Manager, in.UpgradeCmd = KindManaged, name, cmd
		return in, nil
	}

	if err := probeWritable(in.Dir); err != nil {
		in.Kind = KindNotWritable
		return in, nil
	}
	in.Kind = KindWritable
	return in, nil
}

// managerFor reports the package manager that owns exe, if any.
//
// Split out from DetectInstall so it can be tested against paths from every
// platform: os.Executable() cannot be controlled from a test, and the Windows
// rules in particular have to be exercised on the Linux CI runner.
func managerFor(exe string) (name, upgradeCmd string, ok bool) {
	segs := pathSegments(exe)

	switch {
	case hasSegment(segs, "Cellar") || hasSegment(segs, "Caskroom"):
		return "Homebrew", "brew upgrade " + BinaryName, true
	case hasAdjacentSegments(segs, "scoop", "apps") || hasAdjacentSegments(segs, "scoop", "shims"):
		return "Scoop", "scoop update " + BinaryName, true
	case hasAdjacentSegments(segs, "WinGet", "Packages") || hasAdjacentSegments(segs, "WinGet", "Links"):
		return "winget", "winget upgrade " + Repo, true
	case hasSegment(segs, "chocolatey"):
		return "Chocolatey", "choco upgrade " + BinaryName, true
	}

	// conda is deliberately checked by package OWNERSHIP, not by path prefix.
	// Installing vmup into an environment's bin directory by hand is a normal
	// thing to do, and that install is an ordinary writable one — refusing to
	// update it merely because the path sits under an environment prefix would
	// break the common case to guard against the rare one.
	if env, owned := condaEnvOwning(exe); owned {
		return "conda", "conda update -n " + filepath.Base(env) + " " + BinaryName, true
	}

	return "", "", false
}

// condaEnvOwning reports the conda environment prefix that owns exe, if any.
//
// conda records every file it installs in <prefix>/conda-meta/<pkg>.json under
// a "files" array, so ownership is an exact question with an exact answer.
func condaEnvOwning(exe string) (string, bool) {
	prefix := exe
	for {
		next := filepath.Dir(prefix)
		if next == prefix {
			return "", false
		}
		prefix = next
		metaDir := filepath.Join(prefix, "conda-meta")
		if st, err := os.Stat(metaDir); err != nil || !st.IsDir() {
			continue
		}
		rel, err := filepath.Rel(prefix, exe)
		if err != nil {
			return "", false
		}
		if condaMetaLists(metaDir, filepath.ToSlash(rel)) {
			return prefix, true
		}
		// A conda prefix was found but does not own this file: it was copied in
		// by hand, which is a writable install.
		return "", false
	}
}

func condaMetaLists(metaDir, relPath string) bool {
	entries, err := filepath.Glob(filepath.Join(metaDir, "*.json"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		data, err := os.ReadFile(e)
		if err != nil {
			continue
		}
		var meta struct {
			Files []string `json:"files"`
		}
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		for _, f := range meta.Files {
			if f == relPath {
				return true
			}
		}
	}
	return false
}

// probeWritable tests the directory by creating a file, because permission
// bits alone do not account for ACLs, read-only mounts, or immutable flags.
func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".vmup-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
