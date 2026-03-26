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

# Detect download tool
if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1"; }
    download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO- "$1"; }
    download() { wget -qO "$2" "$1"; }
else
    error "curl or wget is required but neither was found."
fi

# Get latest release tag
info "Fetching latest release..."
TAG=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    error "Could not determine latest release. Check your internet connection."
fi
info "Latest release: ${TAG}"

# Download archive
ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

TMPDIR=$(mktemp -d)
info "Downloading ${ARCHIVE}..."
download "$URL" "${TMPDIR}/${ARCHIVE}"

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
