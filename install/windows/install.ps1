# DevClaw installer — Windows PowerShell
# Usage: iwr -useb https://raw.githubusercontent.com/jholhewres/devclaw/master/install/windows/install.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo = "jholhewres/devclaw"
$Binary = "devclaw"
$InstallDir = "$env:LOCALAPPDATA\devclaw"

function Info($msg) { Write-Host "[info]  $msg" -ForegroundColor Cyan }
function Ok($msg)   { Write-Host "[ok]    $msg" -ForegroundColor Green }
function Err($msg)  { Write-Host "[error] $msg" -ForegroundColor Red; exit 1 }

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { Err "32-bit Windows not supported" }

Info "Detected: windows/$Arch"

# Get latest version
Info "Fetching latest version..."
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
$Version = $Release.tag_name
Info "Latest version: $Version"

# Download
$Archive = "${Binary}_$($Version.TrimStart('v'))_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Archive"
$TmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path "$_.dir" }
$TmpFile = Join-Path $TmpDir $Archive

Info "Downloading $Url..."
Invoke-WebRequest -Uri $Url -OutFile $TmpFile -UseBasicParsing

# Extract
Info "Extracting..."
Expand-Archive -Path $TmpFile -DestinationPath $TmpDir -Force

# Install
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Copy-Item -Path (Join-Path $TmpDir "$Binary.exe") -Destination (Join-Path $InstallDir "$Binary.exe") -Force
Ok "Installed to $InstallDir\$Binary.exe"

# Add to PATH
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Info "Added $InstallDir to user PATH"
    Info "Restart your terminal for PATH changes to take effect"
}

# Cleanup
Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue

Write-Host ""
Ok "DevClaw installed! Run:"
Write-Host ""
Write-Host "  devclaw serve    # start + setup wizard"
Write-Host "  devclaw --help   # see all commands"
Write-Host ""
