#!/bin/bash
# -----------------------------------------------------------------------------
# Shared migration script for model provider tables.
#
# Called by docker/entrypoint.sh and docker/launch_backend_service.sh.
# Keeps migration stages and versions in one place to avoid divergence.
#
# Usage:
#   PY=python3 tools/scripts/run_migrations.sh [--config CONFIG_PATH]
#
# Environment variables:
#   PY       - Python interpreter path (default: python3)
#   DB_TYPE  - metadata database type (mysql, postgres, gaussdb, oceanbase)
# -----------------------------------------------------------------------------

set -e

PY="${PY:-python3}"
CONFIG="conf/service_conf.yaml"

while [ $# -gt 0 ]; do
    case "$1" in
        --config)
            # Require a real file. mysql_migration.py:90 turns any config-load
            # failure into a warning and falls back to host=localhost user=root
            # database=rag_flow, and this script also passes --execute, so an
            # empty, whitespace, flag-shaped or missing path would silently
            # migrate the default database instead of failing.
            if [ $# -lt 2 ] || [ -z "${2//[[:space:]]/}" ] || [ ! -f "$2" ] || [ "${2#-}" != "$2" ]; then
                echo "Error: --config requires the path of an existing file" >&2
                exit 1
            fi
            CONFIG="$2"
            shift 2
            ;;
        *)
            echo "Error: unknown argument: $1" >&2
            echo "Usage: PY=python3 tools/scripts/run_migrations.sh [--config CONFIG_PATH]" >&2
            exit 1
            ;;
    esac
done

DB_TYPE_NORMALIZED="${DB_TYPE:-mysql}"
DB_TYPE_NORMALIZED="${DB_TYPE_NORMALIZED,,}"

if [[ "$DB_TYPE_NORMALIZED" == "postgres" || "$DB_TYPE_NORMALIZED" == "postgresql" || "$DB_TYPE_NORMALIZED" == "gaussdb" || "$DB_TYPE_NORMALIZED" == "gauss" ]]; then
    MIGRATION_SCRIPT="tools/scripts/postgres_migration.py"
    ENGINE_LABEL="PostgreSQL/GaussDB"
else
    MIGRATION_SCRIPT="tools/scripts/mysql_migration.py"
    ENGINE_LABEL="MySQL"
fi

echo "Running model provider table migrations (${ENGINE_LABEL}, DB_TYPE=${DB_TYPE:-mysql})..."

# Step 1: Create base model provider tables
"$PY" "$MIGRATION_SCRIPT" \
    --stages tenant_model_provider,tenant_model_instance,tenant_model,model_id_config \
    --config "$CONFIG" \
    --execute \
    --database-version "v0.26.0" \
    --mark-database-version-on-success

# Step 2: Seed, merge model types, and migrate model IDs
"$PY" "$MIGRATION_SCRIPT" \
    --stages tenant_model_seeding,model_type_merge,tenant_model_id_migration \
    --config "$CONFIG" \
    --execute \
    --database-version "v0.27.1" \
    --mark-database-version-on-success

echo "Model provider table migrations completed."
