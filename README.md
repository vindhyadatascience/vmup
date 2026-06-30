# vmup - GCP Instance Manager

A single-binary TUI app for launching and managing GCP compute instances with Terraform, SSH tunneling via IAP, and interactive terminal UI.

## Prerequisites

- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) (`gcloud` CLI) - Required for IAP SSH tunneling

That's it. Terraform is auto-installed on first run.

## Installation

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/vindhyadatascience/vmup/main/install.sh | sh
```

This downloads the latest release binary and installs it to `/usr/local/bin` (falling back to `~/.local/bin`).

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/vindhyadatascience/vmup/main/install.ps1 | iex
```

This installs the latest release binary to `%LOCALAPPDATA%\vmup` and adds it to your user `PATH`.

### From source

1. Clone this repository:

   ```bash
   git clone https://github.com/vindhyadatascience/vmup.git
   cd vmup
   ```

2. Build and run:

   ```bash
   make run
   ```

   Or build separately:

   ```bash
   make build
   ./vmup
   ```

## Usage

The TUI presents a menu with the following options:

- **Launch New VM** - Configure and provision a new GCP instance. Fill out the form (project ID, VM name, image, machine type, port mappings, etc.), then Terraform runs `init` and `apply` with streaming output. SSH tunnels start automatically after provisioning.

- **Start Tunnels** - Start a stopped VM and re-establish SSH tunnels for the configured port mappings.

- **Stop Tunnels** - Close SSH tunnels and optionally stop the VM to save costs.

- **SSH Session** - Open an interactive SSH session via IAP. The TUI suspends while SSH is active and resumes on exit.

- **Destroy VM** - Tear down all resources with `terraform destroy` and clean up the project directory.

## How It Works

- Project state is stored in `~/.vmup/projects/<vm-name>/` (terraform.tfvars, .terraform/, terraform.tfstate)
- Terraform binary is auto-downloaded to `~/.vmup/bin/` on first run via [hc-install](https://github.com/hashicorp/hc-install)
- The Terraform config (`main.tf`) is embedded in the binary via `go:embed`
- SSH tunnels use `gcloud compute ssh` with IAP tunneling (`--tunnel-through-iap`)

## Building

```bash
make build          # Build for current platform
make build-all      # Cross-compile for darwin/amd64, darwin/arm64, linux/amd64
make clean          # Remove built binaries
```

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions. To publish a new release, tag the commit and push:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers the Release workflow which cross-compiles binaries for macOS (amd64/arm64), Linux (amd64/arm64), and Windows (amd64), signs and notarizes the macOS binaries, and creates a GitHub Release with the archives and checksums attached.

See [RELEASING.md](RELEASING.md) for the one-time macOS signing/notarization setup (Apple Developer ID certificate and App Store Connect API key).

## After Deployment

Once the instance is created, you can access forwarded services through the SSH tunnels. For images with RStudio, navigate to [localhost:8787](http://localhost:8787/). Your username and password are displayed on the status screen after provisioning. You can change the password with `sudo passwd {userNameHere}`.

### Adding git credentials

Use the GitHub CLI to authenticate with GitHub:

```bash
gh auth login
```

### Authenticating Docker to GitHub Container Registry

Create a classic Personal Access Token (PAT) from GitHub (https://github.com/settings/tokens) with the `read:packages` scope selected, save it to a file called `~/.ghcr_token` and authenticate to the `ghcr.io` registry using the following command:

```bash
cat ~/.ghcr_token | docker login ghcr.io -u <username> --password-stdin
```

## License

vmup is licensed under the [Apache License 2.0](LICENSE). See the [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE) files for details.

Copyright 2026 Vindhya Data Science, Inc.
