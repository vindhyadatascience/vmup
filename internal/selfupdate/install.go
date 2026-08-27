package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

// InstallResult reports what was installed.
type InstallResult struct {
	// Version is the version actually installed, read from the release that
	// was downloaded rather than the one a caller may have seen earlier: a
	// release can be published between the check and the install, and
	// reporting the stale number would tell the user they have something they
	// do not.
	Version string
	Path    string
}

// Install downloads the given release and replaces the binary at targetPath.
//
// The order is load-bearing: verify the archive against the published checksum
// BEFORE anything is extracted, stage the new binary as a sibling of the target
// so the final rename cannot cross a filesystem boundary, then swap. Nothing
// executable is ever written directly over the live path.
func Install(ctx context.Context, rel Release, targetPath string) (res InstallResult, err error) {
	target, err := CurrentTarget()
	if err != nil {
		return res, err
	}
	archive, err := ArchiveAsset(rel.Assets, target)
	if err != nil {
		return res, err
	}
	sums, err := ChecksumsAsset(rel.Assets)
	if err != nil {
		return res, err
	}

	want, err := fetchChecksum(ctx, sums, archive.Name)
	if err != nil {
		return res, err
	}

	if err := stageAndSwap(targetPath, func(w io.Writer) error {
		return downloadVerified(ctx, archive, want, w)
	}); err != nil {
		return res, err
	}

	return InstallResult{Version: NormalizeVersion(rel.Tag), Path: targetPath}, nil
}

// stageAndSwap writes a new binary beside target using fill, then renames it
// into place.
//
// The staged file is removed unless the swap actually succeeded. That is
// tracked with an explicit flag rather than by inspecting a named error
// result, because every early return in this path would otherwise have to
// remember to assign the same error variable — and one that forgets leaks an
// executable-sized file into the user's bin directory on every failure.
func stageAndSwap(target string, fill func(io.Writer) error) error {
	dir := filepath.Dir(target)

	// CreateTemp uses O_EXCL and a random name, so this cannot open a file an
	// attacker planted in a group-writable directory. Creating it in the
	// destination directory also keeps the rename below on one filesystem.
	staged, err := os.CreateTemp(dir, ".vmup-new-*")
	if err != nil {
		return fmt.Errorf("creating a staging file in %s: %w", dir, err)
	}
	stagedPath := staged.Name()

	swapped := false
	defer func() {
		staged.Close()
		if !swapped {
			os.Remove(stagedPath)
		}
	}()

	if err := fill(staged); err != nil {
		return err
	}

	// Match the existing binary's permissions where possible. The stat error is
	// checked before its result is used: the target can legitimately be absent.
	mode := os.FileMode(0o755)
	if st, statErr := os.Stat(target); statErr == nil {
		mode = st.Mode().Perm()
	}
	if err := staged.Chmod(mode); err != nil {
		return fmt.Errorf("setting permissions on the new binary: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return fmt.Errorf("flushing the new binary to disk: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("closing the new binary: %w", err)
	}

	if err := swapBinary(stagedPath, target); err != nil {
		return err
	}
	swapped = true
	return nil
}

// fetchChecksum downloads the checksums file and returns the digest recorded
// for name.
func fetchChecksum(ctx context.Context, sums Asset, name string) (string, error) {
	body, err := get(ctx, sums.URL)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", sanitize(sums.Name), err)
	}
	defer body.Close()

	table, err := ParseChecksums(body)
	if err != nil {
		return "", err
	}
	want, ok := table[name]
	if !ok {
		return "", fmt.Errorf("%s does not list %s", sanitize(sums.Name), sanitize(name))
	}
	return want, nil
}

// downloadVerified streams the archive, checks its digest, and only then
// extracts the binary into dst.
//
// The archive is buffered to a temporary file rather than held in memory, and
// the digest is computed over the bytes as they land, so extraction operates on
// exactly the bytes that were verified.
func downloadVerified(ctx context.Context, archive Asset, wantDigest string, dst io.Writer) error {
	body, err := get(ctx, archive.URL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", sanitize(archive.Name), err)
	}
	defer body.Close()

	tmp, err := os.CreateTemp("", "vmup-archive-*")
	if err != nil {
		return fmt.Errorf("creating a temporary file: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(body, maxArchiveSize))
	if err != nil {
		return fmt.Errorf("downloading %s: %w", sanitize(archive.Name), err)
	}
	if n == maxArchiveSize {
		return fmt.Errorf("%s is larger than the %d byte limit", sanitize(archive.Name), int64(maxArchiveSize))
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != wantDigest {
		return fmt.Errorf("%w: %s\n  expected %s\n  received %s",
			ErrChecksumMismatch, sanitize(archive.Name), wantDigest, got)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extractBinary(tmp, dst)
}

// extractBinary copies the vmup member of a gzipped tar into dst.
//
// The member is selected by name: the archive also contains README.md,
// LICENSE and CHANGELOG.md, so taking the first regular entry would install
// a text file as the binary.
func extractBinary(r io.Reader, dst io.Writer) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}
	defer gz.Close()

	want := binaryMemberName()
	tr := tar.NewReader(gz)
	for i := 0; i < maxArchiveMembers; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Compare the base name so a member stored with a path prefix still
		// matches, while a traversal attempt cannot escape: nothing is written
		// to a path derived from the archive.
		if path.Base(hdr.Name) != want {
			continue
		}
		if _, err := io.Copy(dst, io.LimitReader(tr, maxArchiveSize)); err != nil {
			return fmt.Errorf("extracting %s: %w", want, err)
		}
		return nil
	}
	return fmt.Errorf("%w: expected a member named %s", ErrMemberNotFound, want)
}

// get issues a validated GET and returns the response body.
func get(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	if err := checkURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return resp.Body, nil
}
