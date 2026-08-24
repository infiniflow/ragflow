"""
User-flow tests for running RAGFlow with DB_TYPE=GaussDB.
"""

import os
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
HELPER_DIR = Path(__file__).resolve().parent


def run_isolated_flow(script: str, env_extra: dict[str, str], timeout: int = 60):
    env = os.environ.copy()
    env.update(env_extra)
    env["PYTHONPATH"] = os.pathsep.join(
        [
            str(REPO_ROOT),
            str(HELPER_DIR),
            env.get("PYTHONPATH", ""),
        ]
    )

    bootstrap = """
from gaussdb_test_utils import stub_settings_import_dependencies
stub_settings_import_dependencies()
"""
    result = subprocess.run(
        [sys.executable, "-c", textwrap.dedent(bootstrap + script)],
        cwd=REPO_ROOT,
        env=env,
        text=True,
        capture_output=True,
        timeout=timeout,
        check=False,
    )
    assert result.returncode == 0, f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}"
    return result


def test_user_env_selects_gaussdb_metadata_adapter(monkeypatch):
    monkeypatch.delenv("GAUSSDB_METADATA_OPTIONS", raising=False)
    result = run_isolated_flow(
        """
from common import settings
from api.db.db_models import DB, LongTextField

assert settings.DATABASE_TYPE == "gaussdb"
assert settings.DATABASE["host"] == "gaussdb.local"
assert settings.DATABASE["port"] == 8000
assert settings.DATABASE["user"] == "rag_flow"
assert settings.DATABASE["name"] == "rag_flow"
assert settings.DATABASE["options"] == "-c search_path=ragflow_meta -c client_encoding=UTF8 -c default_transaction_read_only=off"
assert DB.__class__.__name__ == "RetryingPooledGaussDBDatabase"
assert DB.lock.__name__ == "GaussDBDatabaseLock"
assert LongTextField.field_type == "TEXT"
print("gaussdb-adapter-ok")
""",
        {
            "DB_TYPE": "GaussDB",
            "GAUSSDB_METADATA_HOST": "gaussdb.local",
            "GAUSSDB_METADATA_PORT": "8000",
            "GAUSSDB_METADATA_USER": "rag_flow",
            "GAUSSDB_METADATA_PASSWORD": "secret",
            "GAUSSDB_METADATA_DBNAME": "rag_flow",
            "GAUSSDB_METADATA_SCHEMA": "ragflow_meta",
        },
    )

    assert "gaussdb-adapter-ok" in result.stdout


