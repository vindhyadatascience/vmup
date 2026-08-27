# Installation

vmup ships as a single static binary for macOS, Linux, and Windows. The install scripts
download the latest release, verify your platform, and put the binary on your `PATH`.

=== "macOS / Linux"

    ```bash
    curl -fsSL https://raw.githubusercontent.com/vindhyadatascience/vmup/main/install.sh | sh
    ```

    The script detects your OS and architecture (Intel or Apple Silicon / arm64) and
    installs to `/usr/local/bin` when writable, falling back to `~/.local/bin`. If it
    falls back, it warns when `~/.local/bin` is not on your `PATH`.

    !!! note "macOS Gatekeeper"
        As of v1.9.1, release binaries are signed with a Developer ID certificate and
        notarized by Apple, so they run without a Gatekeeper prompt no matter how you
        install them. A notarization ticket cannot be stapled to a bare binary, so
        Gatekeeper verifies it online the first time a browser-downloaded copy runs —
        on a machine with no network that check can still fail until you reconnect.

        On **v1.9.0 and earlier** the binaries are unsigned. Installing via the script
        is unaffected (`curl` does not set the quarantine flag), but a release archive
        downloaded in a browser is flagged as from an unidentified developer; clear it
        with `xattr -d com.apple.quarantine ./vmup`.

=== "Windows (PowerShell)"

    ```powershell
    irm https://raw.githubusercontent.com/vindhyadatascience/vmup/main/install.ps1 | iex
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
[GitHub release](https://github.com/vindhyadatascience/vmup/releases)
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
git clone https://github.com/vindhyadatascience/vmup.git
cd vmup
make build
./vmup
```

Or `make run` to build and launch in one step.

Next: [launch your first VM](first-vm.md).
