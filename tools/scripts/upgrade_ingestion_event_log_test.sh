#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$script_dir/upgrade_ingestion_event_log.sh"

output="$(bash "$script" --dry-run)"

assert_contains() {
    local needle="$1"
    if ! grep -Fq -- "$needle" <<<"$output"; then
        echo "expected dry-run output to contain: $needle" >&2
        exit 1
    fi
}

assert_contains "SELECT status, COUNT(*) FROM ingestion_task"
assert_contains "UPDATE pipeline_operation_log"
assert_contains "task.pipeline_log_id"
assert_contains "DELETE FROM ingestion_task_log"
assert_contains "DELETE FROM ingestion_task"
assert_contains "bin/ragflow_server --migrate"
assert_contains "dry-run"

if bash "$script" --execute --yes --mysql-bin /definitely/missing/mysql >/dev/null 2>&1; then
    echo "execute mode unexpectedly succeeded with a missing mysql client" >&2
    exit 1
fi

echo "upgrade_ingestion_event_log.sh checks passed"
