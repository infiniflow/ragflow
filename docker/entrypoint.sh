#!/usr/bin/env bash

set -e

cd /ragflow

mode=standalone
mode_api=0 mode_admin=0 mode_ingestor=0 mode_syncer=0 mode_chatter=0 mode_deepdoc=0 mode_migrate=0
for arg in "$@"; do
    case "$arg" in
        --api|--admin|--ingestor|--syncer|--chatter|--deepdoc|--migrate|-api|-admin|-ingestor|-syncer|-chatter|-deepdoc|-migrate)
            flag_name="${arg#-}"
            flag_name="${flag_name#-}"
            printf -v "mode_${flag_name}" '%s' 1
            ;;
        --api=*|--admin=*|--ingestor=*|--syncer=*|--chatter=*|--migrate=*|-api=*|-admin=*|-ingestor=*|-syncer=*|-chatter=*|-migrate=*)
            flag_name="${arg%%=*}"
            flag_name="${flag_name#-}"
            flag_name="${flag_name#-}"
            case "${arg#*=}" in
                1|t|T|true|TRUE|True) printf -v "mode_${flag_name}" '%s' 1 ;;
                0|f|F|false|FALSE|False) printf -v "mode_${flag_name}" '%s' 0 ;;
            esac
            ;;
        --help|-h|--version|-v)
            exec bin/ragflow_server "$@"
            ;;
    esac
done
mode_count=0
for flag_name in api admin ingestor syncer chatter deepdoc migrate; do
    flag_var="mode_${flag_name}"
    if [[ "${!flag_var}" == 1 ]]; then
        mode="$flag_name"
        mode_count=$((mode_count + 1))
    fi
done
if (( mode_count > 1 )); then
    echo "Server modes are mutually exclusive" >&2
    exit 1
fi

echo "Start RAGFlow $mode, version: $(cat /ragflow/VERSION)"

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

if [[ "$mode" == "migrate" ]]; then
    db_type="${DB_TYPE:-mysql}"
    db_type="${db_type,,}"
    if [[ "$db_type" == "gaussdb" || "$db_type" == "gauss" ]]; then
        echo "Skipping MySQL-specific model provider table migrations for DB_TYPE=${DB_TYPE:-mysql}."
        exit 0
    fi
fi

if [[ "$mode" == "standalone" ]]; then
    echo "Starting nginx..."
    /usr/sbin/nginx -c /etc/nginx/nginx.conf
fi

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
exec bin/ragflow_server "$@"
