//go:build !windows

package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func isProcessAlive(proc *os.Process) bool {
	return proc.Signal(syscall.Signal(0)) == nil
}

func killByName(vmName string) {
	_ = exec.Command("pkill", "-f",
		fmt.Sprintf("gcloud compute ssh %s", vmName)).Run()
}
