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
import json
import sys
import types
from contextlib import suppress
from pathlib import Path

import pytest
import psycopg2


_MISSING = object()
_OPTIONAL_STUB_MODULES = (
    "google",
    "google.cloud",
    "google.cloud.storage",
    "google.api_core",
    "google.api_core.exceptions",
    "json_repair",
    "rag.utils.es_conn",
    "rag.utils.infinity_conn",
    "rag.utils.ob_conn",
    "rag.utils.opensearch_conn",
    "rag.utils.azure_sas_conn",
    "rag.utils.azure_spn_conn",
    "rag.utils.gcs_conn",
    "rag.utils.minio_conn",
    "rag.utils.opendal_conn",
    "rag.utils.redis_conn",
    "rag.utils.s3_conn",
    "rag.utils.oss_conn",
    "memory.utils.es_conn",
    "memory.utils.infinity_conn",
    "memory.utils.ob_conn",
)
_LOADED_MODULES = (
    "common.settings",
    "api.db.db_models",
    "api.db.services.doc_metadata_service",
)


def _module(name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    return mod


class FakeDB:
    @staticmethod
    def connection_context():
        def decorator(func):
            func.__wrapped__ = func
            return func

        return decorator


class FakeField:
    def __eq__(self, _other):
        return self


def _install_optional_import_stubs(monkeypatch):
    class StubClient:
        pass

    class StubNotFound(Exception):
        pass

    class StubDocEngineConnection:
        def db_type(self):
            return "stub"

    class StubStorage:
        def health(self):
            return True

    google = _module("google")
    google_cloud = _module("google.cloud")
    google_storage = _module("google.cloud.storage", Client=StubClient)
    google_api_core = _module("google.api_core")
    google_api_exceptions = _module("google.api_core.exceptions", NotFound=StubNotFound)
    google_cloud.storage = google_storage
    google_api_core.exceptions = google_api_exceptions
    google.cloud = google_cloud
    google.api_core = google_api_core
    for name, mod in (
        ("google", google),
        ("google.cloud", google_cloud),
        ("google.cloud.storage", google_storage),
        ("google.api_core", google_api_core),
        ("google.api_core.exceptions", google_api_exceptions),
    ):
        monkeypatch.setitem(sys.modules, name, mod)

    import rag.utils
    import memory.utils

    rag_modules = {
        "es_conn": {"ESConnection": StubDocEngineConnection},
        "infinity_conn": {"InfinityConnection": StubDocEngineConnection},
        "ob_conn": {"OBConnection": StubDocEngineConnection},
        "opensearch_conn": {"OSConnection": StubDocEngineConnection},
        "azure_sas_conn": {"RAGFlowAzureSasBlob": StubStorage},
        "azure_spn_conn": {"RAGFlowAzureSpnBlob": StubStorage},
        "gcs_conn": {"RAGFlowGCS": StubStorage},
        "minio_conn": {"RAGFlowMinio": StubStorage},
        "opendal_conn": {"OpenDALStorage": StubStorage},
        "redis_conn": {"REDIS_CONN": types.SimpleNamespace(health=lambda: True)},
        "s3_conn": {"RAGFlowS3": StubStorage},
        "oss_conn": {"RAGFlowOSS": StubStorage},
    }
    for short_name, attrs in rag_modules.items():
        mod = _module(f"rag.utils.{short_name}", **attrs)
        monkeypatch.setitem(sys.modules, f"rag.utils.{short_name}", mod)
        monkeypatch.setattr(rag.utils, short_name, mod, raising=False)

    for short_name in ("es_conn", "infinity_conn", "ob_conn"):
        mod = _module(
            f"memory.utils.{short_name}",
            ESConnection=StubDocEngineConnection,
            InfinityConnection=StubDocEngineConnection,
            OBConnection=StubDocEngineConnection,
        )
        monkeypatch.setitem(sys.modules, f"memory.utils.{short_name}", mod)
        monkeypatch.setattr(memory.utils, short_name, mod, raising=False)

    monkeypatch.setitem(sys.modules, "json_repair", _module("json_repair", loads=json.loads))


@pytest.fixture
def doc_metadata_context(monkeypatch):
    tracked = (*_OPTIONAL_STUB_MODULES, *_LOADED_MODULES)
    original_modules = {name: sys.modules.get(name, _MISSING) for name in tracked}

    _install_optional_import_stubs(monkeypatch)
    import common

    fake_settings = _module(
        "common.settings",
        docStoreConn=None,
        DOC_ENGINE_GAUSSDB=False,
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
    )
    monkeypatch.setitem(sys.modules, "common.settings", fake_settings)
    monkeypatch.setattr(common, "settings", fake_settings, raising=False)
    db_models = _module(
        "api.db.db_models",
        DB=FakeDB(),
        Document=types.SimpleNamespace(
            id=FakeField(),
            kb_id=FakeField(),
            select=staticmethod(lambda *_args, **_kwargs: None),
        ),
        Knowledgebase=types.SimpleNamespace(
            id=FakeField(),
            tenant_id=FakeField(),
            get_by_id=staticmethod(lambda *_args, **_kwargs: None),
        ),
    )
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models)
    repo_root = Path(__file__).resolve().parents[5]
    spec = importlib.util.spec_from_file_location(
        "api.db.services.doc_metadata_service",
        repo_root / "api" / "db" / "services" / "doc_metadata_service.py",
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module.settings, "docStoreConn", None, raising=False)
    search_result_cls = importlib.import_module("rag.utils.gaussdb_conn").SearchResult

    try:
        yield types.SimpleNamespace(
            module=module,
            DocMetadataService=module.DocMetadataService,
            settings=module.settings,
            SearchResult=search_result_cls,
        )
    finally:
        services_pkg = sys.modules.get("api.db.services")
        if original_modules["api.db.services.doc_metadata_service"] is _MISSING and services_pkg is not None:
            with suppress(AttributeError):
                delattr(services_pkg, "doc_metadata_service")
        for name, original in original_modules.items():
            if original is _MISSING:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


