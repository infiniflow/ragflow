#!/bin/bash
set -e

# 01_init.sh — Vastbase first-time initialization
# Reads VB_USERNAME and VB_PASSWORD from environment variables
# (injected by docker-compose.yml -> .env)
# Creates user and databases, grants all privileges.

VB_USER="${VB_USERNAME:-ragflow}"
# Default must satisfy Vastbase password policy
# (upper + lower + digit + special chars, >= 8 chars)
VB_PASS="${VB_PASSWORD:-Infini_Rag@123}"
VB_VEC_DB="${VB_VEC_DBNAME:-ragflow}"
VB_META_DB="${VB_META_DBNAME:-rag_flow}"

# SQL encoding — shell quoting and the temp file below perform no SQL
# escaping, so encode here: string literals double embedded single quotes,
# identifiers double embedded double quotes. The SET below pins
# standard_conforming_strings=on so backslashes stay literal, which is what
# the quote-doubling above assumes (PostgreSQL docs, String Constants).
VB_USER_LIT=${VB_USER//\'/\'\'}
VB_USER_IDENT=${VB_USER//\"/\"\"}
VB_PASS_LIT=${VB_PASS//\'/\'\'}
VB_DB_IDENT=${VB_VEC_DB//\"/\"\"}
VB_META_DB_IDENT=${VB_META_DB//\"/\"\"}
VB_DB_LIT=${VB_VEC_DB//\'/\'\'}
VB_META_DB_LIT=${VB_META_DB//\'/\'\'}

# ── Step 1: Create user ──────────────────────────────────────────────────
# Role existence is probed with a plain query first so the CREATE can be a
# bare statement in the temp file: no DO $$ block is involved, so a password
# containing "$$" cannot terminate a dollar-quoted body early (doubling
# single quotes is complete escaping for a plain SQL string literal, but it
# does not protect a dollar-quoted one).
SQL_FILE="$(mktemp /tmp/init_vastbase.XXXXXX.sql)"
trap 'rm -f "${SQL_FILE}"' EXIT

if vsql -tA -c "SELECT 1 FROM pg_roles WHERE rolname = '${VB_USER_LIT}'" | grep -qx 1; then
    echo "User ${VB_USER} already exists"
else
    # Contains the password in cleartext: mktemp gives 0600 perms, and the
    # EXIT trap removes it even when `set -e` aborts on a failed vsql call.
    cat > "${SQL_FILE}" << SQL
SET standard_conforming_strings = on;
CREATE ROLE "${VB_USER_IDENT}" LOGIN PASSWORD '${VB_PASS_LIT}';
SQL

    echo "Creating user ${VB_USER}..."
    vsql -f "${SQL_FILE}"
fi

# ── Step 2: Create databases (outside DO block — CREATE DATABASE requires
#            a transaction commit and cannot run inside a PL/pgSQL block) ──

# Probe pg_database first (same pattern as the role probe in Step 1): a
# CREATE that fails for any reason must abort this script with its real
# error on stderr. The previous form piped stderr to /dev/null and printed
# "already exists" for EVERY failure, so e.g. a rejected DBCOMPATIBILITY
# value surfaced only later as GRANT's "database does not exist".
db_exists() {
    vsql -tA -c "SELECT 1 FROM pg_database WHERE datname = '${1}'" | grep -qx 1
}

echo "Creating database ${VB_VEC_DB}..."
if db_exists "${VB_DB_LIT}"; then
    echo "  Database '${VB_VEC_DB}' already exists"
else
    vsql -c "CREATE DATABASE \"${VB_DB_IDENT}\" OWNER \"${VB_USER_IDENT}\""
fi

# The metadata database must end up in B (MySQL) compatibility mode: the Go
# metadata path issues MySQL-flavored SQL (unquoted references to the reserved
# user table, DATE_FORMAT, REGEXP, DELETE ... LIMIT) through the PostgreSQL
# wire protocol, which only B mode accepts. G100 has no per-database
# DBCOMPATIBILITY option (CREATE DATABASE rejects it with "option not
# recognized"); the image entrypoint fixes the mode instance-wide at vb_initdb
# time via VB_DBCOMPATIBILITY, and every database on the instance inherits
# it. The verify block below fails init if that inheritance did not produce
# B mode (e.g. the instance was initialized with VB_DBCOMPATIBILITY=PG).
echo "Creating database ${VB_META_DB}..."
if db_exists "${VB_META_DB_LIT}"; then
    echo "  Database '${VB_META_DB}' already exists"
else
    vsql -c "CREATE DATABASE \"${VB_META_DB_IDENT}\" OWNER \"${VB_USER_IDENT}\""
fi

# Fail init here rather than at Go startup if the metadata database is not
# in B mode: the MySQL-flavored metadata SQL cannot run in any other mode.
# datcompatibility is the same column the doc engine verifies against.
meta_mode="$(vsql -tA -c "SELECT datcompatibility FROM pg_database WHERE datname = '${VB_META_DB_LIT}'")"
meta_mode="${meta_mode//[[:space:]]/}"
if [ "${meta_mode}" != "B" ]; then
    echo "ERROR: metadata database '${VB_META_DB}' is in '${meta_mode}' compatibility mode, expected 'B'" >&2
    exit 1
fi

# ── Step 3: Grant database-level privileges ──────────────────────────────

echo "Granting privileges on ${VB_VEC_DB}..."
vsql -c "GRANT ALL PRIVILEGES ON DATABASE \"${VB_DB_IDENT}\" TO \"${VB_USER_IDENT}\";"

echo "Granting privileges on ${VB_META_DB}..."
vsql -c "GRANT ALL PRIVILEGES ON DATABASE \"${VB_META_DB_IDENT}\" TO \"${VB_USER_IDENT}\";"

# ── Step 4: Grant schema permissions (must connect to each database) ─────

echo "Granting schema permissions on ${VB_VEC_DB}..."
vsql -d "${VB_VEC_DB}" -c "GRANT ALL ON SCHEMA public TO \"${VB_USER_IDENT}\";"
vsql -d "${VB_VEC_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO \"${VB_USER_IDENT}\";"
vsql -d "${VB_VEC_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO \"${VB_USER_IDENT}\";"

echo "Granting schema permissions on ${VB_META_DB}..."
vsql -d "${VB_META_DB}" -c "GRANT ALL ON SCHEMA public TO \"${VB_USER_IDENT}\";"
vsql -d "${VB_META_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO \"${VB_USER_IDENT}\";"
vsql -d "${VB_META_DB}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO \"${VB_USER_IDENT}\";"

echo "ragflow initialization complete."
