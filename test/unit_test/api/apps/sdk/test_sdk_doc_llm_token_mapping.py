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
"""Unit tests verifying that llm_token_num is exposed as llm_token_count in SDK
document responses.

Loads the real api/apps/sdk/doc.py module and exercises the actual handlers so
that any removal of the "llm_token_num" → "llm_token_count" key rename is
immediately caught — unlike a test that only checks a local copy of the mapping.

Three handlers in doc.py apply this rename independently:
  • list_docs    (~line 624)  — tested here
  • update_doc   (~line 347)  — tested here
  • list_chunks  (~line 1084) — shares the same pattern; add tests as needed
"""

import asyncio
import functools
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


# ---------------------------------------------------------------------------
# Module loader (mirrors the approach in test_doc_sdk_routes_unit.py)
# ---------------------------------------------------------------------------

class _DummyManager:
    """Replaces the Quart/Flask manager so @manager.route() is a no-op."""
    def route(self, *_args, **_kwargs):
        def decorator(fn):
            return fn
        return decorator


def _load_doc_module(monkeypatch):
    """Load api/apps/sdk/doc.py with all heavy external deps stubbed out."""
    repo_root = Path(__file__).resolve().parents[5]

    # ---- third-party packages not available outside the ragflow venv ----

    xxhash_mod = ModuleType("xxhash")
    monkeypatch.setitem(sys.modules, "xxhash", xxhash_mod)

    peewee_mod = ModuleType("peewee")
    peewee_mod.OperationalError = Exception
    monkeypatch.setitem(sys.modules, "peewee", peewee_mod)

    pydantic_mod = ModuleType("pydantic")

    class _BaseModel:
        def __init__(self, **kwargs):
            for k, v in kwargs.items():
                setattr(self, k, v)

    pydantic_mod.BaseModel = _BaseModel
    pydantic_mod.Field = lambda *_a, **_k: None
    pydantic_mod.validator = lambda *_a, **_k: (lambda fn: fn)
    monkeypatch.setitem(sys.modules, "pydantic", pydantic_mod)

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args={})
    quart_mod.send_file = lambda *a, **k: None
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    # ---- common package ----

    for pkg in ("common", "common.doc_store", "common.doc_store.doc_store_base"):
        if pkg not in sys.modules:
            mod = ModuleType(pkg)
            mod.__path__ = []
            monkeypatch.setitem(sys.modules, pkg, mod)

    sys.modules["common"].__path__ = [str(repo_root / "common")]

    common_constants = ModuleType("common.constants")
    common_constants.RetCode = SimpleNamespace(SUCCESS=0, ARGUMENT_ERROR=102, NOT_FOUND=404)
    common_constants.FileSource = SimpleNamespace()
    common_constants.LLMType = SimpleNamespace(CHAT="chat", EMBEDDING="embedding")
    common_constants.ParserType = SimpleNamespace()
    common_constants.TaskStatus = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "common.constants", common_constants)

    common_meta = ModuleType("common.metadata_utils")
    common_meta.convert_conditions = lambda c: c
    common_meta.meta_filter = lambda *a, **k: []
    monkeypatch.setitem(sys.modules, "common.metadata_utils", common_meta)

    common_misc = ModuleType("common.misc_utils")
    common_misc.get_uuid = lambda: "uuid"
    common_misc.thread_pool_exec = lambda fn, *a, **k: fn(*a, **k)
    monkeypatch.setitem(sys.modules, "common.misc_utils", common_misc)

    common_str = ModuleType("common.string_utils")
    common_str.remove_redundant_spaces = lambda s: s
    monkeypatch.setitem(sys.modules, "common.string_utils", common_str)

    common_settings = ModuleType("common.settings")
    monkeypatch.setitem(sys.modules, "common.settings", common_settings)

    # ---- rag stubs ----

    for pkg in ("rag", "rag.nlp", "rag.app", "rag.app.qa", "rag.app.tag",
                "rag.prompts", "rag.prompts.generator"):
        mod = ModuleType(pkg)
        mod.__path__ = []
        monkeypatch.setitem(sys.modules, pkg, mod)
    sys.modules["rag.nlp"].rag_tokenizer = SimpleNamespace()
    sys.modules["rag.nlp"].search = SimpleNamespace(index_name=lambda *a: "idx")
    sys.modules["rag.app.qa"].beAdoc = lambda *a, **k: None
    sys.modules["rag.app.qa"].rmPrefix = lambda *a, **k: None
    sys.modules["rag.app.tag"].label_question = lambda *a, **k: None
    sys.modules["rag.prompts.generator"].cross_languages = lambda *a, **k: None
    sys.modules["rag.prompts.generator"].keyword_extraction = lambda *a, **k: None

    # ---- api package stubs ----

    for pkg in ("api", "api.db", "api.db.services", "api.db.joint_services", "api.utils"):
        if pkg not in sys.modules:
            mod = ModuleType(pkg)
            mod.__path__ = []
            monkeypatch.setitem(sys.modules, pkg, mod)

    api_constants = ModuleType("api.constants")
    api_constants.FILE_NAME_LEN_LIMIT = 255
    monkeypatch.setitem(sys.modules, "api.constants", api_constants)

    sys.modules["api.db"].FileType = SimpleNamespace()

    api_db_models = ModuleType("api.db.db_models")
    for name in ("APIToken", "File", "Task"):
        setattr(api_db_models, name, type(name, (), {}))
    monkeypatch.setitem(sys.modules, "api.db.db_models", api_db_models)

    # token_required must use functools.wraps so __wrapped__ is set on the handlers
    def _token_required(fn):
        @functools.wraps(fn)
        def wrapper(*args, **kwargs):
            return fn(*args, **kwargs)
        return wrapper

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.token_required = _token_required
    api_utils_mod.RetCode = common_constants.RetCode
    api_utils_mod.get_result = lambda code=0, message="", data=None, total=None: {
        "code": 0, "data": data, "message": message,
    }
    api_utils_mod.get_error_data_result = lambda code=1, message="error": {
        "code": 1, "message": message,
    }
    api_utils_mod.server_error_response = lambda e: {"code": 500, "message": str(e)}
    api_utils_mod.construct_json_result = lambda *a, **k: {"code": 0}
    api_utils_mod.check_duplicate_ids = lambda *a, **k: ([], [])
    api_utils_mod.get_parser_config = lambda *a, **k: {}
    api_utils_mod.get_request_json = lambda: None
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    api_image_utils = ModuleType("api.utils.image_utils")
    api_image_utils.store_chunk_image = lambda *a, **k: None
    monkeypatch.setitem(sys.modules, "api.utils.image_utils", api_image_utils)

    # Service stubs — tests patch specific methods via monkeypatch.setattr(raising=False)
    # because the stub classes created with type() have no pre-existing attributes.
    for svc, attrs in (
        ("api.db.services.document_service",     ("DocumentService",)),
        ("api.db.services.knowledgebase_service", ("KnowledgebaseService",)),
        ("api.db.services.doc_metadata_service",  ("DocMetadataService",)),
        ("api.db.services.file2document_service", ("File2DocumentService",)),
        ("api.db.services.file_service",          ("FileService",)),
        ("api.db.services.task_service",          ("TaskService",
                                                   "cancel_all_task_of",
                                                   "queue_tasks")),
        ("api.db.services.tenant_llm_service",    ("TenantLLMService",)),
        ("api.db.services.llm_service",           ("LLMBundle",)),
    ):
        mod = ModuleType(svc)
        for attr in attrs:
            setattr(mod, attr, type(attr, (), {}))
        monkeypatch.setitem(sys.modules, svc, mod)

    sys.modules["api.db.services.task_service"].cancel_all_task_of = lambda *a, **k: None
    sys.modules["api.db.services.task_service"].queue_tasks = lambda *a, **k: None

    joint_mod = ModuleType("api.db.joint_services.tenant_model_service")
    joint_mod.get_model_config_by_id = lambda *a, **k: {}
    joint_mod.get_model_config_by_type_and_name = lambda *a, **k: {}
    joint_mod.get_tenant_default_model_by_type = lambda *a, **k: {}
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", joint_mod)

    # Load the real doc.py
    module_path = repo_root / "api" / "apps" / "sdk" / "doc.py"
    spec = importlib.util.spec_from_file_location("_test_doc_sdk", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _run(coro):
    return asyncio.run(coro)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_db_doc(**overrides):
    """Return a raw document dict as returned by DocumentService.get_list."""
    doc = {
        "id": "doc-1",
        "kb_id": "kb-1",
        "name": "report.pdf",
        "chunk_num": 10,
        "token_num": 500,
        "llm_token_num": 0,
        "parser_id": "naive",
        "run": "3",
        "status": True,
        "process_duration": 1.5,
    }
    doc.update(overrides)
    return doc


class _FakeDoc:
    """ORM-like document object whose to_dict() returns _make_db_doc."""

    def __init__(self, **overrides):
        self._data = _make_db_doc(**overrides)
        for k, v in self._data.items():
            setattr(self, k, v)
        self.id = self._data["id"]
        self.kb_id = self._data["kb_id"]
        self.parser_id = self._data["parser_id"]
        self.type = 1
        self.run = self._data["run"]

    def to_dict(self):
        return dict(self._data)


class _AsyncValue:
    """Awaitable that resolves to a fixed value."""
    def __init__(self, value):
        self._v = value

    def __await__(self):
        async def _co():
            return self._v
        return _co().__await__()


def _minimal_request(monkeypatch, module):
    """Patch module.request with an object whose args.get/getlist() returns None/[]."""
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(args=SimpleNamespace(
            get=lambda key, default=None: default,
            getlist=lambda key, default=None: (default if default is not None else []),
            to_dict=lambda: {},
        )),
    )