class FakeGaussDBStore:
    def __init__(self, *, exists=True, create_result=True, create_error=None, insert_result=None):
        self.created_meta = []
        self.deleted = []
        self.inserted = []
        self.executed = []
        self.existence_checks = []
        self.exists = exists
        self.create_result = create_result
        self.create_error = create_error
        self.insert_result = [] if insert_result is None else insert_result

    def db_type(self):
        return "gaussdb"

    def create_doc_meta_idx(self, index_name):
        self.created_meta.append(index_name)
        if self.create_error is not None:
            raise self.create_error
        return self.create_result

    def index_exist(self, *args):
        self.existence_checks.append(args)
        return self.exists

    def get(self, *_args):
        return {"id": "doc1", "kb_id": "kb1", "meta_fields": {"old": "value"}}

    def delete(self, condition, index_name, kb_id):
        self.deleted.append((condition, index_name, kb_id))
        return 1

    def insert(self, rows, index_name, kb_id):
        self.inserted.append((rows, index_name, kb_id))
        return self.insert_result

    def fetch_metadata_doc_ids(self, index_name, kb_ids, sql_filter, filter_params, limit):
        self.executed.append((index_name, kb_ids, sql_filter, filter_params, limit))
        return ["doc-risk"]


class FakeDocQuery:
    def join(self, *_args, **_kwargs):
        return self

    def where(self, *_args, **_kwargs):
        return self

    def first(self):
        return types.SimpleNamespace(
            kb_id="kb1",
            knowledgebase=types.SimpleNamespace(tenant_id="tenant1"),
        )


