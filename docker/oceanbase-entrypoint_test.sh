#!/usr/bin/env bash
# Framework-free regression test for docker/oceanbase-entrypoint.sh.
#
# Sources the entrypoint (its top-level side effects are guarded behind
# `[[ "${BASH_SOURCE[0]}" == "${0}" ]]`, so sourcing it only defines
# functions) and replaces `obclient` and `sleep` with shell functions --
# bash resolves functions before any PATH binary within the same shell, so
# no real OceanBase connection is ever made and no test waits on a timer.
#
# Run with: bash docker/oceanbase-entrypoint_test.sh

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export OB_SYS_PASSWORD="unused"
export OB_TENANT_PASSWORD="unused"
export OB_TENANT_NAME="ragflow"
export OCEANBASE_DOC_DBNAME="ragflow_doc"

# shellcheck source=./oceanbase-entrypoint.sh
source "${SCRIPT_DIR}/oceanbase-entrypoint.sh"

# The entrypoint's own `set -euo pipefail` (line 2) is now active in this
# shell too, since `source` runs it in the caller's context. Turn errexit
# back off so a function under test returning non-zero (e.g. the rejection
# cases below) doesn't abort this test script instead of being asserted on.
set +e +o pipefail

FAILURES=0
pass() { echo "ok   - $1"; }
fail() { echo "FAIL - $1"; FAILURES=$((FAILURES + 1)); }

sleep() { :; }

# tenant_exists() pipes obclient's output into `grep`, and the sql_mode
# lookup runs obclient inside a command substitution -- both fork a
# subshell in bash, so any mock state obclient writes to a plain shell
# variable during those specific calls would be lost. Route state that must
# survive through a temp-file instead; it's the only mechanism that crosses
# a subshell boundary reliably.
MOCK_STATE_DIR="$(mktemp -d)"
trap 'rm -rf "$MOCK_STATE_DIR"' EXIT

MOCK_SQL_MODE="STRICT_TRANS_TABLES"
MOCK_WITH_PW_SELECT1_EXIT=0
MOCK_WITHOUT_PW_SELECT1_EXIT=0
MOCK_TENANT_EXISTS=1
MOCK_WRITE_EXIT=0
MOCK_PASSWORD_APPLIED=0

obclient() {
  echo "x" >>"$MOCK_STATE_DIR/calls"
  local prev="" sql="" has_password=0 arg
  for arg in "$@"; do
    [[ "$prev" == "-e" ]] && sql="$arg"
    case "$arg" in
      -p*) has_password=1 ;;
    esac
    prev="$arg"
  done

  case "$sql" in
    *"@@SESSION.sql_mode"*)
      printf '%s\n' "$MOCK_SQL_MODE"
      return 0
      ;;
    *"DBA_OB_TENANTS"*)
      printf '%s' "$sql" >"$MOCK_STATE_DIR/tenant_query_sql"
      [[ "$MOCK_TENANT_EXISTS" == "1" ]] && printf '1\n'
      return 0
      ;;
    *"ALTER USER"*)
      printf '%s' "$sql" >"$MOCK_STATE_DIR/alter_sql"
      if [[ "$MOCK_WRITE_EXIT" == "0" ]]; then
        MOCK_PASSWORD_APPLIED=1
      fi
      return "$MOCK_WRITE_EXIT"
      ;;
    *"CREATE DATABASE"*)
      printf '%s' "$sql" >"$MOCK_STATE_DIR/create_db_sql"
      return "$MOCK_WRITE_EXIT"
      ;;
    *"SELECT 1"*)
      if [[ "$has_password" == "1" ]]; then
        if [[ "$MOCK_PASSWORD_APPLIED" == "1" ]]; then
          return 0
        fi
        return "$MOCK_WITH_PW_SELECT1_EXIT"
      fi
      return "$MOCK_WITHOUT_PW_SELECT1_EXIT"
      ;;
    *)
      return 0
      ;;
  esac
}

mock_call_count() {
  if [[ -f "$MOCK_STATE_DIR/calls" ]]; then
    wc -l <"$MOCK_STATE_DIR/calls" | tr -d ' '
  else
    echo 0
  fi
}

reset_mock() {
  MOCK_SQL_MODE="STRICT_TRANS_TABLES"
  MOCK_WITH_PW_SELECT1_EXIT=0
  MOCK_WITHOUT_PW_SELECT1_EXIT=0
  MOCK_TENANT_EXISTS=1
  MOCK_WRITE_EXIT=0
  MOCK_PASSWORD_APPLIED=0
  rm -f "$MOCK_STATE_DIR"/calls "$MOCK_STATE_DIR"/tenant_query_sql "$MOCK_STATE_DIR"/alter_sql "$MOCK_STATE_DIR"/create_db_sql
}

# Drives ensure_tenant_ready() down the "tenant exists, empty root
# password, apply OB_TENANT_PASSWORD" branch so ALTER USER is reached.
drive_to_alter_user() {
  MOCK_WITH_PW_SELECT1_EXIT=1
  MOCK_WITHOUT_PW_SELECT1_EXIT=0
  MOCK_TENANT_EXISTS=1
  ensure_tenant_ready >/dev/null
}

