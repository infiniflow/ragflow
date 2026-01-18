#!/usr/bin/env bash

# Enable verbose debugging
if [[ "${DEBUG_ENTRYPOINT}" == "true" ]]; then
    set -x
fi
set -e

# -----------------------------------------------------------------------------
# Usage and command-line argument parsing
# -----------------------------------------------------------------------------
function usage() {
    echo "Usage: $0 [--disable-webserver] [--disable-taskexecutor] [--disable-datasync] [--consumer-no-beg=<num>] [--consumer-no-end=<num>] [--workers=<num>] [--host-id=<string>]"
    echo
    echo "  --disable-webserver             Disables the web server (nginx + ragflow_server)."
    echo "  --disable-taskexecutor          Disables task executor workers."
    echo "  --disable-datasync              Disables synchronization of datasource workers."
    echo "  --enable-mcpserver              Enables the MCP server."
    echo "  --enable-adminserver            Enables the Admin server."
    echo "  --init-superuser                Initializes the superuser."
    echo "  --consumer-no-beg=<num>         Start range for consumers (if using range-based)."
    echo "  --consumer-no-end=<num>         End range for consumers (if using range-based)."
    echo "  --workers=<num>                 Number of task executors to run (if range is not used)."
    echo "  --host-id=<string>              Unique ID for the host (defaults to \`hostname\`)."
    echo
    echo "Examples:"
    echo "  $0 --disable-taskexecutor"
    echo "  $0 --disable-webserver --consumer-no-beg=0 --consumer-no-end=5"
    echo "  $0 --disable-webserver --workers=2 --host-id=myhost123"
    echo "  $0 --enable-mcpserver"
    echo "  $0 --enable-adminserver"
    echo "  $0 --init-superuser"
    exit 1
}

ENABLE_WEBSERVER=1 # Default to enable web server
ENABLE_TASKEXECUTOR=1  # Default to enable task executor
ENABLE_DATASYNC=1
ENABLE_MCP_SERVER=0
ENABLE_ADMIN_SERVER=0 # Default close admin server
INIT_SUPERUSER_ARGS="" # Default to not initialize superuser
CONSUMER_NO_BEG=0
CONSUMER_NO_END=0
WORKERS=1

MCP_HOST="127.0.0.1"
MCP_PORT=9382
MCP_BASE_URL="http://127.0.0.1:9380"
MCP_SCRIPT_PATH="/ragflow/mcp/server/server.py"
MCP_MODE="self-host"
MCP_HOST_API_KEY=""
MCP_TRANSPORT_SSE_FLAG="--transport-sse-enabled"
MCP_TRANSPORT_STREAMABLE_HTTP_FLAG="--transport-streamable-http-enabled"
MCP_JSON_RESPONSE_FLAG="--json-response"

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
    --disable-datasync)
      ENABLE_DATASYNC=0
      shift
      ;;
    --enable-mcpserver)
      ENABLE_MCP_SERVER=1
      shift
      ;;
    --enable-adminserver)
      ENABLE_ADMIN_SERVER=1
      shift
      ;;
    --init-superuser)
      INIT_SUPERUSER_ARGS="--init-superuser"
      shift
      ;;
    --mcp-host=*)
      MCP_HOST="${arg#*=}"
      shift
      ;;
    --mcp-port=*)
      MCP_PORT="${arg#*=}"
      shift
      ;;
    --mcp-base-url=*)
      MCP_BASE_URL="${arg#*=}"
      shift
      ;;
    --mcp-mode=*)
      MCP_MODE="${arg#*=}"
      shift
      ;;
    --mcp-host-api-key=*)
      MCP_HOST_API_KEY="${arg#*=}"
      shift
      ;;
    --mcp-script-path=*)
      MCP_SCRIPT_PATH="${arg#*=}"
      shift
      ;;
    --no-transport-sse-enabled)
      MCP_TRANSPORT_SSE_FLAG="--no-transport-sse-enabled"
      shift
      ;;
    --no-transport-streamable-http-enabled)
      MCP_TRANSPORT_STREAMABLE_HTTP_FLAG="--no-transport-streamable-http-enabled"
      shift
      ;;
    --no-json-response)
      MCP_JSON_RESPONSE_FLAG="--no-json-response"
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
DEF_ENV_VALUE_PATTERN="\$\{([^:]+):-([^}]+)\}"
while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ $DEF_ENV_VALUE_PATTERN ]]; then
        varname="${BASH_REMATCH[1]}"
        default="${BASH_REMATCH[2]}"

        if [ -n "${!varname}" ]; then
            set +x
            eval "echo \"$line"\" >> "${CONF_FILE}"
            if [[ "${DEBUG_ENTRYPOINT}" == "true" ]]; then set -x; fi
        else
            echo "$line" | sed -E "s/\\\$\{[^:]+:-([^}]+)\}/\1/g" >> "${CONF_FILE}"
        fi
    else
        set +x
        eval "echo \"$line\"" >> "${CONF_FILE}"
        if [[ "${DEBUG_ENTRYPOINT}" == "true" ]]; then set -x; fi
    fi
done < "${TEMPLATE_FILE}"

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
PY=python3

