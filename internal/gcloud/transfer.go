package gcloud

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vindhyadatascience/vmup/internal/config"
)

// Direction is the direction of a file transfer relative to the local machine.
type Direction int

const (
	Upload   Direction = iota // local → VM
	Download                  // VM → local
)

func (d Direction) String() string {
	if d == Download {
		return "download"
	}
	return "upload"
}

// TransferSpec describes a single scp transfer between the local machine and a VM.
type TransferSpec struct {
	Direction Direction
	Local     string // source on upload, destination on download
	Remote    string // path on the VM
	Recurse   bool   // required when either side is a directory
	Compress  bool
}

// SCPCommand builds a `gcloud compute scp --tunnel-through-iap` invocation.
//
// This is the file-transfer counterpart to SSHCommand: it takes the same IAP
// path, relies on the same roles/iap.tunnelResourceAccessor grant, and reuses
// the SSH key gcloud already manages. Note that scp takes --scp-flag and
// --strict-host-key-checking rather than the --ssh-flag used elsewhere in this
// package; passing --ssh-flag here is rejected by gcloud.
func SCPCommand(cfg config.Config, spec TransferSpec) *exec.Cmd {
	remote := fmt.Sprintf("%s:%s", cfg.VMName, spec.Remote)

	args := []string{
		"compute", "scp",
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
		"--tunnel-through-iap",
		"--strict-host-key-checking=no",
	}
	if spec.Recurse {
		args = append(args, "--recurse")
	}
	if spec.Compress {
		args = append(args, "--compress")
	}

	if spec.Direction == Upload {
		args = append(args, config.CanonicalPath(spec.Local), remote)
	} else {
		args = append(args, remote, config.CanonicalPath(spec.Local))
	}

	return exec.Command("gcloud", args...)
}

// DescribeTransfer renders a one-line summary of a transfer, used as the first
// line of the progress log.
func DescribeTransfer(cfg config.Config, spec TransferSpec) string {
	local := config.CanonicalPath(spec.Local)
	remote := fmt.Sprintf("%s:%s", cfg.VMName, spec.Remote)
	if spec.Direction == Upload {
		return fmt.Sprintf("Copying %s → %s", local, remote)
	}
	return fmt.Sprintf("Copying %s → %s", remote, local)
}

// gcloudKeyPath is the SSH key `gcloud compute ssh` generates and manages.
// Native tools reaching the VM through a transfer tunnel need it explicitly,
// since it is not the default identity ssh(1) would pick.
func gcloudKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.ssh/google_compute_engine"
	}
	return filepath.Join(home, ".ssh", "google_compute_engine")
}

// TransferCommands returns ready-to-run native-tool commands for a live
// transfer tunnel listening on localPort.
//
// Every VM reached this way appears to ssh(1) as localhost:<localPort>, so
// without -o HostKeyAlias the second VM you connect to trips
// "REMOTE HOST IDENTIFICATION HAS CHANGED". The alias keeps one known_hosts
// entry per VM instead of one per local port.
func TransferCommands(cfg config.Config, localPort string) []string {
	key := gcloudKeyPath()
	user := cfg.Username
	if user == "" {
		user = "<user>"
	}
	sshOpts := fmt.Sprintf("-i %s -o HostKeyAlias=%s", key, cfg.VMName)

	return []string{
		fmt.Sprintf("scp -P %s %s ./local-file %s@localhost:~/", localPort, sshOpts, user),
		fmt.Sprintf("rsync -av --progress -e 'ssh -p %s %s' ./local-dir/ %s@localhost:~/local-dir/", localPort, sshOpts, user),
		fmt.Sprintf("sftp -P %s %s %s@localhost", localPort, sshOpts, user),
	}
}

// TransferHint is the short reminder shown alongside the generated commands.
func TransferHint(vmName string) string {
	return strings.TrimSpace(fmt.Sprintf(
		"The tunnel stays open until you close it (x on %s, or quitting vmup).", vmName))
}
