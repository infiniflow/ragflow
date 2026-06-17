#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

usage() {
    local exit_code=${1:-1}
    echo "Usage: $0 [ragflow|task_executor|admin|data_sync]..."
    echo
    echo "Without arguments, starts ragflow and task_executor."
    echo "Available service types:"
    echo "  ragflow         Start RAGFlow server based on API_PROXY_SCHEME"
    echo "  task_executor   Start rag/svr/task_executor.py workers"
    echo "  admin           Start Admin server based on API_PROXY_SCHEME"
    echo "  data_sync       Start rag/svr/sync_data_source.py"
    echo
    echo "Examples:"
    echo "  $0"
    echo "  $0 ragflow"
    echo "  $0 task_executor"
    echo "  $0 admin"
    echo "  $0 data_sync"
    exit "$exit_code"
}

# Function to load environment variables from .env file
load_env_file() {
    # Get the directory of the current script
    local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local env_file="$script_dir/.env"

    # Check if .env file exists
    if [ -f "$env_file" ]; then
        echo "Loading environment variables from: $env_file"
        # Source the .env file
        set -a
        source "$env_file"
        set +a
    else
        echo "Warning: .env file not found at: $env_file"
    fi
}

# Load environment variables
load_env_file

# Unset HTTP proxies that might be set by Docker daemon
export http_proxy=""; export https_proxy=""; export no_proxy=""; export HTTP_PROXY=""; export HTTPS_PROXY=""; export NO_PROXY=""
export PYTHONPATH=$(pwd)

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/
JEMALLOC_PATH=$(pkg-config --variable=libdir jemalloc)/libjemalloc.so

PY=python3

# Set default number of workers if WS is not set or less than 1
if [[ -z "$WS" || $WS -lt 1 ]]; then
  WS=1
fi

# Maximum number of retries for each task executor and server
MAX_RETRIES=5

# Flag to control termination
STOP=false

# Array to keep track of child PIDs
PIDS=()

# Set the path to the NLTK data directory
export NLTK_DATA="./nltk_data"

# Function to handle termination signals
cleanup() {
  echo "Termination signal received. Shutting down..."
  STOP=true
  # Terminate all child processes
  for pid in "${PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      echo "Killing process $pid"
      kill "$pid"
    fi
  done
  exit 0
}

# Trap SIGINT and SIGTERM to invoke cleanup
trap cleanup SIGINT SIGTERM

# Function to execute task_executor with retry logic
task_exe(){
    local task_id=$1
    local retry_count=0
    while ! $STOP && [ $retry_count -lt $MAX_RETRIES ]; do
        echo "Starting task_executor.py for task $task_id (Attempt $((retry_count+1)))"
        LD_PRELOAD=$JEMALLOC_PATH $PY rag/svr/task_executor.py -i "$task_id"
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 0 ]; then
            echo "task_executor.py for task $task_id exited successfully."
            break
        else
            echo "task_executor.py for task $task_id failed with exit code $EXIT_CODE. Retrying..." >&2
            retry_count=$((retry_count + 1))
            sleep 2
        fi
    done

    if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "task_executor.py for task $task_id failed after $MAX_RETRIES attempts. Exiting..." >&2
        cleanup
    fi
}

# Function to execute ragflow_server with retry logic
run_server(){
    local server_name="ragflow_server.py"
    local -a server_cmd=("$PY" "api/ragflow_server.py")
    if [[ "${API_PROXY_SCHEME}" == "go" ]]; then
        prepare_for_go
        server_name="ragflow_server"
        server_cmd=("bin/ragflow_server")
    fi
    local retry_count=0
    while ! $STOP && [ $retry_count -lt $MAX_RETRIES ]; do
        echo "Starting $server_name (Attempt $((retry_count+1)))"
        "${server_cmd[@]}"
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 0 ]; then
            echo "$server_name exited successfully."
            break
        else
            echo "$server_name failed with exit code $EXIT_CODE. Retrying..." >&2
            retry_count=$((retry_count + 1))
            sleep 2
        fi
    done

    if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "$server_name failed after $MAX_RETRIES attempts. Exiting..." >&2
        cleanup
    fi
}