# Ensure database exists before any service accesses it
echo "Ensuring database exists..."
db_init_output=$("$PY" -c "from api.db.connection import ensure_database_exists; ensure_database_exists()" 2>&1)
db_init_status=$?
if [ $db_init_status -ne 0 ]; then
  echo "Database initialization failed running: $PY -c 'from api.db.connection import ensure_database_exists; ensure_database_exists()' (exit $db_init_status)" >&2
  if [ -n "$db_init_output" ]; then
    echo "$db_init_output" >&2
  fi
  exit $db_init_status
fi

# ------------------------------------------------------------------------------
# Function(s)
# -----------------------------------------------------------------------------

function task_exe() {
    local consumer_id="$1"
    local host_id="$2"

    JEMALLOC_PATH="$(pkg-config --variable=libdir jemalloc)/libjemalloc.so"
    while true; do
        LD_PRELOAD="$JEMALLOC_PATH" \
        "$PY" rag/svr/task_executor.py "${host_id}_${consumer_id}"  &
        wait;
        sleep 1;
    done
}

function start_mcp_server() {
    echo "Starting MCP Server on ${MCP_HOST}:${MCP_PORT} with base URL ${MCP_BASE_URL}..."
    "$PY" "${MCP_SCRIPT_PATH}" \
        --host="${MCP_HOST}" \
        --port="${MCP_PORT}" \
        --base-url="${MCP_BASE_URL}" \
        --mode="${MCP_MODE}" \
        --api-key="${MCP_HOST_API_KEY}" \
        "${MCP_TRANSPORT_SSE_FLAG}" \
        "${MCP_TRANSPORT_STREAMABLE_HTTP_FLAG}" \
        "${MCP_JSON_RESPONSE_FLAG}" &
}

# Generalized pip dependency installation with persistent caching
# Follows AGENTS.md modularization principles: reusable, testable components
# Usage: ensure_pip_dependency <package_name> <package_spec> <env_flag> [import_name]
# Example: ensure_pip_dependency "docling" "docling==2.58.0" "USE_DOCLING"
# Example with different import name: ensure_pip_dependency "Pillow" "Pillow==10.0.0" "USE_PIL" "PIL"
# Future: If adding 3+ optional dependencies, refactor to config-driven approach
function ensure_pip_dependency() {
    local package_name="$1"
    local package_spec="$2"
    local env_flag="$3"
    local import_name="${4:-$package_name}"  # Use package_name as default if import_name not provided
    
    [[ "${!env_flag}" == "true" ]] || { echo "[$package_name] disabled by $env_flag"; return 0; }
    
    local marker_file="/opt/ragflow/.deps/${package_name}-installed"
    
    # Verify cache validity: marker exists AND package actually imports
    # Use import_name for Python import check, package_name for display/marker
    if [[ -f "$marker_file" ]]; then
        if "$PY" -c "import importlib.util; exit(0 if importlib.util.find_spec('${import_name}') else 1)" 2>/dev/null; then
            echo "[$package_name] already installed (cached), skipping..."
            return 0
        else
            echo "[$package_name] cache corrupted, reinstalling..."
            rm "$marker_file"
        fi
    fi
    
    # Install with persistent cache directory
    echo "[$package_name] installing ${package_spec}..."
    "$PY" -c 'import pip' >/dev/null 2>&1 || "$PY" -m ensurepip --upgrade || true
    
    if "$PY" -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple --extra-index-url https://pypi.org/simple --cache-dir /root/.cache/pip "${package_spec}"; then
        mkdir -p /opt/ragflow/.deps
        touch "$marker_file"
        echo "[$package_name] installation complete"
        return 0
    else
        echo "[$package_name] installation FAILED - check pip output above"
        return 1
    fi
}

# Install optional dependencies
docling_version="${DOCLING_VERSION:-2.58.0}"
if [[ "${docling_version}" =~ ^[[:space:]]*[\<\>=!~] ]]; then
  docling_spec="docling${docling_version}"
else
  docling_spec="docling==${docling_version}"
fi

ensure_pip_dependency "docling" "${docling_spec}" "USE_DOCLING"

# -----------------------------------------------------------------------------
# Start components based on flags
# -----------------------------------------------------------------------------

if [[ "${ENABLE_WEBSERVER}" -eq 1 ]]; then
    echo "Starting nginx..."
    /usr/sbin/nginx

    echo "Starting ragflow_server..."
    while true; do
        "$PY" api/ragflow_server.py ${INIT_SUPERUSER_ARGS} &
        wait;
        sleep 1;
    done &
fi

if [[ "${ENABLE_DATASYNC}" -eq 1 ]]; then
    echo "Starting data sync..."
    while true; do
        "$PY" rag/svr/sync_data_source.py &
        wait;
        sleep 1;
    done &
fi

if [[ "${ENABLE_ADMIN_SERVER}" -eq 1 ]]; then
    echo "Starting admin_server..."
    while true; do
        "$PY" admin/server/admin_server.py &
        wait;
        sleep 1;
    done &
fi

if [[ "${ENABLE_MCP_SERVER}" -eq 1 ]]; then
    start_mcp_server
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
