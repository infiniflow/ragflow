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
#   PY  - Python interpreter path (default: python3)
# -----------------------------------------------------------------------------

set -e

PY="${PY:-python3}"
CONFIG="${1:-conf/service_conf.yaml}"

echo "Running model provider table migrations..."

# Step 1: Create base model provider tables
"$PY" tools/scripts/mysql_migration.py \
    --stages tenant_model_provider,tenant_model_instance,tenant_model,model_id_config \
    --config "$CONFIG" \
    --execute \
    --database-version "v0.26.0" \
    --mark-database-version-on-success

# Step 2: Seed, merge model types, and migrate model IDs
"$PY" tools/scripts/mysql_migration.py \
    --stages tenant_model_seeding,model_type_merge,tenant_model_id_migration \
    --config "$CONFIG" \
    --execute \
    --database-version "v0.27.0.dev1" \
    --mark-database-version-on-success

echo "Model provider table migrations completed."
