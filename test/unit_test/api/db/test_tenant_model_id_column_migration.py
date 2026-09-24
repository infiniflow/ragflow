"""
Tests for tenant_*_id column type migration on upgrade (#18756 / #18777).

migrate_tenant_model_id_column_types is a fallback for when the pre-startup
script (tenant_model_id_migration) did not run. It only converts leftover
integer columns; backfill still belongs in the script / postgres family stages.
"""

import logging

import pytest

from api.db import db_models
from common import settings


def test_migrate_tenant_model_id_column_types_skips_non_integer_columns(monkeypatch):
    inspected = {}

    def fake_get_column_data_type(table_name, column_name):
        inspected[(table_name, column_name)] = True
        return "character varying"

    monkeypatch.setattr(db_models, "_get_column_data_type", fake_get_column_data_type)
    monkeypatch.setattr(db_models, "DB", type("DB", (), {"table_exists": staticmethod(lambda _name: True)})())
    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: (_ for _ in ()).throw(AssertionError("should not add column")))

    executed = []

    class Migrator:
        def alter_column_type(self, *_args):
            executed.append("alter")

    db_models.migrate_tenant_model_id_column_types(Migrator())

    assert len(inspected) == len(db_models.TENANT_MODEL_ID_COLUMNS)
    assert not executed


def test_migrate_tenant_model_id_column_types_skips_add_when_inspection_fails(monkeypatch, caplog):
    added = []

    def boom(*_args):
        raise RuntimeError("catalog unavailable")

    monkeypatch.setattr(db_models, "_get_column_data_type", boom)
    monkeypatch.setattr(db_models, "DB", type("DB", (), {"table_exists": staticmethod(lambda _name: True)})())
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: added.append("add"))

    logging.disable(logging.ERROR)
    try:
        with caplog.at_level(logging.CRITICAL):
            db_models.migrate_tenant_model_id_column_types(object())
    finally:
        logging.disable(logging.NOTSET)

    assert not added
    assert any("column inspection failed" in r.message for r in caplog.records if r.levelno >= logging.CRITICAL)


def test_migrate_tenant_model_id_column_types_adds_missing_columns(monkeypatch):
    added = []

    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: None)
    monkeypatch.setattr(db_models, "DB", type("DB", (), {"table_exists": staticmethod(lambda _name: True)})())
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda _migrator, table, column, field: added.append((table, column, field)))

    db_models.migrate_tenant_model_id_column_types(object())

    assert len(added) == len(db_models.TENANT_MODEL_ID_COLUMNS)
    for table_name, column_name, field in added:
        assert (table_name, column_name) in db_models.TENANT_MODEL_ID_COLUMNS
        assert isinstance(field, db_models.CharField)
        assert field.max_length == 32


def test_migrate_tenant_model_id_column_types_converts_postgres_integer(monkeypatch):
    queries = []

    class FakeDB:
        @staticmethod
        def table_exists(_name):
            return True

        @staticmethod
        def execute_sql(sql, params=None):
            queries.append((sql, params))

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(
        db_models,
        "_get_column_data_type",
        lambda table_name, column_name: "integer" if (table_name, column_name) == ("tenant", "tenant_llm_id") else "character varying",
    )
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: (_ for _ in ()).throw(AssertionError("should not add column")))

    db_models.migrate_tenant_model_id_column_types(object())

    assert len(queries) == 1
    sql, _params = queries[0]
    assert 'ALTER TABLE "tenant" ALTER COLUMN "tenant_llm_id"' in sql
    assert "TYPE varchar(32)" in sql
    assert "USING CAST(NULL AS varchar(32))" in sql
    assert "::text" not in sql


def test_migrate_tenant_model_id_column_types_converts_mysql_integer(monkeypatch):
    altered = []
    queries = []

    class FakeDB:
        @staticmethod
        def table_exists(_name):
            return True

        @staticmethod
        def execute_sql(sql, params=None):
            queries.append(sql)

    class Migrator:
        def alter_column_type(self, table_name, column_name, field):
            altered.append((table_name, column_name, field))
            return f"alter {table_name}.{column_name}"

    def fake_migrate(operation):
        altered.append(("migrate", operation))

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "mysql")
    monkeypatch.setattr(
        db_models,
        "_get_column_data_type",
        lambda table_name, column_name: "int" if (table_name, column_name) == ("knowledgebase", "tenant_embd_id") else "varchar",
    )
    monkeypatch.setattr(db_models, "migrate", fake_migrate)

    db_models.migrate_tenant_model_id_column_types(Migrator())

    converted = [(table, column) for table, column, *_rest in altered if table != "migrate"]
    assert converted == [("knowledgebase", "tenant_embd_id")]
    assert ("migrate", "alter knowledgebase.tenant_embd_id") in altered
    assert any("CHAR_LENGTH(`tenant_embd_id`) <> 32" in sql for sql in queries)


