#!/usr/bin/env bash

# Upgrade helper for the Go ingestion event-log rollout.
#
# The script is deliberately dry-run by default. It does not stop or start
# services on behalf of the operator: the six-step cutover requires the
# deployment's own supervisor/container commands, and silently guessing those
# commands could leave an old writer running while the tables are cleared.
#
# Usage:
#   tools/scripts/upgrade_ingestion_event_log.sh [--dry-run]
#   tools/scripts/upgrade_ingestion_event_log.sh --execute --yes --writers-stopped
#
# Execute-mode connection variables (matching the compose/.env names):
#   MYSQL_HOST (default 127.0.0.1), MYSQL_PORT (default 3306),
#   MYSQL_USER (default root), MYSQL_PASSWORD (default empty),
#   MYSQL_DATABASE (default rag_flow), MYSQL_BIN (default mysql).
#   RAGFLOW_BIN (default bin/ragflow_server) is started with --migrate after
#   the SQL cutover.

set -Eeuo pipefail

usage() {
    cat >&2 <<'EOF'
Usage:
  upgrade_ingestion_event_log.sh [--dry-run]
  upgrade_ingestion_event_log.sh --execute --yes --writers-stopped

Options:
  --dry-run          Print the six-step plan and SQL (the default).
  --execute          Run the checks, SQL cleanup, and binary migration.
  --yes              Required with --execute; confirms destructive table cleanup.
  --writers-stopped  Required with --execute; confirms every Go writer is stopped.
  --mysql-bin PATH   MySQL client executable (default: $MYSQL_BIN or mysql).
  -h, --help         Show this help.
EOF
}

mode="dry-run"
confirmed=0
writers_stopped=0
mysql_bin="${MYSQL_BIN:-mysql}"

while (($# > 0)); do
    case "$1" in
        --dry-run)
            mode="dry-run"
            shift
            ;;
        --execute)
            mode="execute"
            shift
            ;;
        --yes)
            confirmed=1
            shift
            ;;
        --writers-stopped)
            writers_stopped=1
            shift
            ;;
        --mysql-bin)
            if (($# < 2)) || [[ -z "$2" ]] || [[ "$2" == -* ]]; then
                echo "--mysql-bin requires an executable path" >&2
                exit 2
            fi
            mysql_bin="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ "$mode" == "execute" ]]; then
    if ((confirmed == 0)); then
        echo "refusing execute mode without --yes (table cleanup is destructive)" >&2
        exit 2
    fi
    if ((writers_stopped == 0)); then
        echo "refusing execute mode without --writers-stopped" >&2
        exit 2
    fi
    if ! command -v "$mysql_bin" >/dev/null 2>&1 && [[ ! -x "$mysql_bin" ]]; then
        echo "MySQL client not found: $mysql_bin" >&2
        exit 127
    fi
fi

mysql_host="${MYSQL_HOST:-127.0.0.1}"
mysql_port="${MYSQL_PORT:-3306}"
mysql_user="${MYSQL_USER:-root}"
mysql_database="${MYSQL_DATABASE:-rag_flow}"
mysql_password="${MYSQL_PASSWORD:-}"
ragflow_bin="${RAGFLOW_BIN:-bin/ragflow_server}"

status_sql="SELECT status, COUNT(*) FROM ingestion_task GROUP BY status ORDER BY status;"
non_terminal_sql="SELECT COUNT(*) FROM ingestion_task WHERE status NOT IN ('COMPLETED','FAILED','STOPPED');"
settle_sql="UPDATE pipeline_operation_log pol JOIN ingestion_task task ON task.pipeline_log_id = pol.id SET pol.operation_status = CASE task.status WHEN 'COMPLETED' THEN 'DONE' WHEN 'FAILED' THEN 'FAIL' WHEN 'STOPPED' THEN 'CANCEL' END WHERE task.status IN ('COMPLETED','FAILED','STOPPED') AND pol.operation_status IN ('UNSTART','SCHEDULE','RUNNING');"
open_bound_sql="SELECT COUNT(*) FROM pipeline_operation_log pol JOIN ingestion_task task ON task.pipeline_log_id = pol.id WHERE pol.operation_status IN ('UNSTART','SCHEDULE','RUNNING');"
cleanup_sql="START TRANSACTION; DELETE FROM ingestion_task_log; DELETE FROM ingestion_task; COMMIT;"

print_step() {
    printf '\n[%s] %s\n' "$1" "$2"
}

print_sql() {
    printf 'SQL> %s\n' "$1"
}

mysql_exec() {
    local sql="$1"
    MYSQL_PWD="$mysql_password" "$mysql_bin" \
        --batch --skip-column-names --raw \
        --host="$mysql_host" --port="$mysql_port" \
        --user="$mysql_user" --database="$mysql_database" \
        --execute="$sql"
}

print_step 1 "Stop every Go ingestion writer before touching the database."
echo "      API ingestion endpoints, connector/file syncer, and ingestor must be stopped."
if [[ "$mode" == "dry-run" ]]; then
    echo "      (dry-run: pass --writers-stopped in execute mode after your supervisor confirms this.)"
fi

print_step 2 "Verify no non-terminal ingestion task remains."
print_sql "$status_sql"
print_sql "$non_terminal_sql"
if [[ "$mode" == "execute" ]]; then
    mysql_exec "$status_sql"
    non_terminal="$(mysql_exec "$non_terminal_sql" | tr -d '[:space:]')"
    if [[ "$non_terminal" != "0" ]]; then
        echo "refusing cleanup: $non_terminal non-terminal ingestion task(s) remain" >&2
        exit 1
    fi
fi

print_step 3 "Settle only pipeline logs exactly bound to terminal ingestion tasks."
print_sql "$settle_sql"
print_sql "$open_bound_sql"
if [[ "$mode" == "execute" ]]; then
    mysql_exec "$settle_sql"
    open_bound="$(mysql_exec "$open_bound_sql" | tr -d '[:space:]')"
    if [[ "$open_bound" != "0" ]]; then
        echo "refusing cleanup: $open_bound bound pipeline log(s) remain open" >&2
        exit 1
    fi
fi

print_step 4 "Clear the Go-exclusive task and event tables (never pipeline_operation_log)."
print_sql "$cleanup_sql"
if [[ "$mode" == "execute" ]]; then
    mysql_exec "$cleanup_sql"
fi

print_step 5 "Start the new binary once to apply additive schema migrations."
echo "CMD> $ragflow_bin --migrate"
if [[ "$mode" == "execute" ]]; then
    if [[ ! -x "$ragflow_bin" ]]; then
        echo "migration binary is not executable: $ragflow_bin" >&2
        exit 127
    fi
    "$ragflow_bin" --migrate
fi

print_step 6 "Start the new API, syncer, ingestor, worker, and frontend together."
echo "      Use the deployment supervisor/container command for this step; it is intentionally not inferred."
echo "      All six upgrade steps completed in $mode mode."
