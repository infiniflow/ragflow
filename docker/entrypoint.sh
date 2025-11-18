#!/usr/bin/env bash

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
    exit 1
}

ENABLE_WEBSERVER=1 # Default to enable web server
ENABLE_TASKEXECUTOR=1  # Default to enable task executor
ENABLE_DATASYNC=1
ENABLE_MCP_SERVER=0
ENABLE_ADMIN_SERVER=0 # Default close admin server
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

function ensure_docling() {
    [[ "${USE_DOCLING}" == "true" ]] || { echo "[docling] disabled by USE_DOCLING"; return 0; }
    python3 -c 'import pip' >/dev/null 2>&1 || python3 -m ensurepip --upgrade || true
    DOCLING_PIN="${DOCLING_VERSION:-==2.58.0}"
    python3 -c "import importlib.util,sys; sys.exit(0 if importlib.util.find_spec('docling') else 1)" \
      || python3 -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple --extra-index-url https://pypi.org/simple --no-cache-dir "docling${DOCLING_PIN}"
}

function ensure_mineru() {
    [[ "${USE_MINERU}" == "true" ]] || { echo "[mineru] disabled by USE_MINERU"; return 0; }

    export HUGGINGFACE_HUB_ENDPOINT="${HF_ENDPOINT:-https://hf-mirror.com}"

    local default_prefix="/ragflow/uv_tools"
    local venv_dir="${default_prefix}/.venv"
    local exe="${MINERU_EXECUTABLE:-${venv_dir}/bin/mineru}"

    if [[ -x "${exe}" ]]; then
      echo "[mineru] found: ${exe}"
      export MINERU_EXECUTABLE="${exe}"
      return 0
    fi

    echo "[mineru] not found, bootstrapping with uv ..."

    (
        set -e
        mkdir -p "${default_prefix}"
        cd "${default_prefix}"
        [[ -d "${venv_dir}" ]] || uv venv "${venv_dir}"

        source "${venv_dir}/bin/activate"
        uv pip install -U "mineru[core]" -i https://mirrors.aliyun.com/pypi/simple --extra-index-url https://pypi.org/simple
        deactivate
    )
    export MINERU_EXECUTABLE="${exe}"
    if ! "${MINERU_EXECUTABLE}" --help >/dev/null 2>&1; then
      echo "[mineru] installation failed: ${MINERU_EXECUTABLE} not working" >&2
      return 1
    fi
    echo "[mineru] installed: ${MINERU_EXECUTABLE}"
}
# -----------------------------------------------------------------------------
# Start components based on flags
# -----------------------------------------------------------------------------
ensure_docling
ensure_mineru

if [[ "${ENABLE_WEBSERVER}" -eq 1 ]]; then
    echo "Starting nginx..."
    /usr/sbin/nginx

    echo "Starting ragflow_server..."
    while true; do
        "$PY" api/ragflow_server.py &
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
