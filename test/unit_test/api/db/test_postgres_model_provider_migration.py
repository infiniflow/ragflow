"""
Tests for PostgreSQL/GaussDB model-provider migration (#18755 / #18756 / #18777 / #18781).

Lynn-Inf review: type conversion alone leaves tenant_*_id holding old integer
values and skips ModelTypeMergeStage / TenantModelSeedingStage. The postgres
dialect must run the same stages as mysql_migration.py.
"""

import importlib.util
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[4]
MIGRATION_SCRIPT = REPO_ROOT / "tools" / "scripts" / "mysql_migration.py"


def load_migration_module():
    spec = importlib.util.spec_from_file_location("ragflow_mysql_migration_test", MIGRATION_SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class RecordingCursor:
    def __init__(self, rows=None):
        self._rows = list(rows or [])
        self._offset = 0

    def fetchone(self):
        if not self._rows:
            return (0,)
        return self._rows[0]

    def fetchall(self):
        return list(self._rows)

    def fetchmany(self, size):
        chunk = self._rows[self._offset : self._offset + size]
        self._offset += size
        return chunk


class RecordingDB:
    def __init__(self, column_types=None, columns=None, tables=None, select_rows=None):
        self.queries = []
        self.column_types = column_types or {}
        self.columns = columns or set()
        self.tables = tables or set()
        self.select_rows = select_rows or {}

    def execute_sql(self, sql, params=None):
        self.queries.append((sql, params))
        key = sql.strip().split()[0].upper()
        if key == "SELECT":
            for matcher, rows in self.select_rows.items():
                if matcher in sql:
                    return RecordingCursor(rows)
            if "COUNT(*)" in sql:
                return RecordingCursor([(0,)])
            return RecordingCursor([])
        return RecordingCursor([])


def test_postgres_dialect_quotes_and_casts():
    mod = load_migration_module()
    config = mod.MigrationConfig(database="rag_flow")
    db = mod.PostgresMigrationDatabase(config, peewee_db=RecordingDB())

    assert db.quote_ident("tenant_llm_id") == '"tenant_llm_id"'
    assert db.datetime_from_unix("1700000000") == "TO_TIMESTAMP(1700000000)"
    assert db.column_as_text('"llm_id"') == '"llm_id"::text'
    assert db.is_integer_type("integer")
    assert db.is_integer_type("bigint")
    assert not db.is_integer_type("character varying")


def test_postgres_convert_column_uses_text_cast():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)

    db.convert_column_to_varchar32("tenant", "tenant_llm_id")

    sql, _params = recorder.queries[0]
    assert 'ALTER TABLE "tenant" ALTER COLUMN "tenant_llm_id" TYPE varchar(32)' in sql
    assert 'USING "tenant_llm_id"::text' in sql


def test_mysql_convert_column_still_uses_modify():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.MigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)

    db.convert_column_to_varchar32("tenant", "tenant_llm_id")

    sql, _params = recorder.queries[0]
    assert "MODIFY COLUMN `tenant_llm_id` VARCHAR(32) NULL" in sql


def test_tenant_model_id_stage_backfills_from_legacy_model_name():
    mod = load_migration_module()
    recorder = RecordingDB(
        tables={"tenant_model", "tenant_model_provider", "tenant_model_instance", "tenant"},
        columns={
            ("tenant", "tenant_llm_id"),
            ("tenant", "llm_id"),
            ("tenant", "tenant_embd_id"),
            ("tenant", "embd_id"),
        },
        column_types={("tenant", "tenant_llm_id"): "integer"},
        select_rows={
            "FROM tenant_model tm": [
                ("2fe23460a07a11f18c93a9473e17d659", "gpt-4o", 1, "tenant-1", "OpenAI"),
            ],
            "SELECT id,": [("tenant-1", "gpt-4o@default@OpenAI")],
        },
    )
    recorder.column_exists = lambda table, column: (table, column) in recorder.columns
    recorder.table_exists = lambda table: table in recorder.tables
    recorder.get_column_type = lambda table, column: recorder.column_types.get((table, column))

    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    # Peewee wrapper methods used by the stage go through MigrationDatabase.
    db.table_exists = recorder.table_exists
    db.column_exists = recorder.column_exists
    db.get_column_type = recorder.get_column_type

    stage = mod.TenantModelIdMigrationStage(db, dry_run=False)
    # Only migrate tenant.tenant_llm_id in this case.
    stage.TENANT_ID_FIELDS = {"tenant": [("tenant_llm_id", "llm_id", "chat")]}

    rows, tables = stage.execute()

    convert_sql = [sql for sql, _ in recorder.queries if "ALTER TABLE" in sql and "TYPE varchar(32)" in sql]
    assert convert_sql
    assert 'USING "tenant_llm_id"::text' in convert_sql[0]

    updates = [(sql, params) for sql, params in recorder.queries if sql.strip().upper().startswith("UPDATE")]
    assert any(params == ("2fe23460a07a11f18c93a9473e17d659", "tenant-1") for _sql, params in updates)
    assert rows == 1
    assert tables == ["tenant"]


