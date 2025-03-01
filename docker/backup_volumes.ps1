# PowerShell script for backing up Docker volumes
# Version: 2.0
# Usage: ./backup_volumes.ps1 [backup_dir] [project_name]

# Create a log function
function Write-Log {
    param (
        [Parameter(Mandatory=$true)]
        [string]$Message,
        
        [Parameter(Mandatory=$false)]
        [string]$ForegroundColor = "White"
    )
    
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    Write-Host "[$timestamp] $Message" -ForegroundColor $ForegroundColor
    Add-Content -Path "$backupDir/backup_log.txt" -Value "[$timestamp] $Message"
}

# Default backup directory
$backupDir = if ($args[0]) { $args[0] } else { "C:\Docker_Backups" }
# Default project name (used as container and volume prefix)
$projectName = if ($args[1]) { $args[1] } else { "ragflow" }

# Create backup directory if it doesn't exist
if (-not (Test-Path $backupDir)) {
    New-Item -ItemType Directory -Path $backupDir | Out-Null
    Write-Host "Created backup directory: $backupDir"
}

# Create subdirectories for organization
$rawBackupDir = Join-Path $backupDir "raw_volumes"
$readableDataDir = Join-Path $backupDir "readable_data"
$metadataDir = Join-Path $backupDir "metadata"

if (-not (Test-Path $rawBackupDir)) { New-Item -ItemType Directory -Path $rawBackupDir | Out-Null }
if (-not (Test-Path $readableDataDir)) { New-Item -ItemType Directory -Path $readableDataDir | Out-Null }
if (-not (Test-Path $metadataDir)) { New-Item -ItemType Directory -Path $metadataDir | Out-Null }

# Create readable data subdirectories
$mysqlDataDir = Join-Path $readableDataDir "mysql"
$esDataDir = Join-Path $readableDataDir "elasticsearch"
$minioDataDir = Join-Path $readableDataDir "minio"

if (-not (Test-Path $mysqlDataDir)) { New-Item -ItemType Directory -Path $mysqlDataDir | Out-Null }
if (-not (Test-Path $esDataDir)) { New-Item -ItemType Directory -Path $esDataDir | Out-Null }
if (-not (Test-Path $minioDataDir)) { New-Item -ItemType Directory -Path $minioDataDir | Out-Null }

# Get current date for backup filename
$date = Get-Date -Format "yyyyMMdd_HHmmss"

# Start backup process
Write-Log "Starting $projectName backup process" "Green"
Write-Log "Backup location: $backupDir"
Write-Log "Using project name: $projectName" "Cyan"

# Get a list of active Docker volumes
try {
    # 首先尝试查找带有项目名前缀的卷
    $volumesList = docker volume ls --format "{{.Name}}" | Where-Object { 
        $_ -like "${projectName}_*" 
    }
    
    # 如果没有找到带前缀的卷，则使用更通用的搜索方式
    if (-not $volumesList -or $volumesList.Count -eq 0) {
        $volumesList = docker volume ls --format "{{.Name}}" | Where-Object { 
            $_ -like "*mysql*data" -or 
            $_ -like "*esdata*" -or 
            $_ -like "*elastic*data" -or
            $_ -like "*minio*data" -or 
            $_ -like "*redis*data" -or 
            $_ -like "*ragflow*data" -or 
            $_ -like "*infinity*data" 
        }
    }
    
    if ($volumesList) {
        Write-Log "Found the following Docker volumes to backup:" "Cyan"
        foreach ($vol in $volumesList) {
            Write-Log "  - $vol" "Cyan"
        }
    } else {
        Write-Log "No matching Docker volumes found with prefix '$projectName'. This could be because the containers are not running or the volumes have different naming conventions." "Yellow"
    }
    
    # 根据项目名称和docker-compose.yml构建可能的卷名称
    # 为不同的项目命名方式提供备选
    $fallbackVolumes = @(
        # 使用指定的项目名前缀
        "${projectName}_mysql_data",
        "${projectName}_esdata01",
        "${projectName}_elasticsearch_data",
        "${projectName}_minio_data", 
        "${projectName}_redis_data",
        "${projectName}_ragflow_data",
        
        # 标准命名方式: 项目名_服务名_data
        "ragflow_mysql_data",
        "ragflow_esdata01",
        "ragflow_elasticsearch_data",
        "ragflow_minio_data", 
        "ragflow_redis_data",
        "ragflow_ragflow_data",
        
        # 直接使用服务名称的方式
        "mysql_data",
        "esdata01",
        "elasticsearch_data",
        "minio_data", 
        "redis_data",
        "ragflow_data"
    )
    
    # Use detected volumes if available, otherwise try fallbacks
    $volumes = if ($volumesList) { $volumesList } else { $fallbackVolumes }
} catch {
    Write-Log "Error accessing Docker volumes: $_" "Red"
    # 根据项目名称和docker-compose.yml构建可能的卷名称
    $fallbackVolumes = @(
        # 标准命名方式: 项目名_服务名_data
        "${projectName}_mysql_data",
        "${projectName}_esdata01",
        "${projectName}_elasticsearch_data",
        "${projectName}_minio_data", 
        "${projectName}_redis_data",
        "${projectName}_ragflow_data",
        
        # 直接使用服务名称的方式
        "mysql_data",
        "esdata01",
        "elasticsearch_data",
        "minio_data", 
        "redis_data",
        "ragflow_data",
        
        # 使用ragflow作为前缀的命名方式
        "ragflow_mysql_data",
        "ragflow_esdata01",
        "ragflow_elasticsearch_data",
        "ragflow_minio_data", 
        "ragflow_redis_data",
        "ragflow_ragflow_data",
        
        # 从docker-compose.yml中已知的卷名
        "ragflow_data"
    )
    $volumes = $fallbackVolumes
    Write-Log "Falling back to predefined volume list" "Yellow"
}

