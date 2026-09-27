"""Tests for GaussDB Peewee ORM support."""

import pytest
from peewee import CharField, Model, OperationalError, SqliteDatabase
from playhouse.pool import PooledPostgresqlDatabase

from api.db.db_models import (
    DatabaseLock,
    DatabaseMigrator,
    GaussDBDatabaseLock,
    GaussDBMigrator,
    PostgresDatabaseLock,
    PooledDatabase,
    RetryingPooledGaussDBDatabase,
    RetryingPooledPostgresqlDatabase,
    TextFieldType,
)
from common.settings import load_database_config, normalize_database_type
import api.db.db_models as db_models


class RecordingCursor:
    def __init__(self, rows):
        self.rows = rows

    def fetchall(self):
        return self.rows


class LockCursor:
    def __init__(self, value):
        self.value = value

    def fetchone(self):
        return (self.value,)


class SequenceLockDB:
    def __init__(self, values):
        self.values = iter(values)
        self.calls = []

    def execute_sql(self, sql, params=None):
        self.calls.append((sql, params))
        return LockCursor(next(self.values))


class InitializationCursor:
    def __init__(self, rows):
        self.rows = iter(rows)
        self.calls = []

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        return False

    def execute(self, sql, params=None):
        self.calls.append((sql, params))

    def fetchone(self):
        return next(self.rows)


class InitializationConnection:
    def __init__(self, rows, cursor_class=InitializationCursor):
        self.cursor_instance = cursor_class(rows)
        self.rollback_calls = 0

    def cursor(self):
        return self.cursor_instance

    def rollback(self):
        self.rollback_calls += 1


class UndefinedTableError(Exception):
    pgcode = "42P01"


class FeatureNotSupportedError(Exception):
    pgcode = "0A000"


class MissingPgxcNodeCursor(InitializationCursor):
    def execute(self, sql, params=None):
        super().execute(sql, params)
        if isinstance(sql, str) and "pg_catalog.pgxc_node" in sql:
            raise UndefinedTableError("relation pg_catalog.pgxc_node does not exist")


class DefinitionOnlyPgxcNodeCursor(InitializationCursor):
    def execute(self, sql, params=None):
        super().execute(sql, params)
        if isinstance(sql, str) and "pg_catalog.pgxc_node" in sql:
            raise FeatureNotSupportedError("pgxc_node data cannot be queried in centralized mode")


class UpdateByIdModel(Model):
    id = CharField(primary_key=True)
    name = CharField(null=True)

    class Meta:
        database = None


class RecordingCloseDatabase:
    def __init__(self, closed=False):
        self.closed = closed
        self.close_calls = 0
        self.close_stale_calls = []

    def __bool__(self):
        return True

    def is_closed(self):
        return self.closed

    def close(self):
        self.close_calls += 1
        self.closed = True

    def close_stale(self, age):
        self.close_stale_calls.append(age)


