#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock, patch

import pytest

import common


class ImportFakePool:
    def getconn(self):
        raise RuntimeError("import fake pool should not be used")

    def putconn(self, conn, close=False):
        pass

    def closeall(self):
        pass


def _load_pool_module_without_import_state_leaks():
    module_name = "_test_gaussdb_conn_pool_module"
    module_path = Path(__file__).resolve().parents[4] / "common" / "doc_store" / "gaussdb_conn_pool.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    fake_settings = ModuleType("common.settings")
    fake_settings.GAUSSDB = {
        "host": "gaussdb.local",
        "port": "19995",
        "database": "postgres",
        "user": "sqlbuilder",
        "password": "fake-unit-password",
    }
    fake_settings.get_base_config = lambda *_args, **_kwargs: {}

    missing = object()
    previous_settings_module = sys.modules.get("common.settings", missing)
    previous_test_module = sys.modules.get(module_name, missing)
    previous_common_settings = getattr(common, "settings", missing)
    sys.modules["common.settings"] = fake_settings
    sys.modules[module_name] = module
    common.settings = fake_settings
    try:
        with patch("psycopg2.pool.ThreadedConnectionPool", lambda *_args, **_kwargs: ImportFakePool()):
            spec.loader.exec_module(module)
    finally:
        for name, previous in (
            ("common.settings", previous_settings_module),
            (module_name, previous_test_module),
        ):
            if previous is missing:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = previous
        if previous_common_settings is missing:
            delattr(common, "settings")
        else:
            common.settings = previous_common_settings
    return module


pool_mod = _load_pool_module_without_import_state_leaks()
GaussDBAuthenticationError = pool_mod.GaussDBAuthenticationError
GaussDBConfig = pool_mod.GaussDBConfig
GaussDBConnectionError = pool_mod.GaussDBConnectionError
GaussDBConnectionPool = pool_mod.GaussDBConnectionPool
GaussDBPermissionError = pool_mod.GaussDBPermissionError
InvalidGaussDBConfig = pool_mod.InvalidGaussDBConfig
LazyGaussDBConnectionPool = pool_mod._LazyGaussDBConnectionPool
_normalize_schema = pool_mod._normalize_schema
classify_gaussdb_exception = pool_mod.classify_gaussdb_exception
load_gaussdb_config = pool_mod.load_gaussdb_config
mask_gaussdb_uri = pool_mod.mask_gaussdb_uri


def _raw_config(**overrides):
    config = {
        "host": "127.0.0.1",
        "port": "19995",
        "database": "postgres",
        "user": "sqlbuilder",
        "password": "fake-unit-password",
    }
    config.update(overrides)
    return config


def _install_runtime_settings(monkeypatch, gaussdb, get_base_config):
    runtime_settings = ModuleType("common.settings")
    runtime_settings.GAUSSDB = gaussdb
    runtime_settings.get_base_config = get_base_config
    monkeypatch.setitem(sys.modules, "common.settings", runtime_settings)
    monkeypatch.setattr(common, "settings", runtime_settings, raising=False)
    return runtime_settings


def test_tc_cfg_001_load_gaussdb_config_loads_complete_config():
    cfg = load_gaussdb_config(_raw_config())

    assert cfg == GaussDBConfig(
        host="127.0.0.1",
        port=19995,
        database="postgres",
        user="sqlbuilder",
        password="fake-unit-password",
        schema="public",
    )
    assert type(cfg.port) is int
    for database_key in ("database", "db_name", "name"):
        raw = _raw_config()
        raw.pop("database")
        raw[database_key] = f"{database_key}-literal"
        assert load_gaussdb_config(raw).database == f"{database_key}-literal"
    with pytest.raises(InvalidGaussDBConfig, match=r"^missing gaussdb config field\(s\): host, port, database, user, password$"):
        load_gaussdb_config({"config": _raw_config()})


