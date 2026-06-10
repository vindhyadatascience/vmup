# Troubleshooting & FAQ

## Installation

### `gh: command not found` or auth errors during install

The repository is private, so the installer must authenticate. Install the
[GitHub CLI](https://cli.github.com) and run `gh auth login`, or export a
`GITHUB_TOKEN` with `repo` scope. See [Installation](../getting-started/installation.md).

### `vmup: command not found` after installing

- **macOS/Linux** — if the script fell back to `~/.local/bin`, make sure it's on your
  `PATH`: `export PATH="$HOME/.local/bin:$PATH"` in your shell profile.
- **Windows** — the installer updates your user `PATH`, but only new terminals pick it
  up. Close and reopen your terminal.

## Authentication

### "gcloud not authenticated" or auth prompts inside vmup

Run `gcloud auth login`. For API-backed features (cost estimates, batch status queries)
you may also need Application Default Credentials:
`gcloud auth application-default login`. vmup offers to run these for you when it
detects missing credentials.

### IAP tunnel: permission denied

Your Google account needs `roles/iap.tunnelResourceAccessor` on the instance. vmup
grants this at launch to `<username>@vindhyadatascience.com` — check that the username
you launched with matches your Vindhya identity, and that you're logged into the right
Google account (`gcloud auth list`).

## Tunnels & connectivity

### "Address already in use" when starting tunnels

Another process is listening on the local port (often a previous tunnel that didn't shut
down, or a local RStudio/Jupyter). Find it with `lsof -i :8787` (macOS/Linux) and stop
it, or edit the VM (++e++) to use a different local port.

### Tunnel connects but the service doesn't load

Right after provisioning, the startup script is still installing updates — give it a
few minutes. If it persists, SSH in (++c++) and check the service:
`sudo systemctl status rstudio-server`.

### SSH is slow to become available after launch

Normal. The VM boots, runs a full system upgrade, and restarts once before it's ready.
vmup polls SSH and brings tunnels up as soon as it responds.

## State & recovery

### vmup lost track of a VM

vmup's view of the world is the Terraform state under `~/.vmup/projects/<vm-name>/`
(or your custom [data directory](../usage/settings.md)). If that directory was deleted
or moved, the VM still exists in GCP — manage it via the
[Cloud Console](https://console.cloud.google.com/compute/instances) or restore the
state directory from wherever it went.

### A destroy failed halfway

Re-run destroy (++shift+d++) — Terraform picks up where it left off. If the local state
is gone entirely, delete the leftover resources in the Cloud Console (the VPC, subnet,
router, firewall rules, and instance all carry the VM name and timestamp).

## Costs

### The cost estimate seems off or missing

Estimates use the Cloud Billing Catalog API; without permission to it, vmup silently
falls back to built-in us-central1 on-demand rates, which may not match your region or
discounts. Estimates also exclude disks and network egress.

### Am I paying for a stopped VM?

A stopped VM stops machine-hour billing, but you still pay for its boot disk and any
data disks. Destroy VMs you won't use for a while — and keep your data on
[data disks](../usage/data-disks.md), which survive the destroy.

## Where to get help

Open an issue on
[GitHub](https://github.com/vindhyadatascience/vds-gcp-launch-instance/issues) with the
output from the progress screen (++p++) if the problem involves a Terraform operation.
