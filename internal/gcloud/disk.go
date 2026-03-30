package gcloud

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"

	"vds-gcp-launch-instance/internal/config"
)

type DiskStatus struct {
	Status   string   // READY, CREATING, DELETING, FAILED
	Users    []string // instance names attached to (empty = unattached)
	Mode     string   // READ_WRITE or READ_ONLY (empty if not attached)
	SizeGB   string   // current size from GCP
	DiskType string   // short type name, e.g. "pd-ssd"
}

// gcloudDiskDescribe is the JSON shape returned by gcloud compute disks describe.
type gcloudDiskDescribe struct {
	Status string   `json:"status"`
	Users  []string `json:"users"`
	SizeGb string   `json:"sizeGb"`
	Type   string   `json:"type"`
}

// GetDiskStatus queries GCP for the current state of a disk.
func GetDiskStatus(name, projectID, zone string) DiskStatus {
	cmd := exec.Command("gcloud", "compute", "disks", "describe",
		name,
		"--project", projectID,
		"--zone", zone,
		"--format=json(status,users,sizeGb,type)",
	)
	out, err := cmd.Output()
	if err != nil {
		return DiskStatus{Status: "UNKNOWN"}
	}

	var d gcloudDiskDescribe
	if err := json.Unmarshal(out, &d); err != nil {
		return DiskStatus{Status: "UNKNOWN"}
	}

	// Extract instance names from self-link URLs
	var users []string
	for _, u := range d.Users {
		users = append(users, path.Base(u))
	}

	ds := DiskStatus{
		Status:   d.Status,
		Users:    users,
		SizeGB:   d.SizeGb,
		DiskType: path.Base(d.Type), // extract short name from full resource URL
	}

	// Fetch attachment mode from the first attached instance
	if len(users) > 0 && len(d.Users) > 0 {
		ds.Mode = getDiskModeOnInstance(projectID, zone, users[0], name)
	}

	return ds
}