def test_tc_cfg_002_load_gaussdb_config_preserves_explicit_schema():
    cfg = load_gaussdb_config(_raw_config(schema="tenant_schema"))

    assert cfg.schema == "tenant_schema"


@pytest.mark.parametrize("port", [1, 65535])
def test_tc_cfg_003_load_gaussdb_config_accepts_port_boundaries(port):
    cfg = load_gaussdb_config(_raw_config(port=port))

    assert cfg.port == port
    assert cfg.schema == "public"


def test_tc_cfg_004_mask_gaussdb_uri_omits_password_from_masked_uri():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "secret-password", "public")

    masked = mask_gaussdb_uri(cfg)

    assert masked == "sqlbuilder@db.example:19995/postgres?schema=public"
    assert "secret-password" not in masked


@pytest.mark.parametrize("schema", ["", "   "])
def test_tc_cfg_102_normalize_empty_schema_to_public(schema):
    assert _normalize_schema(schema) == "public"


def test_tc_cfg_105_load_gaussdb_config_accepts_hash_and_dollar_in_schema_name():
    cfg = load_gaussdb_config(
        {
            "host": "127.0.0.1",
            "port": "19995",
            "database": "postgres",
            "user": "sqlbuilder",
            "password": "fake-unit-password",
            "schema": "ragflow#tenant$1",
        }
    )

    assert cfg.schema == "ragflow#tenant$1"


def test_tc_cfg_106_load_gaussdb_config_accepts_high_bit_schema_name():
    cfg = load_gaussdb_config(
        {
            "host": "127.0.0.1",
            "port": "19995",
            "database": "postgres",
            "user": "sqlbuilder",
            "password": "fake-unit-password",
            "schema": "租户_schema1",
        }
    )

    assert cfg.schema == "租户_schema1"


def test_tc_cfg_201_load_gaussdb_config_rejects_missing_host_without_leaking_values():
    raw = _raw_config(database="private-db", user="private-user", password="private-password")
    del raw["host"]

    with pytest.raises(InvalidGaussDBConfig) as exc_info:
        load_gaussdb_config(raw)

    assert str(exc_info.value) == "missing gaussdb config field(s): host"
    assert all(secret not in str(exc_info.value) for secret in ("private-db", "private-user", "private-password"))


@pytest.mark.parametrize("field", ["port", "database", "user", "password"])
def test_tc_cfg_202_load_gaussdb_config_rejects_each_missing_required_field(field):
    raw = _raw_config(host="private-host", database="private-db", user="private-user", password="private-password")
    del raw[field]

    with pytest.raises(InvalidGaussDBConfig) as exc_info:
        load_gaussdb_config(raw)

    assert str(exc_info.value) == f"missing gaussdb config field(s): {field}"
    assert all(secret not in str(exc_info.value) for secret in ("private-host", "private-db", "private-user", "private-password"))


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("host", 127001),
        ("database", 1234),
        ("user", ""),
        ("password", "   "),
    ],
)
def test_tc_cfg_202_load_gaussdb_config_rejects_invalid_required_string_fields(field, value):
    with pytest.raises(InvalidGaussDBConfig, match=rf"^invalid gaussdb config field: {field}$"):
        load_gaussdb_config(_raw_config(**{field: value}))


@pytest.mark.parametrize("schema", ["1abc", "sch ema", "public;drop table x", "s" * 64])
def test_tc_cfg_104_normalize_schema_rejects_unsafe_or_overlong_names(schema):
    with pytest.raises(InvalidGaussDBConfig) as exc_info:
        _normalize_schema(schema)

    assert str(exc_info.value) == f"invalid gaussdb schema: {schema}"


@pytest.mark.parametrize(
    ("port", "expected_message"),
    [
        (-1, "invalid gaussdb config field: port"),
        (0, "invalid gaussdb config field: port"),
        (65536, "invalid gaussdb config field: port"),
    ],
)
def test_tc_cfg_203_load_gaussdb_config_rejects_invalid_port_values(port, expected_message):
    with pytest.raises(InvalidGaussDBConfig) as exc_info:
        load_gaussdb_config(_raw_config(port=port))

    assert str(exc_info.value) == expected_message


