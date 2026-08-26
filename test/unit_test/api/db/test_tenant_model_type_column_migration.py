"""
Tests for tenant_model.model_type varchar-to-integer migration on upgrade (#18755).
"""

from api.db import db_models
from common import settings


def test_migrate_tenant_model_type_column_skips_integer(monkeypatch):
    executed = []

    class FakeDB:
        @staticmethod
        def table_exists(_name):
            return True

        @staticmethod
        def execute_sql(sql, params=None):
            executed.append("sql")

    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: "integer")
    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: executed.append("add"))

    db_models.migrate_tenant_model_type_column(object())

    assert not executed


def test_migrate_tenant_model_type_column_adds_missing_column(monkeypatch):
    added = []

    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: None)
    monkeypatch.setattr(db_models, "DB", type("DB", (), {"table_exists": staticmethod(lambda _name: True)})())
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda _migrator, table, column, field: added.append((table, column, field)))

    db_models.migrate_tenant_model_type_column(object())

    assert len(added) == 1
    table, column, field = added[0]
    assert (table, column) == ("tenant_model", "model_type")
    assert isinstance(field, db_models.IntegerField)


def test_migrate_tenant_model_type_column_converts_postgres_varchar(monkeypatch):
    queries = []

    class FakeDB:
        @staticmethod
        def table_exists(_name):
            return True

        @staticmethod
        def execute_sql(sql, params=None):
            queries.append(sql)

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: "character varying")

    db_models.migrate_tenant_model_type_column(object())

    assert len(queries) == 1
    sql = queries[0]
    assert 'ALTER TABLE "tenant_model" ALTER COLUMN "model_type" TYPE integer USING' in sql
    assert "WHEN btrim(\"model_type\") ~ '^[0-9]+$' THEN btrim(\"model_type\")::integer" in sql
    assert "string_to_array" in sql
    assert "WHEN 'chat' = ANY(" in sql
    assert "WHEN 'vision' = ANY(" in sql
    assert "COALESCE(NULLIF((" in sql


def test_migrate_tenant_model_type_column_converts_mysql_varchar(monkeypatch):
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
        def alter_column_type(self, table_name, column_name, field):
            altered.append((table_name, column_name, field))
            return "alter tenant_model.model_type"

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "mysql")
    monkeypatch.setattr(db_models, "_get_column_data_type", lambda *_args: "varchar")
    monkeypatch.setattr(db_models, "migrate", lambda operation: altered.append(("migrate", operation)))

    db_models.migrate_tenant_model_type_column(Migrator())

    assert len(queries) == 1
    sql = queries[0]
    assert "UPDATE `tenant_model` SET `model_type` = CASE" in sql
    assert "WHEN TRIM(`model_type`) REGEXP '^[0-9]+$' THEN TRIM(`model_type`)" in sql
    assert "WHEN `model_type` IS NULL OR TRIM(`model_type`) = '' THEN '1'" in sql
    assert "FIND_IN_SET('chat'" in sql
    assert "FIND_IN_SET('vision'" in sql
    assert "IFNULL(NULLIF((" in sql
    assert ("tenant_model", "model_type") in [(table, column) for table, column, *_rest in altered if table != "migrate"]
    assert ("migrate", "alter tenant_model.model_type") in altered


def test_migrate_db_invokes_tenant_model_type_column_migration(monkeypatch):
    events = []

    monkeypatch.setattr(settings, "DATABASE_TYPE", "mysql")
    monkeypatch.setattr(db_models, "alter_db_add_column", lambda *_args: events.append("add"))
    monkeypatch.setattr(db_models, "alter_db_column_type", lambda *_args: events.append("alter_type"))
    monkeypatch.setattr(db_models, "alter_db_rename_column", lambda *_args: events.append("rename"))
    monkeypatch.setattr(db_models, "alter_db_drop_index", lambda *_args: events.append("drop_index"))
    monkeypatch.setattr(db_models, "migrate_tenant_model_type_column", lambda _migrator: events.append("tenant_model_type"))
    monkeypatch.setattr(db_models, "relax_gaussdb_empty_string_compatible_columns", lambda: events.append("relax"))
    monkeypatch.setattr(db_models, "migrate", lambda *_operations: None)
    monkeypatch.setattr(db_models, "migrate_add_unique_email", lambda _migrator: None)
    monkeypatch.setattr(db_models, "migrate_model_type_names", lambda: None)
    monkeypatch.setattr(db_models, "ensure_model_indexes", lambda _migrator: None)

    db_models.migrate_db()

    assert "tenant_model_type" in events
    assert events.index("tenant_model_type") > events.index("add")