def test_gaussdb_empty_string_compatible_fields_are_nullable_only_for_gaussdb():
    """Verify empty-string field behavior across supported metadata databases."""

    gaussdb_result = run_isolated_flow(
        """
from api.db.db_models import API4Conversation, Dialog, Document, File, Knowledgebase, Memory, SyncLogs, SystemSettings, Task, Tenant, User, UserCanvas

fields = [
    User.nickname,
    Tenant.llm_id,
    Tenant.embd_id,
    Tenant.asr_id,
    Tenant.img2txt_id,
    Tenant.rerank_id,
    File.source_type,
    Knowledgebase.embd_id,
    Dialog.description,
    Dialog.icon,
    Dialog.llm_id,
    Dialog.rerank_id,
    Memory.embd_id,
    Memory.llm_id,
    Document.suffix,
    SystemSettings.value,
    Task.task_type,
    Task.progress_msg,
    Task.chunk_ids,
    SyncLogs.error_msg,
    SyncLogs.full_exception_trace,
    API4Conversation.user_id,
    UserCanvas.tags,
]
for field in fields:
    assert field.null is True
    assert field.db_value("") is None
    assert field.python_value(None) == ""

eq_sql, eq_params = File.select().where(File.source_type == "").sql()
ne_sql, ne_params = File.select().where(File.source_type != "").sql()
field_eq_sql, field_eq_params = Tenant.select().where(Tenant.llm_id == Tenant.embd_id).sql()
field_ne_sql, field_ne_params = Tenant.select().where(Tenant.llm_id != Tenant.embd_id).sql()

assert '"source_type" IS NULL' in eq_sql
assert 'LENGTH("t1"."source_type") = %s' in eq_sql
assert eq_params == [0]
assert 'LENGTH("t1"."source_type") > %s' in ne_sql
assert ne_params == [0]
assert '"t1"."llm_id" = "t1"."embd_id"' in field_eq_sql
assert field_eq_params == []
assert '"t1"."llm_id" != "t1"."embd_id"' in field_ne_sql
assert field_ne_params == []
print("gaussdb-empty-string-compatible-ok")
""",
        {
            "DB_TYPE": "GaussDB",
            "GAUSSDB_METADATA_HOST": "gaussdb.local",
            "GAUSSDB_METADATA_PORT": "8000",
            "GAUSSDB_METADATA_USER": "rag_flow",
            "GAUSSDB_METADATA_PASSWORD": "secret",
            "GAUSSDB_METADATA_DBNAME": "rag_flow",
        },
    )
    assert "gaussdb-empty-string-compatible-ok" in gaussdb_result.stdout

    mysql_result = run_isolated_flow(
        """
from api.db.db_models import API4Conversation, Dialog, Document, File, Knowledgebase, Memory, SyncLogs, SystemSettings, Task, Tenant, User, UserCanvas

fields = [
    (User.nickname, False),
    (Tenant.llm_id, False),
    (Tenant.embd_id, False),
    (Tenant.asr_id, False),
    (Tenant.img2txt_id, False),
    (Tenant.rerank_id, False),
    (File.source_type, False),
    (Knowledgebase.embd_id, False),
    (Dialog.description, True),
    (Dialog.icon, True),
    (Dialog.llm_id, False),
    (Dialog.rerank_id, False),
    (Memory.embd_id, False),
    (Memory.llm_id, False),
    (Document.suffix, False),
    (SystemSettings.value, False),
    (Task.task_type, False),
    (Task.progress_msg, True),
    (Task.chunk_ids, True),
    (SyncLogs.error_msg, False),
    (SyncLogs.full_exception_trace, True),
    (API4Conversation.user_id, False),
    (UserCanvas.tags, False),
]
for field, expected_null in fields:
    assert field.null is expected_null
    assert field.db_value("") == ""

for field in (Dialog.description, Dialog.icon):
    assert field.python_value(None) is None

assert Task.chunk_ids.field_type == "LONGTEXT"
sql, params = File.select().where(File.source_type == "").sql()
assert "IS NULL" not in sql
assert "LENGTH" not in sql
assert params == [""]
print("mysql-nullability-ok")
""",
        {
            "DB_TYPE": "mysql",
        },
    )
    assert "mysql-nullability-ok" in mysql_result.stdout

    postgres_result = run_isolated_flow(
        """
from common import config_utils

config_utils.CONFIGS["postgres"] = {
    "name": "rag_flow",
    "user": "rag_flow",
    "password": "test-password",
    "host": "postgres.local",
    "port": 5432,
    "max_connections": 1,
    "stale_timeout": 30,
}
from api.db.db_models import Dialog

for field in (Dialog.description, Dialog.icon):
    assert field.null is True
    assert field.db_value("") == ""
    assert field.python_value(None) is None

sql, params = Dialog.select().where(Dialog.icon == "").sql()
assert "IS NULL" not in sql
assert "LENGTH" not in sql
assert params == [""]
print("postgres-empty-string-behavior-ok")
""",
        {
            "DB_TYPE": "postgres",
        },
    )
    assert "postgres-empty-string-behavior-ok" in postgres_result.stdout


