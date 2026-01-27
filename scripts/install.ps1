# Codesfer Installation Script for Windows
# https://github.com/GNITOAHC/codesfer
#
# Usage:
#   powershell -ExecutionPolicy ByPass -c "irm https://raw.githubusercontent.com/GNITOAHC/codesfer/main/scripts/install.ps1 | iex"
#
# Environment Variables:
#   CODESFER_INSTALL_DIR  - Installation directory (default: $env:USERPROFILE\.local\bin)
#   CODESFER_VERSION      - Version to install (default: latest)
#   CODESFER_BINARY       - Binary to install: codesfer, codeserver, all (default: codesfer)

$ErrorActionPreference = "Stop"

# Configuration
$GitHubRepo = "GNITOAHC/codesfer"
$DefaultInstallDir = Join-Path $env:USERPROFILE ".local\bin"
$DefaultBinary = "codesfer"

# Print functions
function Write-Info {
    param([string]$Message)
    Write-Host $Message -ForegroundColor Blue
}

function Write-Success {
    param([string]$Message)
    Write-Host $Message -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host $Message -ForegroundColor Yellow
}

function Write-Error-And-Exit {
    param([string]$Message)
    Write-Host "error: $Message" -ForegroundColor Red
    exit 1
}

# Detect architecture
function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "x86" { Write-Error-And-Exit "32-bit Windows is not supported" }
        "ARM64" { Write-Error-And-Exit "Windows ARM64 is not supported" }
        default { Write-Error-And-Exit "Unsupported architecture: $arch" }
    }
}

# Fetch the latest release version from GitHub API
function Get-LatestVersion {
    $apiUrl = "https://api.github.com/repos/$GitHubRepo/releases/latest"

    try {
        $response = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
        return $response.tag_name
    }
    catch {
        Write-Error-And-Exit "Failed to fetch latest version from GitHub: $_"
    }
}

# Install a single binary
function Install-Binary {
    param(
        [string]$BinaryName,
        [string]$Version,
        [string]$Arch,
        [string]$InstallDir
    )

    # Construct download URL
    # Format: {binary}-{version}-windows-{arch}.zip
    $archiveName = "$BinaryName-$Version-windows-$Arch.zip"
    $downloadUrl = "https://github.com/$GitHubRepo/releases/download/$Version/$archiveName"

    Write-Info "Downloading $BinaryName $Version for windows-$Arch..."

    # Create temp directory
    $tmpDir = Join-Path $env:TEMP "codesfer-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download archive
        $archivePath = Join-Path $tmpDir $archiveName
        try {
            Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
        }
        catch {
            Write-Error-And-Exit "Failed to download $BinaryName from $downloadUrl"
        }

        # Extract archive
        $extractDir = Join-Path $tmpDir "extracted"
        try {
            Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force
        }
        catch {
            Write-Error-And-Exit "Failed to extract archive"
        }

        # Find the binary in extracted files
        $binaryFileName = "$BinaryName.exe"
        $binaryPath = Join-Path $extractDir $binaryFileName

        if (-not (Test-Path $binaryPath)) {
            # Try to find it in subdirectories
            $foundBinary = Get-ChildItem -Path $extractDir -Recurse -Filter $binaryFileName | Select-Object -First 1
            if ($foundBinary) {
                $binaryPath = $foundBinary.FullName
            }
            else {
                Write-Error-And-Exit "Binary $binaryFileName not found in archive"
            }
        }

        # Create install directory if it doesn't exist
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Move binary to install directory
        $destPath = Join-Path $InstallDir $binaryFileName
        Copy-Item -Path $binaryPath -Destination $destPath -Force

        Write-Success "Installed $BinaryName to $destPath"
    }
    finally {
        # Clean up temp directory
        if (Test-Path $tmpDir) {
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# Main installation function
function Main {
    # Get configuration from environment or use defaults
    $installDir = if ($env:CODESFER_INSTALL_DIR) { $env:CODESFER_INSTALL_DIR } else { $DefaultInstallDir }
    $version = if ($env:CODESFER_VERSION) { $env:CODESFER_VERSION } else { "latest" }
    $binary = if ($env:CODESFER_BINARY) { $env:CODESFER_BINARY } else { $DefaultBinary }

    # Detect platform
    $arch = Get-Architecture

    Write-Info "Detected platform: windows-$arch"

    # Resolve version
    if ($version -eq "latest") {
        Write-Info "Fetching latest version..."
        $version = Get-LatestVersion
    }

    Write-Info "Version: $version"
    Write-Info "Install directory: $installDir"

    # Determine which binaries to install
    $binaries = switch ($binary) {
        "codesfer" { @("codesfer") }
        "codeserver" { @("codeserver") }
        "all" { @("codesfer", "codeserver") }
        default { Write-Error-And-Exit "Invalid binary option: $binary. Use 'codesfer', 'codeserver', or 'all'." }
    }

    # Install each binary
    foreach ($bin in $binaries) {
        Install-Binary -BinaryName $bin -Version $version -Arch $arch -InstallDir $installDir
    }

    Write-Host ""
    Write-Success "Installation complete!"
    Write-Host ""

    # Check if install directory is in PATH
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -split ";" -contains $installDir) {
        Write-Info "$installDir is already in your PATH."
    }
    else {
        Write-Warn "To use codesfer, add $installDir to your PATH:"
        Write-Host ""
        Write-Host "  PowerShell (current session):"
        Write-Host "    `$env:Path += `";$installDir`""
        Write-Host ""
        Write-Host "  PowerShell (permanent for current user):"
        Write-Host "    [Environment]::SetEnvironmentVariable(`"Path`", `$env:Path + `";$installDir`", `"User`")"
        Write-Host ""
        Write-Host "Or add it via: Settings > System > About > Advanced system settings > Environment Variables"
    }
}

# Run main function
Main