class FakeSearchResultStore(FakeGaussDBStore):
    def __init__(self, search_result_cls):
        super().__init__()
        self.search_result_cls = search_result_cls

    def search(self, **kwargs):
        self.executed.append(kwargs)
        return self.search_result_cls(
            total=1,
            chunks=[{"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}],
        )


class PagingSearchResultStore(FakeGaussDBStore):
    def __init__(self, search_result_cls):
        super().__init__()
        self.search_result_cls = search_result_cls

    def search(self, **kwargs):
        self.executed.append(kwargs)
        offset = kwargs["offset"]
        limit = kwargs["limit"]
        if offset == 0:
            chunks = [{"id": f"doc{i}", "kb_id": "kb1", "meta_fields": {"seq": i}} for i in range(limit)]
        elif offset == limit:
            chunks = [{"id": "doc1000", "kb_id": "kb1", "meta_fields": {"seq": 1000}}]
        else:
            chunks = []
        return self.search_result_cls(total=1001, chunks=chunks)


def test_tc_ret_407_iter_search_results_accepts_gaussdb_search_result_shape(doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    result = doc_metadata_context.SearchResult(total=1, chunks=[{"id": "doc1", "meta_fields": {"author": "Alice"}}])

    docs = list(service._iter_search_results(result))

    assert docs == [("doc1", {"id": "doc1", "meta_fields": {"author": "Alice"}})]


def test_tc_ret_407_search_metadata_accepts_gaussdb_search_result_shape(monkeypatch, doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    module = doc_metadata_context.module
    settings = doc_metadata_context.settings
    fake = FakeSearchResultStore(doc_metadata_context.SearchResult)
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(
        module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    docs = service._search_metadata("kb1")

    assert docs == [{"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}]
    assert fake.executed[0]["index_names"] == "ragflow_doc_meta_tenant1"
    assert fake.executed[0]["knowledgebase_ids"] == ["kb1"]


def test_tc_ret_407_search_metadata_paginates_gaussdb_search_result_total(monkeypatch, doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    module = doc_metadata_context.module
    settings = doc_metadata_context.settings
    fake = PagingSearchResultStore(doc_metadata_context.SearchResult)
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(
        module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    docs = service._search_metadata("kb1")

    doc_ids = [doc["id"] for doc in docs]
    assert doc_ids == [f"doc{i}" for i in range(1001)]
    assert len(set(doc_ids)) == 1001
    assert [call["offset"] for call in fake.executed] == [0, 1000]
    assert len(fake.executed) == 2


@pytest.mark.parametrize(
    ("table_exists", "created_meta"),
    [
        (True, []),
        (False, ["ragflow_doc_meta_tenant1"]),
    ],
    ids=("existing-table", "missing-table"),
)
def test_tc_wrt_306_update_document_metadata_checks_table_before_gaussdb_upsert(
    monkeypatch,
    doc_metadata_context,
    table_exists,
    created_meta,
):
    service = doc_metadata_context.DocMetadataService
    module = doc_metadata_context.module
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore(exists=table_exists)
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(module.Document, "select", staticmethod(lambda *_args, **_kwargs: FakeDocQuery()))

    ok = service.update_document_metadata.__wrapped__(
        service,
        "doc1",
        {"author": "Alice"},
    )

    assert ok is True
    assert fake.existence_checks == [("ragflow_doc_meta_tenant1", "kb1")]
    assert fake.created_meta == created_meta
    assert fake.deleted == []
    assert fake.inserted == [([{"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}], "ragflow_doc_meta_tenant1", "kb1")]


@pytest.mark.parametrize(
    ("create_result", "create_error"),
    [
        (False, None),
        (True, psycopg2.OperationalError("metadata table creation failed")),
    ],
    ids=("returns-false", "raises"),
)
def test_tc_wrt_307_update_document_metadata_stops_when_gaussdb_meta_table_creation_fails(
    monkeypatch,
    doc_metadata_context,
    create_result,
    create_error,
):
    service = doc_metadata_context.DocMetadataService
    module = doc_metadata_context.module
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore(exists=False, create_result=create_result, create_error=create_error)
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(module.Document, "select", staticmethod(lambda *_args, **_kwargs: FakeDocQuery()))

    ok = service.update_document_metadata.__wrapped__(
        service,
        "doc1",
        {"author": "Alice"},
    )

    assert ok is False
    assert fake.existence_checks == [("ragflow_doc_meta_tenant1", "kb1")]
    assert fake.created_meta == ["ragflow_doc_meta_tenant1"]
    assert fake.inserted == []
    assert fake.deleted == []


def test_tc_wrt_308_update_document_metadata_returns_false_for_gaussdb_upsert_errors(monkeypatch, doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    module = doc_metadata_context.module
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore(insert_result=["doc1"])
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(module.Document, "select", staticmethod(lambda *_args, **_kwargs: FakeDocQuery()))

    ok = service.update_document_metadata.__wrapped__(
        service,
        "doc1",
        {"author": "Alice"},
    )

    assert ok is False
    assert fake.existence_checks == [("ragflow_doc_meta_tenant1", "kb1")]
    assert fake.created_meta == []
    assert fake.inserted == [([{"id": "doc1", "kb_id": "kb1", "meta_fields": {"author": "Alice"}}], "ragflow_doc_meta_tenant1", "kb1")]
    assert fake.deleted == []


def test_tc_mgf_710_gaussdb_pushdown_routes_scoped_filter_with_probe_limit(monkeypatch, doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore()
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(
        doc_metadata_context.module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    doc_ids = service.filter_doc_ids_by_meta_pushdown(
        ["kb1"],
        [{"key": "author", "op": "=", "value": "Alice"}],
        "and",
        limit=25,
    )

    assert doc_ids == ["doc-risk"]
    index_name, kb_ids, sql_filter, params, limit = fake.executed[0]
    assert index_name == "ragflow_doc_meta_tenant1"
    assert kb_ids == ["kb1"]
    assert sql_filter == "(lower(meta_fields #>> '{author}') = %s OR jsonb_exists(meta_fields #> '{author}', %s))"
    assert params == ["alice", "alice"]
    assert limit == 26
    assert fake.created_meta == []
    assert fake.inserted == []
    assert fake.deleted == []


@pytest.mark.parametrize(
    ("returned_ids", "expected"),
    [
        (["doc1", "doc2"], ["doc1", "doc2"]),
        (["doc1", "doc1", "doc2"], ["doc1", "doc2"]),
        (["doc1", "doc2", "doc3"], None),
    ],
)
def test_tc_mgf_710_gaussdb_pushdown_enforces_complete_result_cap(
    monkeypatch,
    doc_metadata_context,
    returned_ids,
    expected,
):
    service = doc_metadata_context.DocMetadataService
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore()
    calls = []

    def fetch_metadata_doc_ids(*args):
        calls.append(args)
        return returned_ids

    fake.fetch_metadata_doc_ids = fetch_metadata_doc_ids
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(
        doc_metadata_context.module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    doc_ids = service.filter_doc_ids_by_meta_pushdown(
        ["kb1"],
        [{"key": "author", "op": "=", "value": "Alice"}],
        "and",
        limit=2,
    )

    assert doc_ids == expected
    assert len(calls) == 1
    index_name, kb_ids, sql_filter, params, probe_limit = calls[0]
    assert index_name == "ragflow_doc_meta_tenant1"
    assert kb_ids == ["kb1"]
    assert sql_filter == "(lower(meta_fields #>> '{author}') = %s OR jsonb_exists(meta_fields #> '{author}', %s))"
    assert params == ["alice", "alice"]
    assert probe_limit == 3
    assert fake.created_meta == []
    assert fake.inserted == []
    assert fake.deleted == []


@pytest.mark.parametrize(
    ("filters", "logic"),
    [
        ([{"key": "author", "op": "regex", "value": "^A"}], "and"),
        ([{"key": "author", "op": "=", "value": "Alice"}], "xor"),
    ],
)
def test_tc_mgf_711_gaussdb_pushdown_rejects_unsupported_filter_without_fetch(
    monkeypatch,
    doc_metadata_context,
    caplog,
    filters,
    logic,
):
    service = doc_metadata_context.DocMetadataService
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore()
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(
        doc_metadata_context.module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    caplog.clear()
    doc_ids = service.filter_doc_ids_by_meta_pushdown(
        ["kb1"],
        filters,
        logic,
        limit=2,
    )

    assert doc_ids is None
    assert fake.executed == []
    assert fake.created_meta == []
    assert fake.inserted == []
    assert fake.deleted == []
    messages = [record.getMessage() for record in caplog.records]
    assert all("author" not in message and "Alice" not in message and "^A" not in message for message in messages)


def test_tc_mgf_712_gaussdb_pushdown_falls_back_without_partial_ids_on_fetch_failure(monkeypatch, doc_metadata_context):
    service = doc_metadata_context.DocMetadataService
    settings = doc_metadata_context.settings
    fake = FakeGaussDBStore()
    calls = []

    def raise_fetch(*args, **_kwargs):
        calls.append((args, _kwargs))
        raise RuntimeError("gaussdb unavailable")

    fake.fetch_metadata_doc_ids = raise_fetch
    monkeypatch.setattr(settings, "docStoreConn", fake)
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True)
    monkeypatch.setattr(
        doc_metadata_context.module.Knowledgebase,
        "get_by_id",
        staticmethod(lambda _kb_id: type("KB", (), {"tenant_id": "tenant1"})()),
    )

    doc_ids = service.filter_doc_ids_by_meta_pushdown(
        ["kb1"],
        [{"key": "author", "op": "=", "value": "Alice"}],
        "and",
        limit=2,
    )

    assert doc_ids is None
    assert len(calls) == 1
    args, kwargs = calls[0]
    assert args[0:2] == ("ragflow_doc_meta_tenant1", ["kb1"])
    assert args[2] == "(lower(meta_fields #>> '{author}') = %s OR jsonb_exists(meta_fields #> '{author}', %s))"
    assert args[3:] == (["alice", "alice"], 3)
    assert kwargs == {}
    assert fake.created_meta == []
    assert fake.inserted == []
    assert fake.deleted == []