def test_live_gaussdb_init_lock_and_user_crud_flow():
    if os.getenv("RAGFLOW_GAUSSDB_LIVE") != "1":
        pytest.skip("Set RAGFLOW_GAUSSDB_LIVE=1 and GAUSSDB_METADATA_* env vars to run live GaussDB flow")

    required = ["GAUSSDB_METADATA_HOST", "GAUSSDB_METADATA_PORT", "GAUSSDB_METADATA_USER", "GAUSSDB_METADATA_PASSWORD", "GAUSSDB_METADATA_DBNAME"]
    missing = [name for name in required if not os.getenv(name)]
    if missing:
        pytest.skip(f"Missing live GaussDB env vars: {', '.join(missing)}")

    env_extra = {
        "DB_TYPE": "GaussDB",
        "GAUSSDB_METADATA_HOST": os.environ["GAUSSDB_METADATA_HOST"],
        "GAUSSDB_METADATA_PORT": os.environ["GAUSSDB_METADATA_PORT"],
        "GAUSSDB_METADATA_USER": os.environ["GAUSSDB_METADATA_USER"],
        "GAUSSDB_METADATA_PASSWORD": os.environ["GAUSSDB_METADATA_PASSWORD"],
        "GAUSSDB_METADATA_DBNAME": os.environ["GAUSSDB_METADATA_DBNAME"],
        "GAUSSDB_METADATA_SCHEMA": os.getenv("GAUSSDB_METADATA_SCHEMA", "public"),
        "GAUSSDB_METADATA_MAX_CONNECTIONS": os.getenv("GAUSSDB_METADATA_MAX_CONNECTIONS", "20"),
        "GAUSSDB_METADATA_STALE_TIMEOUT": os.getenv("GAUSSDB_METADATA_STALE_TIMEOUT", "30"),
    }

    result = run_isolated_flow(
        """
import os
import uuid

from common import settings
from api.db.db_models import DB, Tenant, User, init_database_tables
from api.db.services.user_service import TenantService
from api.utils.health_utils import get_database_status

assert settings.DATABASE_TYPE == "gaussdb"
init_database_tables()

expected_distributed = os.environ.get("EXPECT_DISTRIBUTED") == "1"
assert DB.is_distributed is expected_distributed
if expected_distributed:
    rows = DB.execute_sql(
        "SELECT c.relname, x.pclocatortype "
        "FROM pg_catalog.pgxc_class x "
        "JOIN pg_catalog.pg_class c ON c.oid = x.pcrelid "
        "JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace "
        "WHERE n.nspname = current_schema() "
        "AND c.relname IN ('compilation_template', 'compilation_template_group', "
        "'file_commit_item', 'tenant_llm', 'tenant_model_provider', 'user')"
    ).fetchall()
    assert dict(rows) == {
        "compilation_template": "R",
        "compilation_template_group": "R",
        "file_commit_item": "R",
        "tenant_llm": "R",
        "tenant_model_provider": "R",
        "user": "R",
    }

health = get_database_status()
assert health["status"] == "alive"
assert health["message"]["database"] == "gaussdb"
assert health["message"]["result"] == 1

uid = uuid.uuid4().hex
email = f"{uid}@ragflow-gaussdb-flow.test"
tenant_id = uuid.uuid4().hex
replacement_tenant_id = uuid.uuid4().hex

with DB.connection_context():
    with DB.lock("gaussdb_user_flow_test", 10):
        try:
            User.create(id=uid, email=email, nickname="gaussdb-flow", password="x")
            row = User.get(User.id == uid)
            assert row.id == uid
            assert row.email == email
        finally:
            User.delete().where(User.id == uid).execute()

    assert User.select().where(User.id == uid).count() == 0

    try:
        Tenant.create(
            id=tenant_id,
            name="gaussdb-empty-string-flow",
            llm_id="",
            embd_id="",
            asr_id="",
            img2txt_id="",
            rerank_id="",
            parser_ids="naive:General",
        )
        raw_row = DB.execute_sql(
            "select llm_id, embd_id, asr_id, img2txt_id, rerank_id from tenant where id=%s",
            (tenant_id,),
        ).fetchone()
        assert raw_row == (None, None, None, None, None)

        tenant = Tenant.get(Tenant.id == tenant_id)
        assert tenant.llm_id == ""
        assert tenant.embd_id == ""
        assert tenant.asr_id == ""
        assert tenant.img2txt_id == ""
        assert tenant.rerank_id == ""

        assert Tenant.select().where(Tenant.id == tenant_id, Tenant.llm_id == "").count() == 1
        assert Tenant.select().where(Tenant.id == tenant_id, Tenant.llm_id != "").count() == 0

        assert TenantService.update_by_id(
            tenant_id,
            {"id": replacement_tenant_id, "name": "gaussdb-primary-key-replacement"},
        ) == 1
        assert Tenant.select().where(Tenant.id == tenant_id).count() == 0
        assert Tenant.select().where(Tenant.id == replacement_tenant_id).count() == 1
    finally:
        Tenant.delete().where(Tenant.id == tenant_id).execute()
        Tenant.delete().where(Tenant.id == replacement_tenant_id).execute()

DB.close()
print("gaussdb-live-flow-ok")
""",
        env_extra,
        timeout=120,
    )

    assert "gaussdb-live-flow-ok" in result.stdout