@pytest.mark.parametrize("port", ["not-a-port", "1.5", 1.5, 5432.0, True])
def test_tc_cfg_204_load_gaussdb_config_rejects_non_integer_port(port):
    with pytest.raises(InvalidGaussDBConfig) as exc_info:
        load_gaussdb_config(_raw_config(port=port))

    assert str(exc_info.value) == "invalid gaussdb config field: port"


def test_tc_cfg_205_load_gaussdb_config_rejects_explicit_and_default_empty_config(monkeypatch):
    expected = "missing gaussdb config field(s): host, port, database, user, password"

    with pytest.raises(InvalidGaussDBConfig) as raw_empty_exc:
        load_gaussdb_config({})

    get_base_config = Mock(return_value={})
    _install_runtime_settings(monkeypatch, {}, get_base_config)
    with pytest.raises(InvalidGaussDBConfig) as default_exc:
        load_gaussdb_config(None)

    assert str(raw_empty_exc.value) == expected
    assert str(default_exc.value) == expected
    get_base_config.assert_called_once_with("gaussdb", {})


def test_tc_cfg_005_load_gaussdb_config_without_raw_prefers_gaussdb_block(monkeypatch):
    get_base_config = Mock(side_effect=AssertionError("fallback used"))
    _install_runtime_settings(
        monkeypatch,
        {
            "host": "gaussdb.local",
            "port": "19995",
            "database": "postgres",
            "user": "sqlbuilder",
            "password": "fake-unit-password",
        },
        get_base_config,
    )

    cfg = load_gaussdb_config()

    assert cfg == GaussDBConfig(
        host="gaussdb.local",
        port=19995,
        database="postgres",
        user="sqlbuilder",
        password="fake-unit-password",
        schema="public",
    )
    get_base_config.assert_not_called()


def test_tc_cfg_006_load_gaussdb_config_without_raw_falls_back_to_base_config(monkeypatch):
    get_base_config = Mock(
        return_value={
            "host": "gaussdb.local",
            "port": "19995",
            "database": "postgres",
            "user": "sqlbuilder",
            "password": "fake-unit-password",
            "schema": "ragflow_gaussdb_docengine_it",
        }
    )
    _install_runtime_settings(monkeypatch, None, get_base_config)

    cfg = load_gaussdb_config()

    assert cfg == GaussDBConfig(
        host="gaussdb.local",
        port=19995,
        database="postgres",
        user="sqlbuilder",
        password="fake-unit-password",
        schema="ragflow_gaussdb_docengine_it",
    )
    get_base_config.assert_called_once_with("gaussdb", {})


def test_tc_cfg_301_pool_initializes_driver_with_complete_connection_options(monkeypatch):
    created = {}
    sentinel_pool = object()

    def fake_threaded_pool(*args, **kwargs):
        created["args"] = args
        created["kwargs"] = kwargs
        return sentinel_pool

    monkeypatch.setattr(pool_mod.psycopg2_pool, "ThreadedConnectionPool", fake_threaded_pool)
    cfg = GaussDBConfig("h", 5432, "d", "u", "p", "zlw")

    pool = GaussDBConnectionPool(cfg, minconn=2, maxconn=4)

    assert pool._pool is sentinel_pool
    assert pool.resolved_schema == "zlw"
    assert created["args"] == (2, 4)
    assert created["kwargs"] == {
        "host": "h",
        "port": 5432,
        "dbname": "d",
        "user": "u",
        "password": "p",
        "options": ("-c search_path=zlw -c client_encoding=UTF8 -c default_transaction_read_only=off"),
    }


