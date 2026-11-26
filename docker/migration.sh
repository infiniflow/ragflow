#!/bin/bash

# RAGFlow Data Migration Script
# Usage: ./migration.sh [-p project_name] [backup|restore] [backup_folder]
#
# This script helps you backup and restore RAGFlow Docker volumes
# selected by the RAGFlow Compose configuration.

set -eo pipefail

# Default values
DEFAULT_BACKUP_FOLDER="backup"
DEFAULT_PROJECT_NAME="docker"

# Resolve the same profiles and .env interpolation as the deployment. Compose
# reports volume keys (e.g. osdata39), even after the containers are stopped.
build_backup_plan() {
    local operation=$1 backup_folder=$2 compose_dir volume_keys base archive saved_engine
    compose_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
    local compose=(docker compose --project-directory "$compose_dir" -p "$PROJECT_NAME" -f "$compose_dir/docker-compose-base.yml")
    DOC_ENGINE=$("${compose[@]}" config --environment | sed -n 's/^DOC_ENGINE=//p')
    DOC_ENGINE=${DOC_ENGINE:-elasticsearch}

    local index_volume=""
    local bind_dirs=()
    case "$DOC_ENGINE" in
        elasticsearch) index_volume=esdata01 ;;
        infinity)      index_volume=infinity_data ;;
        opensearch)    index_volume='osdata[0-9]*' ;;
        serenedb)      index_volume=serenedb_data ;;
        vastbase)      index_volume=vastbase_data ;;
        oceanbase)     bind_dirs=(oceanbase/data oceanbase/conf) ;;
        seekdb)        bind_dirs=(seekdb) ;;
        *)
            echo "No bundled index volume for DOC_ENGINE=$DOC_ENGINE. Use the database's backup and restore procedure." >&2
            return 1 ;;
    esac

    # Before engine-aware backups, the script always wrote these four archives.
    if [[ -f $backup_folder/.doc_engine.pending ]]; then
        if [[ $operation == restore ]]; then
            echo "Backup is incomplete. Run backup again before restoring." >&2
            return 1
        fi
        saved_engine=$(cat -- "$backup_folder/.doc_engine.pending")
    elif [[ -f $backup_folder/doc_engine ]]; then
        saved_engine=$(cat -- "$backup_folder/doc_engine")
    elif [[ -f $backup_folder/es_backup.tar.gz ]]; then
        saved_engine=elasticsearch
    else
        saved_engine=$DOC_ENGINE
    fi
    if [[ $saved_engine != "$DOC_ENGINE" ]]; then
        echo "Backup folder uses DOC_ENGINE=$saved_engine, but the deployment uses $DOC_ENGINE." >&2
        return 1
    fi
    if [[ $operation == restore && ! -f $backup_folder/doc_engine && -f $backup_folder/es_backup.tar.gz ]]; then
        volume_keys=$'mysql_data\nminio_data\nredis_data\nesdata01'
    else
        volume_keys=$("${compose[@]}" config --volumes | LC_ALL=C sort)
    fi

    SOURCES=()
    BACKUP_FILES=()
    local found_index=false
    while IFS= read -r base; do
        [[ -n $base ]] || continue
        if [[ $base == $index_volume ]]; then
            found_index=true
        fi
        case "$base" in
            mysql_data) archive=mysql_backup.tar.gz ;;
            minio_data) archive=minio_backup.tar.gz ;;
            redis_data) archive=redis_backup.tar.gz ;;
            esdata01)   archive=es_backup.tar.gz ;;
            *)          archive="$base.tar.gz" ;;
        esac
        SOURCES+=("${PROJECT_NAME}_${base}")
        BACKUP_FILES+=("$archive")
    done <<< "$volume_keys"

    if [[ ${#bind_dirs[@]} -gt 0 ]]; then
        local services dir
        services=$("${compose[@]}" config --services)
        if grep -Fxq -- "$DOC_ENGINE" <<< "$services"; then
            for dir in "${bind_dirs[@]}"; do
                SOURCES+=("$compose_dir/$dir")
                BACKUP_FILES+=("${dir//\//-}.tar.gz")
            done
            found_index=true
        fi
    fi
    if [[ $found_index == false ]]; then
        echo "No active Compose index storage for DOC_ENGINE=$DOC_ENGINE. Check the selected Compose profiles." >&2
        return 1
    fi
}

# Function to display help information
show_help() {
    echo "RAGFlow Data Migration Tool"
    echo ""
    echo "USAGE:"
    echo "  $0 [-p project_name] <operation> [backup_folder]"
    echo ""
    echo "OPERATIONS:"
    echo "  backup   - Create backup of the RAGFlow data volumes listed below"
    echo "  restore  - Restore RAGFlow data volumes from backup"
    echo "  help     - Show this help message"
    echo ""
    echo "OPTIONS:"
    echo "  -p project_name  - Docker Compose project name (default: '$DEFAULT_PROJECT_NAME')"
    echo "                     Use this when you started RAGFlow with 'docker compose -p <name>'"
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
    echo "The script archives active Compose volumes and the OceanBase or SeekDB index directories."
    echo "Set DOC_ENGINE in docker/.env or the shell, and restore with the same engine and profiles."
    echo "Use a separate backup folder for each backup. Restore index directories into empty paths."
    echo "Back up externally managed databases and custom mounts separately."
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

# Bind mounts are absolute host paths. Named volumes are scoped to the Compose project.
source_exists() {
    if [[ $1 == /* ]]; then
        [[ -d $1 ]]
    else
        docker volume inspect "$1" >/dev/null 2>&1
    fi
}

# Function to check if any containers are using the target volumes
check_containers_using_volumes() {
    echo "🔍 Checking for running containers that might be using target volumes..."

    # Get all running containers
    local running_containers
    running_containers=$(docker ps --format "{{.Names}}")

    if [ -z "$running_containers" ]; then
        echo "✅ No running containers found"
        return 0
    fi

    # Check each running container for volume usage
    local containers_using_volumes=()
    local volume_usage_details=()

    for container in $running_containers; do
        # Get container's mount information
        local mounts=$(docker inspect "$container" --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{else}}{{println .Source}}{{end}}{{end}}' 2>/dev/null || echo "")

        # Check if any of our target volumes are used by this container
        for volume in "${SOURCES[@]}"; do
            if grep -Fxq -- "$volume" <<< "$mounts"; then
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

    local volume
    # Restore requires every selected archive. Check the sources before writing any.
    for volume in "${SOURCES[@]}"; do
        if ! source_exists "$volume"; then
            echo "❌ Error: Required backup source $volume does not exist" >&2
            return 1
        fi
    done

    # Create backup folder if it doesn't exist
    mkdir -p "$backup_folder"

    backup_folder=$(cd -- "$backup_folder" && pwd -P)
    for volume in "${SOURCES[@]}"; do
        if [[ $volume == /* ]]; then
            local source_dir
            source_dir=$(cd -- "$volume" && pwd -P)
            if [[ $backup_folder == "$source_dir" || $backup_folder == "$source_dir/"* ]]; then
                echo "Choose a backup folder outside the index directory $volume." >&2
                return 1
            fi
        fi
    done

    # A failed overwrite can leave archives from different points in time.
    # Publish the engine marker only after all tar commands have succeeded.
    printf '%s\n' "$DOC_ENGINE" > "$backup_folder/.doc_engine.pending"
    local total=${#SOURCES[@]}

    # Backup each volume
    for i in "${!SOURCES[@]}"; do
        local volume="${SOURCES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))

        echo "📦 Step $step/$total: Backing up $volume..."

        docker run --rm \
            -v "$volume":/source:ro \
            -v "$backup_folder":/backup \
            alpine tar czf "/backup/$backup_file" -C /source .
        echo "✅ Successfully backed up $volume to $backup_folder/$backup_file"
        echo ""
    done

    mv -- "$backup_folder/.doc_engine.pending" "$backup_folder/doc_engine"
    echo "🎉 Backup completed successfully!"
    echo "📍 Backup location: $backup_folder"

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
    local volume entries
    for volume in "${SOURCES[@]}"; do
        if [[ $volume == /* && -e $volume ]]; then
            if [[ ! -d $volume ]] || ! entries=$(ls -Aq -- "$volume") || [[ -n $entries ]]; then
                echo "Restore requires an empty index directory at $volume." >&2
                return 1
            fi
        elif source_exists "$volume"; then
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
        echo "   $0 -p $PROJECT_NAME backup current_backup_$(date +%Y%m%d_%H%M%S)"
        echo ""

        if ! confirm_action "Do you want to continue with the restore operation?"; then
            echo "❌ Restore operation cancelled by user"
            exit 0
        fi
    fi

    local total=${#SOURCES[@]}

    # Create volumes and restore data
    for i in "${!SOURCES[@]}"; do
        local volume="${SOURCES[$i]}"
        local backup_file="${BACKUP_FILES[$i]}"
        local step=$((i + 1))

        echo "🔧 Step $step/$total: Restoring $volume..."

        # Create volume if it doesn't exist
        if [[ $volume == /* ]]; then
            mkdir -p -- "$volume"
        elif ! source_exists "$volume"; then
            echo "  📋 Creating Docker volume: $volume"
            docker volume create "$volume"
        else
            echo "  📋 Using existing Docker volume: $volume"
        fi

        # Restore data
        echo "  📥 Restoring data from $backup_file..."
        docker run --rm \
            -v "$volume":/target \
            -v "$backup_folder":/backup:ro \
            alpine tar xzf "/backup/$backup_file" -C /target

        echo "✅ Successfully restored $volume"
        echo ""
    done

    echo "🎉 Restore completed successfully!"
    echo "💡 You can now start your RAGFlow services"
}

# Main script logic
main() {
    # Parse -p flag
    PROJECT_NAME="$DEFAULT_PROJECT_NAME"
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
            *)
                break
                ;;
        esac
    done

    # Parse remaining positional arguments
    local operation=${1:-}
    local backup_folder=${2:-$DEFAULT_BACKUP_FOLDER}

    # Handle help or no arguments
    if [ -z "$operation" ] || [ "$operation" = "help" ] || [ "$operation" = "-h" ] || [ "$operation" = "--help" ]; then
        show_help
        exit 0
    fi

    if [[ $backup_folder != /* ]]; then
        backup_folder="$PWD/$backup_folder"
    fi

    case "$operation" in
        backup|restore) ;;
        *)
            echo "❌ Error: Invalid operation '$operation'"
            echo ""
            show_help
            exit 1
            ;;
    esac
    check_docker
    build_backup_plan "$operation" "$backup_folder"
    "perform_$operation" "$backup_folder"
}

# Run main function with all arguments
main "$@"
