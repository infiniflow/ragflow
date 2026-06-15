#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

# Windows PowerShell installation script for RAGFlow CLI

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:PROGRAMFILES\RAGFlow",
    [string]$GitHubRepo = "infiniflow/ragflow",
    [string]$CliName = "ragflow_cli"
)

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-ErrorMessage {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Get-Platform {
    $os = "windows"
    $arch = "amd64"

    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
        $arch = "arm64"
    }
    elseif ([Environment]::Is64BitOperatingSystem) {
        $arch = "amd64"
    }
    else {
        Write-ErrorMessage "Unsupported Windows architecture: 386"
        exit 1
    }

    Write-Info "Detected platform: ${os}/${arch}"

    return @{
        OS = $os
        Arch = $arch
    }
}

function Get-LatestVersion {
    param(
        [string]$Version,
        [string]$GitHubRepo
    )

    if ($Version -ne "latest") {
        Write-Info "Using specified version: $Version"
        return $Version
    }

    Write-Info "Fetching latest release information..."

    try {
        $releaseUrl = "https://api.github.com/repos/${GitHubRepo}/releases/latest"
        $response = Invoke-WebRequest -Uri $releaseUrl -UseBasicParsing -TimeoutSec 10
        $release = ConvertFrom-Json $response.Content
        $latestVersion = $release.tag_name

        if ([string]::IsNullOrWhiteSpace($latestVersion)) {
            Write-ErrorMessage "Could not determine latest version"
            exit 1
        }

        Write-Info "Latest version: $latestVersion"
        return $latestVersion
    }
    catch {
        Write-ErrorMessage "Failed to fetch release information: $_"
        exit 1
    }
}

function Build-DownloadUrl {
    param(
        [string]$Version,
        [string]$OS,
        [string]$Arch,
        [string]$GitHubRepo,
        [string]$CliName
    )

    $filename = "${CliName}-${Version}-${OS}-${Arch}.exe"
    $url = "https://github.com/${GitHubRepo}/releases/download/${Version}/${filename}"

    Write-Info "Download URL: $url"
    return $url
}

function Download-CLI {
    param([string]$Url)

    $tempFile = [System.IO.Path]::GetTempFileName()

    Write-Info "Downloading CLI from $Url..."

    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $Url -OutFile $tempFile -TimeoutSec 60

        $fileSize = (Get-Item $tempFile).Length
        if ($fileSize -lt 1MB) {
            Write-Warn "Downloaded file seems suspiciously small ($fileSize bytes)"
        }

        return $tempFile
    }
    catch {
        Remove-Item $tempFile -Force -ErrorAction SilentlyContinue
        Write-ErrorMessage "Failed to download CLI binary: $_"
        exit 1
    }
}

function Install-CLI {
    param(
        [string]$TempFile,
        [string]$InstallDir,
        [string]$CliName
    )

    if (-not (Test-Path $InstallDir)) {
        Write-Info "Creating directory: $InstallDir"

        try {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        catch {
            Write-ErrorMessage "Failed to create installation directory: $_"
            exit 1
        }
    }

    $targetFile = Join-Path $InstallDir "${CliName}.exe"

    Write-Info "Installing CLI to $targetFile..."

    try {
        Stop-Process -Name $CliName -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500

        Copy-Item -Path $TempFile -Destination $targetFile -Force
        Write-Info "CLI installed successfully at $targetFile"

        $envPath = [Environment]::GetEnvironmentVariable("Path", "User")

        if ($envPath -notlike "*$InstallDir*") {
            Write-Info "Adding $InstallDir to user PATH..."

            if ([string]::IsNullOrWhiteSpace($envPath)) {
                $newPath = $InstallDir
            }
            else {
                $newPath = "$envPath;$InstallDir"
            }

            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            Write-Info "PATH updated. Please restart your terminal for changes to take effect."
        }
    }
    catch {
        Write-ErrorMessage "Failed to install CLI: $_"
        exit 1
    }
    finally {
        Remove-Item $TempFile -Force -ErrorAction SilentlyContinue
    }
}

function Verify-Installation {
    param(
        [string]$InstallDir,
        [string]$CliName
    )

    $cliPath = Join-Path $InstallDir "${CliName}.exe"

    Write-Info "Verifying installation..."

    if (Test-Path $cliPath) {
        try {
            & $cliPath --version 2>$null

            if ($LASTEXITCODE -eq 0) {
                Write-Info "Installation verified successfully!"
                return
            }

            & $cliPath -h 2>$null

            if ($LASTEXITCODE -eq 0) {
                Write-Info "Installation verified successfully!"
                return
            }

            Write-Warn "Could not verify CLI installation, but it may still work"
        }
        catch {
            Write-Warn "Could not verify CLI installation, but it may still work"
        }
    }
    else {
        Write-Warn "CLI not found at expected location: $cliPath"
    }
}

function Main {
    Write-Host "=========================================="
    Write-Host "RAGFlow CLI Installer (Windows)"
    Write-Host "=========================================="
    Write-Host

    $platform = Get-Platform
    $version = Get-LatestVersion -Version $Version -GitHubRepo $GitHubRepo
    $downloadUrl = Build-DownloadUrl -Version $version -OS $platform.OS -Arch $platform.Arch -GitHubRepo $GitHubRepo -CliName $CliName

    $tempFile = Download-CLI -Url $downloadUrl
    Install-CLI -TempFile $tempFile -InstallDir $InstallDir -CliName $CliName
    Verify-Installation -InstallDir $InstallDir -CliName $CliName

    Write-Host
    Write-Host "==========================================" -ForegroundColor Green
    Write-Host "Installation complete! 🎉" -ForegroundColor Green
    Write-Host "==========================================" -ForegroundColor Green
    Write-Host
    Write-Info "You can now use '${CliName}' command"
    Write-Info "Installation directory: $InstallDir"
}

# Run main function
Main