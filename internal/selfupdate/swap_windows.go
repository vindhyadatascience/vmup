//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// oldSuffix marks a binary displaced by an update.
const oldSuffix = ".old"

// swapBinary replaces the target on Windows, which will not allow a running
// executable to be deleted or overwritten but does allow one to be renamed.
//
// The live binary is moved aside, the new one takes its place, and the
// displaced file is deleted on a best-effort basis — that delete fails while
// the old process is still running, which is why sweepLeftovers exists. On
// failure the original is moved back rather than leaving no binary at all.
func swapBinary(staged, target string) error {
	old := target + oldSuffix
	os.Remove(old)

	restore := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return fmt.Errorf("moving the current binary aside: %w", err)
		}
		restore = true
	}

	if err := os.Rename(staged, target); err != nil {
		if restore {
			os.Rename(old, target)
		}
		return fmt.Errorf("replacing %s: %w", target, err)
	}

	os.Remove(old)
	return nil
}

// sweepLeftovers removes a displaced binary from a previous update. It is
// called at startup, by which time the process that was holding the old file
// open has exited.
func sweepLeftovers(target string) {
	os.Remove(target + oldSuffix)
}
