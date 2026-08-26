#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

: "${ZHIPU_AI_API_KEY:?ZHIPU_AI_API_KEY is required}"

PYTHONPATH="${REPO_ROOT}/test" uv run -m benchmark retrieval \
  --base-url http://127.0.0.1:9380 \
  --allow-register \
  --login-email "qa@infiniflow.org" \
  --login-password "123" \
  --bootstrap-llm \
  --llm-factory ZHIPU-AI \
  --llm-api-key "$ZHIPU_AI_API_KEY" \
  --dataset-name "bench_dataset" \
  --dataset-payload '{"name":"bench_dataset","embedding_model":"BAAI/bge-small-en-v1.5@Builtin"}' \
  --document-path "${SCRIPT_DIR}/test_docs/Doc1.pdf" \
  --document-path "${SCRIPT_DIR}/test_docs/Doc2.pdf" \
  --document-path "${SCRIPT_DIR}/test_docs/Doc3.pdf" \
  --question "What does RAG mean?" \
  --iterations 10 \
  --concurrency 8 \
  --teardown
