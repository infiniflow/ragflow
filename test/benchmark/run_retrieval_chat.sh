#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

: "${ZHIPU_AI_API_KEY:?ZHIPU_AI_API_KEY is required}"

BASE_URL="http://127.0.0.1:9380"
LOGIN_EMAIL="qa@infiniflow.org"
LOGIN_PASSWORD="123"
DATASET_PAYLOAD='{"name":"bench_dataset","embedding_model":"BAAI/bge-small-en-v1.5@Builtin"}'
CHAT_PAYLOAD='{"name":"bench_chat","llm":{"model_name":"glm-4-flash@ZHIPU-AI"}}'
DATASET_ID=""

cleanup_dataset() {
  if [[ -z "${DATASET_ID}" ]]; then
    return
  fi
  set +e
  BENCH_BASE_URL="${BASE_URL}" \
  BENCH_LOGIN_EMAIL="${LOGIN_EMAIL}" \
  BENCH_LOGIN_PASSWORD="${LOGIN_PASSWORD}" \
  BENCH_DATASET_ID="${DATASET_ID}" \
  PYTHONPATH="${REPO_ROOT}/test" uv run python - <<'PY'
import os
import sys

from benchmark import auth
from benchmark.auth import AuthError
from benchmark.dataset import delete_dataset
from benchmark.http_client import HttpClient

base_url = os.environ["BENCH_BASE_URL"]
email = os.environ["BENCH_LOGIN_EMAIL"]
password = os.environ["BENCH_LOGIN_PASSWORD"]
dataset_id = os.environ["BENCH_DATASET_ID"]

client = HttpClient(base_url=base_url, api_version="v1")

try:
    password_enc = auth.encrypt_password(password)
    nickname = email.split("@")[0]
    try:
        auth.register_user(client, email, nickname, password_enc)
    except AuthError as exc:
        print(f"Register warning: {exc}", file=sys.stderr)
    login_token = auth.login_user(client, email, password_enc)
    client.login_token = login_token
    client.api_key = auth.create_api_token(client, login_token, None)
    delete_dataset(client, dataset_id)
except Exception as exc:
    print(f"Cleanup warning: failed to delete dataset {dataset_id}: {exc}", file=sys.stderr)
PY
}

trap cleanup_dataset EXIT

retrieval_output="$(PYTHONPATH="${REPO_ROOT}/test" uv run -m benchmark retrieval \
  --base-url "${BASE_URL}" \
  --allow-register \
  --login-email "${LOGIN_EMAIL}" \
  --login-password "${LOGIN_PASSWORD}" \
  --bootstrap-llm \
  --llm-factory ZHIPU-AI \
  --llm-api-key "${ZHIPU_AI_API_KEY}" \
  --dataset-name "bench_dataset" \
  --dataset-payload "${DATASET_PAYLOAD}" \
  --document-path "${SCRIPT_DIR}/test_docs/Doc1.pdf" \
  --document-path "${SCRIPT_DIR}/test_docs/Doc2.pdf" \
  --document-path "${SCRIPT_DIR}/test_docs/Doc3.pdf" \
  --iterations 10 \
  --concurrency 8 \
  --question "What does RAG mean?")"
printf '%s\n' "${retrieval_output}"

DATASET_ID="$(printf '%s\n' "${retrieval_output}" | sed -n 's/^Created Dataset ID: //p' | head -n 1)"
if [[ -z "${DATASET_ID}" ]]; then
  echo "Failed to parse Created Dataset ID from retrieval output." >&2
  exit 1
fi

PYTHONPATH="${REPO_ROOT}/test" uv run -m benchmark chat \
  --base-url "${BASE_URL}" \
  --allow-register \
  --login-email "${LOGIN_EMAIL}" \
  --login-password "${LOGIN_PASSWORD}" \
  --bootstrap-llm \
  --llm-factory ZHIPU-AI \
  --llm-api-key "${ZHIPU_AI_API_KEY}" \
  --dataset-id "${DATASET_ID}" \
  --chat-name "bench_chat" \
  --chat-payload "${CHAT_PAYLOAD}" \
  --message "What is the purpose of RAGFlow?" \
  --model "glm-4-flash@ZHIPU-AI" \
  --iterations 10 \
  --concurrency 8 \
  --teardown
