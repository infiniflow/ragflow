#!/usr/bin/env bash

set -e

cd /ragflow

echo "Start RAGFlow, version: $(cat /ragflow/VERSION)"

# Replace env variables in the service_conf.yaml file.
CONF_DIR="/ragflow/conf"
TEMPLATE_FILE="${CONF_DIR}/service_conf.yaml.template"
CONF_FILE="${CONF_DIR}/service_conf.yaml"

rm -f "${CONF_FILE}"
DEF_ENV_VALUE_PATTERN="\$\{([^:]+):-([^}]+)\}"
while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ DEF_ENV_VALUE_PATTERN ]]; then
        varname="${BASH_REMATCH[1]}"
        default="${BASH_REMATCH[2]}"

        if [ -n "${!varname}" ]; then
            eval "echo \"$line"\" >> "${CONF_FILE}"
        else
            echo "$line" | sed -E "s/\\\$\{[^:]+:-([^}]+)\}/\1/g" >> "${CONF_FILE}"
        fi
    else
        eval "echo \"$line\"" >> "${CONF_FILE}"
    fi
done < "${TEMPLATE_FILE}"

case "${RAGFLOW_WEB:-on}" in
    on)
        echo "Starting nginx..."
        /usr/sbin/nginx -c /etc/nginx/nginx.conf
        ;;
    off) ;;
    only)
        if (( $# > 0 )); then
            echo "RAGFLOW_WEB=only does not accept server arguments" >&2
            exit 1
        fi
        exec /usr/sbin/nginx -c /etc/nginx/web.conf -g 'daemon off;'
        ;;
    *)
        echo "RAGFLOW_WEB must be on, off, or only" >&2
        exit 1
        ;;
esac

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
exec bin/ragflow_server "$@"