# normal_tenant: query succeeds, built as a mode-independent hex literal.
test_tenant_exists_normal() {
  reset_mock
  OB_TENANT_NAME="ragflow"
  tenant_exists
  local rc=$? sql
  sql="$(cat "$MOCK_STATE_DIR/tenant_query_sql" 2>/dev/null)"
  if [[ $rc -eq 0 && "$sql" == *"tenant_name=x'726167666c6f77'"* ]]; then
    pass "tenant_exists: normal_tenant"
  else
    fail "tenant_exists: normal_tenant -> rc=$rc sql=$sql"
  fi
}

# tenant'quoted: input stays data, SQL structure (hex literal) unchanged.
test_tenant_exists_quoted() {
  reset_mock
  OB_TENANT_NAME="tenant'quoted"
  tenant_exists
  local rc=$? sql
  sql="$(cat "$MOCK_STATE_DIR/tenant_query_sql" 2>/dev/null)"
  if [[ $rc -eq 0 && "$sql" =~ ^SELECT\ 1\ FROM\ oceanbase\.DBA_OB_TENANTS\ WHERE\ tenant_name=x\'[0-9a-f]+\'$ ]]; then
    pass "tenant_exists: tenant'quoted stays data"
  else
    fail "tenant_exists: tenant'quoted -> rc=$rc sql=$sql"
  fi
}

# A value containing backslashes: ALTER USER escapes correctly under the
# default sql_mode (backslash is the active escape character).
test_alter_user_backslash_default_mode() {
  reset_mock
  MOCK_SQL_MODE="STRICT_TRANS_TABLES"
  OB_TENANT_NAME="ragflow"
  OB_TENANT_PASSWORD='pa\ssword'
  drive_to_alter_user
  local sql
  sql="$(cat "$MOCK_STATE_DIR/alter_sql" 2>/dev/null)"
  if [[ "$sql" == "ALTER USER root IDENTIFIED BY 'pa\\\\ssword'" ]]; then
    pass "ALTER USER: backslash under default sql_mode"
  else
    fail "ALTER USER: backslash under default sql_mode -> sql=$sql"
  fi
}

# A value containing backslashes under NO_BACKSLASH_ESCAPES: backslash is
# left alone (it has no escape meaning in this mode).
test_alter_user_backslash_no_backslash_escapes_mode() {
  reset_mock
  MOCK_SQL_MODE="NO_BACKSLASH_ESCAPES"
  OB_TENANT_NAME="ragflow"
  OB_TENANT_PASSWORD='pa\ssword'
  drive_to_alter_user
  local sql
  sql="$(cat "$MOCK_STATE_DIR/alter_sql" 2>/dev/null)"
  if [[ "$sql" == "ALTER USER root IDENTIFIED BY 'pa\ssword'" ]]; then
    pass "ALTER USER: backslash under NO_BACKSLASH_ESCAPES"
  else
    fail "ALTER USER: backslash under NO_BACKSLASH_ESCAPES -> sql=$sql"
  fi
}

# A password containing quotes: password update preserves the intended
# value (quote doubled, statement structure unchanged).
test_alter_user_password_with_quote() {
  reset_mock
  MOCK_SQL_MODE="STRICT_TRANS_TABLES"
  OB_TENANT_NAME="ragflow"
  OB_TENANT_PASSWORD="o'brien"
  drive_to_alter_user
  local sql
  sql="$(cat "$MOCK_STATE_DIR/alter_sql" 2>/dev/null)"
  if [[ "$sql" == "ALTER USER root IDENTIFIED BY 'o''brien'" ]]; then
    pass "ALTER USER: password containing a quote"
  else
    fail "ALTER USER: password containing a quote -> sql=$sql"
  fi
}

# A malicious-looking tenant name: never rejected (tenant_exists() has no
# identifier gate -- OB_TENANT_NAME only ever reaches SQL as hex-literal
# data), and it never breaks out of the hex literal.
test_tenant_exists_malicious_payload_stays_data() {
  reset_mock
  OB_TENANT_NAME="x'; DROP DATABASE oceanbase; --"
  tenant_exists
  local rc=$? sql
  sql="$(cat "$MOCK_STATE_DIR/tenant_query_sql" 2>/dev/null)"
  if [[ "$sql" =~ ^SELECT\ 1\ FROM\ oceanbase\.DBA_OB_TENANTS\ WHERE\ tenant_name=x\'[0-9a-f]+\'$ ]]; then
    pass "tenant_exists: malicious-looking tenant name stays hex-encoded data (rc=$rc)"
  else
    fail "tenant_exists: malicious-looking tenant name -> rc=$rc sql=$sql"
  fi
}

# An invalid database identifier: rejected before SQL execution.
test_invalid_dbname_rejected() {
  reset_mock
  OB_TENANT_NAME="ragflow"
  OCEANBASE_DOC_DBNAME='ragflow`; DROP DATABASE x; --'
  ensure_database
  local rc=$?
  if [[ $rc -ne 0 && $(mock_call_count) -eq 0 ]]; then
    pass "ensure_database: invalid database identifier rejected before any obclient call"
  else
    fail "ensure_database: invalid database identifier -> rc=$rc calls=$(mock_call_count)"
  fi
}

for t in \
  test_tenant_exists_normal \
  test_tenant_exists_quoted \
  test_alter_user_backslash_default_mode \
  test_alter_user_backslash_no_backslash_escapes_mode \
  test_alter_user_password_with_quote \
  test_tenant_exists_malicious_payload_stays_data \
  test_invalid_dbname_rejected; do
  "$t"
done

echo "---"
if (( FAILURES == 0 )); then
  echo "all tests passed"
else
  echo "$FAILURES test(s) failed"
  exit 1
fi
