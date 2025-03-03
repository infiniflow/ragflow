# PowerShell script for auto-restoring Docker volumes
# Version: 1.0
# Usage: ./auto_restore.ps1 [backup_dir] [project_name]

# Default backup directory
$backupDir = if ($args[0]) { $args[0] } else { "D:\Docker_Backups" }
# Default project name (used as container and volume prefix)
$projectName = if ($args[1]) { $args[1] } else { "ragflow" }

# Create backup directory structure if it doesn't exist
if (-not (Test-Path $backupDir)) {
    try {
        New-Item -ItemType Directory -Path $backupDir -Force | Out-Null
        Write-Host "Created backup directory: $backupDir" -ForegroundColor Cyan
    } catch {
        Write-Host "Failed to create backup directory: $backupDir" -ForegroundColor Red
        Write-Host "No backups available for restoration. Continuing with empty volumes." -ForegroundColor Yellow
        exit 0 # Exit with success so docker compose can continue
    }
}

# Create necessary subdirectories for future backups
$metadataDir = Join-Path $backupDir "metadata"
$rawVolumesDir = Join-Path $backupDir "raw_volumes"
$readableDataDir = Join-Path $backupDir "readable_data"

try {
    if (-not (Test-Path $metadataDir)) {
        New-Item -ItemType Directory -Path $metadataDir -Force | Out-Null
        Write-Host "Created metadata directory: $metadataDir" -ForegroundColor Cyan
    }
    
    if (-not (Test-Path $rawVolumesDir)) {
        New-Item -ItemType Directory -Path $rawVolumesDir -Force | Out-Null
        Write-Host "Created raw volumes directory: $rawVolumesDir" -ForegroundColor Cyan
    }
    
    if (-not (Test-Path $readableDataDir)) {
        New-Item -ItemType Directory -Path $readableDataDir -Force | Out-Null
        Write-Host "Created readable data directory: $readableDataDir" -ForegroundColor Cyan
    }
} catch {
    Write-Host "Warning: Failed to create some backup subdirectories. This won't affect restoration but may affect future backups." -ForegroundColor Yellow
}

# Now set the log file path after ensuring directory exists
$global:logFile = "$backupDir\restore_log.txt"

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
    
    # If log file variable exists, write to it
    if (Test-Path variable:global:logFile) {
        try {
            Add-Content -Path $global:logFile -Value "[$timestamp] $Message"
        } catch {
            # Silently continue if we can't write to the log file
            # This prevents errors from being displayed
        }
    }
}

Write-Log "Starting RAGFlow auto-restore process" "Green"
Write-Log "Looking for backups in: $backupDir" "Cyan"
Write-Log "Using project name: $projectName" "Cyan"

# Look for backup metadata
$latestMetadataPath = Join-Path $metadataDir "latest_backup.json"

if (-not (Test-Path $latestMetadataPath)) {
    # If no latest metadata, look for the most recent one
    $metadataFiles = Get-ChildItem -Path $metadataDir -Filter "backup_metadata_*.json" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending
    
    if (-not $metadataFiles -or $metadataFiles.Count -eq 0) {
        Write-Log "No backup metadata found. This is normal for first run." "Yellow"
        Write-Log "Continuing with empty volumes. Future backups will be stored in $backupDir" "Cyan"
        exit 0 # Exit with success so docker compose can continue
    }
    
    $latestMetadataPath = $metadataFiles[0].FullName
    Write-Log "Using most recent backup metadata: $($metadataFiles[0].Name)" "Cyan"
} else {
    Write-Log "Found latest backup metadata file" "Cyan"
}

# Load backup metadata
try {
    $backupMetadata = Get-Content -Path $latestMetadataPath -Raw | ConvertFrom-Json
    Write-Log "Backup from: $($backupMetadata.BackupDate)" "Cyan"
    if ($backupMetadata.ProjectName) {
        $backupProjectName = $backupMetadata.ProjectName
        Write-Log "Project name from backup: $backupProjectName" "Cyan"
        
        # 如果备份的项目名称与当前不同，提供警告
        if ($backupProjectName -ne $projectName) {
            Write-Log "Warning: Backup project name ($backupProjectName) is different from current project name ($projectName)" "Yellow"
            Write-Log "Volume names may need to be remapped." "Yellow"
        }
    }
} catch {
    Write-Log "Error loading backup metadata." "Red"
    Write-Log "Continuing with empty volumes." "Yellow"
    exit 0
}

# Check if Docker is running
try {
    docker info | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Log "Docker is not running. Please start Docker and try again." "Red"
        exit 1
    }
} catch {
    Write-Log "Error checking Docker status." "Red"
    exit 1
}

# Process volumes for restoration
# 如果备份来自不同的项目名称，需要重新映射卷名
$volumesToRestore = @()
foreach ($volume in $backupMetadata.Volumes) {
    $targetVolume = $volume
    
    # 如果备份项目名称与当前项目名称不同，并且卷名包含项目名称前缀，则替换前缀
    if ($backupMetadata.ProjectName -and $backupMetadata.ProjectName -ne $projectName) {
        if ($volume -like "$($backupMetadata.ProjectName)_*") {
            # 从旧前缀切换到新前缀
            $withoutPrefix = $volume.Substring($backupMetadata.ProjectName.Length + 1)
            $targetVolume = "${projectName}_${withoutPrefix}"
            Write-Log "Remapping volume: $volume -> $targetVolume" "Yellow"
        }
    }
    
    $volumesToRestore += $targetVolume
}

