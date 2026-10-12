#!/bin/bash
set -euo pipefail

log() {
  echo "[ragflow-ob-entrypoint] $*"
}

# obclient has no bind-parameter/prepared-statement mechanism at the shell
# level -- every statement is one pre-built string passed to `-e` -- so each
# variable that reaches SQL text is made safe using the tool that matches its
# position:
#
#   * Identifiers (e.g. the database name backtick-quoted in CREATE
#     DATABASE): validated against a strict allowlist; SQL is never built
#     from an invalid value. Quote/backslash escaping is the wrong tool for
#     identifier position and is not used here.
#   * Values in a general SQL expression (e.g. the tenant name in a WHERE
#     clause): encoded as a hex binary-string literal (x'...'), which is
#     safe regardless of sql_mode because its body is restricted to
#     [0-9a-f] by the lexer -- any byte sequence can be embedded as data
#     without rejecting it.
#   * Values that must be a quoted string literal because the surrounding
#     grammar only accepts a literal token there (ALTER USER ... IDENTIFIED
#     BY 'auth_string' does not accept a hex literal or any expression) are
#     escaped using the convention that matches the session's actual
#     sql_mode, which is queried first rather than assumed.

is_valid_sql_identifier() {
  local name="$1"
  [[ "$name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || return 1
  local len=${#name}
  (( len >= 1 && len <= 64 ))
}

require_valid_sql_identifier() {
  local label="$1" name="$2"
  if ! is_valid_sql_identifier "$name"; then
    log "error: ${label}='${name}' is not a safe SQL identifier (letters/digits/underscore, cannot start with a digit, max 64 chars); refusing to build SQL"
    return 1
  fi
}

# Encodes $1 as a MySQL/OceanBase x'<hex>' binary-string literal. Operates on
# raw bytes (LC_ALL=C) so multi-byte UTF-8 round-trips correctly, and masks
# each byte with & 0xff because bash's `printf '%d' "'c"` sign-extends
# characters with the high bit set.
sql_hex_literal() {
  local value="$1"
  local LC_ALL=C
  local length=${#value}
  local i dec byte hex=""
  for ((i = 0; i < length; i++)); do
    printf -v dec '%d' "'${value:i:1}"
    printf -v byte '%02x' "$(( dec & 0xff ))"
    hex+="$byte"
  done
  printf "x'%s'" "$hex"
}

# Escapes $1 for use inside a single-quoted SQL string literal, given the
# connection's actual $2 (@@SESSION.sql_mode). Doubling the quote character
# ('') is a context-free SQL escape valid in every sql_mode. Doubling
# backslash is required only when backslash is the active escape character;
# doing it unconditionally would corrupt the value under NO_BACKSLASH_ESCAPES,
# and omitting it when backslash IS the escape character would let a trailing
# backslash swallow the closing quote and reopen the literal.
sql_escape_literal_for_mode() {
  local value="$1" mode="$2"
  local LC_ALL=C
  local length=${#value}
  local i c out="" no_backslash=0
  [[ "$mode" == *NO_BACKSLASH_ESCAPES* ]] && no_backslash=1
  for ((i = 0; i < length; i++)); do
    c="${value:i:1}"
    if [[ "$c" == "'" ]]; then
      out+="''"
    elif [[ "$c" == '\' && "$no_backslash" -eq 0 ]]; then
      out+='\\'
    else
      out+="$c"
    fi
  done
  printf '%s' "$out"
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
  local tenant_literal
  tenant_literal="$(sql_hex_literal "${OB_TENANT_NAME}")"
  ob_sys "SELECT 1 FROM oceanbase.DBA_OB_TENANTS WHERE tenant_name=${tenant_literal}" 2>/dev/null | grep -q '^1$'
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

        local sql_mode password_literal
        if ! sql_mode="$(ob_tenant_without_password "SELECT @@SESSION.sql_mode" 2>/dev/null)"; then
          log "warning: unable to read sql_mode; refusing to build the ALTER USER statement without knowing the active escaping convention"
          return 1
        fi
        password_literal="$(sql_escape_literal_for_mode "${OB_TENANT_PASSWORD}" "$sql_mode")"

        ob_tenant_without_password "ALTER USER root IDENTIFIED BY '${password_literal}'" >/dev/null 2>&1 || {
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
  require_valid_sql_identifier "OCEANBASE_DOC_DBNAME" "${OCEANBASE_DOC_DBNAME}" || return 1

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

main() {
  /usr/sbin/sshd
  /root/boot/start.sh &
  local start_pid=$!

  reconcile_oceanbase || true &

  wait "${start_pid}"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
