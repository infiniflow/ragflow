#!/bin/bash
set -euo pipefail

log() {
  echo "[ragflow-ob-entrypoint] $*"
}

ob_sys() {
  obclient -h127.0.0.1 -P2881 -uroot@sys -p"${OB_SYS_PASSWORD}" -Doceanbase -A -N -e "$1"
}

ob_tenant_with_password() {
  obclient -h127.0.0.1 -P2881 -uroot@"${OB_TENANT_NAME}" -p"${OB_TENANT_PASSWORD}" -A -N -e "$1"
}

ob_tenant_without_password() {
  obclient -h127.0.0.1 -P2881 -uroot@"${OB_TENANT_NAME}" -A -N -e "$1"
}

wait_for_sys() {
  for _ in $(seq 1 120); do
    if ob_sys "SELECT 1" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

tenant_exists() {
  ob_sys "SELECT 1 FROM oceanbase.DBA_OB_TENANTS WHERE tenant_name='${OB_TENANT_NAME}'" 2>/dev/null | grep -q '^1$'
}

wait_for_tenant_password() {
  for _ in $(seq 1 60); do
    if ob_tenant_with_password "SELECT 1" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

ensure_tenant_ready() {
  if ob_tenant_with_password "SELECT 1" >/dev/null 2>&1; then
    log "tenant ${OB_TENANT_NAME} accepts the configured password"
    return 0
  fi

  if tenant_exists; then
    for _ in $(seq 1 120); do
      if ob_tenant_with_password "SELECT 1" >/dev/null 2>&1; then
        log "tenant ${OB_TENANT_NAME} became reachable with the configured password"
        return 0
      fi

      if ob_tenant_without_password "SELECT 1" >/dev/null 2>&1; then
        log "tenant ${OB_TENANT_NAME} exists with an empty root password; applying OB_TENANT_PASSWORD"
        ob_tenant_without_password "ALTER USER root IDENTIFIED BY '${OB_TENANT_PASSWORD}'" >/dev/null 2>&1 || {
          log "warning: failed to update the tenant root password"
          return 1
        }
        wait_for_tenant_password && return 0
        log "warning: tenant password update did not become effective in time"
        return 1
      fi

      sleep 1
    done

    log "warning: tenant ${OB_TENANT_NAME} exists but never accepted the configured or empty password during reconciliation"
    return 1
  fi

  log "tenant ${OB_TENANT_NAME} is missing; creating it with the configured password"
  if ! obd cluster tenant create obcluster -n "${OB_TENANT_NAME}" -o "${OB_SCENARIO:-htap}" --password "${OB_TENANT_PASSWORD}" >/dev/null 2>&1; then
    log "warning: failed to create tenant ${OB_TENANT_NAME}"
    return 1
  fi

  if wait_for_tenant_password; then
    return 0
  fi

  log "warning: tenant ${OB_TENANT_NAME} did not become connectable in time"
  return 1
}

ensure_database() {
  if ob_tenant_with_password "CREATE DATABASE IF NOT EXISTS \`${OCEANBASE_DOC_DBNAME}\`" >/dev/null 2>&1; then
    log "database ${OCEANBASE_DOC_DBNAME} is ready in tenant ${OB_TENANT_NAME}"
    return 0
  fi

  log "warning: failed to ensure database ${OCEANBASE_DOC_DBNAME}"
  return 1
}

reconcile_oceanbase() {
  if ! wait_for_sys; then
    log "warning: sys tenant never became reachable; skipping reconciliation"
    return 0
  fi

  ensure_tenant_ready || return 0
  ensure_database || return 0
}

/usr/sbin/sshd
/root/boot/start.sh &
start_pid=$!

reconcile_oceanbase || true &

wait "${start_pid}"
