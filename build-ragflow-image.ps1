<#
.СИНОПСИС
    Собирает локальный Docker-образ RAGFlow из текущего рабочего дерева.

.ОПИСАНИЕ
    Идёт по официальному пути сборки (корневой Dockerfile, базовый образ
    берётся с Docker Hub). Использует постоянные cache-монтирования BuildKit
    (uv/npm/apt), поэтому повторные сборки после изменений в исходниках быстрые.
    Скрипт должен лежать в корне репозитория рядом с Dockerfile.

    Скрипт только собирает образ и никогда не пересоздаёт запущенный
    контейнер приложения. Чтобы использовать свежесобранный образ, укажите
    новый тег в RAGFLOW_IMAGE в docker/.env и запустите приложение сами.

.ПАРАМЕТР Tag
    Имя образа и тег. По умолчанию: citysense/ragflow:<дата-время>
    (например, citysense/ragflow:20260814-153000)

.ПАРАМЕТР NoCache
    Полная пересборка без учёта кэша BuildKit.

.ПАРАМЕТР UseMirror
    Передаёт NEED_MIRROR=1, чтобы сборка использовала зеркала Aliyun
    (для ограниченных сетей).

.ПРИМЕР
    .\build-ragflow-image.ps1

.ПРИМЕР
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
        throw "$what завершилось с ошибкой (код выхода $LASTEXITCODE)"
    }
}

$RepoRoot = $PSScriptRoot

# ---------------------------------------------------------------------------
# 1. Предварительные проверки
# ---------------------------------------------------------------------------
Write-Step "Предварительные проверки"

docker info *> $null
Assert-ExitOk "docker info (запущен ли Docker Desktop?)"

if (-not (Test-Path (Join-Path $RepoRoot "Dockerfile"))) {
    throw "Dockerfile не найден в '$RepoRoot'. Положите скрипт в корень репозитория ragflow."
}

Push-Location $RepoRoot
try {
    $gitVersion = git describe --tags --match=v* --first-parent --always 2>$null
    if ($gitVersion) {
        Write-Host "  Версия, которая попадёт в образ: $gitVersion" -ForegroundColor Yellow
    }
} catch {
    Write-Host "  (git describe недоступен; VERSION образа будет коротким хешем)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# 2. Сборка
# ---------------------------------------------------------------------------
Write-Step "Сборка образа: $Tag"

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
Write-Host "  Сборка завершена за $([math]::Round($sw.Elapsed.TotalMinutes, 1)) мин" -ForegroundColor Green
docker images $Tag --format "  {{.Repository}}:{{.Tag}}   ID={{.ID}}   Size={{.Size}}"

Write-Host ""
Write-Host "  Чтобы запустить приложение с этим образом, укажите RAGFLOW_IMAGE=$Tag в docker\.env и выполните:" -ForegroundColor Yellow
Write-Host "    docker compose -f docker\docker-compose.yml up -d ragflow-cpu" -ForegroundColor Yellow
