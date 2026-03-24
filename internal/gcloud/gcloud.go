package gcloud

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"vds-gcp-launch-instance/internal/config"
)

func IsInstalled() bool {
	_, err := exec.LookPath("gcloud")
	return err == nil
}

func HasAuth() bool {
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func HasADC() bool {
	cmd := exec.Command("gcloud", "auth", "application-default", "print-access-token")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func AuthLoginCommand() *exec.Cmd {
	return exec.Command("gcloud", "auth", "login")
}

func ADCLoginCommand() *exec.Cmd {
	return exec.Command("gcloud", "auth", "application-default", "login")
}

func StartInstance(cfg config.Config) error {
	cmd := exec.Command("gcloud", "compute", "instances", "start",
		cfg.VMName,
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start instance: %s: %w", string(out), err)
	}
	return nil
}

func StopInstance(cfg config.Config) error {
	cmd := exec.Command("gcloud", "compute", "instances", "stop",
		cfg.VMName,
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop instance: %s: %w", string(out), err)
	}
	return nil
}

func InstanceStatus(vmName, projectID, zone string) string {
	cmd := exec.Command("gcloud", "compute", "instances", "describe",
		vmName,
		"--project", projectID,
		"--zone", zone,
		"--format=value(status)",
	)
	out, err := cmd.Output()
	if err != nil {
		return "UNKNOWN"
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "UNKNOWN"
	}
	return s
}

const (
	sshRetryInterval  = 5 * time.Second
	sshAttemptTimeout = 20 * time.Second
)

// WaitForSSH polls until an SSH connection through IAP succeeds or the
// timeout is reached. The onRetry callback is invoked before each retry so
// the caller can report progress.
func WaitForSSH(cfg config.Config, timeout time.Duration, onRetry func(attempt int, elapsed time.Duration)) error {
	start := time.Now()
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), sshAttemptTimeout)
		cmd := exec.CommandContext(ctx, "gcloud", "compute", "ssh", cfg.VMName,
			"--project", cfg.ProjectID,
			"--zone", cfg.Zone,
			"--tunnel-through-iap",
			"--command=true",
			"--ssh-flag=-o ConnectTimeout=10",
			"--ssh-flag=-o StrictHostKeyChecking=no",
			"--ssh-flag=-o UserKnownHostsFile=/dev/null",
		)
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		err := cmd.Run()
		cancel()

		if err == nil {
			return nil
		}

		elapsed := time.Since(start)
		if elapsed+sshRetryInterval > timeout {
			return fmt.Errorf("SSH not ready after %v (%d attempts)", elapsed.Round(time.Second), attempt)
		}

		if onRetry != nil {
			onRetry(attempt, elapsed)
		}
		time.Sleep(sshRetryInterval)
	}
}

func SSHCommand(cfg config.Config) *exec.Cmd {
	return exec.Command("gcloud", "compute", "ssh",
		cfg.VMName,
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
		"--tunnel-through-iap",
	)
}
