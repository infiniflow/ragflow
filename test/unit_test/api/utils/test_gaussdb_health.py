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
import json
from pathlib import Path
import sys
import types
from unittest.mock import Mock

import pytest

CFG614_PASSWORD_SENTINEL = "secret123"
PASSWORD_SENTINEL = "cfg711-'quoted;password'"
FULL_DSN_SENTINEL = "postgresql://sqlbuilder:cfg711-dsn%27%22%3Bsecret@db.example:19995/postgres?schema=private"
ACCESS_TOKEN_SENTINEL = "cfg711-token-'quoted![]{}'"


class FakeGaussDBConnection:
    def db_type(self):
        return "gaussdb"

    def health(self):
        return {
            "status": "healthy",
            "uri": "sqlbuilder@db.example:19995/postgres?schema=public",
            "version_comment": "GaussDB",
            "sql_compatibility": "A",
        }

    def get_performance_metrics(self):
        return {"connection": "connected", "latency_ms": 3.2}


class FailingGaussDBConnection:
    def health(self):
        raise RuntimeError(f"FATAL: password authentication failed password={CFG614_PASSWORD_SENTINEL} dsn={FULL_DSN_SENTINEL} access_token={ACCESS_TOKEN_SENTINEL}")


class UnhealthyGaussDBConnection(FakeGaussDBConnection):
    def health(self):
        return {
            "status": "unhealthy",
            "error": "compatibility unsupported password=fake-unit-password",
        }

    def get_performance_metrics(self):
        return {"connection": "connected", "latency_ms": 1.2, "error": "dsn password:fake-unit-password"}


class IncompatibleGaussDBConnection(FakeGaussDBConnection):
    def health(self):
        return {
            "status": "unhealthy",
            "uri": "sqlbuilder@db.example:19995/postgres?schema=public",
            "version_comment": "GaussDB",
            "sql_compatibility": "PG",
            "error": "unsupported GaussDB compatibility, expected A/ORA: sql_compatibility=PG",
        }


class SecretBearingGaussDBConnection(FakeGaussDBConnection):
    def health(self):
        return {
            "status": "unhealthy",
            "schema": "public",
            "credentials": {
                "password": PASSWORD_SENTINEL,
                "connections": [{"dsn": FULL_DSN_SENTINEL}, ({"ACCESS_TOKEN": ACCESS_TOKEN_SENTINEL},)],
            },
            "error": f'authentication failed password="{PASSWORD_SENTINEL}"',
        }

    def get_performance_metrics(self):
        return {
            "connection": "disconnected",
            "schema": "public",
            "details": [
                {"dsn": FULL_DSN_SENTINEL},
                ({"access-token": ACCESS_TOKEN_SENTINEL},),
            ],
        }


