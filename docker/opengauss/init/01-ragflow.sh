#!/usr/bin/env bash
(
    set -euo pipefail
    : "${OPENGAUSS_PASSWORD:?Set the RAGFlow database password in docker/.env}"

    # The official entrypoint runs init files as omm on the local socket.
    # gsql quotes the password as a literal and the names as identifiers.
    gsql -d postgres -v ON_ERROR_STOP=1 -v app_user="$OPENGAUSS_USER" <<'SQL'
\set app_password `printenv OPENGAUSS_PASSWORD`
CREATE USER :"app_user" PASSWORD :'app_password';
SQL
    if [[ "$OPENGAUSS_DATABASE" != postgres ]]; then
        gsql -d postgres -v ON_ERROR_STOP=1 -v db="$OPENGAUSS_DATABASE" <<'SQL'
CREATE DATABASE :"db" ENCODING 'UTF8' DBCOMPATIBILITY 'A';
SQL
    fi
    if [[ "$OPENGAUSS_SCHEMA" != public ]]; then
        gsql -d "$OPENGAUSS_DATABASE" -v ON_ERROR_STOP=1 \
            -v app_schema="$OPENGAUSS_SCHEMA" -v app_user="$OPENGAUSS_USER" <<'SQL'
CREATE SCHEMA IF NOT EXISTS :"app_schema" AUTHORIZATION :"app_user";
SQL
    fi
    gsql -d "$OPENGAUSS_DATABASE" -v ON_ERROR_STOP=1 \
        -v app_schema="$OPENGAUSS_SCHEMA" -v app_user="$OPENGAUSS_USER" <<'SQL'
GRANT USAGE, CREATE ON SCHEMA :"app_schema" TO :"app_user";
SQL
)
