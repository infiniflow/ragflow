#!/usr/bin/env bash

set -e

cd /ragflow

echo "Start RAGFlow $@, version: $(cat /ragflow/VERSION)"

# Replace env variables in the service_conf.yaml file.
CONF_DIR="/ragflow/conf"
TEMPLATE_FILE="${CONF_DIR}/service_conf.yaml.template"
CONF_FILE="${CONF_DIR}/service_conf.yaml"

rm -f "${CONF_FILE}"
# Expand environment placeholders without reinterpreting the YAML around them.
ENV_PLACEHOLDER_PATTERN='[$][{]([a-zA-Z_][a-zA-Z0-9_]*)(:-([^}]*))?[}]|[$]([a-zA-Z_][a-zA-Z0-9_]*)'
while IFS= read -r line || [[ -n "$line" ]]; do
    rendered=""
    while [[ "$line" =~ $ENV_PLACEHOLDER_PATTERN ]]; do
        placeholder="${BASH_REMATCH[0]}"
        varname="${BASH_REMATCH[1]:-${BASH_REMATCH[4]}}"
        value="${!varname}"
        rendered+="${line%%"$placeholder"*}${value:-${BASH_REMATCH[3]}}"
        line="${line#*"$placeholder"}"
    done
    printf '%s%s\n' "$rendered" "$line"
done < "${TEMPLATE_FILE}" > "${CONF_FILE}"

if [ "$#" -eq 0 ]; then
    echo "Starting nginx..."
    /usr/sbin/nginx -c /etc/nginx/nginx.conf
fi

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
exec bin/ragflow_server "$@"