# ---------------------------------------------------------------------------
# Tests — list_docs handler (~line 624 in doc.py)
# ---------------------------------------------------------------------------

@pytest.mark.p2
class TestListDocsLlmTokenMapping:

    def test_llm_token_num_renamed_to_llm_token_count(self, monkeypatch):
        """list_docs must expose llm_token_num as llm_token_count in output."""
        module = _load_doc_module(monkeypatch)

        monkeypatch.setattr(module.KnowledgebaseService, "accessible",
                            lambda **_k: True, raising=False)
        monkeypatch.setattr(module.DocumentService, "get_list",
                            lambda *_a, **_k: ([_make_db_doc(llm_token_num=150)], 1),
                            raising=False)
        _minimal_request(monkeypatch, module)

        res = module.list_docs.__wrapped__("ds-1", "tenant-1")

        assert res["code"] == 0
        docs = res["data"]["docs"]
        assert len(docs) == 1
        assert "llm_token_count" in docs[0], "llm_token_num must be renamed to llm_token_count"
        assert docs[0]["llm_token_count"] == 150
        assert "llm_token_num" not in docs[0], "original key must not leak through"

    def test_llm_token_count_zero_forwarded(self, monkeypatch):
        """llm_token_num=0 (no LLM parsing) must appear as llm_token_count=0."""
        module = _load_doc_module(monkeypatch)

        monkeypatch.setattr(module.KnowledgebaseService, "accessible",
                            lambda **_k: True, raising=False)
        monkeypatch.setattr(module.DocumentService, "get_list",
                            lambda *_a, **_k: ([_make_db_doc(llm_token_num=0)], 1),
                            raising=False)
        _minimal_request(monkeypatch, module)

        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        assert res["data"]["docs"][0]["llm_token_count"] == 0

    def test_other_key_renames_unaffected(self, monkeypatch):
        """Adding the llm_token_num rename must not break any existing renames."""
        module = _load_doc_module(monkeypatch)

        monkeypatch.setattr(module.KnowledgebaseService, "accessible",
                            lambda **_k: True, raising=False)
        monkeypatch.setattr(module.DocumentService, "get_list",
                            lambda *_a, **_k: ([_make_db_doc(llm_token_num=99)], 1),
                            raising=False)
        _minimal_request(monkeypatch, module)

        res = module.list_docs.__wrapped__("ds-1", "tenant-1")
        doc = res["data"]["docs"][0]
        assert "chunk_count" in doc     # chunk_num  → chunk_count
        assert "dataset_id" in doc      # kb_id      → dataset_id
        assert "token_count" in doc     # token_num  → token_count
        assert "chunk_method" in doc    # parser_id  → chunk_method
        assert "llm_token_count" in doc # llm_token_num → llm_token_count