# Save volume metadata for restoration reference
$volumeMetadata = [PSCustomObject]@{
    BackupDate = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
    Volumes = $volumes
    ProjectName = $projectName
}
$volumeMetadata | ConvertTo-Json | Out-File -FilePath "$metadataDir/volumes_${date}.json"

# Backup each volume
foreach ($volume in $volumes) {
    Write-Log "Backing up volume: $volume" "Cyan"
    
    # Check if volume exists
    $volumeExists = docker volume ls --format "{{.Name}}" | Where-Object { $_ -eq $volume }
    
    if (-not $volumeExists) {
        Write-Log "Volume $volume does not exist, skipping..." "Yellow"
        continue
    }
    
    # Create a temporary container to access the volume
    $outputFile = "$rawBackupDir/${volume}_${date}.tar.gz"
    docker run --rm -v "${volume}:/source" -v "${rawBackupDir}:/backup" alpine sh -c "cd /source && tar -czf /backup/$(Split-Path $outputFile -Leaf) ."
    
    if ($LASTEXITCODE -eq 0) {
        Write-Log "Successfully backed up $volume to $outputFile" "Green"
    } else {
        Write-Log "Failed to backup $volume" "Red"
    }
}

# Backup readable data: MySQL dumps
try {
    Write-Log "Exporting MySQL database to readable format..." "Cyan"
    
    # 尝试多种可能的MySQL容器名称
    $mysqlContainers = @(
        "ragflow-mysql",
        "${projectName}_mysql_1",
        "${projectName}-mysql-1",
        "mysql"
    )
    
    $mysqlContainerRunning = $null
    foreach ($container in $mysqlContainers) {
        $containerExists = docker ps --format "{{.Names}}" | Where-Object { $_ -eq $container }
        if ($containerExists) {
            $mysqlContainerRunning = $container
            break
        }
    }
    
    # 如果没有找到精确匹配，尝试模糊匹配
    if (-not $mysqlContainerRunning) {
        $mysqlContainerRunning = docker ps --format "{{.Names}}" | Where-Object { $_ -like "*mysql*" } | Select-Object -First 1
    }
    
    if ($mysqlContainerRunning) {
        Write-Log "Found MySQL container: $mysqlContainerRunning" "Cyan"
        $mysqlDumpFile = "$mysqlDataDir/mysql_dump_${date}.sql"
        
        # 从环境文件获取MySQL密码
        $password = "infini_rag_flow"  # 默认密码
        $envFile = "./.env"
        if (Test-Path $envFile) {
            $envContent = Get-Content $envFile
            $mysqlPasswordLine = $envContent | Where-Object { $_ -like "MYSQL_PASSWORD=*" }
            if ($mysqlPasswordLine) {
                $password = $mysqlPasswordLine.Split('=', 2)[1].Trim()
            }
        }
        
        docker exec $mysqlContainerRunning mysqldump -uroot -p"$password" rag_flow > $mysqlDumpFile
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "MySQL data exported to $mysqlDumpFile" "Green"
        } else {
            Write-Log "Failed to export MySQL data" "Red"
        }
    } else {
        Write-Log "MySQL container not running, skipping readable export" "Yellow"
    }
} catch {
    Write-Log "Error exporting MySQL data: $_" "Red"
}

# Backup bind mount directories
$bindMounts = @(
    @{
        "source" = "./data/ragflow-logs"
        "name" = "ragflow_logs"
    },
    @{
        "source" = "./data/conf"
        "name" = "ragflow_conf"
    }
)

# Get the current directory (should be the docker directory)
$currentDir = Get-Location

foreach ($mount in $bindMounts) {
    $sourcePath = Join-Path $currentDir $mount.source
    $name = $mount.name
    
    if (Test-Path $sourcePath) {
        Write-Log "Backing up bind mount: $name from $sourcePath" "Cyan"
        $outputFile = "$rawBackupDir/${name}_${date}.zip"
        Compress-Archive -Path "$sourcePath\*" -DestinationPath $outputFile -Force
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "Successfully backed up $name to $outputFile" "Green"
        } else {
            Write-Log "Failed to backup $name" "Red"
        }
    } else {
        Write-Log "Bind mount source not found: $sourcePath" "Yellow"
    }
}

# Save backup metadata file (useful for auto-restore)
$backupMetadata = [PSCustomObject]@{
    BackupDate = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss")
    BackupDir = $backupDir
    ProjectName = $projectName
    Volumes = $volumes
    BindMounts = $bindMounts
    RawBackupFiles = (Get-ChildItem -Path $rawBackupDir -Filter "*${date}*" | Select-Object -ExpandProperty FullName)
    ReadableBackupFiles = (Get-ChildItem -Path $readableDataDir -Recurse -Filter "*${date}*" | Select-Object -ExpandProperty FullName)
}

$backupMetadata | ConvertTo-Json | Out-File -FilePath "$metadataDir/backup_metadata_${date}.json"
Copy-Item "$metadataDir/backup_metadata_${date}.json" "$metadataDir/latest_backup.json" -Force

Write-Log "Backup completed. All backups stored in: $backupDir" "Green"
Write-Log "-------------------------------------------" "Green"
Write-Host "Consider setting up a scheduled task to run this script regularly." 