from unittest.mock import MagicMock

from tools.scripts.mysql_migration import ModelTypeMergeStage, TenantModelIdMigrationStage


def test_model_type_merge_dry_run_is_read_only():
    """Verify that ModelTypeMergeStage in dry-run mode does not create or alter tables."""
    mock_db = MagicMock()
    mock_db.table_exists.return_value = True

    sample_rows = [
        ("id1", "qwen-plus", "prov1", "inst1", "chat", "active", "{}", 1000, None, 1000, None),
        ("id2", "qwen-vl", "prov1", "inst1", "ocr", "active", "{}", 1000, None, 1000, None),
    ]
    mock_cursor = MagicMock()
    mock_cursor.fetchall.return_value = sample_rows
    mock_db.execute_sql.return_value = mock_cursor

    stage = ModelTypeMergeStage(mock_db, dry_run=True)
    rows_affected, tables = stage.execute()

    assert rows_affected == 2
    assert tables == ["tenant_model_merge_tmp"]

    # Verify that NO DDL or data modification queries were executed
    executed_queries = [call.args[0] for call in mock_db.execute_sql.call_args_list]
    for query in executed_queries:
        upper_query = query.strip().upper()
        assert not upper_query.startswith("CREATE TABLE"), f"Unexpected CREATE TABLE in dry-run: {query}"
        assert not upper_query.startswith("ALTER TABLE"), f"Unexpected ALTER TABLE in dry-run: {query}"
        assert not upper_query.startswith("DROP TABLE"), f"Unexpected DROP TABLE in dry-run: {query}"
        assert not upper_query.startswith("INSERT INTO"), f"Unexpected INSERT INTO in dry-run: {query}"


def test_model_type_merge_execute_runs_ddl_and_inserts():
    """Verify that ModelTypeMergeStage with dry_run=False creates temp table, inserts, and renames."""
    mock_db = MagicMock()
    mock_db.table_exists.return_value = True

    sample_rows = [
        ("id1", "qwen-plus", "prov1", "inst1", "chat", "active", "{}", 1000, None, 1000, None),
    ]
    mock_cursor = MagicMock()
    mock_cursor.fetchall.return_value = sample_rows
    mock_db.execute_sql.return_value = mock_cursor

    stage = ModelTypeMergeStage(mock_db, dry_run=False)
    rows_affected, tables = stage.execute()

    assert rows_affected == 1
    assert "tenant_model" in tables

    executed_queries = [call.args[0] for call in mock_db.execute_sql.call_args_list]
    # Verify CREATE TABLE was executed
    assert any("CREATE TABLE IF NOT EXISTS tenant_model_merge_tmp" in q for q in executed_queries)
    # Verify INSERT was executed
    assert any("INSERT INTO tenant_model_merge_tmp" in q for q in executed_queries)
    # Verify table renames were executed
    assert any("RENAME TO tenant_model" in q for q in executed_queries)


def test_model_type_merge_handles_str_and_numeric_model_types():
    """Verify that ModelTypeMergeStage correctly computes bitmasks for string aliases, numeric strings, and ints."""
    mock_db = MagicMock()
    mock_db.table_exists.return_value = True

    sample_rows = [
        ("id1", "ocr-model", "prov1", "inst1", "ocr", "active", "{}", 1000, None, 1000, None),
        ("id2", "ocr-model-str", "prov1", "inst2", "64", "active", "{}", 1000, None, 1000, None),
        ("id3", "ocr-model-int", "prov1", "inst3", 64, "active", "{}", 1000, None, 1000, None),
    ]
    mock_cursor = MagicMock()
    mock_cursor.fetchall.return_value = sample_rows
    mock_db.execute_sql.return_value = mock_cursor

    stage = ModelTypeMergeStage(mock_db, dry_run=True)
    rows_affected, _tables = stage.execute()

    assert rows_affected == 3


def test_tenant_model_id_migration_build_model_lookup_type_guard():
    """Verify that TenantModelIdMigrationStage._build_model_lookup handles int, numeric str, and aliases without TypeError."""
    mock_db = MagicMock()
    mock_cursor = MagicMock()
    # (model_id, model_name, model_type, tenant_id, provider_name)
    mock_cursor.fetchall.return_value = [
        ("m1", "gpt-4o", 1, "t1", "OpenAI"),
        ("m2", "paddle-ocr", "64", "t1", "Paddle"),
        ("m3", "whisper", "speech2text", "t1", "OpenAI"),
    ]
    mock_db.execute_sql.return_value = mock_cursor

    stage = TenantModelIdMigrationStage(mock_db, dry_run=True)
    lookup = stage._build_model_lookup()

    assert lookup.get(("t1", "gpt-4o", "OpenAI", "chat")) == "m1"
    assert lookup.get(("t1", "paddle-ocr", "Paddle", "ocr")) == "m2"
    assert lookup.get(("t1", "whisper", "OpenAI", "asr")) == "m3"
    assert lookup.get(("t1", "whisper", "OpenAI", "speech2text")) == "m3"
