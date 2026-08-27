# vmup installer for Windows (PowerShell)

$ErrorActionPreference = "Stop"

$Repo = "vindhyadatascience/vmup"
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

$archive = "${Binary}_windows_${arch}.tar.gz"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "vmup-install-$([System.Guid]::NewGuid().ToString('N').Substring(0,8))"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

try {
    # Public repo — no authentication required.
    $headers = @{ "User-Agent" = "vmup-installer" }

    Write-Info "Fetching latest release..."
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
        $tag = $release.tag_name
    } catch {
        Write-Err "Could not determine the latest release of $Repo."
    }
    Write-Info "Latest release: $tag"

    Write-Info "Downloading $archive..."
    $archivePath = Join-Path $tmpDir $archive
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$tag/$archive" -OutFile $archivePath -Headers $headers -UseBasicParsing

    # Verify the download against the checksums published with the release. The
    # checksum file embeds the version without its leading "v" (GoReleaser strips
    # it), e.g. vmup_1.9.0_checksums.txt for tag v1.9.0.
    Write-Info "Verifying checksum..."
    $checksumFile = "${Binary}_$($tag.TrimStart('v'))_checksums.txt"
    try {
        $checksumText = (Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$tag/$checksumFile" -Headers $headers -UseBasicParsing).Content
    } catch {
        Write-Err "Could not download $checksumFile to verify the release."
    }

    $pattern = '\s' + [regex]::Escape($archive) + '\s*$'
    $line = $checksumText -split '\r?\n' | Where-Object { $_ -match $pattern } | Select-Object -First 1
    if (-not $line) {
        Write-Err "Could not find a checksum for $archive in $checksumFile."
    }

    $expected = ($line -split '\s+')[0]
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash
    # -ne on strings is case-insensitive in PowerShell; GoReleaser writes the
    # digest lowercase and Get-FileHash returns it uppercase.
    if ($actual -ne $expected) {
        Write-Err "Checksum mismatch for $archive.`n  expected: $expected`n  actual:   $actual`nRefusing to install. Please report this at https://github.com/$Repo/issues"
    }

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
    if (-not (Get-Command gcloud -ErrorAction SilentlyContinue)) {
        Write-Host "  Prerequisites: Google Cloud SDK (gcloud CLI) must be installed."
        Write-Host "  Install it from: https://cloud.google.com/sdk/docs/install"
        Write-Host ""
    }
    Write-Host "  Run '$Binary' to get started."
    Write-Host "  Note: You may need to restart your terminal for PATH changes to take effect."
} finally {
    # Cleanup
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir
    }
}
