<#
.СИНОПСИС
    Мониторинг деградации контейнера RAGFlow (Windows / Docker Desktop).
    Каждый запуск дописывает строку в лог: время, число процессов/потоков,
    результат резолва DNS (es01/mysql/minio), соединения и память.

.ОПИСАНИЕ
    Запускайте через Планировщик задач каждые 5-10 минут. Скрипт пишет в лог
    одну строку на запуск, что позволяет увидеть момент начала деградации
    (рост pids/threads и появление FAIL в резолве DNS).

.ПАРАМЕТР Container
    Имя контейнера (по умолчанию docker-ragflow-cpu-1).

.ПАРАМЕТР LogPath
    Путь к лог-файлу (по умолчанию $env:TEMP\ragflow-monitor.log).
#>

[CmdletBinding()]
param(
    [string]$Container = "docker-ragflow-cpu-1",
    [string]$LogPath = (Join-Path $env:TEMP "ragflow-monitor.log")
)

$ErrorActionPreference = "Stop"

function Get-DockerStat([string]$name, [string]$format) {
    $out = docker stats --no-stream --format $format $name 2>$null
    return $out
}

$ts = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

# Контейнер может быть временно недоступен (перезапуск).
$running = docker inspect $Container 2>$null
if ($LASTEXITCODE -ne 0 -or -not $running) {
    Add-Content -Path $LogPath -Value "$ts container $Container not running"
    exit 0
}

$pids = (docker exec $Container sh -c "ps -e | wc -l" 2>$null).Trim()
$threads = (docker exec $Container sh -c "ps -eL | wc -l" 2>$null).Trim()
$python = (docker exec $Container sh -c "ps -e | grep -c python3" 2>$null).Trim()
$stat = Get-DockerStat $Container "{{.MemUsage}} {{.PIDs}}"

$dns = ""
foreach ($h in @("es01", "mysql", "minio", "redis")) {
    docker exec $Container getent hosts $h *> $null
    if ($LASTEXITCODE -eq 0) { $dns += " $h=ok" } else { $dns += " $h=FAIL" }
}

$mysqlConn = (docker exec $Container sh -c "ss -tn 2>/dev/null | grep -c ':3306'" 2>$null).Trim()
$esConn = (docker exec $Container sh -c "ss -tn 2>/dev/null | grep -c ':9200'" 2>$null).Trim()

Add-Content -Path $LogPath -Value "$ts pids=$pids threads=$threads python=$python mysql_conn=$mysqlConn es_conn=$esConn mem=$stat dns:$dns"