def test_tc_cfg_310_gaussdb_config_and_pool_do_not_read_ssl_timeout_or_pool_tuning(monkeypatch):
    created = {}
    sentinel_pool = object()

    def fake_threaded_pool(*args, **kwargs):
        created["args"] = args
        created["kwargs"] = kwargs
        return sentinel_pool

    raw = _raw_config(
        ssl=True,
        sslmode="require",
        timeout=30,
        connect_timeout=30,
        minconn=4,
        maxconn=32,
    )
    cfg = load_gaussdb_config(raw)

    assert list(GaussDBConfig.__dataclass_fields__) == ["host", "port", "database", "user", "password", "schema"]
    for attr in ("ssl", "sslmode", "timeout", "connect_timeout", "minconn", "maxconn"):
        with pytest.raises(AttributeError, match=attr):
            getattr(cfg, attr)

    monkeypatch.setattr(pool_mod.psycopg2_pool, "ThreadedConnectionPool", fake_threaded_pool)
    pool = GaussDBConnectionPool(cfg, minconn=1, maxconn=8)

    assert pool._pool is sentinel_pool
    assert created["args"] == (1, 8)
    assert set(created["kwargs"]) == {"host", "port", "dbname", "user", "password", "options"}
    assert created["kwargs"]["options"] == ("-c search_path=public -c client_encoding=UTF8 -c default_transaction_read_only=off")


def test_tc_cfg_320_lazy_gaussdb_pool_reuses_one_pool_and_resets_after_close(monkeypatch):
    created = []

    class SharedPool:
        masked_uri = "u@h:5432/d?schema=public"

        def __init__(self):
            self.closed = 0

        def close_all(self):
            self.closed += 1

    def create_pool():
        pool = SharedPool()
        created.append(pool)
        return pool

    monkeypatch.setattr(pool_mod, "GaussDBConnectionPool", create_pool)
    lazy_pool = LazyGaussDBConnectionPool()

    first = lazy_pool.get_pool()
    assert lazy_pool.get_pool() is first
    assert lazy_pool.masked_uri == first.masked_uri
    assert created == [first]

    lazy_pool.close_all()
    assert first.closed == 1
    second = lazy_pool.get_pool()
    assert second is not first
    assert created == [first, second]


def test_tc_cfg_404_pool_auth_failure_prevents_adapter_write(monkeypatch):
    def fake_threaded_pool(*_args, **_kwargs):
        raise RuntimeError("password authentication failed")

    monkeypatch.setattr(pool_mod.psycopg2_pool, "ThreadedConnectionPool", fake_threaded_pool)
    cfg = GaussDBConfig("h", 5432, "d", "u", "p", "public")

    with pytest.raises(GaussDBAuthenticationError) as exc_info:
        GaussDBConnectionPool(cfg)

    assert str(exc_info.value) == "password authentication failed"

    from common.doc_store.gaussdb_conn_base import GaussDBDDLBuilder
    from rag.utils.gaussdb_conn import GaussDBConnection

    connection = GaussDBConnection.__new__(GaussDBConnection)
    connection.ddl = GaussDBDDLBuilder(schema="public")
    connection.schema = "public"
    connection.pool = Mock()
    connection.pool.get_conn.side_effect = GaussDBAuthenticationError("auth failed")

    assert connection.update({"id": "c1"}, {"pagerank_fea": 1}, "ragflow_tenant", "kb1") is False
    connection.pool.get_conn.assert_called_once_with()
    connection.pool.put_conn.assert_not_called()
    connection.pool.execute.assert_not_called()
    connection.pool.commit.assert_not_called()
    connection.pool.rollback.assert_not_called()


@pytest.mark.parametrize(
    "message",
    [
        'FATAL: password authentication failed for user "test"',
        "ERROR: invalid authentication method",
        "invalid username or password",
    ],
)
def test_tc_cfg_401_classify_gaussdb_exception_maps_authentication_errors(message):
    classified = classify_gaussdb_exception(RuntimeError(message))

    assert type(classified) is GaussDBAuthenticationError
    assert str(classified) == message


