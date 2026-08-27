package selfupdate

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// archivePattern matches the release archive for one target.
//
// The updater compiled into a release is frozen for the life of that install,
// so it must not depend on today's exact filename. Matching a pattern rather
// than concatenating a name means a later change of archive format, or an
// added version segment, does not 404 every client already in the field. The
// extension is anchored so a future .sig or .sbom.json sidecar cannot match.
func archivePattern(t Target) *regexp.Regexp {
	return regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(BinaryName) +
		`[_-](?:[^/]*[_-])?` + regexp.QuoteMeta(t.GOOS) + `[_-]` + regexp.QuoteMeta(t.GOARCH) +
		`\.(?:tar\.gz|tgz|zip)$`)
}

var checksumsPattern = regexp.MustCompile(`(?i)(checksums?|sha256sums?)`)

var sidecarSuffixes = []string{".sig", ".asc", ".pem", ".sbom.json"}

func isSidecar(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range sidecarSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

// ArchiveAsset resolves the release archive for t.
//
// On more than one match it prefers the canonical GoReleaser name and
// otherwise fails loudly. Silently taking the first match is how an updater
// ends up installing the wrong architecture.
func ArchiveAsset(assets []Asset, t Target) (Asset, error) {
	re := archivePattern(t)
	var matches []Asset
	for _, a := range assets {
		if !isSidecar(a.Name) && re.MatchString(a.Name) {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 0:
		return Asset{}, fmt.Errorf("%w for %s", ErrAssetNotFound, t)
	case 1:
		return matches[0], nil
	}
	canonical := fmt.Sprintf("%s_%s_%s.tar.gz", BinaryName, t.GOOS, t.GOARCH)
	for _, a := range matches {
		if a.Name == canonical {
			return a, nil
		}
	}
	names := make([]string, 0, len(matches))
	for _, a := range matches {
		names = append(names, sanitize(a.Name))
	}
	return Asset{}, fmt.Errorf("%w for %s: %s", ErrAssetAmbiguous, t, strings.Join(names, ", "))
}

// ChecksumsAsset resolves the published checksums file.
func ChecksumsAsset(assets []Asset) (Asset, error) {
	var matches []Asset
	for _, a := range assets {
		if !isSidecar(a.Name) && checksumsPattern.MatchString(a.Name) {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		return Asset{}, fmt.Errorf("%w: no checksums file", ErrAssetNotFound)
	}
	best := matches[0]
	for _, a := range matches[1:] {
		if len(a.Name) < len(best.Name) {
			best = a
		}
	}
	return best, nil
}

// ParseChecksums reads a GNU coreutils style checksums file into a map of
// filename to lowercase hex digest.
//
// Fields are split on whitespace rather than sliced at a fixed offset, so a
// switch from text mode ("hash  name") to binary mode ("hash *name") does not
// silently mismatch every entry.
func ParseChecksums(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(io.LimitReader(r, 1<<20))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != 64 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		out[name] = digest
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("checksums file contained no usable entries")
	}
	return out, nil
}
