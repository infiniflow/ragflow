#!/bin/bash

# RAGFlow Data Migration Script
# Usage: ./migration.sh [backup|restore] [backup_folder]
# 
# This script helps you backup and restore RAGFlow Docker volumes
# including MySQL, MinIO, Redis, and Elasticsearch data.

set -e  # Exit on any error
# Instead, we'll handle errors manually for better debugging experience

# Default values
DEFAULT_BACKUP_FOLDER="backup"
VOLUMES=("docker_mysql_data" "docker_minio_data" "docker_redis_data" "docker_esdata01")
BACKUP_FILES=("mysql_backup.tar.gz" "minio_backup.tar.gz" "redis_backup.tar.gz" "es_backup.tar.gz")

# Function to display help information
show_help() {
    echo "RAGFlow Data Migration Tool"
    echo ""
    echo "USAGE:"
    echo "  $0 <operation> [backup_folder]"
    echo ""
    echo "OPERATIONS:"
    echo "  backup   - Create backup of all RAGFlow data volumes"
    echo "  restore  - Restore RAGFlow data volumes from backup"
    echo "  help     - Show this help message"
    echo ""
    echo "PARAMETERS:"
    echo "  backup_folder  - Name of backup folder (default: '$DEFAULT_BACKUP_FOLDER')"
    echo ""
    echo "EXAMPLES:"
    echo "  $0 backup                    # Backup to './backup' folder"
    echo "  $0 backup my_backup          # Backup to './my_backup' folder"
    echo "  $0 restore                   # Restore from './backup' folder"
    echo "  $0 restore my_backup         # Restore from './my_backup' folder"
    echo ""
    echo "DOCKER VOLUMES:"
    echo "  - docker_mysql_data     (MySQL database)"
    echo "  - docker_minio_data     (MinIO object storage)"
    echo "  - docker_redis_data     (Redis cache)"
    echo "  - docker_esdata01       (Elasticsearch indices)"
}

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        echo "❌ Error: Docker is not running or not accessible"
        echo "Please start Docker and try again"
        exit 1
    fi
}

# Function to check if volume exists
volume_exists() {
    local volume_name=$1
    docker volume inspect "$volume_name" >/dev/null 2>&1
}

