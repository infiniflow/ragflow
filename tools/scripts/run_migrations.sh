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
    --database-version "v0.27.1" \
    --mark-database-version-on-success

echo "Model provider table migrations completed."
