#!/bin/bash
set -e

# 01_init.sh — Vastbase first-time initialization
# Reads VB_USERNAME and VB_PASSWORD from environment variables
# (injected by docker-compose.yml -> .env)
# Creates user and databases, grants all privileges.

VB_USER="${VB_USERNAME:-ragflow}"
VB_PASS="${VB_PASSWORD:-Ragflow@123}"
VB_VEC_DB="${VB_VEC_DBNAME:-ragflow}"
VB_META_DB="${VB_META_DBNAME:-rag_flow}"

# ── Step 1: Create user (must be in a DO block) ──────────────────────────
SQL_FILE="/tmp/init_vastbase_$$.sql"

cat > "${SQL_FILE}" << SQL
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${VB_USER}') THEN
        CREATE ROLE ${VB_USER} LOGIN PASSWORD '${VB_PASS}';
    END IF;
END
\$\$;
SQL

echo "Creating user ${VB_USER}..."
vsql -f "${SQL_FILE}"
rm -f "${SQL_FILE}"

# ── Step 2: Create databases (outside DO block — CREATE DATABASE requires
#            a transaction commit and cannot run inside a PL/pgSQL block) ──

echo "Creating database ${VB_VEC_DB}..."
vsql -c "CREATE DATABASE ${VB_VEC_DB} OWNER ${VB_USER}" 2>/dev/null \
    || echo "  Database '${VB_VEC_DB}' already exists"

echo "Creating database ${VB_META_DB}..."
vsql -c "CREATE DATABASE ${VB_META_DB} OWNER ${VB_USER}" 2>/dev/null \
    || echo "  Database '${VB_META_DB}' already exists"

# ── Step 3: Grant database-level privileges ──────────────────────────────

echo "Granting privileges on ${VB_VEC_DB}..."
vsql -c "GRANT ALL PRIVILEGES ON DATABASE ${VB_VEC_DB} TO ${VB_USER};"

echo "Granting privileges on ${VB_META_DB}..."
vsql -c "GRANT ALL PRIVILEGES ON DATABASE ${VB_META_DB} TO ${VB_USER};"

# ── Step 4: Grant schema permissions (must connect to each database) ─────

echo "Granting schema permissions on ${VB_VEC_DB}..."
vsql -d "${VB_VEC_DB}" -c "GRANT ALL ON SCHEMA public TO ${VB_USER};"
vsql -d "${VB_VEC_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${VB_USER};"
vsql -d "${VB_VEC_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${VB_USER};"

echo "Granting schema permissions on ${VB_META_DB}..."
vsql -d "${VB_META_DB}" -c "GRANT ALL ON SCHEMA public TO ${VB_USER};"
vsql -d "${VB_META_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${VB_USER};"
vsql -d "${VB_META_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${VB_USER};"

echo "ragflow initialization complete."
