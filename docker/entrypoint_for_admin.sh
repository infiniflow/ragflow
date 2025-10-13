#!/usr/bin/env bash

set -e

# -----------------------------------------------------------------------------
# Host ID logic:
#   1. By default, use the system hostname if length <= 32
#   2. Otherwise, use the full MD5 hash of the hostname (32 hex chars)
# -----------------------------------------------------------------------------
CURRENT_HOSTNAME="$(hostname)"
if [ ${#CURRENT_HOSTNAME} -le 32 ]; then
  DEFAULT_HOST_ID="$CURRENT_HOSTNAME"
else
  DEFAULT_HOST_ID="$(echo -n "$CURRENT_HOSTNAME" | md5sum | cut -d ' ' -f 1)"
fi

HOST_ID="$DEFAULT_HOST_ID"

# -----------------------------------------------------------------------------
# Replace env variables in the service_conf.yaml file
# -----------------------------------------------------------------------------
CONF_DIR="/ragflow/conf"
TEMPLATE_FILE="${CONF_DIR}/service_conf.yaml.template"
CONF_FILE="${CONF_DIR}/service_conf.yaml"

rm -f "${CONF_FILE}"
while IFS= read -r line || [[ -n "$line" ]]; do
    eval "echo \"$line\"" >> "${CONF_FILE}"
done < "${TEMPLATE_FILE}"

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"
PY=python3

# -----------------------------------------------------------------------------
# Start components
# -----------------------------------------------------------------------------


echo "Starting admin_server..."
while true; do
  "$PY" admin/admin_server.py
  done &

wait
