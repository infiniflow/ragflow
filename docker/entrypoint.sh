#!/usr/bin/env bash

set -e

# -----------------------------------------------------------------------------
# Usage and command-line argument parsing
# -----------------------------------------------------------------------------
function usage() {
    echo "Usage: $0 [--disable-webserver] [--disable-taskexecutor] [--consumer-no-beg=<num>] [--consumer-no-end=<num>] [--workers=<num>] [--host-id=<string>]"
    echo
    echo "  --disable-webserver             Disables the web server (nginx + ragflow_server)."
    echo "  --disable-taskexecutor          Disables task executor workers."
    echo "  --consumer-no-beg=<num>         Start range for consumers (if using range-based)."
    echo "  --consumer-no-end=<num>         End range for consumers (if using range-based)."
    echo "  --workers=<num>                 Number of task executors to run (if range is not used)."
    echo "  --host-id=<string>              Unique ID for the host (defaults to \`hostname\`)."
    echo
    echo "Examples:"
    echo "  $0 --disable-taskexecutor"
    echo "  $0 --disable-webserver --consumer-no-beg=0 --consumer-no-end=5"
    echo "  $0 --disable-webserver --workers=2 --host-id=myhost123"
    exit 1
}

ENABLE_WEBSERVER=1 # Default to enable web server
ENABLE_TASKEXECUTOR=1  # Default to enable task executor
CONSUMER_NO_BEG=0
CONSUMER_NO_END=0
WORKERS=1

# -----------------------------------------------------------------------------
# Host ID logic:
#   1. By default, use the system hostname if length <= 32
#   2. Otherwise, use the full MD5 hash of the hostname (32 hex chars)
# -----------------------------------------------------------------------------
CURRENT_HOSTNAME="$(hostname)"
if [ ${#CURRENT_HOSTNAME} -le 32 ]; then
  DEFAULT_HOST_ID="$CURRENT_HOSTNAME"
else
  DEFAULT_HOST_ID="$(echo -n "$CURRENT_HOSTNAME" | md5sum | cut -d ' ' -f 1)"
fi

HOST_ID="$DEFAULT_HOST_ID"

# Parse arguments
for arg in "$@"; do
  case $arg in
    --disable-webserver)
      ENABLE_WEBSERVER=0
      shift
      ;;
    --disable-taskexecutor)
      ENABLE_TASKEXECUTOR=0
      shift
      ;;
    --consumer-no-beg=*)
      CONSUMER_NO_BEG="${arg#*=}"
      shift
      ;;
    --consumer-no-end=*)
      CONSUMER_NO_END="${arg#*=}"
      shift
      ;;
    --workers=*)
      WORKERS="${arg#*=}"
      shift
      ;;
    --host-id=*)
      HOST_ID="${arg#*=}"
      shift
      ;;
    *)
      usage
      ;;
  esac
done

# -----------------------------------------------------------------------------
# Replace env variables in the service_conf.yaml file
# -----------------------------------------------------------------------------
CONF_DIR="/ragflow/conf"
TEMPLATE_FILE="${CONF_DIR}/service_conf.yaml.template"
CONF_FILE="${CONF_DIR}/service_conf.yaml"

rm -f "${CONF_FILE}"
while IFS= read -r line || [[ -n "$line" ]]; do
    eval "echo \"$line\"" >> "${CONF_FILE}"
done < "${TEMPLATE_FILE}"

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
PY=python3

# -----------------------------------------------------------------------------
# Function(s)
# -----------------------------------------------------------------------------

function task_exe() {
    local consumer_id="$1"
    local host_id="$2"

    JEMALLOC_PATH="$(pkg-config --variable=libdir jemalloc)/libjemalloc.so"
    while true; do
        LD_PRELOAD="$JEMALLOC_PATH" \
        "$PY" rag/svr/task_executor.py "${host_id}_${consumer_id}"
    done
}

# -----------------------------------------------------------------------------
# Start components based on flags
# -----------------------------------------------------------------------------

if [[ "${ENABLE_WEBSERVER}" -eq 1 ]]; then
    echo "Starting nginx..."
    /usr/sbin/nginx

    echo "Starting ragflow_server..."
    while true; do
        "$PY" api/ragflow_server.py
    done &
fi

if [[ "${ENABLE_TASKEXECUTOR}" -eq 1 ]]; then
    if [[ "${CONSUMER_NO_END}" -gt "${CONSUMER_NO_BEG}" ]]; then
        echo "Starting task executors on host '${HOST_ID}' for IDs in [${CONSUMER_NO_BEG}, ${CONSUMER_NO_END})..."
        for (( i=CONSUMER_NO_BEG; i<CONSUMER_NO_END; i++ ))
        do
          task_exe "${i}" "${HOST_ID}" &
        done
    else
        # Otherwise, start a fixed number of workers
        echo "Starting ${WORKERS} task executor(s) on host '${HOST_ID}'..."
        for (( i=0; i<WORKERS; i++ ))
        do
          task_exe "${i}" "${HOST_ID}" &
        done
    fi
fi

wait
