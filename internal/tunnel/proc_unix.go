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

func killProcessTree(proc *os.Process) error {
	// Setpgid: true makes the process the group leader (PGID == PID).
	// Negative PID sends the signal to every process in that group,
	// killing gcloud AND its child ssh process.
	return syscall.Kill(-proc.Pid, syscall.SIGKILL)
}

func killByName(vmName string) {
	// Two process shapes to match: service tunnels run `gcloud compute ssh`,
	// transfer tunnels run `gcloud compute start-iap-tunnel`.
	for _, pat := range []string{
		fmt.Sprintf("gcloud compute ssh %s", vmName),
		fmt.Sprintf("gcloud compute start-iap-tunnel %s", vmName),
	} {
		_ = exec.Command("pkill", "-f", pat).Run()
	}
}
