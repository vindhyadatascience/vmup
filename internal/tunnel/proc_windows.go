//go:build windows

package tunnel

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// No Setpgid equivalent on Windows; tunnel processes run in the default group.
}

func isProcessAlive(proc *os.Process) bool {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	const STILL_ACTIVE = 259

	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == STILL_ACTIVE
}

func killByName(vmName string) {
	// No reliable equivalent to `pkill -f` on Windows.
	// proc.Kill() in StopAll handles the primary case.
}
