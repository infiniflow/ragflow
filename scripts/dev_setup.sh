#!/bin/bash

# Development environment setup script for RAGFlow
# This script processes the service_conf.yaml.template with your .env.dev variables

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔧 Setting up RAGFlow development environment..."

# Load environment variables from .env.dev
if [ -f ".env.dev" ]; then
    echo "📋 Loading environment variables from .env.dev"
    set -a
    source .env.dev
    set +a
else
    echo "❌ Error: .env.dev file not found"
    exit 1
fi

# Process the template to generate conf/service_conf.yaml
TEMPLATE_FILE="docker/service_conf.yaml.dev.template"
CONF_FILE="conf/service_conf.yaml"

# Validate template file exists and is readable
if [ ! -f "${TEMPLATE_FILE}" ] || [ ! -r "${TEMPLATE_FILE}" ]; then
    echo "❌ Error: template file '${TEMPLATE_FILE}' not found or not readable"
    exit 1
fi

echo "🔄 Processing ${TEMPLATE_FILE} → ${CONF_FILE} (PostgreSQL config)"

rm -f "${CONF_FILE}"
if command -v envsubst >/dev/null 2>&1; then
    envsubst < "${TEMPLATE_FILE}" > "${CONF_FILE}"
else
    echo "⚠️ 'envsubst' not found; using safe awk-based substitution."
    awk '{
        out=$0;
        while (match(out, /\$\{[A-Za-z_][A-Za-z0-9_]*\}/)) {
            var = substr(out, RSTART+2, RLENGTH-3);
            val = ENVIRON[var];
            out = substr(out,1,RSTART-1) val substr(out,RSTART+RLENGTH);
        }
        print out;
    }' "${TEMPLATE_FILE}" > "${CONF_FILE}"
fi

echo "✅ Configuration file generated successfully"
echo ""
echo "📝 Active configuration:"
echo "   PostgreSQL: ${POSTGRES_HOST}:${POSTGRES_PORT}"
echo "   Redis: ${REDIS_HOST}:${REDIS_PORT}"
echo "   Elasticsearch: ${ES_HOST}:${ES_PORT}"
echo "   MinIO: ${MINIO_HOST}:${MINIO_PORT}"
echo ""
echo "🚀 To start RAGFlow backend, run:"
echo ""
echo "   # Terminal 1 - Task Executor:"
echo "   source .venv/bin/activate"
echo "   export PYTHONPATH=\$(pwd)"
echo "   JEMALLOC_PATH=\$(pkg-config --variable=libdir jemalloc)/libjemalloc.so"
echo "   LD_PRELOAD=\$JEMALLOC_PATH python rag/svr/task_executor.py 1"
echo ""
echo "   # Terminal 2 - API Server:"
echo "   source .venv/bin/activate"
echo "   export PYTHONPATH=\$(pwd)"
echo "   python api/ragflow_server.py"
echo ""
