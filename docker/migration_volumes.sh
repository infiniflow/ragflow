#!/bin/bash
# Helper for docker/migration.sh: maps DOC_ENGINE to the index volume and
# backup-file name, and reports engines that the migration script cannot
# back up (oceanbase / seekdb use bind mounts, not named volumes).
#
# Sourced by both docker/migration.sh (the runtime) and the pytest test
# test/unit_test/deploy/test_migration_volumes.py (the regression guard).
# Keep this file's API stable: the test reads the function output as
# "<volume_base> <backup_file>" (space-separated) and treats an empty
# stdout as "engine not supported by this script".

# Echoes "<volume_base> <backup_file>" for named-volume engines, or
# nothing for engines that the migration script cannot back up.
get_index_volume_for_engine() {
    local engine="${1:-elasticsearch}"
    case "$engine" in
        elasticsearch) echo "esdata01 es_backup.tar.gz" ;;
        opensearch)     echo "osdata01 os_backup.tar.gz" ;;
        infinity)       echo "infinity_data infinity_backup.tar.gz" ;;
        serenedb)       echo "serenedb_data serenedb_backup.tar.gz" ;;
        oceanbase)      return 0 ;;  # bind mount, skipped
        seekdb)         return 0 ;;  # bind mount, skipped
        *)              echo "esdata01 es_backup.tar.gz" ;;  # default fallback
    esac
}