@pytest.mark.parametrize("message", ["ERROR: permission denied for schema public", "ERROR: privilege not granted"])
def test_tc_cfg_402_classify_gaussdb_exception_maps_permission_errors(message):
    classified = classify_gaussdb_exception(RuntimeError(message))

    assert type(classified) is GaussDBPermissionError
    assert str(classified) == message


@pytest.mark.parametrize("message", ["connection timeout", "timeout expired", "network error", "generic connection error"])
def test_tc_cfg_403_classify_gaussdb_exception_maps_timeout_and_generic_errors(message):
    classified = classify_gaussdb_exception(RuntimeError(message))

    assert type(classified) is GaussDBConnectionError
    assert str(classified) == message


def test_tc_cfg_405_classify_gaussdb_exception_preserves_existing_classification():
    existing = GaussDBPermissionError("already classified")

    assert classify_gaussdb_exception(existing) is existing


class FakeCursor:
    def __init__(self, row):
        self.row = row
        self.executed = []
        self.closed = False

    def execute(self, sql, params=None):
        self.executed.append((sql, params))

    def fetchone(self):
        return self.row

    def close(self):
        self.closed = True


class FakeConnection:
    def __init__(self, row):
        self.cursor_obj = FakeCursor(row)
        self.rollbacks = 0
        self.closed = False

    def cursor(self):
        return self.cursor_obj

    def rollback(self):
        self.rollbacks += 1


class FakePool:
    def __init__(self, row=(True, True)):
        self.conn = FakeConnection(row)
        self.returned = []
        self.closed = False

    def getconn(self):
        return self.conn

    def putconn(self, conn, close=False):
        if close:
            conn.closed = True
        self.returned.append(conn)

    def closeall(self):
        self.closed = True


def _assert_schema_privilege_query(sql, params, user, schema):
    assert " ".join(sql.split()) == ("SELECT has_schema_privilege(%s, %s, %s) AS has_usage, has_schema_privilege(%s, %s, %s) AS has_create")
    assert params == (user, schema, "USAGE", user, schema, "CREATE")


def test_tc_cfg_318_pool_check_schema_access_verifies_usage_and_create_privileges():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    pool.check_schema_access()

    sql, params = fake_pool.conn.cursor_obj.executed[1]
    _assert_schema_privilege_query(sql, params, "sqlbuilder", "ragflow_gaussdb_docengine_it")
    assert fake_pool.returned == [fake_pool.conn]
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.conn.rollbacks == 2


def test_tc_cfg_308_pool_check_schema_access_rejects_missing_create_privilege():
    cfg = GaussDBConfig("h", 5432, "d", "u", "p", "public")
    fake_pool = FakePool(row=(True, False))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBPermissionError) as exc_info:
        pool.check_schema_access()

    assert str(exc_info.value) == "GaussDB user u lacks CREATE on schema public"
    sql, params = fake_pool.conn.cursor_obj.executed[1]
    _assert_schema_privilege_query(sql, params, "u", "public")
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]
    assert fake_pool.conn.rollbacks == 2


def test_tc_cfg_307_pool_check_schema_access_rejects_missing_usage_privilege():
    cfg = GaussDBConfig("h", 5432, "d", "u", "p", "public")
    fake_pool = FakePool(row=(False, True))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBPermissionError) as exc_info:
        pool.check_schema_access()

    assert str(exc_info.value) == "GaussDB user u lacks USAGE on schema public"
    sql, params = fake_pool.conn.cursor_obj.executed[1]
    _assert_schema_privilege_query(sql, params, "u", "public")
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]
    assert fake_pool.conn.rollbacks == 2


