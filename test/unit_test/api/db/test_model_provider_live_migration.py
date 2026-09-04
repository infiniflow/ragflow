"""Live PostgreSQL / MySQL checks for model_type merge and tenant_*_id backfill.

Lynn-Inf: unit stubs do not exercise the tenant_model table swap or INTEGER →
varchar(32) backfill. These tests start ephemeral Docker databases when Docker
is available, otherwise they skip.
"""

from __future__ import annotations

import importlib.util
import socket
import subprocess
import time
import uuid
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
MIGRATION_SCRIPT = REPO_ROOT / "tools" / "scripts" / "mysql_migration.py"

TENANT_ID = "tenant00000000000000000000000001"
PROVIDER_ID = "provider000000000000000000000001"
INSTANCE_ID = "instance00000000000000000000001"
CHAT_ROW_ID = "modelchat00000000000000000000001"
VISION_ROW_ID = "modelvisn00000000000000000000001"


def load_migration_module():
    spec = importlib.util.spec_from_file_location("ragflow_mysql_migration_live_test", MIGRATION_SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _docker_available() -> bool:
    try:
        subprocess.run(["docker", "info"], check=True, capture_output=True, timeout=15)
        return True
    except Exception:
        return False


def _wait_port(host: str, port: int, timeout: float = 90.0):
    deadline = time.time() + timeout
    last_err = None
    while time.time() < deadline:
        try:
            with socket.create_connection((host, port), timeout=1):
                return
        except OSError as exc:
            last_err = exc
            time.sleep(0.5)
    raise TimeoutError(f"{host}:{port} did not accept connections: {last_err}")


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def _docker_run(args: list[str]) -> str:
    name = f"ragflow-mig-{uuid.uuid4().hex[:10]}"
    subprocess.run(["docker", "run", "-d", "--rm", "--name", name, *args], check=True, capture_output=True, text=True)
    return name


def _docker_rm(name: str | None):
    if not name:
        return
    subprocess.run(["docker", "rm", "-f", name], check=False, capture_output=True)


def _legacy_schema_sql(dialect: str) -> list[str]:
    ts = "TIMESTAMP" if dialect == "postgres" else "DATETIME"
    int_type = "INTEGER" if dialect == "postgres" else "INT"
    return [
        f"""
        CREATE TABLE system_settings (
            name VARCHAR(255) PRIMARY KEY,
            source VARCHAR(255),
            data_type VARCHAR(32),
            value TEXT,
            create_time BIGINT,
            create_date {ts},
            update_time BIGINT,
            update_date {ts}
        )
        """,
        """
        CREATE TABLE tenant_model_provider (
            id VARCHAR(32) PRIMARY KEY,
            provider_name VARCHAR(128) NOT NULL,
            tenant_id VARCHAR(32) NOT NULL
        )
        """,
        """
        CREATE TABLE tenant_model_instance (
            id VARCHAR(32) PRIMARY KEY,
            instance_name VARCHAR(128),
            provider_id VARCHAR(32) NOT NULL,
            api_key TEXT,
            status VARCHAR(32),
            extra VARCHAR(1024) DEFAULT '{}'
        )
        """,
        f"""
        CREATE TABLE tenant_model (
            id VARCHAR(32) PRIMARY KEY,
            model_name VARCHAR(128),
            provider_id VARCHAR(32) NOT NULL,
            instance_id VARCHAR(32) NOT NULL,
            model_type VARCHAR(128) NOT NULL,
            status VARCHAR(32) DEFAULT 'active',
            extra VARCHAR(1024) DEFAULT '{{}}',
            create_time BIGINT,
            create_date {ts},
            update_time BIGINT,
            update_date {ts}
        )
        """,
        f"""
        CREATE TABLE tenant (
            id VARCHAR(32) PRIMARY KEY,
            llm_id VARCHAR(128),
            tenant_llm_id {int_type}
        )
        """,
    ]


def _seed_legacy_rows(db):
    db.execute_sql(
        "INSERT INTO tenant_model_provider (id, provider_name, tenant_id) VALUES (%s, %s, %s)",
        (PROVIDER_ID, "OpenAI", TENANT_ID),
    )
    db.execute_sql(
        "INSERT INTO tenant_model_instance (id, instance_name, provider_id, api_key, status, extra) VALUES (%s, %s, %s, %s, %s, %s)",
        (INSTANCE_ID, "default", PROVIDER_ID, "sk-test", "1", "{}"),
    )
    ts = 1700000000000
    for row_id, model_type in ((CHAT_ROW_ID, "chat"), (VISION_ROW_ID, "vision")):
        db.execute_sql(
            """
            INSERT INTO tenant_model
            (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, update_time)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            """,
            (row_id, "gpt-4o", PROVIDER_ID, INSTANCE_ID, model_type, "active", "{}", ts, ts),
        )
    db.execute_sql(
        "INSERT INTO tenant (id, llm_id, tenant_llm_id) VALUES (%s, %s, %s)",
        (TENANT_ID, "gpt-4o@default@OpenAI", 42),
    )


def _run_data_stages(mod, dialect: str, database: str, host: str, port: int, user: str, password: str):
    config = mod.MigrationConfig(host=host, port=port, user=user, password=password, database=database)
    mod.run_migration(
        config=config,
        stages=["model_type_merge", "tenant_model_id_migration"],
        dry_run=False,
        database_version="v0.27.0",
        mark_database_version_on_success=True,
        dialect=dialect,
    )


def _assert_upgraded(db, dialect: str):
    model_type = db.get_column_type("tenant_model", "model_type")
    assert db.is_integer_type(model_type), model_type
    assert db.table_exists("tenant_model_backup")

    rows = db.execute_sql("SELECT model_name, model_type, status FROM tenant_model").fetchall()
    assert len(rows) == 1
    assert rows[0][0] == "gpt-4o"
    assert int(rows[0][1]) == 9  # chat | vision
    assert rows[0][2] == "active"

    llm_type = db.get_column_type("tenant", "tenant_llm_id")
    assert llm_type and "char" in llm_type.lower()
    tenant_row = db.execute_sql("SELECT tenant_llm_id FROM tenant WHERE id = %s", (TENANT_ID,)).fetchone()
    hex_id = tenant_row[0]
    assert hex_id and len(hex_id) == 32
    assert hex_id != "42"
    model_ids = {row[0] for row in db.execute_sql("SELECT id FROM tenant_model").fetchall()}
    assert hex_id in model_ids

    version = db.get_database_version()
    assert version == "v0.27.0"


def test_run_migration_does_not_mark_version_when_a_stage_raises(monkeypatch):
    mod = load_migration_module()
    versions = []

    class FakeDB:
        def connect(self):
            return None

        def close(self):
            return None

        def get_database_version(self):
            return None

        def set_database_version(self, version):
            versions.append(version)

    class BoomStage:
        description = "boom"
        source_tables = []
        target_tables = []

        def __init__(self, db, dry_run=False, create_table_only=False):
            pass

        def check(self):
            return True

        def execute(self):
            raise RuntimeError("merge exploded")

    monkeypatch.setattr(mod, "create_migration_database", lambda *args, **kwargs: FakeDB())
    monkeypatch.setattr(mod, "MIGRATION_STAGES", {"model_type_merge": BoomStage})

    with pytest.raises(RuntimeError, match="merge exploded"):
        mod.run_migration(
            config=mod.MigrationConfig(database="rag_flow"),
            stages=["model_type_merge"],
            dry_run=False,
            database_version="v0.27.0",
            mark_database_version_on_success=True,
            dialect="postgres",
        )

    assert versions == []


@pytest.mark.skipif(not _docker_available(), reason="Docker is required for live migration tests")
def test_postgres_merge_swaps_table_and_backfills_tenant_llm_id():
    from peewee import PostgresqlDatabase

    port = _free_port()
    name = None
    db = None
    try:
        name = _docker_run(
            [
                "-e",
                "POSTGRES_PASSWORD=test",
                "-e",
                "POSTGRES_DB=rag_flow_mig",
                "-p",
                f"{port}:5432",
                "postgres:16",
            ]
        )
        _wait_port("127.0.0.1", port)
        deadline = time.time() + 90
        last_err = None
        while time.time() < deadline:
            try:
                db = PostgresqlDatabase("rag_flow_mig", host="127.0.0.1", port=port, user="postgres", password="test")
                db.connect()
                break
            except Exception as exc:
                last_err = exc
                time.sleep(0.5)
        else:
            raise TimeoutError(f"postgres not ready: {last_err}")

        for stmt in _legacy_schema_sql("postgres"):
            db.execute_sql(stmt)
        _seed_legacy_rows(db)
        db.close()

        mod = load_migration_module()
        _run_data_stages(mod, "postgres", "rag_flow_mig", "127.0.0.1", port, "postgres", "test")

        db = PostgresqlDatabase("rag_flow_mig", host="127.0.0.1", port=port, user="postgres", password="test")
        db.connect()
        wrapper = mod.PostgresMigrationDatabase(mod.MigrationConfig(database="rag_flow_mig"), peewee_db=db)
        _assert_upgraded(wrapper, "postgres")

        before = db.execute_sql("SELECT id, model_type FROM tenant_model").fetchall()
        _run_data_stages(mod, "postgres", "rag_flow_mig", "127.0.0.1", port, "postgres", "test")
        after = db.execute_sql("SELECT id, model_type FROM tenant_model").fetchall()
        assert after == before
    finally:
        if db is not None and not db.is_closed():
            db.close()
        _docker_rm(name)


@pytest.mark.skipif(not _docker_available(), reason="Docker is required for live migration tests")
def test_mysql_merge_swaps_table_and_backfills_tenant_llm_id():
    from peewee import MySQLDatabase

    port = _free_port()
    name = None
    db = None
    try:
        name = _docker_run(
            [
                "-e",
                "MYSQL_ROOT_PASSWORD=test",
                "-e",
                "MYSQL_DATABASE=rag_flow_mig",
                "-p",
                f"{port}:3306",
                "mysql:8.0.40",
            ]
        )
        _wait_port("127.0.0.1", port)
        deadline = time.time() + 120
        last_err = None
        while time.time() < deadline:
            try:
                db = MySQLDatabase("rag_flow_mig", host="127.0.0.1", port=port, user="root", password="test")
                db.connect()
                break
            except Exception as exc:
                last_err = exc
                time.sleep(1)
        else:
            raise TimeoutError(f"mysql not ready: {last_err}")

        for stmt in _legacy_schema_sql("mysql"):
            db.execute_sql(stmt)
        _seed_legacy_rows(db)
        db.close()

        mod = load_migration_module()
        _run_data_stages(mod, "mysql", "rag_flow_mig", "127.0.0.1", port, "root", "test")

        db = MySQLDatabase("rag_flow_mig", host="127.0.0.1", port=port, user="root", password="test")
        db.connect()
        wrapper = mod.MigrationDatabase(mod.MigrationConfig(database="rag_flow_mig"), peewee_db=db)
        _assert_upgraded(wrapper, "mysql")

        before = db.execute_sql("SELECT id, model_type FROM tenant_model").fetchall()
        _run_data_stages(mod, "mysql", "rag_flow_mig", "127.0.0.1", port, "root", "test")
        after = db.execute_sql("SELECT id, model_type FROM tenant_model").fetchall()
        assert after == before
    finally:
        if db is not None and not db.is_closed():
            db.close()
        _docker_rm(name)
