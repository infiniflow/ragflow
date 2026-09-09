#!/usr/bin/env bash
# Smoke-test apply_web_base_path.sh transformations for subpath deployment.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_PATH="/ragflow"
TMP_CONF="$(mktemp)"
trap 'rm -f "$TMP_CONF"' EXIT

cp "${SCRIPT_DIR}/ragflow.conf.python" "$TMP_CONF"
RAGFLOW_NGINX_CONF="$TMP_CONF" RAGFLOW_WEB_BASE_PATH="$BASE_PATH" \
  bash "${SCRIPT_DIR}/apply_web_base_path.sh"

grep -q 'alias /ragflow/web/dist/;' "$TMP_CONF"
grep -q 'location /ragflow/ {' "$TMP_CONF"
grep -q 'rewrite ^/ragflow/(.*)\$ /\$1 break;' "$TMP_CONF"
grep -q 'location ~ \^/ragflow/(v1|api)' "$TMP_CONF"
grep -q 'try_files \$uri \$uri/ /ragflow/index.html' "$TMP_CONF"

# Idempotent when re-run on an already-prefixed config.
COUNT_BEFORE="$(grep -c 'rewrite ^/ragflow/(.*)\$ /\$1 break;' "$TMP_CONF" || true)"
RAGFLOW_NGINX_CONF="$TMP_CONF" RAGFLOW_WEB_BASE_PATH="$BASE_PATH" \
  bash "${SCRIPT_DIR}/apply_web_base_path.sh"
COUNT_AFTER="$(grep -c 'rewrite ^/ragflow/(.*)\$ /\$1 break;' "$TMP_CONF" || true)"
test "$COUNT_BEFORE" -eq "$COUNT_AFTER"
test "$COUNT_AFTER" -eq 3

# Regex locations must escape metacharacters in the base path (e.g. '.').
DOT_CONF="$(mktemp)"
cp "${SCRIPT_DIR}/ragflow.conf.python" "$DOT_CONF"
RAGFLOW_NGINX_CONF="$DOT_CONF" RAGFLOW_WEB_BASE_PATH="/rag.flow" \
  bash "${SCRIPT_DIR}/apply_web_base_path.sh"
grep -F 'location ~ ^/rag\.flow/(v1|api)' "$DOT_CONF"
grep -F 'rewrite ^/rag\.flow/(.*)$ /$1 break;' "$DOT_CONF"
grep -F 'location /rag.flow/ {' "$DOT_CONF"
rm -f "$DOT_CONF"

echo "apply_web_base_path.sh: ok"