def test_tc_cfg_314_pool_check_schema_access_classifies_query_failure():
    class FailingSchemaCursor(FakeCursor):
        def execute(self, sql, params=None):
            super().execute(sql, params)
            if "has_schema_privilege" in sql:
                raise RuntimeError("permission denied while checking schema")

    class FailingSchemaConnection(FakeConnection):
        def __init__(self):
            super().__init__(row=None)
            self.cursor_obj = FailingSchemaCursor(row=None)

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    fake_pool.conn = FailingSchemaConnection()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBPermissionError) as exc_info:
        pool.check_schema_access()

    assert str(exc_info.value) == "permission denied while checking schema"
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]


def test_tc_cfg_314_pool_check_schema_access_classifies_cursor_creation_failure():
    class CursorFailsAfterValidationConnection(FakeConnection):
        def __init__(self):
            super().__init__(row=(True, True))
            self.cursor_calls = 0

        def cursor(self):
            self.cursor_calls += 1
            if self.cursor_calls == 1:
                return self.cursor_obj
            raise RuntimeError("network timeout before privilege check")

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    fake_pool.conn = CursorFailsAfterValidationConnection()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBConnectionError) as exc_info:
        pool.check_schema_access()

    assert str(exc_info.value) == "network timeout before privilege check"
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]


class FailingPingCursor(FakeCursor):
    def execute(self, sql, params=None):
        self.executed.append((sql, params))
        if sql == "SELECT 1":
            raise RuntimeError("SSL SYSCALL error: EOF detected")


class SequencedPool:
    def __init__(self):
        self.dead_conn = FakeConnection(row=None)
        self.dead_conn.cursor_obj = FailingPingCursor(row=None)
        self.live_conn = FakeConnection(row=None)
        self.returned = []
        self.closed = []

    def getconn(self):
        if not self.returned:
            return self.dead_conn
        return self.live_conn

    def putconn(self, conn, close=False):
        self.returned.append(conn)
        if close:
            self.closed.append(conn)


def test_tc_cfg_315_pool_get_conn_discards_stale_connection_and_retries_once():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = SequencedPool()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    conn = pool.get_conn()

    assert conn is fake_pool.live_conn
    assert fake_pool.closed == [fake_pool.dead_conn]
    assert fake_pool.dead_conn.cursor_obj.closed is True
    assert fake_pool.live_conn.cursor_obj.closed is True
    assert fake_pool.live_conn.rollbacks == 1


def test_tc_cfg_315_pool_get_conn_retries_checkout_failure_without_discarding_none():
    class CheckoutFailureThenSuccessPool:
        def __init__(self):
            self.calls = 0
            self.live_conn = FakeConnection(row=(1,))
            self.returned = []

        def getconn(self):
            self.calls += 1
            if self.calls == 1:
                raise RuntimeError("pool temporarily exhausted")
            return self.live_conn

        def putconn(self, conn, close=False):
            self.returned.append((conn, close))

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = CheckoutFailureThenSuccessPool()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    conn = pool.get_conn()

    assert conn is fake_pool.live_conn
    assert fake_pool.calls == 2
    assert fake_pool.returned == []


def test_tc_cfg_304_pool_put_conn_rolls_back_before_returning_connection():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)
    conn = FakeConnection(row=None)

    pool.put_conn(conn)

    assert fake_pool.returned == [conn]
    assert conn.rollbacks == 1
    assert conn.closed is False


class RollbackFailingConnection(FakeConnection):
    def rollback(self):
        self.rollbacks += 1
        raise RuntimeError("connection lost")


def test_tc_cfg_305_pool_put_conn_discards_connection_when_rollback_fails():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)
    conn = RollbackFailingConnection(row=None)

    pool.put_conn(conn)

    assert fake_pool.returned == [conn]
    assert conn.closed is True
    assert conn.rollbacks == 1


def test_tc_cfg_316_pool_put_conn_ignores_discard_failure_after_rollback_failure():
    class DiscardFailingPool(FakePool):
        def putconn(self, conn, close=False):
            raise RuntimeError("discard failed")

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    pool = GaussDBConnectionPool(cfg, pool=DiscardFailingPool(row=(True, True)))
    conn = RollbackFailingConnection(row=None)
    discard = Mock(wraps=pool._discard_conn)
    pool._discard_conn = discard

    pool.put_conn(conn)

    assert conn.rollbacks == 1
    discard.assert_called_once_with(conn)
    assert pool._pool.returned == []