class TestGaussDBDatabase:
    """Test cases for GaussDB metadata database support."""

    def test_gaussdb_database_class_exists(self):
        assert RetryingPooledGaussDBDatabase is not None

    def test_gaussdb_in_pooled_database_enum(self):
        assert hasattr(PooledDatabase, "GAUSSDB")
        assert PooledDatabase.GAUSSDB.value == RetryingPooledGaussDBDatabase

    def test_gaussdb_in_database_migrator_enum(self):
        assert hasattr(DatabaseMigrator, "GAUSSDB")
        assert DatabaseMigrator.GAUSSDB.value == GaussDBMigrator

    def test_gaussdb_in_database_lock_enum(self):
        assert hasattr(DatabaseLock, "GAUSSDB")
        assert DatabaseLock.GAUSSDB.value == GaussDBDatabaseLock

    def test_gaussdb_in_text_field_type_enum(self):
        assert hasattr(TextFieldType, "GAUSSDB")
        assert TextFieldType.GAUSSDB.value == "TEXT"

    def test_gaussdb_database_uses_dedicated_adapter(self):
        assert not issubclass(RetryingPooledGaussDBDatabase, RetryingPooledPostgresqlDatabase)
        assert issubclass(RetryingPooledGaussDBDatabase, PooledPostgresqlDatabase)
        assert issubclass(GaussDBMigrator, DatabaseMigrator.POSTGRES.value)
        assert issubclass(GaussDBDatabaseLock, PostgresDatabaseLock)

    def test_gaussdb_database_init(self):
        db = RetryingPooledGaussDBDatabase(
            "test_db",
            host="localhost",
            port=8000,
            user="gaussdb",
            password="password",
        )
        assert db is not None
        assert db.max_retries == 5
        assert db.retry_delay == 1

    def test_gaussdb_database_custom_retries(self):
        db = RetryingPooledGaussDBDatabase(
            "test_db",
            host="localhost",
            max_retries=10,
            retry_delay=2,
        )
        assert db.max_retries == 10
        assert db.retry_delay == 2

    @pytest.mark.parametrize(("topology_row", "expected"), [((False,), False), ((True,), True)])
    def test_gaussdb_initializes_search_path_and_detects_topology(self, topology_row, expected):
        db = RetryingPooledGaussDBDatabase(None)
        connection = InitializationConnection([("ragflow_meta",), topology_row])

        db._initialize_connection(connection)

        calls = connection.cursor_instance.calls
        assert calls[0] == ("SELECT current_schema()", None)
        assert "SET search_path TO" in str(calls[1][0])
        assert "Identifier('ragflow_meta')" in str(calls[1][0])
        assert "pg_catalog.pgxc_node" in calls[2][0]
        assert db.is_distributed is expected

    @pytest.mark.parametrize("cursor_class", [MissingPgxcNodeCursor, DefinitionOnlyPgxcNodeCursor])
    def test_unavailable_pgxc_node_is_detected_as_centralized(self, cursor_class):
        db = RetryingPooledGaussDBDatabase(None)
        connection = InitializationConnection([("ragflow_meta",)], cursor_class=cursor_class)

        db._initialize_connection(connection)

        assert db.is_distributed is False
        assert connection.rollback_calls == 1
        assert "SET search_path TO" in str(connection.cursor_instance.calls[-1][0])

    def test_first_connection_failure_is_retried_before_ddl_preparation(self, monkeypatch):
        db = RetryingPooledGaussDBDatabase(None, max_retries=1, retry_delay=0)
        connection_calls = []
        executed = []
        sql = 'CREATE TABLE IF NOT EXISTS "user" ("id" VARCHAR(32))'

        def connection():
            connection_calls.append(None)
            if len(connection_calls) == 1:
                raise OperationalError("connection refused")
            db._is_distributed = True

        def base_execute(_self, prepared_sql, params=None, commit=True):
            executed.append((prepared_sql, params, commit))
            return "cursor"

        monkeypatch.setattr(db, "connection", connection)
        monkeypatch.setattr(db, "_handle_connection_loss", lambda: None)
        monkeypatch.setattr(PooledPostgresqlDatabase, "execute_sql", base_execute)

        assert db.execute_sql(sql) == "cursor"
        assert len(connection_calls) == 2
        assert executed == [(f"{sql} DISTRIBUTE BY REPLICATION", None, True)]

    def test_connection_failure_in_transaction_is_not_retried(self, monkeypatch):
        db = RetryingPooledGaussDBDatabase(None, max_retries=1, retry_delay=0)
        db._is_distributed = False
        connection_error = OperationalError("connection refused")
        executed = []
        reconnect_calls = []

        def base_execute(_self, sql, params=None, commit=True):
            executed.append((sql, params, commit))
            raise connection_error

        monkeypatch.setattr(db, "in_transaction", lambda: True)
        monkeypatch.setattr(db, "_handle_connection_loss", lambda: reconnect_calls.append(None))
        monkeypatch.setattr(PooledPostgresqlDatabase, "execute_sql", base_execute)

        with pytest.raises(OperationalError) as exc_info:
            db.execute_sql("SELECT 1")

        assert exc_info.value is connection_error
        assert executed == [("SELECT 1", None, True)]
        assert reconnect_calls == []

    def test_postgresql_execution_keeps_sql_unchanged(self, monkeypatch):
        db = RetryingPooledPostgresqlDatabase(None, max_retries=0)
        executed = []

        def base_execute(_self, sql, params=None, commit=True):
            executed.append((sql, params, commit))
            return "cursor"

        monkeypatch.setattr(PooledPostgresqlDatabase, "execute_sql", base_execute)

        assert db.execute_sql("SELECT 1") == "cursor"
        assert executed == [("SELECT 1", None, True)]

    def test_distributed_gaussdb_removes_only_redundant_primary_key_assignment(self):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = True
        source = {"id": "record-1", "name": "updated"}

        prepared = db.prepare_update_by_id(UpdateByIdModel, "record-1", source)

        assert prepared == {"name": "updated"}
        assert prepared is not source
        assert source == {"id": "record-1", "name": "updated"}

    def test_distributed_gaussdb_replaces_row_for_primary_key_change(self, monkeypatch):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = True
        sqlite_db = SqliteDatabase(":memory:")

        with sqlite_db.bind_ctx([UpdateByIdModel]):
            sqlite_db.create_tables([UpdateByIdModel])
            UpdateByIdModel.create(id="record-1", name="original")
            monkeypatch.setattr(db, "atomic", sqlite_db.atomic)

            assert db.replace_update_by_id(UpdateByIdModel, "record-1", {"id": "record-2", "name": "updated"}) == 1
            assert UpdateByIdModel.get_or_none(UpdateByIdModel.id == "record-1") is None
            assert UpdateByIdModel.get_by_id("record-2").name == "updated"
        sqlite_db.close()

    def test_distributed_gaussdb_replace_returns_zero_for_missing_row(self, monkeypatch):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = True
        sqlite_db = SqliteDatabase(":memory:")

        with sqlite_db.bind_ctx([UpdateByIdModel]):
            sqlite_db.create_tables([UpdateByIdModel])
            monkeypatch.setattr(db, "atomic", sqlite_db.atomic)

            assert db.replace_update_by_id(UpdateByIdModel, "missing", {"id": "record-2"}) == 0
        sqlite_db.close()

    def test_centralized_gaussdb_keeps_primary_key_assignment_unchanged(self):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = False
        source = {"id": "record-1", "name": "updated"}

        assert db.prepare_update_by_id(UpdateByIdModel, "record-1", source) is source
        assert db.replace_update_by_id(UpdateByIdModel, "record-1", {"id": "record-2"}) is None

    def test_postgresql_adapter_has_no_gaussdb_update_hook(self):
        assert not hasattr(RetryingPooledPostgresqlDatabase(None), "prepare_update_by_id")
        assert not hasattr(RetryingPooledPostgresqlDatabase(None), "replace_update_by_id")

    @pytest.mark.parametrize(
        "table_name",
        [
            "compilation_template",
            "compilation_template_group",
            "file_commit_item",
            "tenant_llm",
            "tenant_model_provider",
            "user",
        ],
    )
    def test_distributed_gaussdb_replicates_independent_unique_key_tables(self, table_name):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = True
        sql = f'CREATE TABLE IF NOT EXISTS "{table_name}" ("id" VARCHAR(32))'

        assert db._prepare_distributed_ddl(sql) == f"{sql} DISTRIBUTE BY REPLICATION"

    def test_centralized_gaussdb_keeps_candidate_ddl_unchanged(self):
        db = RetryingPooledGaussDBDatabase(None)
        db._is_distributed = False
        sql = 'CREATE TABLE IF NOT EXISTS "user" ("id" VARCHAR(32))'

        assert db._prepare_distributed_ddl(sql) == sql

    def test_gaussdb_get_tables_defaults_to_current_schema(self, monkeypatch):
        db = RetryingPooledGaussDBDatabase(None)
        monkeypatch.setattr(db, "execute_sql", lambda *_args, **_kwargs: RecordingCursor([("tenant",), ("user",)]))

        assert db.get_tables() == ["tenant", "user"]

    def test_gaussdb_request_cleanup_returns_current_connection(self, monkeypatch):
        db = RecordingCloseDatabase()
        monkeypatch.setattr(db_models, "DB", db)
        monkeypatch.setattr(db_models.settings, "DATABASE_TYPE", "gaussdb")

        db_models.close_connection()

        assert db.close_calls == 1
        assert db.close_stale_calls == []

    def test_non_gaussdb_cleanup_preserves_existing_behavior(self, monkeypatch):
        db = RecordingCloseDatabase()
        monkeypatch.setattr(db_models, "DB", db)
        monkeypatch.setattr(db_models.settings, "DATABASE_TYPE", "mysql")

        db_models.close_connection()

        assert db.close_calls == 0
        assert db.close_stale_calls == [30]

    def test_pooled_database_enum_values(self):
        expected = {"MYSQL", "OCEANBASE", "POSTGRES", "GAUSSDB"}
        actual = {e.name for e in PooledDatabase}
        assert expected.issubset(actual), f"Missing: {expected - actual}"

    def test_database_lock_enum_values(self):
        expected = {"MYSQL", "OCEANBASE", "POSTGRES", "GAUSSDB"}
        actual = set(DatabaseLock.__members__.keys())
        assert expected.issubset(actual), f"Missing: {expected - actual}"

    def test_gaussdb_lock_waits_until_holder_releases(self, monkeypatch):
        now = [10.0]
        sleeps = []
        monkeypatch.setattr(db_models.time, "monotonic", lambda: now[0])

        def sleep(seconds):
            sleeps.append(seconds)
            now[0] += seconds

        monkeypatch.setattr(db_models.time, "sleep", sleep)
        db = SequenceLockDB([False, False, True])

        assert GaussDBDatabaseLock("init_database_tables", timeout=1.0, db=db).lock()
        assert sleeps == [0.1, 0.1]

    def test_gaussdb_lock_waits_indefinitely_with_blocking_lock(self):
        db = SequenceLockDB([None])
        lock = GaussDBDatabaseLock("update_progress", timeout=-1, db=db)

        assert lock.lock()
        assert lock.timeout == -1
        assert db.calls == [("SELECT pg_advisory_lock(%s)", (lock.lock_id,))]

    def test_gaussdb_lock_timeout_zero_only_tries_once(self, monkeypatch):
        monkeypatch.setattr(db_models.time, "monotonic", lambda: 30.0)
        db = SequenceLockDB([False])
        lock = GaussDBDatabaseLock("update_progress", timeout=0, db=db)

        with pytest.raises(Exception, match="acquire gaussdb lock update_progress timeout"):
            lock.lock()

        assert db.calls == [("SELECT pg_try_advisory_lock(%s)", (lock.lock_id,))]

    def test_gaussdb_lock_stops_at_timeout(self, monkeypatch):
        now = [20.0]
        monkeypatch.setattr(db_models.time, "monotonic", lambda: now[0])
        monkeypatch.setattr(db_models.time, "sleep", lambda seconds: now.__setitem__(0, now[0] + seconds))
        db = SequenceLockDB([False, False, False, False])

        with pytest.raises(Exception, match="acquire gaussdb lock init_database_tables timeout"):
            GaussDBDatabaseLock("init_database_tables", timeout=0.25, db=db).lock()

        assert now[0] == pytest.approx(20.25)