# Function to execute admin_server with retry logic
run_admin_server(){
    local server_name="admin_server.py"
    local -a server_cmd=("$PY" "admin/server/admin_server.py")
    if [[ "${API_PROXY_SCHEME}" == "go" ]]; then
        prepare_for_go
        server_name="admin_server"
        server_cmd=("bin/admin_server")
    fi
    local retry_count=0
    while ! $STOP && [ $retry_count -lt $MAX_RETRIES ]; do
        echo "Starting $server_name (Attempt $((retry_count+1)))"
        "${server_cmd[@]}"
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 0 ]; then
            echo "$server_name exited successfully."
            break
        else
            echo "$server_name failed with exit code $EXIT_CODE. Retrying..." >&2
            retry_count=$((retry_count + 1))
            sleep 2
        fi
    done
    if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "$server_name failed after $MAX_RETRIES attempts. Exiting..." >&2
        cleanup
    fi
}

# Function to execute sync_data_source with retry logic
run_data_sync(){
    local retry_count=0
    while ! $STOP && [ $retry_count -lt $MAX_RETRIES ]; do
        echo "Starting sync_data_source.py (Attempt $((retry_count+1)))"
        $PY rag/svr/sync_data_source.py
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 0 ]; then
            echo "sync_data_source.py exited successfully."
            break
        else
            echo "sync_data_source.py failed with exit code $EXIT_CODE. Retrying..." >&2
            retry_count=$((retry_count + 1))
            sleep 2
        fi
    done

    if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "sync_data_source.py failed after $MAX_RETRIES attempts. Exiting..." >&2
        cleanup
    fi
}

ensure_db_init() {
    echo "Initializing database tables..."
    "$PY" -c "from api.db.db_models import init_database_tables as init_web_db; init_web_db()"
    echo "Database tables initialized."
}

run_mysql_migrations() {
    echo "Running model provider table migrations..."
    "$PY" tools/scripts/mysql_migration.py \
        --stages tenant_model_provider,tenant_model_instance,tenant_model,model_id_config \
        --config conf/service_conf.yaml \
        --execute \
        --database-version "v0.26.1" \
        --mark-database-version-on-success
    echo "Model provider table migrations completed."
}

prepare_for_go() {
    if [ -d /usr/share/infinity/resource ]; then
        echo "Resource directory already exists. Skipping preparation."
        return
    fi
    mkdir -p /usr/share/infinity/resource
    if [ "$NEED_MIRROR" == "1" ]; then
        git clone --depth 1 --single-branch https://gitee.com/infiniflow/resource /tmp/resource;
    else
        git clone --depth 1 --single-branch https://github.com/infiniflow/resource.git /tmp/resource;
    fi
    cp -r /tmp/resource/* /usr/share/infinity/resource
    rm -rf /tmp/resource
}

START_RAGFLOW=0
START_TASK_EXECUTOR=0
START_ADMIN=0
START_DATA_SYNC=0

if [ $# -eq 0 ]; then
  START_RAGFLOW=1
  START_TASK_EXECUTOR=1
fi

for arg in "$@"; do
  case $arg in
    ragflow|server|webserver)
      START_RAGFLOW=1
      ;;
    task_executor|task-executor|taskexecutor)
      START_TASK_EXECUTOR=1
      ;;
    admin|admin_server|admin-server)
      START_ADMIN=1
      ;;
    data_sync|data-sync|datasync)
      START_DATA_SYNC=1
      ;;
    all)
      START_RAGFLOW=1
      START_TASK_EXECUTOR=1
      START_ADMIN=1
      START_DATA_SYNC=1
      ;;
    -h|--help)
      usage 0
      ;;
    *)
      echo "Unknown service type: $arg" >&2
      usage
      ;;
  esac
done

if [[ "$START_RAGFLOW" -eq 1 ]]; then
  ensure_db_init
  run_mysql_migrations
fi

# Start task executors
if [[ "$START_TASK_EXECUTOR" -eq 1 ]]; then
  for ((i=0;i<WS;i++))
  do
    task_exe "$i" &
    PIDS+=($!)
  done
fi

# Start the RAGFlow server
if [[ "$START_RAGFLOW" -eq 1 ]]; then
  run_server &
  PIDS+=($!)
fi

# Start the Admin server
if [[ "$START_ADMIN" -eq 1 ]]; then
  run_admin_server &
  PIDS+=($!)
fi

# Start the data sync server
if [[ "$START_DATA_SYNC" -eq 1 ]]; then
  run_data_sync &
  PIDS+=($!)
fi

# Wait for all background processes to finish
wait
