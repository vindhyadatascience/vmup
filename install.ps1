# vmup installer for Windows (PowerShell)
# Usage: irm https://raw.githubusercontent.com/vindhyadatascience/vds-gcp-launch-instance/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "vindhyadatascience/vds-gcp-launch-instance"
$Binary = "vmup"
$InstallDir = Join-Path $env:LOCALAPPDATA "vmup"

function Write-Info($msg)  { Write-Host "==> $msg" -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host "warning: $msg" -ForegroundColor Yellow }
function Write-Err($msg)   { Write-Host "error: $msg" -ForegroundColor Red; exit 1 }

# Detect architecture
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64"  { $arch = "amd64" }
    "x86"    { Write-Err "32-bit Windows is not supported." }
    "ARM64"  { Write-Err "Windows ARM64 builds are not available." }
    default  { Write-Err "Unsupported architecture: $arch" }
}

# Get latest release tag
Write-Info "Fetching latest release..."
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "vmup-installer" }
    $tag = $release.tag_name
} catch {
    Write-Err "Could not determine latest release. Check your internet connection."
}
Write-Info "Latest release: $tag"

# Download archive
$archive = "${Binary}_windows_${arch}.tar.gz"
$url = "https://github.com/$Repo/releases/download/$tag/$archive"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "vmup-install-$([System.Guid]::NewGuid().ToString('N').Substring(0,8))"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

try {
    Write-Info "Downloading $archive..."
    $archivePath = Join-Path $tmpDir $archive
    Invoke-WebRequest -Uri $url -OutFile $archivePath -UseBasicParsing

    # Extract
    Write-Info "Extracting..."
    tar xzf $archivePath -C $tmpDir
    $binaryPath = Join-Path $tmpDir "$Binary.exe"

    if (-not (Test-Path $binaryPath)) {
        Write-Err "Binary '$Binary.exe' not found in archive."
    }

    # Install
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $destPath = Join-Path $InstallDir "$Binary.exe"
    Copy-Item $binaryPath $destPath -Force

    # Add to user PATH if not already present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        Write-Info "Adding $InstallDir to user PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$InstallDir;$userPath", "User")
        $env:Path = "$InstallDir;$env:Path"
    }

    # Verify
    if (Test-Path $destPath) {
        Write-Info "Successfully installed $Binary to $destPath"
    } else {
        Write-Err "Installation failed."
    }

    Write-Host ""
    Write-Host "  Prerequisites: Google Cloud SDK (gcloud CLI) must be installed."
    Write-Host "  Install it from: https://cloud.google.com/sdk/docs/install"
    Write-Host ""
    Write-Host "  Run '$Binary' to get started."
    Write-Host "  Note: You may need to restart your terminal for PATH changes to take effect."
} finally {
    # Cleanup
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir
    }
}
