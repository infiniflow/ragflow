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
import importlib
import sys
import types
from unittest.mock import Mock

import pytest


class FakeDocEngineConnection:
    def db_type(self):
        return "stub"


class FakeGaussDBConnection:
    def db_type(self):
        return "gaussdb"


class FakeStorage:
    def health(self):
        return True


def _install_module(monkeypatch, name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _install_settings_import_stubs(monkeypatch):
    import rag.utils
    import memory.utils

    rag_modules = {
        "es_conn": {"ESConnection": FakeDocEngineConnection},
        "infinity_conn": {"InfinityConnection": FakeDocEngineConnection},
        "ob_conn": {"OBConnection": FakeDocEngineConnection},
        "opensearch_conn": {"OSConnection": FakeDocEngineConnection},
        "gaussdb_conn": {"GaussDBConnection": FakeGaussDBConnection},
        "azure_sas_conn": {"RAGFlowAzureSasBlob": FakeStorage},
        "azure_spn_conn": {"RAGFlowAzureSpnBlob": FakeStorage},
        "gcs_conn": {"RAGFlowGCS": FakeStorage},
        "minio_conn": {"RAGFlowMinio": FakeStorage},
        "opendal_conn": {"OpenDALStorage": FakeStorage},
        "s3_conn": {"RAGFlowS3": FakeStorage},
        "oss_conn": {"RAGFlowOSS": FakeStorage},
        "redis_conn": {"REDIS_CONN": types.SimpleNamespace(health=lambda: True)},
    }
    for short_name, attrs in rag_modules.items():
        mod = _install_module(monkeypatch, f"rag.utils.{short_name}", **attrs)
        monkeypatch.setattr(rag.utils, short_name, mod, raising=False)

    for short_name in ("es_conn", "infinity_conn", "ob_conn"):
        mod = _install_module(monkeypatch, f"memory.utils.{short_name}", **{"ESConnection": FakeDocEngineConnection, "InfinityConnection": FakeDocEngineConnection, "OBConnection": FakeDocEngineConnection})
        monkeypatch.setattr(memory.utils, short_name, mod, raising=False)

    fake_search = types.SimpleNamespace(Dealer=lambda conn: ("dealer", conn))
    fake_kg_search = types.SimpleNamespace(KGSearch=lambda conn: ("kg", conn))
    _install_module(monkeypatch, "rag.nlp", search=fake_search)
    _install_module(monkeypatch, "rag.graphrag", search=fake_kg_search)
    _install_module(monkeypatch, "rag.graphrag.search", KGSearch=fake_kg_search.KGSearch)


def _import_settings(monkeypatch):
    _install_settings_import_stubs(monkeypatch)
    monkeypatch.delitem(sys.modules, "common.settings", raising=False)
    return importlib.import_module("common.settings")


def _fake_gaussdb_config(schema="ragflow_gaussdb_docengine_it"):
    return {
        "host": "db.example",
        "port": 19995,
        "database": "postgres",
        "user": "sqlbuilder",
        "password": "fake-unit-password",
        "schema": schema,
    }


def test_tc_cfg_501_doc_engine_gaussdb_initializes_gaussdb_connection(monkeypatch):
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setenv("DB_TYPE", "mysql")
    settings = _import_settings(monkeypatch)
    gaussdb_config = _fake_gaussdb_config()

    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: gaussdb_config if name == "gaussdb" else (default or {}),
    )

    settings.init_settings()

    assert settings.DOC_ENGINE == "gaussdb"
    assert settings.DOC_ENGINE_GAUSSDB is True
    assert settings.DOC_ENGINE_OCEANBASE is False
    assert settings.GAUSSDB == gaussdb_config
    assert type(settings.docStoreConn) is FakeGaussDBConnection
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.DATABASE_TYPE == "mysql"
    assert settings.DATABASE_TYPE != "gaussdb"


def test_tc_cfg_502_doc_engine_gaussdb_is_case_insensitive(monkeypatch):
    monkeypatch.setenv("DOC_ENGINE", "GAUSSDB")
    monkeypatch.setenv("DB_TYPE", "mysql")
    settings = _import_settings(monkeypatch)

    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: _fake_gaussdb_config() if name == "gaussdb" else (default or {}),
    )
    settings.msgStoreConn = None

    settings.init_settings()

    assert settings.DOC_ENGINE == "GAUSSDB"
    assert settings.DOC_ENGINE_GAUSSDB is True
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.DATABASE_TYPE == "mysql"
    assert settings.msgStoreConn is None


def test_tc_cfg_503_default_doc_engine_is_not_gaussdb(monkeypatch):
    settings = _import_settings(monkeypatch)
    gaussdb_module = sys.modules["rag.utils.gaussdb_conn"]
    gaussdb_constructor = Mock(side_effect=AssertionError("GaussDB must not be constructed"))
    monkeypatch.setattr(gaussdb_module, "GaussDBConnection", gaussdb_constructor)

    monkeypatch.delenv("DOC_ENGINE", raising=False)
    monkeypatch.setattr(settings, "get_base_config", lambda _name, default=None: default or {})

    settings.init_settings()

    assert settings.DOC_ENGINE == "elasticsearch"
    assert settings.DOC_ENGINE_GAUSSDB is False
    assert settings.docStoreConn.db_type() == "stub"
    gaussdb_constructor.assert_not_called()


def test_tc_cfg_504_unknown_doc_engine_is_rejected(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "unknown_db")
    monkeypatch.setattr(settings, "get_base_config", lambda _name, default=None: default or {})
    settings.docStoreConn = None

    with pytest.raises(Exception) as exc_info:
        settings.init_settings()

    assert str(exc_info.value) == "Not supported doc engine: unknown_db"
    assert settings.DOC_ENGINE == "unknown_db"
    assert settings.DOC_ENGINE_GAUSSDB is False
    assert settings.docStoreConn is None


def test_tc_cfg_505_gaussdb_does_not_initialize_message_store(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: _fake_gaussdb_config() if name == "gaussdb" else (default or {}),
    )
    settings.msgStoreConn = None

    settings.init_settings()

    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.msgStoreConn is None


def test_tc_cfg_506_doc_engine_gaussdb_does_not_change_database_type(monkeypatch):
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setenv("DB_TYPE", "mysql")
    settings = _import_settings(monkeypatch)
    existing_message_store = object()

    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: _fake_gaussdb_config(schema="public") if name == "gaussdb" else (default or {}),
    )
    settings.msgStoreConn = existing_message_store

    settings.init_settings()

    assert settings.DOC_ENGINE == "gaussdb"
    assert settings.DATABASE_TYPE == "mysql"
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.msgStoreConn is existing_message_store
