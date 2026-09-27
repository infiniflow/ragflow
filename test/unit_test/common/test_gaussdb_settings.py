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

FAKE_GAUSSDB_POOL = object()


class FakeDocEngineConnection:
    def db_type(self):
        return "stub"


class FakeGaussDBConnection:
    def __init__(self, pool=None):
        self.pool = pool or FAKE_GAUSSDB_POOL

    def db_type(self):
        return "gaussdb"


class FakeGaussDBMemoryConnection:
    def __init__(self, pool=None):
        self.pool = pool or FAKE_GAUSSDB_POOL

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
    parent_name, _, child_name = name.rpartition(".")
    parent = sys.modules.get(parent_name)
    if parent is not None:
        monkeypatch.setattr(parent, child_name, mod, raising=False)
    return mod


def _install_settings_import_stubs(monkeypatch):
    importlib.import_module("rag.utils")
    importlib.import_module("memory.utils")

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
        _install_module(monkeypatch, f"rag.utils.{short_name}", **attrs)

    memory_modules = {
        "es_conn": {"ESConnection": FakeDocEngineConnection},
        "infinity_conn": {"InfinityConnection": FakeDocEngineConnection},
        "ob_conn": {"OBConnection": FakeDocEngineConnection},
        "gaussdb_conn": {"GaussDBMemoryConnection": FakeGaussDBMemoryConnection},
    }
    for short_name, attrs in memory_modules.items():
        _install_module(monkeypatch, f"memory.utils.{short_name}", **attrs)

    fake_search = types.SimpleNamespace(Dealer=lambda conn: ("dealer", conn))
    fake_kg_search = types.SimpleNamespace(KGSearch=lambda conn: ("kg", conn))
    _install_module(monkeypatch, "rag.nlp", search=fake_search)
    _install_module(monkeypatch, "rag.graphrag", search=fake_kg_search)
    _install_module(monkeypatch, "rag.graphrag.search", KGSearch=fake_kg_search.KGSearch)


def _import_settings(monkeypatch):
    _install_settings_import_stubs(monkeypatch)
    import common

    # Import machinery assigns a freshly imported submodule to
    # ``common.settings`` on the parent package. Restoring only
    # ``sys.modules["common.settings"]`` leaves that parent attribute pointing
    # at this test's stub-backed module and contaminates later tests.
    monkeypatch.setattr(
        common,
        "settings",
        getattr(common, "settings", None),
        raising=False,
    )
    monkeypatch.delitem(sys.modules, "common.settings", raising=False)
    return importlib.import_module("common.settings")


def test_doc_engine_gaussdb_initializes_gaussdb_connection(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: (
            {
                "host": "db.example",
                "port": 19995,
                "database": "postgres",
                "user": "sqlbuilder",
                "password": "fake-unit-password",
                "schema": "ragflow_gaussdb_docengine_it",
            }
            if name == "gaussdb"
            else (default or {})
        ),
    )

    settings.init_settings()

    assert settings.DOC_ENGINE == "gaussdb"
    assert settings.DOC_ENGINE_GAUSSDB is True
    assert settings.DOC_ENGINE_OCEANBASE is False
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.msgStoreConn.db_type() == "gaussdb"
    assert settings.docStoreConn is not settings.msgStoreConn
    assert settings.docStoreConn.pool is settings.msgStoreConn.pool
    assert settings.GAUSSDB["schema"] == "ragflow_gaussdb_docengine_it"


def test_doc_engine_gaussdb_does_not_change_database_type(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setenv("DB_TYPE", "mysql")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: (
            {
                "host": "h",
                "port": 1,
                "database": "d",
                "user": "u",
                "password": "fake-unit-password",
            }
            if name == "gaussdb"
            else (default or {})
        ),
    )

    settings.init_settings()

    assert settings.DATABASE_TYPE != "gaussdb"
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.msgStoreConn.db_type() == "gaussdb"
    assert settings.docStoreConn.pool is settings.msgStoreConn.pool


def test_doc_engine_gaussdb_initializes_message_store_case_insensitive(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "GaussDB")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: (
            {
                "host": "h",
                "port": 19995,
                "database": "d",
                "user": "u",
                "password": "fake-unit-password",
            }
            if name == "gaussdb"
            else (default or {})
        ),
    )

    settings.init_settings()

    assert settings.DOC_ENGINE == "GaussDB"
    assert settings.DOC_ENGINE_GAUSSDB is True
    assert settings.docStoreConn.db_type() == "gaussdb"
    assert settings.msgStoreConn.db_type() == "gaussdb"
    assert settings.docStoreConn.pool is settings.msgStoreConn.pool


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


def test_db_type_and_doc_engine_gaussdb_use_separate_configs(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DB_TYPE", "gaussdb")
    monkeypatch.setenv("DOC_ENGINE", "gaussdb")
    monkeypatch.setenv("GAUSSDB_METADATA_HOST", "metadata.example")
    monkeypatch.setenv("GAUSSDB_METADATA_PORT", "8000")
    monkeypatch.setenv("GAUSSDB_METADATA_DBNAME", "metadata_db")
    monkeypatch.setenv("GAUSSDB_METADATA_USER", "metadata_user")
    monkeypatch.setenv("GAUSSDB_METADATA_PASSWORD", "metadata-secret")
    monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "metadata_schema")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: (
            {
                "host": "doc.example",
                "port": 19995,
                "database": "doc_db",
                "user": "doc_user",
                "password": "doc-secret",
                "schema": "doc_schema",
            }
            if name == "gaussdb"
            else (default or {})
        ),
    )

    settings.init_settings()

    assert settings.DATABASE_TYPE == "gaussdb"
    assert settings.DATABASE["host"] == "metadata.example"
    assert settings.DATABASE["name"] == "metadata_db"
    assert settings.DATABASE["options"].startswith("-c search_path=metadata_schema ")
    assert settings.GAUSSDB["host"] == "doc.example"
    assert settings.GAUSSDB["database"] == "doc_db"
    assert settings.docStoreConn.pool is settings.msgStoreConn.pool
