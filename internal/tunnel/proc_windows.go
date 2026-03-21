//go:build windows

package tunnel

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No Setpgid equivalent on Windows; tunnel processes run in the default group.
}
