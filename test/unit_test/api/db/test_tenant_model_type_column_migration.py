"""
Tests for tenant_model.model_type integer conversion on upgrade (#18755 / #18781).

Lynn-Inf review: an in-place ALTER in migrate_db() only does type/value
conversion. ModelTypeMergeStage also merges duplicate rows, and
TenantModelSeedingStage seeds missing models — both skip when model_type is
already INT. Postgres upgrades must run those stages, not a shallow ALTER.
"""

import importlib.util
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[4]
MIGRATION_SCRIPT = REPO_ROOT / "tools" / "scripts" / "mysql_migration.py"
DB_MODELS_PATH = REPO_ROOT / "api" / "db" / "db_models.py"


def load_migration_module():
    spec = importlib.util.spec_from_file_location("ragflow_mysql_migration_model_type_test", MIGRATION_SCRIPT)
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
    def __init__(self, select_rows=None):
        self.queries = []
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


def _tenant_row(row_id, model_name, provider_id, instance_id, model_type, status="active", extra="{}"):
    ts = 1700000000000
    return (row_id, model_name, provider_id, instance_id, model_type, status, extra, ts, None, ts, None)


def test_model_type_to_bits_maps_names_aliases_numbers_and_combined_tokens():
    merge = load_migration_module().ModelTypeMergeStage

    assert merge.model_type_to_bits(None) == 0
    assert merge.model_type_to_bits("") == 0
    assert merge.model_type_to_bits("   ") == 0
    assert merge.model_type_to_bits("chat") == 1
    assert merge.model_type_to_bits("embedding") == 2
    assert merge.model_type_to_bits("speech2text") == 4
    assert merge.model_type_to_bits("asr") == 4
    assert merge.model_type_to_bits("image2text") == 8
    assert merge.model_type_to_bits("vision") == 8
    assert merge.model_type_to_bits("rerank") == 16
    assert merge.model_type_to_bits("tts") == 32
    assert merge.model_type_to_bits("ocr") == 64
    assert merge.model_type_to_bits("9") == 9
    assert merge.model_type_to_bits(" 9 ") == 9
    assert merge.model_type_to_bits(9) == 9
    assert merge.model_type_to_bits("chat|vision") == 9
    assert merge.model_type_to_bits(" chat | vision ") == 9
    assert merge.model_type_to_bits("unknown") == 0


def test_model_type_merge_skips_when_column_is_already_integer():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = lambda table: table == "tenant_model"
    db.get_column_type = lambda table, column: "integer"

    stage = mod.ModelTypeMergeStage(db, dry_run=False)

    assert stage.check() is False
    assert stage.noop_completes_migration() is True


def test_model_type_merge_runs_while_column_is_varchar():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = lambda table: table == "tenant_model"
    db.get_column_type = lambda table, column: "character varying"

    stage = mod.ModelTypeMergeStage(db, dry_run=False)

    assert stage.check() is True
    assert stage.noop_completes_migration() is False


def test_tenant_model_seeding_skips_when_model_type_is_already_integer():
    mod = load_migration_module()
    recorder = RecordingDB()
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = lambda table: table in {"tenant_model", "tenant_model_provider", "tenant_model_instance"}
    db.get_column_type = lambda table, column: "integer"

    stage = mod.TenantModelSeedingStage(db, dry_run=False)

    assert stage.check() is False
    assert stage.noop_completes_migration() is True


def test_provider_data_stages_seed_then_merge_then_id_backfill():
    mod = load_migration_module()

    assert mod.PROVIDER_DATA_STAGES == [
        "tenant_model_seeding",
        "model_type_merge",
        "tenant_model_id_migration",
    ]


def test_model_type_merge_dedupes_chat_and_embedding_rows_to_bitmask():
    mod = load_migration_module()
    rows = [
        _tenant_row("id-chat", "gpt-4o", "p1", "i1", "chat"),
        _tenant_row("id-embd", "gpt-4o", "p1", "i1", "embedding"),
        _tenant_row("id-other", "text-embedding-3", "p1", "i1", "embedding"),
    ]
    recorder = RecordingDB(select_rows={"FROM tenant_model ORDER BY id": rows})
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = lambda table: table == "tenant_model"

    stage = mod.ModelTypeMergeStage(db, dry_run=False)
    uuids = iter(["merged00000000000000000000000001", "merged00000000000000000000000002"])
    stage.generate_uuid = lambda: next(uuids)

    inserted, tables = stage.execute()

    assert inserted == 2
    assert "tenant_model" in tables
    insert_sql = [sql for sql, _ in recorder.queries if sql.strip().upper().startswith("INSERT")]
    assert len(insert_sql) == 1
    assert ", 3, 'active'" in insert_sql[0]
    assert ", 2, 'active'" in insert_sql[0]
    assert "CREATE TABLE IF NOT EXISTS tenant_model_merge_tmp" in "".join(sql for sql, _ in recorder.queries)
    assert "model_type INT NOT NULL" in "".join(sql for sql, _ in recorder.queries)
    assert "CREATE INDEX IF NOT EXISTS idx_instance_id_merge_tmp ON tenant_model_merge_tmp" in "".join(sql for sql, _ in recorder.queries)
    assert any("RENAME TO tenant_model_backup" in sql for sql, _ in recorder.queries)
    assert any("RENAME TO tenant_model" in sql and "tenant_model_merge_tmp" in sql for sql, _ in recorder.queries)


def test_model_type_merge_ors_combined_tokens_and_defaults_empty_to_chat():
    mod = load_migration_module()
    rows = [
        _tenant_row("id-combo", "gpt-4o", "p1", "i1", "chat|vision"),
        _tenant_row("id-empty", "legacy-model", "p1", "i1", ""),
        _tenant_row("id-unsupported", "gpt-4o", "p1", "i1", "embedding", status="unsupported"),
    ]
    recorder = RecordingDB(select_rows={"FROM tenant_model ORDER BY id": rows})
    db = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow"), peewee_db=recorder)
    db.table_exists = lambda table: table == "tenant_model"

    stage = mod.ModelTypeMergeStage(db, dry_run=False)
    uuids = iter(["idcombo000000000000000000000001", "idempty000000000000000000000001"])
    stage.generate_uuid = lambda: next(uuids)

    inserted, _tables = stage.execute()

    assert inserted == 2
    insert_sql = [sql for sql, _ in recorder.queries if sql.strip().upper().startswith("INSERT")][0]
    # chat|vision (9) with unsupported embedding (2) keeps chat|vision bits.
    assert ", 9, 'active'" in insert_sql
    # empty/unknown bits default to chat (1) so lookups still match.
    assert ", 1, 'active'" in insert_sql


def test_migrate_db_does_not_alter_model_type_in_place_before_merge():
    source = DB_MODELS_PATH.read_text()

    assert "def migrate_tenant_model_type_column" not in source
    assert 'ALTER COLUMN "model_type" TYPE integer' not in source
    assert "migrate_postgres_family_model_provider_tables()" in source

    helper = source.split("def migrate_tenant_model_id_column_types(", 1)[1].split("\ndef ", 1)[0]
    helper_flat = " ".join(helper.split())
    assert "Fallback:" in helper
    assert "converts if the script did not run" in helper_flat

    migrate_db = source.split("def migrate_db(", 1)[1].split("\ndef ", 1)[0]
    assert "migrate_postgres_family_model_provider_tables()" in migrate_db
    assert "migrate_tenant_model_id_column_types(migrator)" in migrate_db
    assert "Fallback only:" in migrate_db
    assert migrate_db.index("migrate_postgres_family_model_provider_tables()") < migrate_db.index("ensure_model_indexes(migrator)")
