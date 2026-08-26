"""
Regression tests for the v0.27.0 upgrade migration that converts legacy
INTEGER tenant_*_id reference columns to VARCHAR(32).

Before v0.27.0 the columns referenced the legacy llm_name / embd_id string
identifiers. After v0.27.0 the application code writes tenant_model.id (a
32-char hex PK) into them, so the column type must match the ORM. On a fresh
install the ORM creates the right type, but an in-place upgrade left the
columns as INTEGER and writes failed with "invalid input syntax for type
integer". See issue #18756.

The fix lives in `api.db.db_models.migrate_tenant_id_columns_to_varchar` and
is wired into the main `migrate_db` runner so it executes on every startup
across MySQL, PostgreSQL, GaussDB, and OceanBase. These tests inspect the
source AST to make sure every column the issue calls out is covered, and
that the migration runner still invokes the helper.
"""

import ast
import inspect
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]

EXPECTED_COLUMNS = {
    ("tenant", "tenant_llm_id"),
    ("tenant", "tenant_embd_id"),
    ("tenant", "tenant_asr_id"),
    ("tenant", "tenant_img2txt_id"),
    ("tenant", "tenant_rerank_id"),
    ("tenant", "tenant_tts_id"),
    ("tenant", "tenant_ocr_id"),
    ("knowledgebase", "tenant_embd_id"),
    ("dialog", "tenant_llm_id"),
    ("dialog", "tenant_rerank_id"),
    ("memory", "tenant_embd_id"),
    ("memory", "tenant_llm_id"),
}


def _read_db_models_source() -> str:
    return (REPO_ROOT / "api" / "db" / "db_models.py").read_text()


def _parse_function(source: str, function_name: str) -> ast.FunctionDef:
    """Return the AST node for ``function_name`` defined in ``source``."""
    tree = ast.parse(source)
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == function_name:
            return node
    raise AssertionError(f"{function_name} not found in db_models.py")


def _collect_alter_column_type_calls(func: ast.FunctionDef) -> dict[tuple[str, str], ast.Call]:
    """Map (table, column) -> AST call node for every alter_db_column_type(...)."""
    calls: dict[tuple[str, str], ast.Call] = {}
    for node in ast.walk(func):
        if not isinstance(node, ast.Call):
            continue
        if not (isinstance(node.func, ast.Name) and node.func.id == "alter_db_column_type"):
            continue
        # Signature: alter_db_column_type(migrator, table_name, column_name, new_column_type)
        if len(node.args) < 3:
            continue
        table = node.args[1]
        column = node.args[2]
        if isinstance(table, ast.Constant) and isinstance(column, ast.Constant):
            calls[(table.value, column.value)] = node
    return calls


def test_migrate_tenant_id_columns_alter_type_for_every_column():
    """The helper must call alter_db_column_type for every column the issue calls out."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_id_columns_to_varchar")
    calls = _collect_alter_column_type_calls(func)

    missing = EXPECTED_COLUMNS - calls.keys()
    extra = calls.keys() - EXPECTED_COLUMNS
    assert not missing, f"migrate_tenant_id_columns_to_varchar is missing alter_db_column_type for: {sorted(missing)}"
    assert not extra, f"migrate_tenant_id_columns_to_varchar has unexpected alter_db_column_type calls: {sorted(extra)}"
    assert len(calls) == 12, f"expected exactly 12 alter_db_column_type calls, got {len(calls)}"


def test_migrate_tenant_id_columns_uses_varchar_32():
    """Every alter_db_column_type call must target CharField(max_length=32)."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_id_columns_to_varchar")
    calls = _collect_alter_column_type_calls(func)
    assert calls, "no alter_db_column_type calls collected"

    for (table, column), call in calls.items():
        new_type = call.args[3]
        assert isinstance(new_type, ast.Call), f"{table}.{column}: new type must be a CharField(...) call"
        callee = new_type.func
        assert isinstance(callee, ast.Name) and callee.id == "CharField", f"{table}.{column}: new type must be CharField, got {ast.dump(callee)}"
        max_length_kw = next(
            (kw for kw in new_type.keywords if kw.arg == "max_length"),
            None,
        )
        assert max_length_kw is not None, f"{table}.{column}: CharField() must set max_length=32"
        assert isinstance(max_length_kw.value, ast.Constant) and max_length_kw.value.value == 32, f"{table}.{column}: max_length must be 32, got {ast.dump(max_length_kw.value)}"
        null_kw = next((kw for kw in new_type.keywords if kw.arg == "null"), None)
        assert null_kw is not None and isinstance(null_kw.value, ast.Constant) and null_kw.value.value is True, (
            f"{table}.{column}: CharField() must be null=True so the migration matches the ORM nullability"
        )


def test_migrate_db_invokes_tenant_id_varchar_helper():
    """migrate_db must call migrate_tenant_id_columns_to_varchar so the fix runs on every startup."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_db")
    found = False
    for node in ast.walk(func):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "migrate_tenant_id_columns_to_varchar":
            found = True
            break
    assert found, "migrate_db must call migrate_tenant_id_columns_to_varchar(migrator) for issue #18756"


def test_helper_signature_is_only_migrator():
    """The helper signature must stay free of live database dependencies that
    would force a real MySQL/PostgreSQL connection during the migration."""
    from api.db import db_models

    sig = inspect.signature(db_models.migrate_tenant_id_columns_to_varchar)
    assert list(sig.parameters) == ["migrator"], f"migrate_tenant_id_columns_to_varchar must take a single 'migrator' argument, got {list(sig.parameters)}"


def test_helper_source_is_idempotent_and_table_aware():
    """The helper must guard against missing tables/columns so it is safe to
    re-run on every startup and on fresh installs where columns already match
    the target type."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_id_columns_to_varchar")
    func_src = textwrap.dedent(ast.get_source_segment(source, func) or "")

    assert "DB.table_exists" in func_src, "helper must check DB.table_exists before altering"
    assert "DB.column_exists" in func_src, "helper must check DB.column_exists before altering"
    assert "alter_db_column_type" in func_src, "helper must call alter_db_column_type to convert existing INTEGER columns"
    assert func_src.count("alter_db_column_type") == 12, f"helper must call alter_db_column_type exactly 12 times (one per required column), got {func_src.count('alter_db_column_type')}"


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
