#!/usr/bin/env bash

set -e

cd /ragflow

mode=standalone
for arg in "$@"; do
    case "$arg" in
        --api|--admin|--ingestor|--syncer|--chatter|--deepdoc|--migrate)
            mode="${arg#--}"
            ;;
        --help|-h|--version|-v)
            exec bin/ragflow_server "$@"
            ;;
    esac
done

echo "Start RAGFlow, version: "
cat /ragflow/VERSION

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

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"

if [[ "$mode" == "migrate" ]]; then
    db_type="${DB_TYPE:-mysql}"
    db_type="${db_type,,}"
    if [[ "$db_type" == "gaussdb" || "$db_type" == "gauss" ]]; then
        echo "Skipping MySQL-specific model provider table migrations for DB_TYPE=${DB_TYPE:-mysql}."
        exit 0
    fi
fi

if [[ "$mode" == "api" || "$mode" == "standalone" ]]; then
    echo "Starting nginx..."
    sed -i "s|http://127.0.0.1:9381|http://${RAGFLOW_ADMIN_HOST:-127.0.0.1}:${RAGFLOW_ADMIN_HTTP_PORT:-9381}|g" /etc/nginx/conf.d/ragflow.conf
    env -u LD_LIBRARY_PATH /usr/sbin/nginx -c /etc/nginx/nginx.conf
fi

exec bin/ragflow_server "$@"