def _install_module(monkeypatch, name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _import_health_utils(monkeypatch):
    import common

    settings_stub = types.SimpleNamespace(
        docStoreConn=types.SimpleNamespace(health=lambda: {"status": "healthy"}),
        STORAGE_IMPL=types.SimpleNamespace(health=lambda: True),
        STORAGE_IMPL_TYPE="MINIO",
        MINIO={"host": "minio:9000"},
    )
    monkeypatch.setitem(sys.modules, "common.settings", settings_stub)
    monkeypatch.setattr(common, "settings", settings_stub, raising=False)

    _install_module(monkeypatch, "api.db.db_models", DB=types.SimpleNamespace(execute_sql=lambda *_args, **_kwargs: True))
    _install_module(monkeypatch, "rag.utils.redis_conn", REDIS_CONN=types.SimpleNamespace(health=lambda: True))
    _install_module(monkeypatch, "rag.utils.es_conn", ESConnection=lambda: types.SimpleNamespace(get_cluster_stats=lambda: {}))
    _install_module(monkeypatch, "rag.utils.infinity_conn", InfinityConnection=lambda: types.SimpleNamespace(health=lambda: {}))
    _install_module(monkeypatch, "rag.utils.ob_conn", OBConnection=lambda: types.SimpleNamespace(health=lambda: {}, get_performance_metrics=lambda: {}))
    _install_module(monkeypatch, "rag.utils.gaussdb_conn", GaussDBConnection=FakeGaussDBConnection)

    module_path = Path(__file__).resolve().parents[4] / "api" / "utils" / "health_utils.py"
    spec = importlib.util.spec_from_file_location("_gaussdb_health_utils_under_test", module_path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_tc_cfg_611_get_gaussdb_status_returns_complete_not_configured_response(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "elasticsearch")

    result = health_utils.get_gaussdb_status()

    assert result == {
        "status": "not_configured",
        "message": "GaussDB is not configured as the document engine",
    }


def test_tc_cfg_616_get_gaussdb_status_composes_healthy_probe_results(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    connection = FakeGaussDBConnection()
    get_connection = Mock(return_value=connection)
    monkeypatch.setattr(health_utils, "_get_gaussdb_connection", get_connection)

    result = health_utils.get_gaussdb_status()

    assert result == {
        "status": "alive",
        "message": {
            "health": {
                "status": "healthy",
                "uri": "sqlbuilder@db.example:19995/postgres?schema=public",
                "version_comment": "GaussDB",
                "sql_compatibility": "A",
            },
            "performance": {"connection": "connected", "latency_ms": 3.2},
        },
    }
    get_connection.assert_called_once_with()


def test_tc_cfg_617_get_gaussdb_status_reuses_settings_doc_store_connection(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    doc_store_conn = FakeGaussDBConnection()
    monkeypatch.setattr(health_utils.settings, "docStoreConn", doc_store_conn, raising=False)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    constructor = Mock(side_effect=AssertionError("unexpected GaussDBConnection construction"))
    monkeypatch.setattr(health_utils, "GaussDBConnection", constructor)

    result = health_utils.get_gaussdb_status()

    assert result["status"] == "alive"
    assert result["message"]["performance"]["connection"] == "connected"
    constructor.assert_not_called()


def test_tc_cfg_618_get_gaussdb_status_constructs_adapter_without_shared_connection(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    connection = object()
    constructor = Mock(return_value=connection)
    monkeypatch.setattr(health_utils.settings, "docStoreConn", None, raising=False)
    monkeypatch.setattr(health_utils, "GaussDBConnection", constructor)

    resolved = health_utils._get_gaussdb_connection()

    assert resolved is connection
    constructor.assert_called_once_with()


def test_tc_cfg_618_get_gaussdb_status_constructs_adapter_for_non_gaussdb_connection(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    connection = object()
    constructor = Mock(return_value=connection)
    other_doc_store = types.SimpleNamespace(db_type=lambda: "elasticsearch")
    monkeypatch.setattr(health_utils.settings, "docStoreConn", other_doc_store, raising=False)
    monkeypatch.setattr(health_utils, "GaussDBConnection", constructor)

    resolved = health_utils._get_gaussdb_connection()

    assert resolved is connection
    constructor.assert_called_once_with()


def test_tc_cfg_614_get_gaussdb_status_error_masks_password(monkeypatch, caplog):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(health_utils, "_get_gaussdb_connection", Mock(return_value=FailingGaussDBConnection()))
    caplog.set_level("ERROR")

    result = health_utils.get_gaussdb_status()
    log_text = caplog.text

    assert result["status"] == "timeout"
    assert CFG614_PASSWORD_SENTINEL not in result["message"]
    assert FULL_DSN_SENTINEL not in result["message"]
    assert ACCESS_TOKEN_SENTINEL not in result["message"]
    assert "postgresql://" not in result["message"]
    assert "***" in result["message"]
    assert "GaussDB status check failed (RuntimeError)" in log_text
    assert "***" in log_text
    assert CFG614_PASSWORD_SENTINEL not in log_text
    assert FULL_DSN_SENTINEL not in log_text
    assert ACCESS_TOKEN_SENTINEL not in log_text


def test_tc_cfg_613_get_gaussdb_status_reports_timeout_when_health_is_unhealthy(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(health_utils, "_get_gaussdb_connection", Mock(return_value=UnhealthyGaussDBConnection()))

    result = health_utils.get_gaussdb_status()

    assert result["status"] == "timeout"
    assert result["message"]["health"]["status"] == "unhealthy"
    assert result["message"]["health"]["error"] == "compatibility unsupported password=***"
    assert result["message"]["performance"]["connection"] == "connected"
    assert result["message"]["performance"]["latency_ms"] == 1.2
    assert result["message"]["performance"]["error"] == "dsn password:***"


def test_tc_cfg_619_check_gaussdb_health_reports_healthy_details(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(
        health_utils,
        "_get_gaussdb_connection",
        Mock(return_value=FakeGaussDBConnection()),
    )

    result = health_utils.check_gaussdb_health()

    assert result["status"] == "healthy"
    assert result["details"]["connection"] == "connected"
    assert result["details"]["latency_ms"] == 3.2
    assert result["details"]["sql_compatibility"] == "A"


def test_tc_cfg_620_check_gaussdb_health_reports_incompatible_mode(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(
        health_utils,
        "_get_gaussdb_connection",
        Mock(return_value=IncompatibleGaussDBConnection()),
    )

    result = health_utils.check_gaussdb_health()

    assert result["status"] == "unhealthy"
    assert result["details"]["version"] == "GaussDB"
    assert result["details"]["sql_compatibility"] == "PG"
    assert "A/ORA" in result["details"]["error"]


def test_tc_cfg_614_check_gaussdb_health_logs_masked_exception(monkeypatch, caplog):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(health_utils, "_get_gaussdb_connection", Mock(return_value=FailingGaussDBConnection()))
    caplog.set_level("ERROR")

    result = health_utils.check_gaussdb_health()
    log_text = caplog.text

    assert result["status"] == "unhealthy"
    assert result["details"]["connection"] == "disconnected"
    assert "GaussDB health check failed (RuntimeError)" in log_text
    assert "***" in log_text
    assert CFG614_PASSWORD_SENTINEL not in log_text
    assert FULL_DSN_SENTINEL not in log_text
    assert ACCESS_TOKEN_SENTINEL not in log_text


def test_tc_cfg_711_check_gaussdb_health_masks_nested_performance_errors(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(
        health_utils,
        "_get_gaussdb_connection",
        Mock(return_value=SecretBearingGaussDBConnection()),
    )

    result = health_utils.check_gaussdb_health()
    serialized = json.dumps(result, sort_keys=True)

    assert result["status"] == "unhealthy"
    assert PASSWORD_SENTINEL not in serialized
    assert FULL_DSN_SENTINEL not in serialized
    assert ACCESS_TOKEN_SENTINEL not in serialized


def test_tc_cfg_707_mask_gaussdb_string_masks_password_equals(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)

    assert health_utils._mask_gaussdb_string("host=h password=secret port=5432") == "host=h password=*** port=5432"


def test_tc_cfg_708_mask_gaussdb_string_masks_password_colon(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)

    assert health_utils._mask_gaussdb_string("connect failed password:fake-unit-password") == "connect failed password:***"


@pytest.mark.parametrize("value", ["PASSWORD=fake-unit-password", "Password=fake-unit-password"])
def test_tc_cfg_709_mask_gaussdb_string_is_case_insensitive(monkeypatch, value):
    health_utils = _import_health_utils(monkeypatch)

    assert health_utils._mask_gaussdb_string(value).lower() == "password=***"


def test_tc_cfg_710_mask_gaussdb_secret_masks_nested_lists_and_tuples(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)

    masked = health_utils._mask_gaussdb_secret(
        {
            "nested": {
                "password": PASSWORD_SENTINEL,
                "error": f'password="{PASSWORD_SENTINEL}"',
            },
            "containers": [
                {"ACCESS_TOKEN": ACCESS_TOKEN_SENTINEL},
                ({"dsn": FULL_DSN_SENTINEL},),
            ],
        }
    )
    serialized = json.dumps(masked, sort_keys=True)

    assert masked["nested"]["password"] == "***"
    assert "***" in masked["nested"]["error"]
    assert masked["containers"][0]["ACCESS_TOKEN"] == "***"
    assert masked["containers"][1][0]["dsn"] == "***"
    assert PASSWORD_SENTINEL not in serialized
    assert FULL_DSN_SENTINEL not in serialized
    assert ACCESS_TOKEN_SENTINEL not in serialized


def test_tc_cfg_711_get_gaussdb_status_masks_password_dsn_and_access_token_sentinels(monkeypatch):
    health_utils = _import_health_utils(monkeypatch)
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(health_utils, "_get_gaussdb_connection", Mock(return_value=SecretBearingGaussDBConnection()))

    result = health_utils.get_gaussdb_status()
    serialized = json.dumps(result, sort_keys=True)

    assert result["status"] == "timeout"
    assert result["message"]["health"]["status"] == "unhealthy"
    assert result["message"]["health"]["schema"] == "public"
    assert result["message"]["performance"]["connection"] == "disconnected"
    assert result["message"]["performance"]["schema"] == "public"
    assert result["message"]["health"]["credentials"]["password"] == "***"
    assert result["message"]["health"]["credentials"]["connections"][0]["dsn"] == "***"
    assert result["message"]["health"]["credentials"]["connections"][1][0]["ACCESS_TOKEN"] == "***"
    assert PASSWORD_SENTINEL not in serialized
    assert FULL_DSN_SENTINEL not in serialized
    assert ACCESS_TOKEN_SENTINEL not in serialized
    assert "postgresql://" not in serialized
