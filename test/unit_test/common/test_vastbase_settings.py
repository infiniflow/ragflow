#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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


class FakeDocEngineConnection:
    def db_type(self):
        return "stub"


class FakeVBConnection:
    def db_type(self):
        return "vastbase"


class FakeVBMemoryConnection:
    def db_type(self):
        return "vastbase"


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
        "gaussdb_conn": {"GaussDBConnection": FakeDocEngineConnection},
        "vastbase_conn": {"VBConnection": FakeVBConnection},
        "azure_sas_conn": {"RAGFlowAzureSasBlob": FakeStorage},
        "azure_spn_conn": {"RAGFlowAzureSpnBlob": FakeStorage},
        "gcs_conn": {"RAGFlowGCS": FakeStorage},
        "minio_conn": {"RAGFlowMinio": FakeStorage},
        "opendal_conn": {"OpenDALStorage": FakeStorage},
        "s3_conn": {"RAGFlowS3": FakeStorage},
        "oss_conn": {"RAGFlowOSS": FakeStorage},
        "redis_conn": {"REDIS_CONN": types.SimpleNamespace(health=lambda: True, is_alive=lambda: False)},
    }
    for short_name, attrs in rag_modules.items():
        _install_module(monkeypatch, f"rag.utils.{short_name}", **attrs)

    memory_modules = {
        "es_conn": {"ESConnection": FakeDocEngineConnection},
        "infinity_conn": {"InfinityConnection": FakeDocEngineConnection},
        "ob_conn": {"OBConnection": FakeDocEngineConnection},
        "gaussdb_conn": {"GaussDBMemoryConnection": FakeDocEngineConnection},
        "vastbase_conn": {"VBConnection": FakeVBMemoryConnection},
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

    monkeypatch.setattr(
        common,
        "settings",
        getattr(common, "settings", None),
        raising=False,
    )
    monkeypatch.delitem(sys.modules, "common.settings", raising=False)
    return importlib.import_module("common.settings")


def test_doc_engine_vastbase_initializes_vastbase_connections(monkeypatch):
    settings = _import_settings(monkeypatch)

    monkeypatch.setenv("DOC_ENGINE", "vastbase")
    monkeypatch.setattr(
        settings,
        "get_base_config",
        lambda name, default=None: (
            {
                "host": "vb.local",
                "port": 5432,
                "db_name": "ragflow",
                "user": "ragflow",
                "password": "fake-unit-password",
                "dbcompatibility": "B",
            }
            if name == "vb"
            else (default or {})
        ),
    )
    monkeypatch.setattr(settings, "decrypt_database_config", lambda **kwargs: kwargs.get("database") or {})
    monkeypatch.setattr(settings.StorageFactory, "create", lambda *_args, **_kwargs: FakeStorage())

    settings.init_settings()

    assert settings.DOC_ENGINE == "vastbase"
    assert settings.DOC_ENGINE_VASTBASE is True
    assert settings.DOC_ENGINE_GAUSSDB is False
    assert settings.docStoreConn.db_type() == "vastbase"
    assert settings.msgStoreConn.db_type() == "vastbase"
    assert settings.docStoreConn is not settings.msgStoreConn
    assert settings.VB["host"] == "vb.local"
