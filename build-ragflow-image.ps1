<#
.SYNOPSIS
    Builds a local RAGFlow Docker image from the current working tree and
    optionally recreates the running app container so the new image applies.

.DESCRIPTION
    Follows the official build path (root Dockerfile, base image pulled from
    Docker Hub). Reuses BuildKit's persistent cache mounts (uv/npm/apt), so
    repeated builds after source changes are fast. Place this script in the
    repository root next to the Dockerfile.

.PARAMETER Tag
    Image name and tag. Default: infiniflow/ragflow:nightly

.PARAMETER NoCache
    Force a full rebuild, ignoring BuildKit cache.

.PARAMETER UseMirror
    Pass NEED_MIRROR=1 so the build uses Aliyun mirrors (restricted networks).

.PARAMETER RestartStack
    After a successful build, recreate the ragflow-cpu app container with the
    new image. Base services (ES/MySQL/Redis/MinIO) are started first.

.PARAMETER SkipBaseCheck
    With -RestartStack, skip starting/verifying the base compose services.

.PARAMETER KeepRunning
    Do NOT stop the running ragflow-cpu container before recreating it.

.EXAMPLE
    .\build-ragflow-image.ps1

.EXAMPLE
    .\build-ragflow-image.ps1 -RestartStack

.EXAMPLE
    .\build-ragflow-image.ps1 -Tag myrepo/ragflow:v1 -NoCache -RestartStack
#>

[CmdletBinding()]
param(
    [string]$Tag = "infiniflow/ragflow:nightly",
    [switch]$NoCache,
    [switch]$UseMirror,
    [switch]$RestartStack,
    [switch]$SkipBaseCheck,
    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"

function Write-Step([string]$msg) {
    Write-Host ""
    Write-Host "======== $msg ========" -ForegroundColor Cyan
}

function Assert-ExitOk([string]$what) {
    if ($LASTEXITCODE -ne 0) {
        throw "$what failed (exit code $LASTEXITCODE)"
    }
}

$RepoRoot = $PSScriptRoot

# ---------------------------------------------------------------------------
# 1. Preflight checks
# ---------------------------------------------------------------------------
Write-Step "Preflight checks"

docker info *> $null
Assert-ExitOk "docker info (is Docker Desktop running?)"

if (-not (Test-Path (Join-Path $RepoRoot "Dockerfile"))) {
    throw "Dockerfile not found in '$RepoRoot'. Place this script in the ragflow repository root."
}

Push-Location $RepoRoot
try {
    $gitVersion = git describe --tags --match=v* --first-parent --always 2>$null
    if ($gitVersion) {
        Write-Host "  Version that will be baked into the image: $gitVersion" -ForegroundColor Yellow
    }
} catch {
    Write-Host "  (git describe unavailable; image VERSION will fall back to a short hash)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# 2. Build
# ---------------------------------------------------------------------------
Write-Step "Building image: $Tag"

$buildArgs = @(
    "build"
    "--progress=plain"
)
if ($NoCache) { $buildArgs += "--no-cache" }
if ($UseMirror) { $buildArgs += "--build-arg", "NEED_MIRROR=1" }
$buildArgs += "-f", "Dockerfile"
$buildArgs += "-t", $Tag
$buildArgs += "."

$sw = [System.Diagnostics.Stopwatch]::StartNew()
docker @buildArgs
Assert-ExitOk "docker build"
$sw.Stop()

Write-Host ""
Write-Host "  Build finished in $([math]::Round($sw.Elapsed.TotalMinutes, 1)) min" -ForegroundColor Green
docker images $Tag --format "  {{.Repository}}:{{.Tag}}   ID={{.ID}}   Size={{.Size}}"

# ---------------------------------------------------------------------------
# 3. Optional: recreate the running app container with the new image
# ---------------------------------------------------------------------------
if ($RestartStack) {
    $ComposeDir = Join-Path $RepoRoot "docker"

    if (-not $SkipBaseCheck) {
        Write-Step "Ensuring base services are up (ES / MySQL / Redis / MinIO)"
        Push-Location $ComposeDir
        try {
            docker compose -f docker-compose-base.yml up -d
            Assert-ExitOk "docker compose base services"
        } finally {
            Pop-Location
        }
    }

    Write-Step "Recreating ragflow-cpu with $Tag"
    if (-not $KeepRunning) {
        Write-Host "  Stopping old app container first..."
        docker stop docker-ragflow-cpu-1 2>$null | Out-Null
        docker rm docker-ragflow-cpu-1 2>$null | Out-Null
    }
    Push-Location $ComposeDir
    try {
        docker compose -f docker-compose.yml up -d --force-recreate ragflow-cpu
        Assert-ExitOk "docker compose app container"
    } finally {
        Pop-Location
    }

    Write-Host ""
    Write-Host "  App container recreated." -ForegroundColor Green
    Write-Host "  Follow startup:      docker logs -f docker-ragflow-cpu-1"
    Write-Host "  Check data sync:     Get-Content docker\ragflow-logs\data_sync_0.log -Tail 50"
}