# ---------------------------------------------------------------------------
# Tests — update_doc handler (~line 347 in doc.py)
# ---------------------------------------------------------------------------

@pytest.mark.p2
class TestUpdateDocLlmTokenMapping:

    def test_llm_token_num_renamed_in_update_response(self, monkeypatch):
        """update_doc must expose llm_token_num as llm_token_count in its response."""
        module = _load_doc_module(monkeypatch)
        doc = _FakeDoc(llm_token_num=75)

        monkeypatch.setattr(module.KnowledgebaseService, "accessible",
                            lambda **_k: True, raising=False)
        monkeypatch.setattr(module.KnowledgebaseService, "query",
                            lambda **_k: [SimpleNamespace()], raising=False)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id",
                            lambda _id: (True, SimpleNamespace()), raising=False)
        monkeypatch.setattr(module.DocumentService, "query",
                            lambda **_k: [doc], raising=False)
        monkeypatch.setattr(module.DocumentService, "update_by_id",
                            lambda *_a, **_k: True, raising=False)
        monkeypatch.setattr(module.DocumentService, "get_by_id",
                            lambda _id: (True, doc), raising=False)
        monkeypatch.setattr(module.DocMetadataService, "update_document_metadata",
                            lambda *_a, **_k: True, raising=False)
        monkeypatch.setattr(module, "get_request_json",
                            lambda: _AsyncValue({"name": "report.pdf"}))
        module.settings.docStoreConn = SimpleNamespace(
            update=lambda *a, **k: None,
            get=lambda *a, **k: {},
            index_name=lambda *a: "idx",
        )

        res = _run(module.update_doc.__wrapped__("tenant-1", "ds-1", "doc-1"))

        assert res["code"] == 0
        data = res["data"]
        assert "llm_token_count" in data, \
            "update_doc must rename llm_token_num → llm_token_count"
        assert data["llm_token_count"] == 75
        assert "llm_token_num" not in data
