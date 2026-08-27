package selfupdate

import (
	"context"
	"fmt"
	"os"
	"time"
)

// CheckResult reports the outcome of an update check.
type CheckResult struct {
	Current   string
	Latest    string
	Available bool
}

// Check reports whether a newer release exists.
//
// It is safe to call on every launch: a cached result is reused for a day, and
// the timestamp is recorded even when the check fails so an offline machine
// retries daily rather than on every start. Callers in the TUI are expected to
// discard the error — a failed update check is not something to interrupt
// someone's work with.
func Check(ctx context.Context, current string, force bool) (CheckResult, error) {
	res := CheckResult{Current: current}

	if os.Getenv(EnvDisable) != "" {
		return res, ErrDisabled
	}
	if !IsReleaseVersion(current) {
		return res, ErrNotARelease
	}

	now := time.Now()
	if st := loadState(); !force && st.fresh(now) {
		if st.LatestVersion == "" {
			return res, nil
		}
		res.Latest = st.LatestVersion
		newer, err := Newer(current, st.LatestVersion)
		res.Available = err == nil && newer
		return res, nil
	}

	tag, err := LatestTag(ctx)
	if err != nil {
		// Record the attempt so a persistent failure is retried on the same
		// cadence as a success, rather than on every launch.
		saveState(state{CheckedAt: now})
		return res, err
	}

	saveState(state{CheckedAt: now, LatestVersion: tag})

	res.Latest = tag
	newer, err := Newer(current, tag)
	if err != nil {
		return res, err
	}
	res.Available = newer
	return res, nil
}

// Update downloads and installs the newest release over the running binary.
//
// It refuses rather than improvises: a package-managed install is left to its
// package manager, and an unwritable location is reported with the command
// that will fix it. There is deliberately no privileged path — running a
// downloader, a decompressor and a tar reader as root to work around a
// directory permission is a much worse trade than asking the user to re-run
// the installer.
func Update(ctx context.Context, current string) (InstallResult, error) {
	var res InstallResult

	if reason := WhyNotUpdatable(current); reason != "" {
		return res, fmt.Errorf("%s", reason)
	}

	in, err := DetectInstall()
	if err != nil {
		return res, err
	}
	switch in.Kind {
	case KindManaged:
		return res, fmt.Errorf("vmup was installed with %s; update it with:\n\n    %s", in.Manager, in.UpgradeCmd)
	case KindNotWritable:
		return res, fmt.Errorf("cannot write to %s, so vmup cannot replace itself.\n\nReinstall with:\n\n    %s", in.Dir, InstallerHint())
	}

	rel, err := LatestRelease(ctx)
	if err != nil {
		return res, err
	}
	newer, err := Newer(current, rel.Tag)
	if err != nil {
		return res, err
	}
	if !newer {
		return res, ErrUpToDate
	}

	return Install(ctx, rel, in.Path)
}

// SweepLeftovers removes a binary displaced by a previous update. It is a
// no-op except on Windows, where the file could not be deleted while the
// updating process was still running.
func SweepLeftovers() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	sweepLeftovers(exe)
}