@pytest.mark.parametrize("db_type", ["mysql", "oceanbase"])
def test_migrate_tenant_model_id_column_types_retries_mysql_leftover_cleanup(monkeypatch, db_type):
    queries = []
    altered = []

    class FakeDB:
        @staticmethod
        def table_exists(_name):
            return True

        @staticmethod
        def execute_sql(sql, params=None):
            queries.append(sql)

    class Migrator:
        def alter_column_type(self, *_args):
            altered.append("alter")
            raise AssertionError("already-converted varchar must not be altered again")

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", db_type)
    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: "varchar")
    monkeypatch.setattr(db_models, "migrate", lambda *_args: (_ for _ in ()).throw(AssertionError("should not migrate")))

    db_models.migrate_tenant_model_id_column_types(Migrator())

    assert not altered
    assert len(queries) == len(db_models.TENANT_MODEL_ID_COLUMNS)
    assert all("CHAR_LENGTH(" in sql and "<> 32" in sql for sql in queries)


def test_migrate_db_invokes_tenant_model_id_column_migration(monkeypatch):
    events = []

    monkeypatch.setattr(settings, "DATABASE_TYPE", "mysql")
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: events.append("add"))
    monkeypatch.setattr(db_models, "alter_db_column_type", lambda *_args: events.append("alter_type"))
    monkeypatch.setattr(db_models, "alter_db_rename_column", lambda *_args: events.append("rename"))
    monkeypatch.setattr(db_models, "alter_db_drop_index", lambda *_args: events.append("drop_index"))
    monkeypatch.setattr(db_models, "migrate_tenant_model_id_column_types", lambda _migrator: events.append("tenant_model_id_types"))
    monkeypatch.setattr(db_models, "relax_gaussdb_empty_string_compatible_columns", lambda: events.append("relax"))
    monkeypatch.setattr(db_models, "migrate", lambda *_operations: None)
    monkeypatch.setattr(db_models, "migrate_add_unique_email", lambda _migrator: None)
    monkeypatch.setattr(db_models, "migrate_model_type_names", lambda: None)
    monkeypatch.setattr(db_models, "ensure_model_indexes", lambda _migrator: None)

    db_models.migrate_db()

    assert "tenant_model_id_types" in events
    assert events.index("tenant_model_id_types") > events.index("add")


def test_migrate_db_invokes_postgres_model_provider_data_migration(monkeypatch):
    events = []

    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: events.append("add"))
    monkeypatch.setattr(db_models, "alter_db_column_type", lambda *_args: events.append("alter_type"))
    monkeypatch.setattr(db_models, "alter_db_rename_column", lambda *_args: events.append("rename"))
    monkeypatch.setattr(db_models, "alter_db_drop_index", lambda *_args: events.append("drop_index"))
    monkeypatch.setattr(db_models, "migrate_tenant_model_id_column_types", lambda _migrator: events.append("tenant_model_id_types"))
    monkeypatch.setattr(db_models, "relax_gaussdb_empty_string_compatible_columns", lambda: events.append("relax"))
    monkeypatch.setattr(db_models, "migrate", lambda *_operations: None)
    monkeypatch.setattr(db_models, "migrate_add_unique_email", lambda _migrator: None)
    monkeypatch.setattr(db_models, "migrate_model_type_names", lambda: None)
    ensure_events = []
    monkeypatch.setattr(db_models, "ensure_model_indexes", lambda _migrator: ensure_events.append("ensure_indexes"))
    monkeypatch.setattr(db_models, "migrate_postgres_family_model_provider_tables", lambda: events.append("pg_model_provider"))

    db_models.migrate_db()

    assert "pg_model_provider" in events
    assert events.index("pg_model_provider") > events.index("tenant_model_id_types")
    assert events.index("pg_model_provider") > events.index("relax")
    assert ensure_events == ["ensure_indexes"]
