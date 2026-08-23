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
