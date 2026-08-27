//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
)

// swapBinary moves the staged binary over the target.
//
// rename(2) is the whole point. Writing into the existing file would keep its
// inode, and on macOS the kernel validates page hashes recorded in a Mach-O's
// code signature — Go's linker ad-hoc signs every darwin binary, so mutating
// the pages under a running process gets it killed and leaves a path that can
// never be executed again. Renaming publishes a new inode instead, and any
// process already running from the old one continues undisturbed.
//
// Both paths are in the same directory, so this cannot fail with EXDEV.
func swapBinary(staged, target string) error {
	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("replacing %s: %w", target, err)
	}
	return nil
}

// sweepLeftovers is a no-op on unix: the old inode is released as soon as the
// last process using it exits, so there is nothing left on disk to clean up.
func sweepLeftovers(string) {}
