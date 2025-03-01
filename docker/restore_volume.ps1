# PowerShell script for restoring Docker volumes
# Usage: ./restore_volume.ps1 <backup_file> <volume_name>

param (
    [Parameter(Mandatory=$true)]
    [string]$backupFile,
    
    [Parameter(Mandatory=$true)]
    [string]$volumeName
)

# Check if backup file exists
if (-not (Test-Path $backupFile)) {
    Write-Host "Backup file not found: $backupFile" -ForegroundColor Red
    exit 1
}

# Check if volume exists
$volumeExists = docker volume ls --format "{{.Name}}" | Select-String -Pattern "^$volumeName$"
if (-not $volumeExists) {
    Write-Host "Volume does not exist: $volumeName. Creating it..." -ForegroundColor Yellow
    docker volume create $volumeName
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Failed to create volume: $volumeName" -ForegroundColor Red
        exit 1
    }
}

# Restore the volume
Write-Host "Restoring $backupFile to volume $volumeName..." -ForegroundColor Cyan

# Determine file type and use appropriate extraction method
if ($backupFile -like "*.tar.gz" -or $backupFile -like "*.tgz") {
    # For tar.gz files
    docker run --rm -v ${volumeName}:/destination -v ${backupFile}:/backup.tar.gz alpine sh -c "rm -rf /destination/* && tar -xzf /backup.tar.gz -C /destination"
} elseif ($backupFile -like "*.zip") {
    # For zip files - need to use a container with unzip
    docker run --rm -v ${volumeName}:/destination -v ${backupFile}:/backup.zip alpine sh -c "apk add --no-cache unzip && rm -rf /destination/* && unzip -o /backup.zip -d /destination"
} else {
    Write-Host "Unsupported backup file format. Please use .tar.gz or .zip files." -ForegroundColor Red
    exit 1
}

if ($LASTEXITCODE -eq 0) {
    Write-Host "Successfully restored $volumeName from $backupFile" -ForegroundColor Green
} else {
    Write-Host "Failed to restore $volumeName" -ForegroundColor Red
    exit 1
}

Write-Host "Restore completed. You may need to restart your containers to use the restored data." -ForegroundColor Green 