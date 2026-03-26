# vmup - GCP Instance Manager

A single-binary TUI app for launching and managing GCP compute instances with Terraform, SSH tunneling via IAP, and interactive terminal UI.

## Prerequisites

- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) (`gcloud` CLI) - Required for IAP SSH tunneling

That's it. Terraform is auto-installed on first run.

## Installation

### macOS / Linux

**Using GitHub CLI (recommended):**

```bash
curl -fsSL -H "Authorization: Bearer $(gh auth token)" \
  https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.sh | sh
```

**Using a GitHub token:**

```bash
export GITHUB_TOKEN=ghp_your_token_here
curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" \
  https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.sh | sh
```

### Windows (PowerShell)

**Using GitHub CLI (recommended):**

```powershell
& { $h = @{ Authorization = "Bearer $(gh auth token)" }; iex (irm https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.ps1 -Headers $h) }
```

**Using a GitHub token:**

```powershell
$env:GITHUB_TOKEN = "ghp_your_token_here"
$headers = @{ Authorization = "Bearer $env:GITHUB_TOKEN" }
iex (irm https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.ps1 -Headers $headers)
```

### From source

1. Clone this repository:

   ```bash
   git clone https://github.com/vindhyadatascience/vds-gcp-launch-instance.git
   cd vds-gcp-launch-instance
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

This triggers the Release workflow which cross-compiles binaries for macOS (amd64/arm64), Linux (amd64/arm64), and Windows (amd64), then creates a GitHub Release with the archives attached.

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
