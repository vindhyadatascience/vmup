package gcloud

import (
	"fmt"
	"os/exec"
	"strings"

	"vds-gcp-launch-instance/internal/config"
)

func IsInstalled() bool {
	_, err := exec.LookPath("gcloud")
	return err == nil
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

func SSHCommand(cfg config.Config) *exec.Cmd {
	return exec.Command("gcloud", "compute", "ssh",
		cfg.VMName,
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
		"--tunnel-through-iap",
	)
}
