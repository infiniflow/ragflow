#!/bin/bash

# RAGFlow Data Migration Script
# Usage: ./migration.sh [-p project_name] [backup|restore] [backup_folder]
#
# This script helps you backup and restore RAGFlow Docker volumes
# including MySQL, MinIO, Redis, and Elasticsearch data.

set -e  # Exit on any error
# Instead, we'll handle errors manually for better debugging experience

# Default values
DEFAULT_BACKUP_FOLDER="backup"
DEFAULT_PROJECT_NAME="docker"
VOLUME_BASES=("mysql_data" "minio_data" "redis_data" "esdata01")
BACKUP_FILES=("mysql_backup.tar.gz" "minio_backup.tar.gz" "redis_backup.tar.gz" "es_backup.tar.gz")

# Build volume names from project name and base names
build_volume_names() {
    VOLUMES=()
    for base in "${VOLUME_BASES[@]}"; do
        VOLUMES+=("${PROJECT_NAME}_${base}")
    done
}

# Function to display help information
show_help() {
    echo "RAGFlow Data Migration Tool"
    echo ""
    echo "USAGE:"
    echo "  $0 [-p project_name] <operation> [backup_folder]"
    echo ""
    echo "OPERATIONS:"
    echo "  backup   - Create backup of all RAGFlow data volumes"
    echo "  restore  - Restore RAGFlow data volumes from backup"
    echo "  help     - Show this help message"
    echo ""
    echo "OPTIONS:"
    echo "  -p project_name  - Docker Compose project name (default: '$DEFAULT_PROJECT_NAME')"
    echo "                     Use this when you started RAGFlow with 'docker compose -p <name>'"
    echo "  --force           - (restore only) auto-clear a non-empty target volume"
    echo "                     instead of aborting. Use for automation / CI when"
    echo "                     you've already verified the volume is expendable."
    echo ""
    echo "PARAMETERS:"
    echo "  backup_folder    - Name of backup folder (default: '$DEFAULT_BACKUP_FOLDER')"
    echo ""
    echo "EXAMPLES:"
    echo "  $0 backup                        # Backup with default project name 'docker'"
    echo "  $0 backup my_backup              # Backup to './my_backup' folder"
    echo "  $0 restore                       # Restore from './backup' folder"
    echo "  $0 restore my_backup             # Restore from './my_backup' folder"
    echo "  $0 -p ragflow backup             # Backup volumes for project 'ragflow'"
    echo "  $0 -p ragflow restore my_backup  # Restore volumes for project 'ragflow'"
    echo ""
    echo "DOCKER VOLUMES (with default project name '$DEFAULT_PROJECT_NAME'):"
    echo "  - ${DEFAULT_PROJECT_NAME}_mysql_data     (MySQL database)"
    echo "  - ${DEFAULT_PROJECT_NAME}_minio_data     (MinIO object storage)"
    echo "  - ${DEFAULT_PROJECT_NAME}_redis_data     (Redis cache)"
    echo "  - ${DEFAULT_PROJECT_NAME}_esdata01       (Elasticsearch indices)"
    echo ""
    echo "NOTE:"
    echo "  If you started RAGFlow with 'docker compose -p myproject up', the volume"
    echo "  names will be prefixed with 'myproject' instead of 'docker'. In that case,"
    echo "  use '-p myproject' with this script to match the correct volumes."
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
        if [ "$PROJECT_NAME" = "$DEFAULT_PROJECT_NAME" ]; then
            echo "🛑 SOLUTION: Stop the containers before performing backup/restore operations:"
            echo "   docker compose -f docker/<your-docker-compose-file>.yml down"
        else
            echo "🛑 SOLUTION: Stop the containers before performing backup/restore operations:"
            echo "   docker compose -p $PROJECT_NAME -f docker/<your-docker-compose-file>.yml down"
        fi
        echo ""
        echo "💡 After backup/restore, you can restart with the corresponding 'up -d' command."
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
    echo "🏷️  Project name: $PROJECT_NAME"
    echo ""

    # Check if any containers are using the volumes
    check_containers_using_volumes

    # Create backup folder if it doesn't exist
    mkdir -p "$backup_folder"

    local total=${#VOLUMES[@]}

    # Backup each volume
    for i in "${!VOLUMES[@]}"; do
        local volume="${VOLUMES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))

        echo "📦 Step $step/$total: Backing up $volume..."

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
    echo "🏷️  Project name: $PROJECT_NAME"
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

    if [ ${#existing_volumes[@]} -gt 0 ] && [ "$RESTORE_FORCE" != "true" ]; then
        echo "⚠️  WARNING: The following Docker volumes already exist:"
        for volume in "${existing_volumes[@]}"; do
            echo "  - $volume"
        done
        echo ""
        echo "🔴 IMPORTANT: Restoring will PERMANENTLY REPLACE the contents of these"
        echo "   volumes. Any files in the volumes that are NOT present in the backup"
        echo "   archive will be DELETED before extraction begins — this is necessary to"
        echo "   reproduce a faithful copy of the backup and avoid silent merges of old"
        echo "   and new state across restore cycles. This operation is irreversible."
        echo ""
        echo "� Recommendation: Snapshot the current volumes first (e.g. with"
        echo "   'docker volume create --name <snapshot>' + a tar backup, or stop and"
        echo "   copy the volume via a helper container):"
        echo "   $0 -p $PROJECT_NAME backup current_backup_$(date +%Y%m%d_%H%M%S)"
        echo ""
        echo "Pass --force to skip this prompt and auto-clear the target."
        echo ""

        if ! confirm_action "Continue with the restore? (type 'y' to permanently overwrite these volumes)"; then
            echo "❌ Restore operation cancelled by user"
            exit 0
        fi
    fi

    # Preflight: POSIX-compatible volume inspection (Alpine BusyBox
    # `sh` does not support `shopt`, so we use plain shell globs). Must
    # happen BEFORE creating or clearing any volume so a single dirty
    # volume aborts the whole restore cleanly, instead of partially
    # clearing some and reporting success. Per #18393 review.
    local dirty_volumes=()
    if [ "$RESTORE_FORCE" != "true" ]; then
        for volume in "${VOLUMES[@]}"; do
            if ! volume_exists "$volume"; then
                continue
            fi
            local has_state=0
            for pattern in '/target/*' '/target/.[!.]*' '/target/..?*'; do
                if docker run --rm -v "$volume":/target alpine \
                    sh -c "for f in $pattern; do [ -e \"\$f\" ] && exit 0; done; exit 1"; then
                    has_state=1
                    break
                fi
            done
            if [ "$has_state" -eq 1 ]; then
                dirty_volumes+=("$volume")
            fi
        done
        if [ ${#dirty_volumes[@]} -gt 0 ]; then
            echo ""
            echo "🔴 The following target volumes are NOT empty — refusing to restore:"
            for volume in "${dirty_volumes[@]}"; do
                echo "   - $volume"
                docker run --rm -v "$volume":/target alpine \
                    sh -c "for f in /target/* /target/.[!.]* /target/..?*; do [ -e \"\$f\" ] && printf '       %s\\n' \"\$f\"; done" | head -n 20
            done
            echo ""
            echo "To restore, choose one of:"
            echo "  a) Clean up the target volumes yourself, then re-run:"
            echo "       docker compose down && docker volume rm <volume> && docker volume create <volume>"
            echo "  b) Re-run with --force to accept the destructive auto-clear (legacy behavior)."
            echo ""
            echo "❌ Restore aborted before any volume was touched."
            exit 1
        fi
    fi

    local total=${#VOLUMES[@]}

    # Create volumes and restore data
    for i in "${!VOLUMES[@]}"; do
        local volume="${VOLUMES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))

        echo "🔧 Step $step/$total: Restoring $volume..."

        # Create volume if it doesn't exist
        if ! volume_exists "$volume"; then
            echo "  📋 Creating Docker volume: $volume"
            docker volume create "$volume"
        else
            echo "  📋 Using existing Docker volume: $volume"
        fi

        # Restore data
        # The previous behaviour extracted the archive on top of any
        # existing files in the volume. That silently merged old and new
        # state (deleted-in-new files would still be served from the
        # volume, file format upgrades never took effect, etc.). Clean the
        # target path inside the helper container before extracting, so
        # stateful volumes land as a faithful copy of the backup.
        #
        # The cleanup glob is split into three patterns so we cover every
        # POSIX-basename shape the volume might contain:
        #   /target/*       — non-hidden entries
        #   /target/.[!.]*  — single-dot hidden entries (".env", ".cache")
        #   /target/..?*    — multi-dot hidden entries (".example", "..tmp",
        #                     "...state"). POSIX "[!.]" only excludes the
        #                     second character being ".", which leaves
        #                     names like ".example" and "...state"
        #                     unremoved under the two-character pattern.
        # Regression for #18354.
        echo "  📥 Restoring data from $backup_file..."
        docker run --rm \
            -v "$volume":/target \
            -v "$(pwd)/$backup_folder":/backup \
            alpine sh -c "rm -rf /target/* /target/.[!.]* /target/..?* && tar xzf \"/backup/$backup_file\" -C /target"

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

    # Parse -p / --force flags (force preserves the prior auto-clear
    # behavior). Flags are accepted in any position relative to the
    # operation/folder args so callers don't have to remember the order:
    #   $0 restore --force
    #   $0 --force restore my_backup
    #   $0 -p ragflow restore my_backup --force
    # all behave the same. We use a two-pass scan: first extract every
    # recognised flag, then the remaining positional args are
    # operation + backup_folder.
    PROJECT_NAME="$DEFAULT_PROJECT_NAME"
    RESTORE_FORCE="false"
    local _positional=()
    while [ $# -gt 0 ]; do
        case "$1" in
            -p)
                if [ -z "${2:-}" ]; then
                    echo "❌ Error: -p requires a project name argument"
                    exit 1
                fi
                PROJECT_NAME="$2"
                shift 2
                ;;
            --force)
                RESTORE_FORCE="true"
                shift
                ;;
            *)
                _positional+=("$1")
                shift
                ;;
        esac
    done
    # Restore positional args for the existing operation/folder parser
    # below so it sees the same $@ it would have seen without the flag
    # scan.
    set -- "${_positional[@]}"

    # Reset the positional index because set -- rebuilds $@ starting at 1
    # (the loop above already used _positional, no index needed here).

    # Build volume names based on project name
    build_volume_names

    # Parse remaining positional arguments
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
