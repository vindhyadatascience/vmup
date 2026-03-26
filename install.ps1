# vmup installer for Windows (PowerShell)

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

# Detect auth method
$useGh = $false
$useToken = $false

if (Get-Command gh -ErrorAction SilentlyContinue) {
    $ghAuth = gh auth status 2>&1
    if ($LASTEXITCODE -eq 0) { $useGh = $true }
}
if (-not $useGh -and $env:GITHUB_TOKEN) {
    $useToken = $true
}
if (-not $useGh -and -not $useToken) {
    Write-Err "This is a private repository. Install requires one of:
  1. GitHub CLI (gh) - install from https://cli.github.com then run 'gh auth login'
  2. GITHUB_TOKEN environment variable - set before running this script"
}

$archive = "${Binary}_windows_${arch}.tar.gz"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "vmup-install-$([System.Guid]::NewGuid().ToString('N').Substring(0,8))"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

try {
    if ($useGh) {
        # Use gh CLI (handles private repo auth automatically)
        Write-Info "Fetching latest release via gh CLI..."
        $tag = (gh release view --repo $Repo --json tagName -q '.tagName')
        if (-not $tag) { Write-Err "Could not determine latest release." }
        Write-Info "Latest release: $tag"

        Write-Info "Downloading $archive..."
        gh release download $tag --repo $Repo --pattern $archive --dir $tmpDir
    } else {
        # Use GITHUB_TOKEN
        $headers = @{
            "Authorization" = "Bearer $env:GITHUB_TOKEN"
            "User-Agent"    = "vmup-installer"
        }

        Write-Info "Fetching latest release via GitHub API..."
        try {
            $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
            $tag = $release.tag_name
        } catch {
            Write-Err "Could not determine latest release. Check your GITHUB_TOKEN permissions."
        }
        Write-Info "Latest release: $tag"

        # Find the asset ID for our archive
        $asset = $release.assets | Where-Object { $_.name -eq $archive }
        if (-not $asset) { Write-Err "Asset '$archive' not found in release $tag." }

        Write-Info "Downloading $archive..."
        $archivePath = Join-Path $tmpDir $archive
        $dlHeaders = @{
            "Authorization" = "Bearer $env:GITHUB_TOKEN"
            "Accept"        = "application/octet-stream"
            "User-Agent"    = "vmup-installer"
        }
        Invoke-WebRequest -Uri $asset.url -OutFile $archivePath -Headers $dlHeaders -UseBasicParsing
    }

    # Extract
    Write-Info "Extracting..."
    $archivePath = Join-Path $tmpDir $archive
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
