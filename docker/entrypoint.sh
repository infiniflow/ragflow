#!/usr/bin/env bash

set -e

cd /ragflow

echo "Start RAGFlow $@, version: $(cat /ragflow/VERSION)"

render_service_conf() {
    local -A input_env=()
    local entry name
    while IFS= read -r -d '' entry; do
        name="${entry%%=*}"
        if [[ -n "$name" ]]; then
            input_env["$name"]="${entry#*=}"
        fi
    done

    # Expand environment placeholders without reinterpreting the YAML around them.
    local pattern='[$][{]([a-zA-Z_][a-zA-Z0-9_]*)(:-([^}]*))?[}]|[$]([a-zA-Z_][a-zA-Z0-9_]*)'
    local line rendered placeholder varname value
    while IFS= read -r line || [[ -n "$line" ]]; do
        rendered=""
        while [[ "$line" =~ $pattern ]]; do
            placeholder="${BASH_REMATCH[0]}"
            varname="${BASH_REMATCH[1]:-${BASH_REMATCH[4]}}"
            value="${input_env[$varname]}"
            rendered+="${line%%"$placeholder"*}${value:-${BASH_REMATCH[3]}}"
            line="${line#*"$placeholder"}"
        done
        printf '%s%s\n' "$rendered" "$line"
    done < "$1"
}

# Capture the environment before the renderer creates any temporary variables.
rm -f conf/service_conf.yaml
render_service_conf conf/service_conf.yaml.template < <(command env -0) > conf/service_conf.yaml

if [ "$#" -eq 0 ]; then
    echo "Starting nginx..."
    /usr/sbin/nginx -c /etc/nginx/nginx.conf
fi

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
exec bin/ragflow_server "$@"
