#!/bin/sh
set -e

REPO="vindhyadatascience/vmup"
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

TMPDIR=$(mktemp -d)
ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"

# Public repo — no authentication required. Use curl or wget.
if command -v curl >/dev/null 2>&1; then
    fetch()    { curl -fsSL "$1"; }
    download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch()    { wget -qO- "$1"; }
    download() { wget -qO "$2" "$1"; }
else
    error "curl or wget is required but neither was found."
fi

info "Fetching latest release..."
TAG=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    error "Could not determine the latest release of ${REPO}."
fi
info "Latest release: ${TAG}"

info "Downloading ${ARCHIVE}..."
download "https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}" "${TMPDIR}/${ARCHIVE}"

# Verify the download against the checksums published with the release. The
# checksum file embeds the version without its leading "v" (GoReleaser strips
# it), e.g. vmup_1.9.0_checksums.txt for tag v1.9.0.
CHECKSUMS="${BINARY}_${TAG#v}_checksums.txt"
info "Verifying checksum..."
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
else
    ACTUAL=""
    warn "Neither sha256sum nor shasum found; skipping checksum verification."
fi

if [ -n "$ACTUAL" ]; then
    EXPECTED=$(fetch "https://github.com/${REPO}/releases/download/${TAG}/${CHECKSUMS}" \
        | grep " ${ARCHIVE}$" | awk '{print $1}')
    if [ -z "$EXPECTED" ]; then
        error "Could not find a checksum for ${ARCHIVE} in ${CHECKSUMS}."
    fi
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        error "Checksum mismatch for ${ARCHIVE}.
  expected: ${EXPECTED}
  actual:   ${ACTUAL}
Refusing to install. Please report this at https://github.com/${REPO}/issues"
    fi
fi

# Extract
info "Extracting..."
tar xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

if [ ! -f "${TMPDIR}/${BINARY}" ]; then
    error "Binary '${BINARY}' not found in archive."
fi
chmod +x "${TMPDIR}/${BINARY}"

# Install
#
# A user-owned directory is preferred so that `vmup update` can replace the
# binary in place: replacing an executable requires write permission on its
# DIRECTORY, and /usr/local/bin is root-owned on macOS. Set VMUP_INSTALL_DIR to
# override (e.g. VMUP_INSTALL_DIR="$CONDA_PREFIX/bin" to scope the install to an
# active conda environment).
# Install by writing a new file alongside the target and renaming over it.
# A plain `cp` truncates the destination in place, keeping the same inode: if
# the running vmup is that inode, macOS kills the process (its code signature
# covers page hashes that no longer match) and the binary stays unrunnable.
# rename(2) swaps in a new inode, so a running vmup is untouched.
install_to() {
    dest="$1"
    mkdir -p "$dest" 2>/dev/null || return 1
    [ -w "$dest" ] || return 1
    tmp="${dest}/.${BINARY}.new.$$"
    if ! cp "${TMPDIR}/${BINARY}" "$tmp"; then
        rm -f "$tmp"
        return 1
    fi
    chmod 755 "$tmp" 2>/dev/null || true
    if ! mv -f "$tmp" "${dest}/${BINARY}"; then
        rm -f "$tmp"
        return 1
    fi
    return 0
}

install_to_privileged() {
    dest="$1"
    command -v sudo >/dev/null 2>&1 || return 1
    info "Writing to ${dest} requires elevated permissions..."
    tmp="${dest}/.${BINARY}.new.$$"
    if ! sudo cp "${TMPDIR}/${BINARY}" "$tmp"; then
        sudo rm -f "$tmp"
        return 1
    fi
    sudo chmod 755 "$tmp" 2>/dev/null || true
    if ! sudo mv -f "$tmp" "${dest}/${BINARY}"; then
        sudo rm -f "$tmp"
        return 1
    fi
    return 0
}

note_path() {
    case ":$PATH:" in
        *":$1:"*) ;;
        *)
            warn "$1 is not in your PATH."
            echo "  Add it by appending this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
            echo ""
            echo "    export PATH=\"$1:\$PATH\""
            echo ""
            ;;
    esac
}

INSTALL_DIR=""
if [ -n "${VMUP_INSTALL_DIR:-}" ]; then
    # An explicit target is honoured as given; no fallback, so a typo surfaces
    # as an error instead of silently installing somewhere else.
    if install_to "$VMUP_INSTALL_DIR"; then
        INSTALL_DIR="$VMUP_INSTALL_DIR"
        note_path "$INSTALL_DIR"
    else
        error "Could not install to VMUP_INSTALL_DIR=${VMUP_INSTALL_DIR} (not writable or could not be created)."
    fi
elif install_to "$HOME/.local/bin"; then
    INSTALL_DIR="$HOME/.local/bin"
    note_path "$INSTALL_DIR"
elif install_to "/usr/local/bin" || install_to_privileged "/usr/local/bin"; then
    INSTALL_DIR="/usr/local/bin"
    warn "Installed to /usr/local/bin, which is not user-writable."
    echo "  'vmup update' will not be able to replace this binary; re-run this"
    echo "  installer to upgrade, or reinstall to a user-owned directory."
    echo ""
else
    error "Could not find a writable install directory. Set VMUP_INSTALL_DIR to choose one."
fi

# Verify
if [ -x "${INSTALL_DIR}/${BINARY}" ]; then
    info "Successfully installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
else
    error "Installation failed."
fi

echo ""
if ! command -v gcloud >/dev/null 2>&1; then
    echo "  Prerequisites: Google Cloud SDK (gcloud CLI) must be installed."
    echo "  Install it from: https://cloud.google.com/sdk/docs/install"
    echo ""
fi
echo "  Run '${BINARY}' to get started."
