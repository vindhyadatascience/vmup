package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tarGz builds a gzipped tar from name/content pairs, in order.
func tarGz(t *testing.T, entries ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e[0], Mode: 0o644, Size: int64(len(e[1])), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	member := binaryMemberName()

	// The published archive contains CHANGELOG.md, LICENSE, README.md and the
	// binary — and the binary is NOT first. Taking the first regular entry
	// would install a changelog as the executable.
	t.Run("selects the binary by name, not position", func(t *testing.T) {
		archive := tarGz(t,
			[2]string{"CHANGELOG.md", "# Changelog"},
			[2]string{"LICENSE", "Apache 2.0"},
			[2]string{"README.md", "# vmup"},
			[2]string{member, "ELF-ish binary bytes"},
		)
		var out bytes.Buffer
		if err := extractBinary(bytes.NewReader(archive), &out); err != nil {
			t.Fatalf("extractBinary returned %v", err)
		}
		if out.String() != "ELF-ish binary bytes" {
			t.Errorf("extracted %q", out.String())
		}
	})

	t.Run("missing member is an error", func(t *testing.T) {
		archive := tarGz(t, [2]string{"README.md", "# vmup"})
		var out bytes.Buffer
		if err := extractBinary(bytes.NewReader(archive), &out); !errors.Is(err, ErrMemberNotFound) {
			t.Errorf("want ErrMemberNotFound, got %v", err)
		}
	})

	// Nothing is written to a path taken from the archive, so a traversal
	// entry cannot escape; it simply is not the member being looked for.
	t.Run("path traversal entry is ignored", func(t *testing.T) {
		archive := tarGz(t,
			[2]string{"../../../../etc/passwd", "root::0:0"},
			[2]string{member, "real binary"},
		)
		var out bytes.Buffer
		if err := extractBinary(bytes.NewReader(archive), &out); err != nil {
			t.Fatalf("returned %v", err)
		}
		if out.String() != "real binary" {
			t.Errorf("extracted %q", out.String())
		}
	})

	t.Run("not a gzip stream", func(t *testing.T) {
		var out bytes.Buffer
		if err := extractBinary(strings.NewReader("plain text"), &out); err == nil {
			t.Error("want an error for a non-gzip body")
		}
	})
}

func TestStageAndSwap(t *testing.T) {
	t.Run("replaces the target and leaves nothing behind", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "vmup")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := stageAndSwap(target, func(w io.Writer) error {
			_, err := w.Write([]byte("new"))
			return err
		}); err != nil {
			t.Fatalf("stageAndSwap returned %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "new" {
			t.Errorf("target contains %q, want %q", got, "new")
		}
		assertNoStagedFiles(t, dir)
	})

	// The property that matters: a failure anywhere in the fill step must not
	// leave an executable-sized temporary file in the user's bin directory.
	t.Run("removes the staged file when filling fails", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "vmup")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}

		wantErr := errors.New("download failed")
		if err := stageAndSwap(target, func(w io.Writer) error {
			w.Write([]byte("partial download"))
			return wantErr
		}); !errors.Is(err, wantErr) {
			t.Fatalf("stageAndSwap returned %v, want %v", err, wantErr)
		}

		// The original must survive untouched.
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "old" {
			t.Errorf("target was modified: %q", got)
		}
		assertNoStagedFiles(t, dir)
	})

	t.Run("installs when the target does not exist yet", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "vmup")
		if err := stageAndSwap(target, func(w io.Writer) error {
			_, err := w.Write([]byte("fresh"))
			return err
		}); err != nil {
			t.Fatalf("stageAndSwap returned %v", err)
		}
		if got, _ := os.ReadFile(target); string(got) != "fresh" {
			t.Errorf("target contains %q", got)
		}
	})

	t.Run("preserves the existing file mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix permission bits")
		}
		dir := t.TempDir()
		target := filepath.Join(dir, "vmup")
		if err := os.WriteFile(target, []byte("old"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := stageAndSwap(target, func(w io.Writer) error {
			_, err := w.Write([]byte("new"))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o750 {
			t.Errorf("mode = %v, want 0750", st.Mode().Perm())
		}
	})
}

// The swap must publish a NEW inode rather than rewrite the existing one.
// Overwriting in place is what leaves a running macOS binary permanently
// unexecutable, so this is the property the whole design turns on.
func TestSwapAllocatesANewInode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode semantics are unix-specific")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "vmup")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	if err := stageAndSwap(target, func(w io.Writer) error {
		_, err := w.Write([]byte("new"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("swap reused the original inode; a running process would have been killed")
	}
}

func assertNoStagedFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".vmup-new-") {
			t.Errorf("staged file left behind: %s", e.Name())
		}
	}
}
