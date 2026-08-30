#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Per-dialect rendering tests for the model provider migration.

These assert the SQL each dialect emits, so a regression in one dialect cannot
hide behind the other. End-to-end equivalence against live servers lives in
test/integration/test_model_provider_migration_dialects.py.
"""

import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "..", "tools", "scripts"))

from migration_dialects import (
    CONNECT_OPTION_KEYS,
    MYSQL,
    POSTGRES,
    Column,
    Index,
    MySQLDialect,
    PostgresDialect,
    TableSpec,
    get_dialect,
    normalize_db_type,
)

SPEC = TableSpec(
    name="tenant_model_provider",
    columns=[
        Column("id", "VARCHAR(32)", null=False, primary_key=True),
        Column("provider_name", "VARCHAR(128)", null=False),
        Column("status", "VARCHAR(32)", default="'active'"),
        Column("model_type", "INT", null=False),
        Column("create_date", "TIMESTAMP"),
    ],
    indexes=[
        Index("idx_provider_name", ["provider_name"]),
        Index("idx_tenant_provider_unique", ["tenant_id", "provider_name"], unique=True),
    ],
)


@pytest.mark.parametrize(
    "raw,expected",
    [
        ("mysql", MYSQL),
        ("MySQL", MYSQL),
        ("  mariadb ", MYSQL),
        ("oceanbase", MYSQL),
        ("postgres", POSTGRES),
        ("PostgreSQL", POSTGRES),
        ("serenedb", POSTGRES),
        # Anything unrecognised keeps the behaviour this script had before
        # PostgreSQL support existed, rather than raising at import time.
        ("gaussdb", MYSQL),
        (None, MYSQL),
        ("", MYSQL),
    ],
)
def test_normalize_db_type(raw, expected):
    assert normalize_db_type(raw) == expected


def test_get_dialect_returns_matching_implementation():
    assert isinstance(get_dialect("postgres"), PostgresDialect)
    assert isinstance(get_dialect("mysql"), MySQLDialect)


def test_quoting_is_dialect_specific():
    assert MySQLDialect().quote("value") == "`value`"
    assert PostgresDialect().quote("value") == '"value"'


@pytest.mark.parametrize("dialect", [MySQLDialect(), PostgresDialect()])
def test_quote_rejects_its_own_quote_character(dialect):
    # Identifiers are fixed schema names, so a quote character means a caller
    # bug. Failing loudly beats emitting SQL that silently means something else.
    with pytest.raises(ValueError):
        dialect.quote(f"a{dialect.quote_char}b")


def test_type_mapping_keeps_length_suffix():
    assert MySQLDialect().render_type("VARCHAR(32)") == "VARCHAR(32)"
    assert PostgresDialect().render_type("VARCHAR(32)") == "VARCHAR(32)"
    # Only the base name is mapped.
    assert MySQLDialect().render_type("TIMESTAMP") == "DATETIME"
    assert PostgresDialect().render_type("TIMESTAMP") == "TIMESTAMP"
    assert MySQLDialect().render_type("INT") == "INT"
    assert PostgresDialect().render_type("INT") == "INTEGER"


def test_mysql_creates_table_with_inline_indexes():
    statements = MySQLDialect().create_table_statements(SPEC)

    assert len(statements) == 1, "MySQL declares indexes inside CREATE TABLE"
    sql = statements[0]
    assert "CREATE TABLE IF NOT EXISTS `tenant_model_provider`" in sql
    assert "`id` VARCHAR(32) NOT NULL PRIMARY KEY" in sql
    assert "`status` VARCHAR(32) DEFAULT 'active'" in sql
    assert "`create_date` DATETIME" in sql
    assert "INDEX `idx_provider_name` (`provider_name`)" in sql
    assert "UNIQUE INDEX `idx_tenant_provider_unique` (`tenant_id`, `provider_name`)" in sql
    assert sql.endswith("ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")


def test_postgres_creates_table_then_indexes_separately():
    statements = PostgresDialect().create_table_statements(SPEC)

    # PostgreSQL has no inline index syntax, so each index is its own statement.
    assert len(statements) == 3
    create, *indexes = statements
    assert 'CREATE TABLE IF NOT EXISTS "tenant_model_provider"' in create
    assert '"create_date" TIMESTAMP' in create
    assert '"model_type" INTEGER NOT NULL' in create
    assert "ENGINE" not in create and "CHARSET" not in create
    assert "INDEX" not in create

    assert indexes[0] == 'CREATE INDEX IF NOT EXISTS "idx_provider_name" ON "tenant_model_provider" ("provider_name")'
    assert indexes[1] == 'CREATE UNIQUE INDEX IF NOT EXISTS "idx_tenant_provider_unique" ON "tenant_model_provider" ("tenant_id", "provider_name")'


@pytest.mark.parametrize("dialect", [MySQLDialect(), PostgresDialect()])
def test_create_table_statements_are_all_idempotent(dialect):
    # run_migrations.sh can be re-entered on any container boot, so every
    # statement has to tolerate the objects already existing.
    for sql in dialect.create_table_statements(SPEC):
        assert "IF NOT EXISTS" in sql


def test_epoch_conversion_is_dialect_specific():
    assert MySQLDialect().epoch_to_timestamp("%s") == "FROM_UNIXTIME(%s)"
    assert PostgresDialect().epoch_to_timestamp("%s") == "to_timestamp(%s)"


def test_upsert_uses_each_dialect_own_conflict_clause():
    mysql_sql = MySQLDialect().upsert_system_setting_sql()
    assert "ON DUPLICATE KEY UPDATE" in mysql_sql
    assert "VALUES(`value`)" in mysql_sql

    pg_sql = PostgresDialect().upsert_system_setting_sql()
    assert 'ON CONFLICT ("name") DO UPDATE SET' in pg_sql
    assert 'EXCLUDED."value"' in pg_sql
    assert "ON DUPLICATE KEY" not in pg_sql

    # Both take the same eight bind parameters, so callers stay dialect-blind.
    assert mysql_sql.count("%s") == pg_sql.count("%s") == 8


def test_surrogate_id_column_is_unique_and_backfilled():
    (mysql_sql,) = MySQLDialect().add_surrogate_id_statements("tenant_llm", "id")
    assert mysql_sql == "ALTER TABLE `tenant_llm` ADD COLUMN `id` BIGINT AUTO_INCREMENT UNIQUE"

    pg_statements = PostgresDialect().add_surrogate_id_statements("tenant_llm", "id")
    # BIGSERIAL back-fills existing rows; PostgreSQL needs the UNIQUE
    # constraint as a second statement rather than a column modifier.
    assert pg_statements == [
        'ALTER TABLE "tenant_llm" ADD COLUMN "id" BIGSERIAL',
        'ALTER TABLE "tenant_llm" ADD CONSTRAINT "tenant_llm_id_key" UNIQUE ("id")',
    ]


def test_relaxing_a_column_splits_into_type_and_nullability_on_postgres():
    (mysql_sql,) = MySQLDialect().relax_to_nullable_varchar_statements("tenant", "tenant_llm_id", 32)
    assert mysql_sql == "ALTER TABLE `tenant` MODIFY COLUMN `tenant_llm_id` VARCHAR(32) NULL"

    pg_statements = PostgresDialect().relax_to_nullable_varchar_statements("tenant", "tenant_llm_id", 32)
    assert len(pg_statements) == 2
    # The USING cast is required: these columns start out INTEGER.
    assert "TYPE VARCHAR(32) USING" in pg_statements[0]
    assert pg_statements[1].endswith("DROP NOT NULL")


def test_catalog_scope_is_the_database_on_mysql_and_the_schema_on_postgres():
    class Config:
        database = "rag_flow"
        schema = "public"

    assert MySQLDialect().catalog_scope(Config()) == "rag_flow"
    # information_schema.table_schema means the namespace in PostgreSQL, so
    # filtering by database name would match nothing.
    assert PostgresDialect().catalog_scope(Config()) == "public"


class _RecordingDB:
    def __init__(self):
        self.statements = []

    def execute_sql(self, sql, params=None):
        self.statements.append(sql)


class _Config:
    def __init__(self, schema="public"):
        self.schema = schema
        self.database = "rag_flow"


def test_mysql_session_needs_no_preparation():
    db = _RecordingDB()
    MySQLDialect().prepare_session(db, _Config())
    assert db.statements == []


@pytest.mark.parametrize("schema", ["public", "ragflow", "Mixed_Case"])
def test_postgres_binds_the_session_to_the_configured_schema(schema):
    """Catalog lookups scope to config.schema, so unqualified DDL must land there
    too - otherwise a non-public deployment inspects one namespace and writes
    to another."""
    db = _RecordingDB()
    PostgresDialect().prepare_session(db, _Config(schema))
    assert db.statements == [f'SET search_path TO "{schema}"']


def test_postgres_search_path_rejects_a_quoted_schema_name():
    with pytest.raises(ValueError):
        PostgresDialect().prepare_session(_RecordingDB(), _Config('pub"lic'))


def test_connection_options_cover_each_driver_tls_settings():
    assert "sslmode" in CONNECT_OPTION_KEYS[POSTGRES]
    assert "sslrootcert" in CONNECT_OPTION_KEYS[POSTGRES]
    assert "ssl_ca" in CONNECT_OPTION_KEYS[MYSQL]


# --- config-block selection ------------------------------------------------


def _write_conf(tmp_path, text):
    path = tmp_path / "service_conf.yaml"
    path.write_text(text, encoding="utf-8")
    return str(path)


def _load(monkeypatch, tmp_path, text, db_type):
    import mysql_migration

    monkeypatch.setenv("DB_TYPE", db_type)
    return mysql_migration.MigrationConfig.from_config_file(_write_conf(tmp_path, text))


MYSQL_BLOCK = "mysql:\n  host: mysql-host\n  port: 3306\n  name: rag_flow\n"
PG_BLOCK = "postgres:\n  host: pg-host\n  port: 5432\n  name: rag_flow\n  sslmode: verify-full\n"


def test_postgres_reads_its_own_block(monkeypatch, tmp_path):
    cfg = _load(monkeypatch, tmp_path, MYSQL_BLOCK + PG_BLOCK, "postgres")
    assert (cfg.db_type, cfg.host, cfg.port) == (POSTGRES, "pg-host", 5432)


def test_postgres_never_falls_back_to_the_mysql_block(monkeypatch, tmp_path):
    """The dangerous case: `postgres:` still commented out in service_conf.yaml.
    Reading `mysql:` would point --execute at a different server."""
    with pytest.raises(KeyError):
        _load(monkeypatch, tmp_path, MYSQL_BLOCK, "postgres")


def test_mysql_keeps_the_legacy_block_lookup(monkeypatch, tmp_path):
    cfg = _load(monkeypatch, tmp_path, "database:\n  host: legacy-host\n  name: rag_flow\n", "mysql")
    assert (cfg.db_type, cfg.host) == (MYSQL, "legacy-host")


def test_missing_config_file_is_not_swallowed_into_defaults(monkeypatch, tmp_path):
    import mysql_migration

    monkeypatch.setenv("DB_TYPE", "postgres")
    with pytest.raises(OSError):
        mysql_migration.MigrationConfig.from_config_file(str(tmp_path / "nope.yaml"))


def test_tls_settings_are_carried_over_from_the_config_block(monkeypatch, tmp_path):
    cfg = _load(monkeypatch, tmp_path, PG_BLOCK, "postgres")
    assert cfg.connect_options == {"sslmode": "verify-full"}


def test_unknown_block_keys_are_not_forwarded_to_the_driver(monkeypatch, tmp_path):
    cfg = _load(monkeypatch, tmp_path, PG_BLOCK + "  max_connections: 100\n  stale_timeout: 30\n", "postgres")
    assert "max_connections" not in cfg.connect_options
    assert "stale_timeout" not in cfg.connect_options


# --- information_schema type normalization ---------------------------------


@pytest.mark.parametrize(
    ("reported", "expected"),
    [("integer", "int"), ("INTEGER", "int"), ("int", "int"), ("bigint", "bigint"), ("character varying", "character varying")],
)
def test_column_type_is_normalized_to_the_mysql_spelling(reported, expected):
    """PostgreSQL reports INT as "integer". The merge/seeding stages compare
    against "int", and missing that makes them run again on an already-migrated
    table - the merge then re-maps ints through a string-keyed lookup and writes
    model_type=0."""
    import mysql_migration

    class _Cursor:
        @staticmethod
        def fetchone():
            return (reported,)

    db = mysql_migration.MigrationDatabase.__new__(mysql_migration.MigrationDatabase)
    db.config = _Config()
    db.dialect = PostgresDialect()
    db.execute_sql = lambda *_a, **_k: _Cursor()

    assert db.get_column_type("tenant_model", "model_type") == expected
