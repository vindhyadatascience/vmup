# Installation

vmup ships as a single static binary for macOS, Linux, and Windows. The install scripts
download the latest release, verify your platform, and put the binary on your `PATH`.

!!! note "Private repository"
    Releases live in a private GitHub repository, so every install method requires GitHub
    authentication — either the [`gh` CLI](https://cli.github.com) (recommended) or a
    `GITHUB_TOKEN` with `repo` scope. See [Prerequisites](prerequisites.md).

=== "macOS / Linux"

    **Using GitHub CLI (recommended)**

    First authenticate with GitHub if you haven't already:

    ```bash
    gh auth login
    ```

    Then install vmup:

    ```bash
    curl -fsSL -H "Authorization: Bearer $(gh auth token)" \
      https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.sh | sh
    ```

    **Using a GitHub token**

    ```bash
    export GITHUB_TOKEN=ghp_your_token_here
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" \
      https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.sh | sh
    ```

    The script detects your OS and architecture (Intel or Apple Silicon / arm64) and
    installs to `/usr/local/bin` when writable, falling back to `~/.local/bin`. If it
    falls back, it warns when `~/.local/bin` is not on your `PATH`.

=== "Windows (PowerShell)"

    **Using GitHub CLI (recommended)**

    First authenticate with GitHub if you haven't already:

    ```powershell
    gh auth login
    ```

    Then install vmup:

    ```powershell
    & { $h = @{ Authorization = "Bearer $(gh auth token)" }; iex (irm https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.ps1 -Headers $h) }
    ```

    **Using a GitHub token**

    ```powershell
    $env:GITHUB_TOKEN = "ghp_your_token_here"
    $headers = @{ Authorization = "Bearer $env:GITHUB_TOKEN" }
    iex (irm https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.ps1 -Headers $headers)
    ```

    The script installs to `%LOCALAPPDATA%\vmup` and adds that directory to your user
    `PATH`. **Open a new terminal** afterwards for the `PATH` change to take effect.

## Verify the install

```bash
vmup
```

You should see the vmup menu. Press ++q++ to quit.

## Manual download

Release archives are attached to each
[GitHub release](https://github.com/vindhyadatascience/vds-gcp-launch-instance/releases)
and named `vmup_<os>_<arch>.tar.gz`:

| Platform | Archive |
| --- | --- |
| macOS (Apple Silicon) | `vmup_darwin_arm64.tar.gz` |
| macOS (Intel) | `vmup_darwin_amd64.tar.gz` |
| Linux (x86_64) | `vmup_linux_amd64.tar.gz` |
| Linux (arm64) | `vmup_linux_arm64.tar.gz` |
| Windows (x86_64) | `vmup_windows_amd64.tar.gz` |

Extract the archive and place the `vmup` binary anywhere on your `PATH`.

## From source

Requires [Go](https://go.dev/dl/):

```bash
git clone https://github.com/vindhyadatascience/vds-gcp-launch-instance.git
cd vds-gcp-launch-instance
make build
./vmup
```

Or `make run` to build and launch in one step.

Next: [launch your first VM](first-vm.md).
