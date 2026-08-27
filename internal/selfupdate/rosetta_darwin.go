//go:build darwin

package selfupdate

import "syscall"

// isTranslated reports whether this process is running under Rosetta 2.
//
// On a genuine Intel Mac the OID does not exist and sysctlbyname fails with
// ENOENT; that must read as "not translated" rather than as an error, or every
// Intel user is broken.
func isTranslated() bool {
	v, err := syscall.SysctlUint32("sysctl.proc_translated")
	return err == nil && v == 1
}
