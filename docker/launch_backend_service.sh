#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Unset HTTP proxies that might be set by Docker daemon
export http_proxy=""; export https_proxy=""; export no_proxy=""; export HTTP_PROXY=""; export HTTPS_PROXY=""; export NO_PROXY=""

export LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/

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
        $PY rag/svr/task_executor.py "$task_id"
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
    local retry_count=0
    while ! $STOP && [ $retry_count -lt $MAX_RETRIES ]; do
        echo "Starting ragflow_server.py (Attempt $((retry_count+1)))"
        $PY api/ragflow_server.py
        EXIT_CODE=$?
        if [ $EXIT_CODE -eq 0 ]; then
            echo "ragflow_server.py exited successfully."
            break
        else
            echo "ragflow_server.py failed with exit code $EXIT_CODE. Retrying..." >&2
            retry_count=$((retry_count + 1))
            sleep 2
        fi
    done

    if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "ragflow_server.py failed after $MAX_RETRIES attempts. Exiting..." >&2
        cleanup
    fi
}

# Start task executors
for ((i=0;i<WS;i++))
do
  task_exe "$i" &
  PIDS+=($!)
done

# Start the main server
run_server &
PIDS+=($!)

# Wait for all background processes to finish
wait