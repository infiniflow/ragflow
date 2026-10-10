#!/usr/bin/env bash
# Smoke-test apply_web_base_path.sh transformations for subpath deployment.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_PATH="/ragflow"
REWRITE_PATTERN='rewrite ^/ragflow/(.*)\$ /\$1 break;'

assert_template() {
  local template="$1"
  local expected_rewrites="$2"
  local tmp_conf
  tmp_conf="$(mktemp)"

  cp "${SCRIPT_DIR}/${template}" "$tmp_conf"
  RAGFLOW_NGINX_CONF="$tmp_conf" RAGFLOW_WEB_BASE_PATH="$BASE_PATH" \
    bash "${SCRIPT_DIR}/apply_web_base_path.sh"

  grep -q 'alias /ragflow/web/dist/;' "$tmp_conf"
  grep -q 'location /ragflow/ {' "$tmp_conf"
  grep -q "$REWRITE_PATTERN" "$tmp_conf"
  grep -q 'location ~ \^/ragflow/api/v1' "$tmp_conf"
  grep -q 'location ~ \^/ragflow/api/v1/admin' "$tmp_conf"
  grep -q 'try_files \$uri \$uri/ /ragflow/index.html' "$tmp_conf"
  grep -q 'location = /ragflow {' "$tmp_conf"

  local rewrite_count
  rewrite_count="$(grep -c "$REWRITE_PATTERN" "$tmp_conf" || true)"
  test "$rewrite_count" -eq "$expected_rewrites"

  # Every prefixed proxy/static regex block must strip the base path.
  python3 - "$tmp_conf" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()
blocks = re.findall(r"location ~ \^/ragflow/.*?\{(.*?)\n\s*\}", text, re.S)
assert blocks, "expected prefixed regex locations"
for body in blocks:
    if "proxy_pass" in body or "expires 10y" in body:
        assert "rewrite ^/ragflow/(.*)$ /$1 break;" in body
PY

  # Idempotent when re-run on an already-prefixed config.
  local count_before count_after
  count_before="$rewrite_count"
  RAGFLOW_NGINX_CONF="$tmp_conf" RAGFLOW_WEB_BASE_PATH="$BASE_PATH" \
    bash "${SCRIPT_DIR}/apply_web_base_path.sh"
  count_after="$(grep -c "$REWRITE_PATTERN" "$tmp_conf" || true)"
  test "$count_before" -eq "$count_after"

  rm -f "$tmp_conf"
}

# Current nginx template has admin + API proxy locations plus the static cache block.
assert_template ragflow.conf 3

# Regex locations must escape metacharacters in the base path (e.g. '.').
DOT_CONF="$(mktemp)"
cp "${SCRIPT_DIR}/ragflow.conf" "$DOT_CONF"
RAGFLOW_NGINX_CONF="$DOT_CONF" RAGFLOW_WEB_BASE_PATH="/rag.flow" \
  bash "${SCRIPT_DIR}/apply_web_base_path.sh"
grep -F 'location ~ ^/rag\.flow/api/v1' "$DOT_CONF"
grep -F 'rewrite ^/rag\.flow/(.*)$ /$1 break;' "$DOT_CONF"
grep -F 'location /rag.flow/ {' "$DOT_CONF"
rm -f "$DOT_CONF"

echo "apply_web_base_path.sh: ok"
