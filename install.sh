#!/bin/sh
set -e

REPO="vindhyadatascience/vds-gcp-launch-instance"
BINARY="vmup"

# Colors (if terminal supports them)
if [ -t 1 ]; then
    BOLD='\033[1m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    RED='\033[31m'
    RESET='\033[0m'
else
    BOLD='' GREEN='' YELLOW='' RED='' RESET=''
fi

info()  { printf "${BOLD}${GREEN}==>${RESET} ${BOLD}%s${RESET}\n" "$1"; }
warn()  { printf "${YELLOW}warning:${RESET} %s\n" "$1"; }
error() { printf "${RED}error:${RESET} %s\n" "$1" >&2; exit 1; }

# Cleanup temp directory on exit
TMPDIR=""
cleanup() { [ -n "$TMPDIR" ] && rm -rf "$TMPDIR"; }
trap cleanup EXIT

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin|linux) ;;
    mingw*|msys*|cygwin*) error "Windows detected. Please use install.ps1 with PowerShell instead." ;;
    *) error "Unsupported operating system: $OS" ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64)  ARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

# Detect auth method
USE_GH=false
USE_TOKEN=false

if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    USE_GH=true
elif [ -n "$GITHUB_TOKEN" ]; then
    USE_TOKEN=true
else
    error "This is a private repository. Install requires one of:
  1. GitHub CLI (gh) — install from https://cli.github.com then run 'gh auth login'
  2. GITHUB_TOKEN environment variable — export GITHUB_TOKEN=ghp_... before running this script"
fi

TMPDIR=$(mktemp -d)
ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"

if [ "$USE_GH" = true ]; then
    # Use gh CLI (handles private repo auth automatically)
    info "Fetching latest release via gh CLI..."
    TAG=$(gh release view --repo "$REPO" --json tagName -q '.tagName')
    if [ -z "$TAG" ]; then
        error "Could not determine latest release."
    fi
    info "Latest release: ${TAG}"

    info "Downloading ${ARCHIVE}..."
    gh release download "$TAG" --repo "$REPO" --pattern "$ARCHIVE" --dir "$TMPDIR"
else
    # Use GITHUB_TOKEN with curl/wget
    if command -v curl >/dev/null 2>&1; then
        api_fetch() { curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$1"; }
        api_download() { curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/octet-stream" -o "$2" "$1"; }
    elif command -v wget >/dev/null 2>&1; then
        api_fetch() { wget -qO- --header="Authorization: Bearer $GITHUB_TOKEN" "$1"; }
        api_download() { wget -qO "$2" --header="Authorization: Bearer $GITHUB_TOKEN" --header="Accept: application/octet-stream" "$1"; }
    else
        error "curl or wget is required but neither was found."
    fi

    info "Fetching latest release via GitHub API..."
    RELEASE_JSON=$(api_fetch "https://api.github.com/repos/${REPO}/releases/latest")
    TAG=$(echo "$RELEASE_JSON" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    if [ -z "$TAG" ]; then
        error "Could not determine latest release. Check your GITHUB_TOKEN permissions."
    fi
    info "Latest release: ${TAG}"

    # Find the asset ID for our archive
    ASSET_ID=$(echo "$RELEASE_JSON" | grep -B 3 "\"name\": *\"${ARCHIVE}\"" | grep '"id"' | head -1 | sed -E 's/.*"id": *([0-9]+).*/\1/')
    if [ -z "$ASSET_ID" ]; then
        error "Asset '${ARCHIVE}' not found in release ${TAG}."
    fi

    info "Downloading ${ARCHIVE}..."
    api_download "https://api.github.com/repos/${REPO}/releases/assets/${ASSET_ID}" "${TMPDIR}/${ARCHIVE}"
fi

# Extract
info "Extracting..."
tar xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

if [ ! -f "${TMPDIR}/${BINARY}" ]; then
    error "Binary '${BINARY}' not found in archive."
fi
chmod +x "${TMPDIR}/${BINARY}"

# Install
install_to() {
    dest="$1"
    if [ -w "$dest" ]; then
        cp "${TMPDIR}/${BINARY}" "${dest}/${BINARY}"
        return 0
    elif command -v sudo >/dev/null 2>&1; then
        info "Writing to ${dest} requires elevated permissions..."
        sudo cp "${TMPDIR}/${BINARY}" "${dest}/${BINARY}"
        return 0
    fi
    return 1
}

INSTALL_DIR=""
if install_to "/usr/local/bin"; then
    INSTALL_DIR="/usr/local/bin"
else
    FALLBACK="$HOME/.local/bin"
    mkdir -p "$FALLBACK"
    cp "${TMPDIR}/${BINARY}" "${FALLBACK}/${BINARY}"
    INSTALL_DIR="$FALLBACK"

    case ":$PATH:" in
        *":${FALLBACK}:"*) ;;
        *)
            warn "${FALLBACK} is not in your PATH."
            echo "  Add it by appending this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
            echo ""
            echo "    export PATH=\"${FALLBACK}:\$PATH\""
            echo ""
            ;;
    esac
fi

# Verify
if [ -x "${INSTALL_DIR}/${BINARY}" ]; then
    info "Successfully installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
else
    error "Installation failed."
fi

echo ""
echo "  Prerequisites: Google Cloud SDK (gcloud CLI) must be installed."
echo "  Install it from: https://cloud.google.com/sdk/docs/install"
echo ""
echo "  Run '${BINARY}' to get started."
