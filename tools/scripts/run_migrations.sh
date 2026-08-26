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
CONFIG="${1:-conf/service_conf.yaml}"
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
    --database-version "v0.27.0" \
    --mark-database-version-on-success

echo "Model provider table migrations completed."