# Restore each volume
foreach ($volume in $volumesToRestore) {
    # Check if volume exists already
    $volumeExists = docker volume ls --format "{{.Name}}" | Where-Object { $_ -eq $volume }
    
    # If volume exists and has data, we'll skip restoration
    if ($volumeExists) {
        Write-Log "Volume $volume already exists" "Yellow"
        
        # Check if volume is empty
        $isEmpty = $false
        try {
            # Create a temporary container to check if volume is empty
            $volumeMount = "$volume" + ":/source"
            $checkResult = docker run --rm -v "$volumeMount" alpine sh -c "[ -z \"\$(ls -A /source)\" ] && echo 'empty' || echo 'not-empty'"
            $isEmpty = $checkResult -eq "empty"
        } catch {
            Write-Log "Error checking if volume is empty." "Red"
            # Assume not empty to be safe
            $isEmpty = $false
        }
        
        if (-not $isEmpty) {
            Write-Log "Volume $volume contains data. Skipping restoration to avoid overwriting." "Yellow"
            continue
        } else {
            Write-Log "Volume $volume exists but is empty. Will restore." "Cyan"
        }
    } else {
        # Create the volume if it doesn't exist
        Write-Log "Creating volume $volume" "Cyan"
        docker volume create $volume
    }
    
    # 查找原始备份名称（可能是重映射前的名称）
    $originalVolumeName = $volume
    if ($backupMetadata.ProjectName -and $backupMetadata.ProjectName -ne $projectName) {
        if ($volume -like "${projectName}_*") {
            $withoutPrefix = $volume.Substring($projectName.Length + 1)
            $originalVolumeName = "$($backupMetadata.ProjectName)_${withoutPrefix}"
        }
    }
    
    # Find the latest backup for this volume
    $volumeBackups = Get-ChildItem -Path $rawVolumesDir -Filter "${originalVolumeName}_*.tar.gz" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending
    
    if (-not $volumeBackups -or $volumeBackups.Count -eq 0) {
        Write-Log "No backup found for volume $originalVolumeName. Skipping." "Yellow"
        continue
    }
    
    $latestBackup = $volumeBackups[0].FullName
    Write-Log "Restoring $volume from $latestBackup" "Cyan"
    
    # Restore the volume
    try {
        $volumeMount = "$volume" + ":/destination" 
        $backupMount = "$latestBackup" + ":/backup.tar.gz"
        docker run --rm -v "$volumeMount" -v "$backupMount" alpine sh -c "rm -rf /destination/* && tar -xzf /backup.tar.gz -C /destination"
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "Successfully restored $volume" "Green"
        } else {
            Write-Log "Failed to restore $volume" "Red"
        }
    } catch {
        Write-Log "Error occurred while restoring volume $volume" "Red"
    }
}

# Process bind mounts for restoration
$bindMountsToRestore = @()
if ($backupMetadata.BindMounts) {
    foreach ($mount in $backupMetadata.BindMounts) {
        # 原始绑定挂载信息保持不变
        $bindMountsToRestore += $mount
    }
}

# Restore bind mounts if needed
foreach ($mount in $bindMountsToRestore) {
    $name = $mount.name
    $sourcePath = Join-Path (Get-Location) $mount.source
    
    # Check if bind mount directory already has content
    if (Test-Path $sourcePath) {
        $directoryHasContent = (Get-ChildItem -Path $sourcePath -Recurse -ErrorAction SilentlyContinue | Measure-Object).Count -gt 0
        
        if ($directoryHasContent) {
            Write-Log "Bind mount directory $sourcePath already has content. Skipping." "Yellow"
            continue
        }
    } else {
        # Create directory if it doesn't exist
        New-Item -ItemType Directory -Path $sourcePath -Force | Out-Null
        Write-Log "Created bind mount directory: $sourcePath" "Cyan"
    }
    
    # Find the latest backup for this bind mount
    $bindMountBackups = Get-ChildItem -Path $rawVolumesDir -Filter "${name}_*.zip" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending
    
    if (-not $bindMountBackups -or $bindMountBackups.Count -eq 0) {
        Write-Log "No backup found for bind mount $name. Skipping." "Yellow"
        continue
    }
    
    $latestBackup = $bindMountBackups[0].FullName
    Write-Log "Restoring bind mount $name from $latestBackup" "Cyan"
    
    # Restore the bind mount
    try {
        Expand-Archive -Path $latestBackup -DestinationPath $sourcePath -Force
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "Successfully restored bind mount $name" "Green"
        } else {
            Write-Log "Failed to restore bind mount $name" "Red"
        }
    } catch {
        Write-Log "Error occurred while restoring bind mount $name" "Red"
    }
}

Write-Log "Restoration process completed. You can now run 'docker compose up -d'" "Green"
Write-Log "------------------------------------------------------------------" "Green"
exit 0 