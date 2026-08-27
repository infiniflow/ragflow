#!/usr/bin/env bash
# Prefix nginx location blocks with RAGFLOW_WEB_BASE_PATH for subpath deployments.
set -euo pipefail

BASE="${RAGFLOW_WEB_BASE_PATH:-}"
BASE="${BASE%/}"

if [[ -z "$BASE" ]]; then
  exit 0
fi

CONF="${RAGFLOW_NGINX_CONF:-/etc/nginx/conf.d/ragflow.conf}"
if [[ ! -f "$CONF" ]]; then
  echo "nginx config not found: $CONF" >&2
  exit 1
fi

TMP="$(mktemp)"
python3 - "$BASE" "$CONF" "$TMP" <<'PY'
import sys

base, src, dest = sys.argv[1:4]
text = open(src, encoding="utf-8").read()
replacements = [
    ("location ~ ^/(api|v1)", "location ~ ^%s/(api|v1)" % base),
    ("location ~ ^/(v1|api)", "location ~ ^%s/(v1|api)" % base),
    ("location ~ ^/api", "location ~ ^%s/api" % base),
    ("location / {", "location %s/ {" % base),
    ("try_files $uri $uri/ /index.html", "try_files $uri $uri/ %s/index.html" % base),
    ("location ~ ^/static/", "location ~ ^%s/static/" % base),
]
for old, new in replacements:
    text = text.replace(old, new)
redirect = (
    "    location = %s {\n"
    "        return 301 %s/;\n"
    "    }"
) % (base, base)
text = text.replace("    server_name _;", "    server_name _;\n%s" % redirect, 1)
open(dest, "w", encoding="utf-8").write(text)
PY

mv "$TMP" "$CONF"
echo "Applied RAGFLOW_WEB_BASE_PATH=${BASE} to ${CONF}"
