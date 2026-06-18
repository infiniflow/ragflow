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

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\RAGFlow",
    [string]$GitHubRepo = "infiniflow/ragflow",
    [string]$CliName = "ragflow_cli"
)

$ErrorActionPreference = "Stop"

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

function Get-ReleaseVersion {
    param(
        [string]$RequestedVersion,
        [string]$Repository
    )

    if ($RequestedVersion -ne "latest") {
        Write-Info "Using specified version: $RequestedVersion"
        return $RequestedVersion
    }

    Write-Info "Fetching latest release information"

    $releaseUrl = "https://api.github.com/repos/${Repository}/releases/latest"
    $release = Invoke-RestMethod -Uri $releaseUrl -TimeoutSec 20
    $latestVersion = $release.tag_name

    if ([string]::IsNullOrWhiteSpace($latestVersion)) {
        Write-ErrorMessage "Could not determine latest version"
        exit 1
    }

    Write-Info "Latest version: $latestVersion"
    return $latestVersion
}

function Get-DownloadInfo {
    param(
        [string]$ResolvedVersion,
        [string]$OS,
        [string]$Arch,
        [string]$Repository,
        [string]$BinaryName
    )

    $fileName = "${BinaryName}-${ResolvedVersion}-${OS}-${Arch}.exe"
    $baseUrl = "https://github.com/${Repository}/releases/download/${ResolvedVersion}"

    return @{
        FileName = $fileName
        BinaryUrl = "${baseUrl}/${fileName}"
        ChecksumUrl = "${baseUrl}/SHA256SUMS"
    }
}

function Download-File {
    param(
        [string]$Url,
        [string]$OutputPath
    )

    Write-Info "Downloading $Url"
    $ProgressPreference = "SilentlyContinue"
    Invoke-WebRequest -Uri $Url -OutFile $OutputPath -TimeoutSec 120
}

function Test-Checksum {
    param(
        [string]$BinaryPath,
        [string]$ChecksumPath,
        [string]$FileName
    )

    $checksumLine = Get-Content $ChecksumPath | Where-Object {
        $parts = $_ -split "\s+"
        $parts.Count -ge 2 -and $parts[1] -eq $FileName
    } | Select-Object -First 1

    if ([string]::IsNullOrWhiteSpace($checksumLine)) {
        Write-ErrorMessage "No checksum found for $FileName in SHA256SUMS"
        exit 1
    }

    $expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $BinaryPath).Hash.ToLowerInvariant()

    if ($actual -ne $expected) {
        Write-ErrorMessage "Checksum verification failed for $FileName"
        Write-ErrorMessage "Expected: $expected"
        Write-ErrorMessage "Actual:   $actual"
        exit 1
    }

    Write-Info "Checksum verified"
}

function Install-CLI {
    param(
        [string]$TempFile,
        [string]$TargetDir,
        [string]$BinaryName
    )

    if (-not (Test-Path $TargetDir)) {
        Write-Info "Creating directory: $TargetDir"
        New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
    }

    $targetFile = Join-Path $TargetDir "${BinaryName}.exe"
    Write-Info "Installing CLI to $targetFile"

    Stop-Process -Name $BinaryName -Force -ErrorAction SilentlyContinue
    Copy-Item -Path $TempFile -Destination $targetFile -Force

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathParts = @()
    if (-not [string]::IsNullOrWhiteSpace($userPath)) {
        $pathParts = $userPath -split ";"
    }

    if ($pathParts -notcontains $TargetDir) {
        Write-Info "Adding $TargetDir to user PATH"
        $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $TargetDir } else { "$userPath;$TargetDir" }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Warn "Restart your terminal for PATH changes to take effect"
    }

    Write-Info "CLI installed successfully at $targetFile"
    return $targetFile
}

function Test-Installation {
    param([string]$CliPath)

    if (-not (Test-Path $CliPath)) {
        Write-Warn "CLI not found at expected location: $CliPath"
        return
    }

    try {
        & $CliPath --version
        if ($LASTEXITCODE -eq 0) {
            Write-Info "Installation verified successfully"
            return
        }
    }
    catch {
        Write-Warn "Could not execute version check: $_"
    }

    Write-Warn "Could not verify CLI execution, but the binary was installed"
}

function Main {
    $platform = Get-Platform
    $resolvedVersion = Get-ReleaseVersion -RequestedVersion $Version -Repository $GitHubRepo
    $downloadInfo = Get-DownloadInfo -ResolvedVersion $resolvedVersion -OS $platform.OS -Arch $platform.Arch -Repository $GitHubRepo -BinaryName $CliName

    Write-Info "Download URL: $($downloadInfo.BinaryUrl)"

    $tempBinary = [System.IO.Path]::GetTempFileName()
    $tempSums = [System.IO.Path]::GetTempFileName()

    try {
        Download-File -Url $downloadInfo.BinaryUrl -OutputPath $tempBinary
        Download-File -Url $downloadInfo.ChecksumUrl -OutputPath $tempSums
        Test-Checksum -BinaryPath $tempBinary -ChecksumPath $tempSums -FileName $downloadInfo.FileName

        $cliPath = Install-CLI -TempFile $tempBinary -TargetDir $InstallDir -BinaryName $CliName
        Test-Installation -CliPath $cliPath
        Write-Info "Installation complete"
    }
    finally {
        Remove-Item $tempBinary -Force -ErrorAction SilentlyContinue
        Remove-Item $tempSums -Force -ErrorAction SilentlyContinue
    }
}

Main
