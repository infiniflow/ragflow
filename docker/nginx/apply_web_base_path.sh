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
import re
import sys

base, src, dest = sys.argv[1:4]
text = open(src, encoding="utf-8").read()

if f"location {base}/" in text:
    open(dest, "w", encoding="utf-8").write(text)
    sys.exit(0)

replacements = [
    ("location ~ ^/(v1|api)", f"location ~ ^{base}/(v1|api)"),
    ("location ~ ^/(api|v1)", f"location ~ ^{base}/(api|v1)"),
    ("location ~ ^/api", f"location ~ ^{base}/api"),
    ("location ~ ^/v1", f"location ~ ^{base}/v1"),
    ("location ~ ^/static/", f"location ~ ^{base}/static/"),
]
for old, new in replacements:
    text = text.replace(old, new)

spa_old = """    location / {
        index index.html;
        try_files $uri $uri/ /index.html;
    }"""
spa_new = f"""    location {base}/ {{
        alias /ragflow/web/dist/;
        index index.html;
        try_files $uri $uri/ {base}/index.html;
    }}"""
text = text.replace(spa_old, spa_new)

rewrite_line = f"        rewrite ^{base}/(.*)$ /$1 break;"

def patch_prefixed_regex_blocks(content: str) -> str:
    lines = content.splitlines(keepends=True)
    result = []
    in_block = False
    block_has_rewrite = False
    brace_depth = 0
    block_prefix = f"location ~ ^{base}/"

    for line in lines:
        if line.lstrip().startswith(block_prefix):
            in_block = True
            block_has_rewrite = False
            brace_depth = line.count("{") - line.count("}")
            result.append(line)
            continue

        if in_block:
            brace_depth += line.count("{") - line.count("}")
            if rewrite_line.strip() in line:
                block_has_rewrite = True
            if not block_has_rewrite and (
                "proxy_pass" in line or "expires 10y" in line
            ):
                indent = re.match(r"^(\s*)", line).group(1)
                result.append(f"{indent}rewrite ^{base}/(.*)$ /$1 break;\n")
                block_has_rewrite = True
            result.append(line)
            if brace_depth <= 0:
                in_block = False
            continue

        result.append(line)

    return "".join(result)


text = patch_prefixed_regex_blocks(text)

redirect = (
    f"    location = {base} {{\n"
    f"        return 301 {base}/;\n"
    f"    }}"
)
text = text.replace("    server_name _;", f"    server_name _;\n{redirect}", 1)

open(dest, "w", encoding="utf-8").write(text)
PY

mv "$TMP" "$CONF"
echo "Applied RAGFLOW_WEB_BASE_PATH=${BASE} to ${CONF}"