def test_tenant_model_id_stage_clears_unresolved_legacy_ids():
    mod = load_migration_module()
    recorder = RecordingDB(
        tables={"tenant_model", "tenant_model_provider", "tenant_model_instance", "knowledgebase"},
        columns={
            ("knowledgebase", "tenant_embd_id"),
            ("knowledgebase", "embd_id"),
        },
        column_types={("knowledgebase", "tenant_embd_id"): "character varying"},
        select_rows={
            "FROM tenant_model tm": [],
            "SELECT id, tenant_id": [("kb-1", "tenant-1", "unknown-model@default@OpenAI")],
        },
    )
    recorder.column_exists = lambda table, column: (table, column) in recorder.columns
    recorder.table_exists = lambda table: table in recorder.tables
    recorder.get_column_type = lambda table, column: recorder.column_types.get((table, column), "character varying")

    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = recorder.table_exists
    db.column_exists = recorder.column_exists
    db.get_column_type = recorder.get_column_type

    stage = mod.TenantModelIdMigrationStage(db, dry_run=False)
    stage.TENANT_ID_FIELDS = {"knowledgebase": [("tenant_embd_id", "embd_id", "embedding")]}

    stage.execute()

    updates = [(sql, params) for sql, params in recorder.queries if sql.strip().upper().startswith("UPDATE")]
    assert any(params == ("kb-1",) and "= ''" in sql for sql, params in updates)


def test_postgres_add_auto_increment_quotes_sequence_and_id():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)

    db.add_auto_increment_unique_id_column("tenant_llm")

    assert recorder.queries == [
        ('CREATE SEQUENCE IF NOT EXISTS "tenant_llm_id_seq"', None),
        (
            'ALTER TABLE "tenant_llm" ADD COLUMN "id" BIGINT UNIQUE DEFAULT nextval(\'tenant_llm_id_seq\')',
            None,
        ),
        ('ALTER SEQUENCE "tenant_llm_id_seq" OWNED BY "tenant_llm"."id"', None),
    ]


def test_migrate_postgres_family_is_gated_to_postgres_and_gaussdb():
    source = (REPO_ROOT / "api" / "db" / "db_models.py").read_text()
    func = source.split("def migrate_postgres_family_model_provider_tables():", 1)[1].split("\ndef ", 1)[0]

    assert 'if settings.DATABASE_TYPE.upper() not in {"POSTGRES", "GAUSSDB"}:' in func
    _gate, rest = func.split('{"POSTGRES", "GAUSSDB"}:', 1)
    assert "return" in rest.split("current_version", 1)[0]
    assert "_model_provider_migration_complete(current_version)" in rest
    assert "_load_model_provider_migration_module()" in rest
    assert "run_using_existing_connection(" in rest
    assert 'dialect="postgres"' in rest
    assert "peewee_db=DB" in rest
    assert "except Exception as ex:" in rest
    assert "service will continue" in rest


def test_run_using_existing_connection_runs_both_version_groups(monkeypatch):
    mod = load_migration_module()
    seen = []

    def fake_run_migration(**kwargs):
        seen.append((kwargs["stages"], kwargs["database_version"], kwargs["dialect"], kwargs.get("peewee_db")))

    monkeypatch.setattr(mod, "run_migration", fake_run_migration)

    peewee_db = object()
    mod.run_using_existing_connection(peewee_db, dialect="postgres", database_name="rag_flow")

    assert [item[1] for item in seen] == ["v0.26.0", "v0.27.0"]
    assert seen[0][0] == mod.PROVIDER_TABLE_STAGES
    assert seen[1][0] == mod.PROVIDER_DATA_STAGES
    assert "tenant_model_id_migration" in seen[1][0]
    assert "model_type_merge" in seen[1][0]
    assert all(item[2] == "postgres" for item in seen)
    assert all(item[3] is peewee_db for item in seen)


def test_normalize_migration_dialect_maps_gaussdb(monkeypatch):
    mod = load_migration_module()
    assert mod.normalize_migration_dialect("gaussdb") == "postgres"
    assert mod.normalize_migration_dialect("postgres") == "postgres"
    assert mod.normalize_migration_dialect("mysql") == "mysql"


def test_model_provider_migration_version_helpers():
    from api.db import db_models

    assert db_models._model_provider_migration_complete("v0.27.0")
    assert db_models._model_provider_migration_complete("v0.28.0")
    assert not db_models._model_provider_migration_complete("v0.26.0")
    assert not db_models._model_provider_migration_complete(None)


def test_migrate_postgres_family_skips_when_marker_is_current(monkeypatch):
    from api.db import db_models
    from common import settings

    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(db_models, "_get_model_provider_migration_version", lambda: "v0.27.0")
    calls = []
    monkeypatch.setattr(
        db_models,
        "_load_model_provider_migration_module",
        lambda: calls.append("load") or None,
    )

    db_models.migrate_postgres_family_model_provider_tables()

    assert calls == []


def test_migrate_postgres_family_swallows_import_errors(monkeypatch, caplog):
    import logging

    from api.db import db_models
    from common import settings

    monkeypatch.setattr(settings, "DATABASE_TYPE", "postgres")
    monkeypatch.setattr(db_models, "_get_model_provider_migration_version", lambda: None)

    def boom():
        raise RuntimeError("missing script")

    monkeypatch.setattr(db_models, "_load_model_provider_migration_module", boom)

    with caplog.at_level(logging.CRITICAL):
        db_models.migrate_postgres_family_model_provider_tables()

    assert any("service will continue" in rec.message for rec in caplog.records)