# Function to check if any containers are using the target volumes
check_containers_using_volumes() {
    echo "🔍 Checking for running containers that might be using target volumes..."
    
    # Get all running containers
    local running_containers=$(docker ps --format "{{.Names}}")
    
    if [ -z "$running_containers" ]; then
        echo "✅ No running containers found"
        return 0
    fi
    
    # Check each running container for volume usage
    local containers_using_volumes=()
    local volume_usage_details=()
    
    for container in $running_containers; do
        # Get container's mount information
        local mounts=$(docker inspect "$container" --format '{{range .Mounts}}{{.Source}}{{"|"}}{{end}}' 2>/dev/null || echo "")
        
        # Check if any of our target volumes are used by this container
        for volume in "${VOLUMES[@]}"; do
            if echo "$mounts" | grep -q "$volume"; then
                containers_using_volumes+=("$container")
                volume_usage_details+=("$container -> $volume")
                break
            fi
        done
    done
    
    # If any containers are using our volumes, show error and exit
    if [ ${#containers_using_volumes[@]} -gt 0 ]; then
        echo ""
        echo "❌ ERROR: Found running containers using target volumes!"
        echo ""
        echo "📋 Running containers status:"
        docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"
        echo ""
        echo "🔗 Volume usage details:"
        for detail in "${volume_usage_details[@]}"; do
            echo "  - $detail"
        done
        echo ""
        echo "🛑 SOLUTION: Stop the containers before performing backup/restore operations:"
        echo "   docker-compose -f docker/<your-docker-compose-file>.yml down"
        echo ""
        echo "💡 After backup/restore, you can restart with:"
        echo "   docker-compose -f docker/<your-docker-compose-file>.yml up -d"
        echo ""
        exit 1
    fi
    
    echo "✅ No containers are using target volumes, safe to proceed"
    return 0
}

# Function to confirm user action
confirm_action() {
    local message=$1
    echo -n "$message (y/N): "
    read -r response
    case "$response" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

# Function to perform backup
perform_backup() {
    local backup_folder=$1
    
    echo "🚀 Starting RAGFlow data backup..."
    echo "📁 Backup folder: $backup_folder"
    echo ""
    
    # Check if any containers are using the volumes
    check_containers_using_volumes
    
    # Create backup folder if it doesn't exist
    mkdir -p "$backup_folder"
    
    # Backup each volume
    for i in "${!VOLUMES[@]}"; do
        local volume="${VOLUMES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))
        
        echo "📦 Step $step/4: Backing up $volume..."
        
        if volume_exists "$volume"; then
            docker run --rm \
                -v "$volume":/source \
                -v "$(pwd)/$backup_folder":/backup \
                alpine tar czf "/backup/$backup_file" -C /source .
            echo "✅ Successfully backed up $volume to $backup_folder/$backup_file"
        else
            echo "⚠️  Warning: Volume $volume does not exist, skipping..."
        fi
        echo ""
    done
    
    echo "🎉 Backup completed successfully!"
    echo "📍 Backup location: $(pwd)/$backup_folder"
    
    # List backup files with sizes
    echo ""
    echo "📋 Backup files created:"
    for backup_file in "${BACKUP_FILES[@]}"; do
        if [ -f "$backup_folder/$backup_file" ]; then
            local size=$(ls -lh "$backup_folder/$backup_file" | awk '{print $5}')
            echo "  - $backup_file ($size)"
        fi
    done
}

# Function to perform restore
perform_restore() {
    local backup_folder=$1
    
    echo "🔄 Starting RAGFlow data restore..."
    echo "📁 Backup folder: $backup_folder"
    echo ""
    
    # Check if any containers are using the volumes
    check_containers_using_volumes
    
    # Check if backup folder exists
    if [ ! -d "$backup_folder" ]; then
        echo "❌ Error: Backup folder '$backup_folder' does not exist"
        exit 1
    fi
    
    # Check if all backup files exist
    local missing_files=()
    for backup_file in "${BACKUP_FILES[@]}"; do
        if [ ! -f "$backup_folder/$backup_file" ]; then
            missing_files+=("$backup_file")
        fi
    done
    
    if [ ${#missing_files[@]} -gt 0 ]; then
        echo "❌ Error: Missing backup files:"
        for file in "${missing_files[@]}"; do
            echo "  - $file"
        done
        echo "Please ensure all backup files are present in '$backup_folder'"
        exit 1
    fi
    
    # Check for existing volumes and warn user
    local existing_volumes=()
    for volume in "${VOLUMES[@]}"; do
        if volume_exists "$volume"; then
            existing_volumes+=("$volume")
        fi
    done
    
    if [ ${#existing_volumes[@]} -gt 0 ]; then
        echo "⚠️  WARNING: The following Docker volumes already exist:"
        for volume in "${existing_volumes[@]}"; do
            echo "  - $volume"
        done
        echo ""
        echo "🔴 IMPORTANT: Restoring will OVERWRITE existing data!"
        echo "💡 Recommendation: Create a backup of your current data first:"
        echo "   $0 backup current_backup_$(date +%Y%m%d_%H%M%S)"
        echo ""
        
        if ! confirm_action "Do you want to continue with the restore operation?"; then
            echo "❌ Restore operation cancelled by user"
            exit 0
        fi
    fi
    
    # Create volumes and restore data
    for i in "${!VOLUMES[@]}"; do
        local volume="${VOLUMES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))
        
        echo "🔧 Step $step/4: Restoring $volume..."
        
        # Create volume if it doesn't exist
        if ! volume_exists "$volume"; then
            echo "  📋 Creating Docker volume: $volume"
            docker volume create "$volume"
        else
            echo "  📋 Using existing Docker volume: $volume"
        fi
        
        # Restore data
        echo "  📥 Restoring data from $backup_file..."
        docker run --rm \
            -v "$volume":/target \
            -v "$(pwd)/$backup_folder":/backup \
            alpine tar xzf "/backup/$backup_file" -C /target
        
        echo "✅ Successfully restored $volume"
        echo ""
    done
    
    echo "🎉 Restore completed successfully!"
    echo "💡 You can now start your RAGFlow services"
}

# Main script logic
main() {
    # Check if Docker is available
    check_docker
    
    # Parse command line arguments
    local operation=${1:-}
    local backup_folder=${2:-$DEFAULT_BACKUP_FOLDER}
    
    # Handle help or no arguments
    if [ -z "$operation" ] || [ "$operation" = "help" ] || [ "$operation" = "-h" ] || [ "$operation" = "--help" ]; then
        show_help
        exit 0
    fi
    
    # Validate operation
    case "$operation" in
        backup)
            perform_backup "$backup_folder"
            ;;
        restore)
            perform_restore "$backup_folder"
            ;;
        *)
            echo "❌ Error: Invalid operation '$operation'"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"