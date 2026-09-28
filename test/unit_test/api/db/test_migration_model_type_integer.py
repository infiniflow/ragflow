"""
Regression tests for the v0.27.0 upgrade migration that converts the
``tenant_model.model_type`` column from varchar to INTEGER.

In v0.27.0 the ORM declares ``model_type`` as an ``IntegerField`` bitmask
(bit 0 = chat, bit 1 = embedding, bit 2 = asr, bit 3 = vision, bit 4 = rerank,
bit 5 = tts, bit 6 = ocr) and the query layer uses bitwise operators like
``model_type & 1`` to filter by capability. On an in-place upgrade from
v0.26.x the column was created as a varchar and stays varchar, so the
bitwise query fails with ``operator does not exist: character varying &
integer`` and every model-config lookup breaks. See issue #18755.

The fix lives in
``api.db.db_models.migrate_tenant_model_model_type_to_integer`` and is wired
into ``migrate_db`` so it executes on every startup across MySQL,
PostgreSQL, GaussDB, and OceanBase. These tests inspect the source AST to
make sure the conversion is in place and the migration runner still
invokes the helper.
"""

import ast
import inspect
import re
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]


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


def test_helper_alters_tenant_model_model_type_to_integer():
    """The helper must call alter_db_column_type on tenant_model.model_type targeting IntegerField."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_model_model_type_to_integer")
    calls = _collect_alter_column_type_calls(func)

    assert ("tenant_model", "model_type") in calls, "migrate_tenant_model_model_type_to_integer must call alter_db_column_type on ('tenant_model', 'model_type'); got " + repr(list(calls.keys()))

    call = calls[("tenant_model", "model_type")]
    new_type = call.args[3]
    assert isinstance(new_type, ast.Call), "new type must be a Peewee field call"
    callee = new_type.func
    assert isinstance(callee, ast.Name) and callee.id == "IntegerField", f"new type must be IntegerField (model_type is a bitmask), got {ast.dump(callee)}"


def test_helper_passes_pg_gaussdb_using_cast():
    """PostgreSQL / GaussDB refuse to implicitly cast varchar -> integer.

    Peewee's PostgresqlMigrator.alter_column_type only emits the ``USING``
    clause when a ``cast`` argument is supplied
    (``playhouse.migrate.PostgresqlMigrator.alter_column_type``). The helper
    must therefore pass ``cast="model_type::integer"``; ``alter_db_column_type``
    forwards it to the migrator on PG/GaussDB only (the MySQL/OceanBase
    migrators ignore the kwarg). Without the cast the ALTER fails on
    legacy-string rows that the pre-cleanup missed, and the service keeps
    running with a varchar column that the bitwise query cannot use — see
    the CodeRabbit review on PR #18885 and issue #18755.
    """
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_model_model_type_to_integer")
    calls = _collect_alter_column_type_calls(func)

    call = calls[("tenant_model", "model_type")]
    # Helper must pass the USING cast as a keyword argument.
    cast_kw = next((kw for kw in call.keywords if kw.arg == "cast"), None)
    assert cast_kw is not None, "migrate_tenant_model_model_type_to_integer must pass cast=... to alter_db_column_type so PG/GaussDB get the USING clause"
    assert isinstance(cast_kw.value, ast.Constant) and cast_kw.value.value == "model_type::integer", f"cast must be the PG/GaussDB USING clause 'model_type::integer', got {ast.dump(cast_kw.value)}"


def test_alter_db_column_type_forwards_cast_to_postgres_migrator():
    """alter_db_column_type must forward the ``cast`` kwarg to the
    PostgresqlMigrator (so the USING clause is emitted) and silently drop
    it on non-Postgres migrators (MySQL / OceanBase, which don't accept
    the kwarg).
    """
    source = _read_db_models_source()
    func = _parse_function(source, "alter_db_column_type")
    func_src = textwrap.dedent(ast.get_source_segment(source, func) or "")

    # Forwarding the kwarg means ``cast`` is plumbed through to the
    # underlying ``migrator.alter_column_type(...)`` call (we expect the
    # production code to build a kwargs dict and splat it — the literal
    # ``cast=cast`` form is not required).
    assert "migrator.alter_column_type(" in func_src, "alter_db_column_type must call migrator.alter_column_type"
    assert "kwargs" in func_src, "alter_db_column_type must splat a kwargs dict into migrator.alter_column_type(...) so the cast is forwarded"
    assert re.search(r"\*\*kwargs", func_src), "alter_db_column_type must splat **kwargs into migrator.alter_column_type(...) so the cast is forwarded"
    assert re.search(r'kwargs\["cast"\]\s*=\s*cast', func_src), "alter_db_column_type must populate kwargs['cast'] from the local cast parameter"

    # And gate the forwarding on isinstance(migrator, PostgresqlMigrator)
    # so MySQL/OceanBase don't see an unexpected kwarg.
    assert "isinstance(migrator, PostgresqlMigrator)" in func_src, "alter_db_column_type must gate the cast on isinstance(migrator, PostgresqlMigrator)"

    # The function must now accept the new ``cast`` parameter.
    from api.db import db_models

    sig = inspect.signature(db_models.alter_db_column_type)
    assert "cast" in sig.parameters, "alter_db_column_type must accept a 'cast' parameter"
    assert sig.parameters["cast"].default is None, "alter_db_column_type's 'cast' parameter must default to None"


def test_helper_pre_cleans_legacy_string_model_types():
    """The helper must coerce legacy string enum values to integer bitmasks
    before the ALTER, otherwise the cast will fail on PostgreSQL/GaussDB
    when the column contains non-numeric data.

    The mapping mirrors the one-shot data-migration script's
    ``MODEL_TYPE_TO_INT`` so the bitmasks line up across both paths.
    """
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_model_model_type_to_integer")
    func_src = textwrap.dedent(ast.get_source_segment(source, func) or "")

    expected_legacy = {
        "chat": 1,
        "embedding": 2,
        "asr": 4,
        "speech2text": 4,
        "vision": 8,
        "image2text": 8,
        "rerank": 16,
        "tts": 32,
        "ocr": 64,
    }
    for legacy_name, bitmask in expected_legacy.items():
        assert f'"{legacy_name}"' in func_src, f"helper must pre-cleanup legacy model_type value {legacy_name!r} to integer {bitmask} before the ALTER"
        assert str(bitmask) in func_src, f"helper must use bitmask {bitmask} for legacy value {legacy_name!r}"
    assert "UPDATE tenant_model SET model_type" in func_src, "helper must UPDATE tenant_model.model_type to coerce legacy strings to integers"


def test_helper_is_idempotent_and_table_aware():
    """The helper must guard the conversion with DB.table_exists + DB.column_exists so it
    is safe to call on every startup and on fresh installs where the column already
    matches the target type."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_tenant_model_model_type_to_integer")
    func_src = textwrap.dedent(ast.get_source_segment(source, func) or "")

    assert "DB.table_exists" in func_src, "helper must check DB.table_exists before altering"
    assert "DB.column_exists" in func_src, "helper must check DB.column_exists before altering"
    assert "alter_db_column_type" in func_src, "helper must call alter_db_column_type"


def test_migrate_db_invokes_tenant_model_model_type_helper():
    """migrate_db must call the helper so the fix runs on every startup."""
    source = _read_db_models_source()
    func = _parse_function(source, "migrate_db")
    found = False
    for node in ast.walk(func):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "migrate_tenant_model_model_type_to_integer":
            found = True
            break
    assert found, "migrate_db must call migrate_tenant_model_model_type_to_integer(migrator) for issue #18755"


def test_helper_signature_is_only_migrator():
    """The helper signature must stay free of live database dependencies that
    would force a real MySQL/PostgreSQL connection during the migration."""
    from api.db import db_models

    sig = inspect.signature(db_models.migrate_tenant_model_model_type_to_integer)
    assert list(sig.parameters) == ["migrator"], f"migrate_tenant_model_model_type_to_integer must take a single 'migrator' argument, got {list(sig.parameters)}"


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