// getDiskModeOnInstance queries an instance to find the attachment mode for a disk.
func getDiskModeOnInstance(projectID, zone, instanceName, diskName string) string {
	// Query the instance's disk list filtered to our disk name
	cmd := exec.Command("gcloud", "compute", "instances", "describe",
		instanceName,
		"--project", projectID,
		"--zone", zone,
		"--format=json(disks)",
	)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	var result struct {
		Disks []struct {
			Source string `json:"source"`
			Mode   string `json:"mode"`
		} `json:"disks"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return ""
	}

	for _, d := range result.Disks {
		if path.Base(d.Source) == diskName {
			return d.Mode // READ_WRITE or READ_ONLY
		}
	}
	return ""
}

// DiskExists checks whether a disk exists in the given project and zone.
func DiskExists(name, projectID, zone string) bool {
	cmd := exec.Command("gcloud", "compute", "disks", "describe",
		name,
		"--project", projectID,
		"--zone", zone,
		"--format=value(name)",
	)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// AttachDisk attaches a persistent disk to an instance.
// mode should be "rw" or "ro".
func AttachDisk(projectID, instanceName, diskName, zone, mode string) error {
	cmd := exec.Command("gcloud", "compute", "instances", "attach-disk",
		instanceName,
		"--project", projectID,
		"--zone", zone,
		"--disk", diskName,
		"--device-name", diskName,
		"--mode", mode,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("attach disk: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DetachDisk detaches a persistent disk from an instance.
func DetachDisk(projectID, instanceName, diskName, zone string) error {
	cmd := exec.Command("gcloud", "compute", "instances", "detach-disk",
		instanceName,
		"--project", projectID,
		"--zone", zone,
		"--disk", diskName,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("detach disk: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// --- Remote mount operations (run on VM via SSH) ---

// FindDiskDevice resolves a disk name to its device path on the VM.
// It polls for up to 30 seconds since the device may take a moment to appear
// after GCP reports the disk as attached.
func FindDiskDevice(vmCfg config.Config, diskName string, onRetry func(attempt int)) (string, error) {
	symlink := fmt.Sprintf("/dev/disk/by-id/google-%s", diskName)
	cmd := fmt.Sprintf("readlink -f %s", symlink)

	for attempt := 1; attempt <= 10; attempt++ {
		out, err := RunSSHCommand(vmCfg, cmd)
		if err == nil && out != "" && strings.HasPrefix(out, "/dev/") && out != symlink {
			return out, nil
		}

		if attempt == 10 {
			// On final failure, list available devices for diagnostics
			listing, _ := RunSSHCommand(vmCfg, "ls -1 /dev/disk/by-id/google-* 2>/dev/null || echo '(none)'")
			return "", fmt.Errorf("could not resolve device for disk '%s' after %d attempts. Available devices:\n%s", diskName, attempt, listing)
		}

		if onRetry != nil {
			onRetry(attempt)
		}
		time.Sleep(3 * time.Second)
	}
	return "", fmt.Errorf("could not resolve device for disk '%s'", diskName)
}

// CheckFilesystem returns true if the device already has a filesystem.
func CheckFilesystem(vmCfg config.Config, device string) (bool, error) {
	cmd := fmt.Sprintf("sudo blkid %s", device)
	out, _ := RunSSHCommand(vmCfg, cmd)
	// blkid returns non-zero if no filesystem found — that's not an error for us
	return strings.Contains(out, "TYPE="), nil
}

// FormatDisk formats a device with the given filesystem type.
func FormatDisk(vmCfg config.Config, device, fsType string) error {
	var cmd string
	switch fsType {
	case "xfs":
		cmd = fmt.Sprintf("sudo mkfs.xfs -f %s", device)
	default:
		cmd = fmt.Sprintf("sudo mkfs.ext4 -m 0 -E lazy_itable_init=0,lazy_journal_init=0,discard %s", device)
	}
	_, err := RunSSHCommand(vmCfg, cmd)
	if err != nil {
		return fmt.Errorf("format disk: %w", err)
	}
	return nil
}

// MountDisk creates the mount point and mounts the device.
// mode should be "rw" or "ro".
func MountDisk(vmCfg config.Config, device, mountPoint, mode string) error {
	opts := "discard,defaults"
	if mode == "ro" {
		opts = "ro,discard"
	}
	cmd := fmt.Sprintf("sudo mkdir -p %s && sudo mount -o %s %s %s", mountPoint, opts, device, mountPoint)
	_, err := RunSSHCommand(vmCfg, cmd)
	if err != nil {
		return fmt.Errorf("mount disk: %w", err)
	}
	return nil
}

// ConfigureFstab adds the disk to /etc/fstab for persistence across reboots.
// Uses UUID and nofail to prevent boot failures if the disk is detached.
// mode should be "rw" or "ro".
func ConfigureFstab(vmCfg config.Config, device, mountPoint, fsType, mode string) error {
	// Get UUID
	uuidCmd := fmt.Sprintf("sudo blkid -s UUID -o value %s", device)
	uuid, err := RunSSHCommand(vmCfg, uuidCmd)
	if err != nil {
		return fmt.Errorf("get UUID: %w", err)
	}
	if uuid == "" {
		return fmt.Errorf("could not determine UUID for %s", device)
	}

	mountOpts := "discard,nofail"
	if mode == "ro" {
		mountOpts = "ro,discard,nofail"
	}

	// Remove any existing entry for this UUID (mode may have changed)
	removeCmd := fmt.Sprintf("sudo sed -i '/UUID=%s/d' /etc/fstab", uuid)
	_, _ = RunSSHCommand(vmCfg, removeCmd)

	// Append new entry
	entry := fmt.Sprintf("UUID=%s %s %s %s 0 2", uuid, mountPoint, fsType, mountOpts)
	appendCmd := fmt.Sprintf("echo '%s' | sudo tee -a /etc/fstab > /dev/null", entry)
	_, err = RunSSHCommand(vmCfg, appendCmd)
	if err != nil {
		return fmt.Errorf("update fstab: %w", err)
	}
	return nil
}

// UnmountDisk unmounts a device if it is currently mounted and removes its fstab entry.
func UnmountDisk(vmCfg config.Config, device string) error {
	// Unmount if currently mounted (ignore error if not mounted)
	unmountCmd := fmt.Sprintf("sudo umount %s 2>/dev/null; true", device)
	_, _ = RunSSHCommand(vmCfg, unmountCmd)

	// Remove fstab entry by UUID
	uuidCmd := fmt.Sprintf("sudo blkid -s UUID -o value %s 2>/dev/null", device)
	uuid, _ := RunSSHCommand(vmCfg, uuidCmd)
	if uuid != "" {
		removeCmd := fmt.Sprintf("sudo sed -i '/UUID=%s/d' /etc/fstab", uuid)
		_, _ = RunSSHCommand(vmCfg, removeCmd)
	}

	return nil
}

// SetMountOwnership sets the owner and permissions on the mount point.
func SetMountOwnership(vmCfg config.Config, mountPoint, owner string) error {
	cmd := fmt.Sprintf("sudo chown -R %s:%s %s && sudo chmod 755 %s", owner, owner, mountPoint, mountPoint)
	_, err := RunSSHCommand(vmCfg, cmd)
	if err != nil {
		return fmt.Errorf("set ownership: %w", err)
	}
	return nil
}