def test_tc_cfg_303_pool_get_conn_tries_twice_then_raises_closed_error():
    class AlwaysClosedPool:
        def __init__(self):
            self.connections = []
            self.discarded = []

        def getconn(self):
            conn = FakeConnection(row=None)
            conn.closed = True
            self.connections.append(conn)
            return conn

        def putconn(self, conn, close=False):
            if close:
                self.discarded.append(conn)

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = AlwaysClosedPool()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBConnectionError) as exc_info:
        pool.get_conn()

    assert str(exc_info.value) == "GaussDB connection is closed"
    assert len(fake_pool.connections) == 2
    assert fake_pool.discarded == fake_pool.connections


def test_tc_cfg_314_pool_get_conn_classifies_cursor_creation_failure():
    class CursorCreationFailureConnection(FakeConnection):
        def cursor(self):
            raise RuntimeError("network timeout before cursor")

    class CursorCreationFailurePool:
        def __init__(self):
            self.conn = CursorCreationFailureConnection(row=None)
            self.closed = []

        def getconn(self):
            return self.conn

        def putconn(self, conn, close=False):
            if close:
                self.closed.append(conn)

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = CursorCreationFailurePool()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBConnectionError) as exc_info:
        pool.get_conn()

    assert str(exc_info.value) == "network timeout before cursor"
    assert fake_pool.closed == [fake_pool.conn, fake_pool.conn]


def test_tc_cfg_316_pool_put_conn_ignores_none_and_close_all_delegates():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    pool.put_conn(None)
    pool.close_all()

    assert fake_pool.returned == []
    assert fake_pool.closed is True


def test_tc_cfg_317_pool_fetch_one_executes_query_and_returns_connection():
    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=("GaussDB",))
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    row = pool.fetch_one("SELECT version()", ("arg",))

    assert row == ("GaussDB",)
    assert fake_pool.conn.cursor_obj.executed[-1] == ("SELECT version()", ("arg",))
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]


def test_tc_cfg_317_pool_fetch_one_classifies_cursor_creation_failure_and_returns_connection():
    class CursorFailsAfterValidationConnection(FakeConnection):
        def __init__(self):
            super().__init__(row=(True,))
            self.cursor_calls = 0

        def cursor(self):
            self.cursor_calls += 1
            if self.cursor_calls == 1:
                return self.cursor_obj
            raise RuntimeError("network timeout before query")

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    fake_pool.conn = CursorFailsAfterValidationConnection()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBConnectionError) as exc_info:
        pool.fetch_one("SELECT 1")

    assert str(exc_info.value) == "network timeout before query"
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]


def test_tc_cfg_317_pool_fetch_one_classifies_query_failure_and_cleans_up():
    class FailingQueryCursor(FakeCursor):
        def execute(self, sql, params=None):
            super().execute(sql, params)
            if sql != "SELECT 1":
                raise RuntimeError("authentication failed while querying")

    class FailingQueryConnection(FakeConnection):
        def __init__(self):
            super().__init__(row=None)
            self.cursor_obj = FailingQueryCursor(row=None)

    cfg = GaussDBConfig("db.example", 19995, "postgres", "sqlbuilder", "fake-unit-password", "ragflow_gaussdb_docengine_it")
    fake_pool = FakePool(row=(True, True))
    fake_pool.conn = FailingQueryConnection()
    pool = GaussDBConnectionPool(cfg, pool=fake_pool)

    with pytest.raises(GaussDBAuthenticationError) as exc_info:
        pool.fetch_one("SELECT version()")

    assert str(exc_info.value) == "authentication failed while querying"
    assert fake_pool.conn.cursor_obj.closed is True
    assert fake_pool.returned == [fake_pool.conn]