class TestGaussDBConfiguration:
    """Test cases for DB_TYPE normalization."""

    def test_settings_can_use_gaussdb_case_insensitive(self):
        assert normalize_database_type("GaussDB") == "gaussdb"
        assert normalize_database_type("gaussdb") == "gaussdb"

    def test_settings_accepts_gauss_alias(self):
        assert normalize_database_type("gauss") == "gaussdb"

    def test_load_database_config_uses_gaussdb_env(self, monkeypatch):
        monkeypatch.delenv("GAUSSDB_METADATA_OPTIONS", raising=False)
        monkeypatch.setenv("GAUSSDB_METADATA_HOST", "127.0.0.1")
        monkeypatch.setenv("GAUSSDB_METADATA_PORT", "19995")
        monkeypatch.setenv("GAUSSDB_METADATA_USER", "zws")
        monkeypatch.setenv("GAUSSDB_METADATA_PASSWORD", "secret")
        monkeypatch.setenv("GAUSSDB_METADATA_DBNAME", "zws_test")
        monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "ragflow_meta")

        config = load_database_config("GaussDB")

        assert config["host"] == "127.0.0.1"
        assert config["port"] == 19995
        assert config["user"] == "zws"
        assert config["password"] == "secret"
        assert config["name"] == "zws_test"
        assert config["options"] == "-c search_path=ragflow_meta -c client_encoding=UTF8 -c default_transaction_read_only=off"

    def test_load_database_config_defaults_schema_to_public(self, monkeypatch):
        monkeypatch.delenv("GAUSSDB_METADATA_SCHEMA", raising=False)
        monkeypatch.delenv("GAUSSDB_METADATA_OPTIONS", raising=False)

        assert load_database_config("gaussdb")["options"].startswith("-c search_path=public ")

    def test_load_database_config_rejects_unsafe_schema(self, monkeypatch):
        monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "public;drop schema public")

        with pytest.raises(ValueError, match="GAUSSDB_METADATA_SCHEMA"):
            load_database_config("gaussdb")

    def test_explicit_options_override_generated_schema_option(self, monkeypatch):
        monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "ragflow_meta")
        monkeypatch.setenv("GAUSSDB_METADATA_OPTIONS", "-c search_path=custom_meta -c statement_timeout=5000")

        assert load_database_config("gaussdb")["options"] == "-c search_path=custom_meta -c statement_timeout=5000"
