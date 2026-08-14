<#
.SYNOPSIS
    Builds a local RAGFlow Docker image from the current working tree.

.DESCRIPTION
    Follows the official build path (root Dockerfile, base image pulled from
    Docker Hub). Reuses BuildKit's persistent cache mounts (uv/npm/apt), so
    repeated builds after source changes are fast. Place this script in the
    repository root next to the Dockerfile.

    Only builds the image; it never recreates the running app container. To
    use a freshly built image, set RAGFLOW_IMAGE in docker/.env to the new tag
    and start the app yourself.

.PARAMETER Tag
    Image name and tag. Default: citysense/ragflow:<date-time> (e.g. citysense/ragflow:20260814-153000)

.PARAMETER NoCache
    Force a full rebuild, ignoring BuildKit cache.

.PARAMETER UseMirror
    Pass NEED_MIRROR=1 so the build uses Aliyun mirrors (restricted networks).

.EXAMPLE
    .\build-ragflow-image.ps1

.EXAMPLE
    .\build-ragflow-image.ps1 -Tag myrepo/ragflow:v1 -NoCache
#>

[CmdletBinding()]
param(
    [string]$Tag = "citysense/ragflow:$(Get-Date -Format 'yyyyMMdd-HHmmss')",
    [switch]$NoCache,
    [switch]$UseMirror
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

Write-Host ""
Write-Host "  To run the app with this image, set RAGFLOW_IMAGE=$Tag in docker\.env and run:" -ForegroundColor Yellow
Write-Host "    docker compose -f docker\docker-compose.yml up -d ragflow-cpu" -ForegroundColor Yellow
