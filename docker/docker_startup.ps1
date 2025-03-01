# PowerShell script for automating Docker container lifecycle with backup and restore
# Version: 1.0
# Usage: ./docker_startup.ps1 [action] [options]
# Actions: start, stop, restart, backup

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
}

# Default parameters
$action = if ($args[0]) { $args[0].ToLower() } else { "start" }
$backupDir = "C:\Docker_Backups"
$projectName = "ragflow" # Default project name prefix

# Check if Docker is running
try {
    docker info | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Log "Docker is not running. Starting Docker..." "Yellow"
        # You may need to adjust this command based on how Docker is installed
        Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"
        
        # Wait for Docker to start
        $dockerStarted = $false
        $attempts = 0
        $maxAttempts = 30
        
        while (-not $dockerStarted -and $attempts -lt $maxAttempts) {
            Start-Sleep -Seconds 5
            $attempts++
            Write-Log "Waiting for Docker to start (attempt $attempts/$maxAttempts)..." "Yellow"
            
            try {
                docker info | Out-Null
                if ($LASTEXITCODE -eq 0) {
                    $dockerStarted = $true
                }
            } catch {
                # Continue waiting
            }
        }
        
        if (-not $dockerStarted) {
            Write-Log "Failed to start Docker after $maxAttempts attempts. Please start Docker manually." "Red"
            exit 1
        }
        
        Write-Log "Docker has started successfully." "Green"
    }
} catch {
    Write-Log "Error checking Docker status: $($_.Exception.Message)" "Red"
    exit 1
}

# Display help information
function Show-Help {
    Write-Log "RAGFlow Docker Management Script" "Cyan"
    Write-Log "--------------------------------" "Cyan"
    Write-Log "Usage: ./docker_startup.ps1 [action] [options]" "White"
    Write-Log "" "White"
    Write-Log "Actions:" "White"
    Write-Log "  start    - Start containers with auto-restore (default)" "White"
    Write-Log "  stop     - Stop containers with auto-backup" "White"
    Write-Log "  restart  - Restart containers with backup and restore" "White"
    Write-Log "  backup   - Backup data without stopping containers" "White"
    Write-Log "  help     - Show this help information" "White"
    Write-Log "" "White"
    Write-Log "All containers will be prefixed with: $projectName" "White"
}

# Based on the action parameter, execute the appropriate function
switch ($action) {
    "start" {
        Write-Log "Starting $projectName containers with auto-restore..." "Green"
        
        # First try to restore data if backups exist
        if (Test-Path "$PSScriptRoot\auto_restore.ps1") {
            Write-Log "Attempting to restore data from backups..." "Cyan"
            & "$PSScriptRoot\auto_restore.ps1" $backupDir $projectName
        } else {
            Write-Log "Auto-restore script not found. Continuing with empty volumes." "Yellow"
        }
        
        # Start the containers
        Write-Log "Starting Docker containers with prefix: $projectName..." "Cyan"
        docker compose -p $projectName -f "$PSScriptRoot\docker-compose.yml" up -d
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "$projectName containers started successfully." "Green"
            Write-Log "You can access the application at: http://localhost:3000" "Green"
        } else {
            Write-Log "Failed to start $projectName containers." "Red"
            exit 1
        }
    }
    
    "stop" {
        Write-Log "Stopping $projectName containers with auto-backup..." "Yellow"
        
        # First backup the data
        if (Test-Path "$PSScriptRoot\backup_volumes.ps1") {
            Write-Log "Backing up data before stopping containers..." "Cyan"
            & "$PSScriptRoot\backup_volumes.ps1" $backupDir $projectName
        } else {
            Write-Log "Backup script not found. Continuing without backup." "Yellow"
        }
        
        # Stop the containers
        Write-Log "Stopping Docker containers..." "Cyan"
        docker compose -p $projectName -f "$PSScriptRoot\docker-compose.yml" down
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "$projectName containers stopped successfully." "Green"
        } else {
            Write-Log "Failed to stop $projectName containers." "Red"
            exit 1
        }
    }
    
    "restart" {
        Write-Log "Restarting $projectName containers with backup and restore..." "Yellow"
        
        # First backup the data
        if (Test-Path "$PSScriptRoot\backup_volumes.ps1") {
            Write-Log "Backing up data before restarting containers..." "Cyan"
            & "$PSScriptRoot\backup_volumes.ps1" $backupDir $projectName
        } else {
            Write-Log "Backup script not found. Continuing without backup." "Yellow"
        }
        
        # Stop the containers
        Write-Log "Stopping Docker containers..." "Cyan"
        docker compose -p $projectName -f "$PSScriptRoot\docker-compose.yml" down
        
        # Restore data
        if (Test-Path "$PSScriptRoot\auto_restore.ps1") {
            Write-Log "Restoring data from backups..." "Cyan"
            & "$PSScriptRoot\auto_restore.ps1" $backupDir $projectName
        } else {
            Write-Log "Auto-restore script not found. Continuing with existing volumes." "Yellow"
        }
        
        # Start the containers
        Write-Log "Starting Docker containers..." "Cyan"
        docker compose -p $projectName -f "$PSScriptRoot\docker-compose.yml" up -d
        
        if ($LASTEXITCODE -eq 0) {
            Write-Log "$projectName containers restarted successfully." "Green"
            Write-Log "You can access the application at: http://localhost:3000" "Green"
        } else {
            Write-Log "Failed to restart $projectName containers." "Red"
            exit 1
        }
    }
    
    "backup" {
        Write-Log "Backing up $projectName data without stopping containers..." "Cyan"
        
        if (Test-Path "$PSScriptRoot\backup_volumes.ps1") {
            & "$PSScriptRoot\backup_volumes.ps1" $backupDir $projectName
            
            if ($LASTEXITCODE -eq 0) {
                Write-Log "$projectName data backup completed successfully." "Green"
            } else {
                Write-Log "Failed to backup $projectName data." "Red"
                exit 1
            }
        } else {
            Write-Log "Backup script not found at $PSScriptRoot\backup_volumes.ps1" "Red"
            exit 1
        }
    }
    
    "help" {
        Show-Help
    }
    
    default {
        Write-Log "Unknown action: $action" "Red"
        Show-Help
        exit 1
    }
}

exit 0 