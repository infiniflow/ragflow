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

# -----------------------------------------------------------------------------
# Install PyTorch with CUDA support when DEVICE=gpu
# Auto-detects CUDA version based on NVIDIA driver
# -----------------------------------------------------------------------------

# Map NVIDIA driver major version to CUDA version
# Driver >= 550 -> cu124 (RTX 40/50 series)
# Driver >= 525 -> cu121 (RTX 30/40 series)
# Driver >= 450 -> cu118 (GTX 10/16/20 series)
function get_cuda_version_from_driver() {
    local major_version="$1"
    if [[ "$major_version" -ge 550 ]]; then
        echo "cu124"
    elif [[ "$major_version" -ge 525 ]]; then
        echo "cu121"
    elif [[ "$major_version" -ge 450 ]]; then
        echo "cu118"
    else
        echo "cu124"  # default for unknown/newer drivers
    fi
}

function ensure_torch_cuda() {
    [[ "${DEVICE}" == "gpu" ]] || { echo "[torch] DEVICE is not gpu, skipping CUDA PyTorch installation"; return 0; }
    
    # Check if torch with CUDA is already installed
    if python3 -c "import torch; exit(0 if torch.cuda.is_available() else 1)" 2>/dev/null; then
        echo "[torch] PyTorch with CUDA support already installed"
        return 0
    fi
    
    echo "[torch] Installing PyTorch with CUDA support..."
    
    # Auto-detect CUDA version from NVIDIA driver
    local cuda_version="cu124"  # default
    local driver_version=""
    local major_version=""
    
    if command -v nvidia-smi &> /dev/null; then
        driver_version=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null | head -n1)
    elif [[ -f /proc/driver/nvidia/version ]]; then
        driver_version=$(grep -oP 'Kernel Module\s+\K[\d.]+' /proc/driver/nvidia/version 2>/dev/null)
    fi
    
    if [[ -n "$driver_version" ]]; then
        major_version=$(echo "$driver_version" | cut -d. -f1)
        echo "[torch] Detected NVIDIA driver version: $driver_version (major: $major_version)"
        cuda_version=$(get_cuda_version_from_driver "$major_version")
    fi
    
    echo "[torch] Using CUDA version: $cuda_version"
    local index_url="https://download.pytorch.org/whl/${cuda_version}"
    local cn_mirror_url="https://mirrors.aliyun.com/pytorch-wheels/${cuda_version}"
    
    # Install using uv (preferred) or pip, try Chinese mirror first
    if command -v uv &> /dev/null; then
        uv pip install "torch>=2.5.0,<3.0.0" --index-url "$cn_mirror_url" 2>/dev/null || \
        uv pip install "torch>=2.5.0,<3.0.0" --index-url "$index_url" || \
        python3 -m pip install "torch>=2.5.0,<3.0.0" --index-url "$index_url"
    else
        python3 -m pip install "torch>=2.5.0,<3.0.0" --index-url "$cn_mirror_url" 2>/dev/null || \
        python3 -m pip install "torch>=2.5.0,<3.0.0" --index-url "$index_url"
    fi
    
    # Verify installation
    if python3 -c "import torch; print(f'PyTorch {torch.__version__} installed, CUDA available: {torch.cuda.is_available()}')" 2>/dev/null; then
        echo "[torch] PyTorch with CUDA support installed successfully"
    else
        echo "[torch] Warning: PyTorch installation may have issues"
    fi
}

# -----------------------------------------------------------------------------
# Start components based on flags
# -----------------------------------------------------------------------------
ensure_torch_cuda
ensure_docling

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
