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
"""Unit tests for DocumentService.increment_chunk_num – llm_token_num handling.

Loads the real document_service module (heavy deps stubbed) and calls the
actual increment_chunk_num method via a spy on Document.update, so that any
change to the production conditional will be caught.

The fix changed `if llm_token_num:` to `if llm_token_num > 0:` so that:
- llm_token_num=0  → llm_token_num field is NOT added to the UPDATE
- llm_token_num>0  → llm_token_num field IS added to the UPDATE
- llm_token_num<0  → llm_token_num field is NOT added to the UPDATE (new)
"""

import functools
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


# ---------------------------------------------------------------------------
# Module loader
# ---------------------------------------------------------------------------

def _load_document_service(monkeypatch):
    """Load api/db/services/document_service.py with all heavy deps stubbed."""
    repo_root = Path(__file__).resolve().parents[5]

    # -- stdlib-like stubs that aren't available outside the ragflow venv --

    xxhash_mod = ModuleType("xxhash")
    xxhash_mod.xxh64 = lambda *a, **k: SimpleNamespace(hexdigest=lambda: "")
    monkeypatch.setitem(sys.modules, "xxhash", xxhash_mod)

    peewee_mod = ModuleType("peewee")
    peewee_mod.fn = SimpleNamespace()
    peewee_mod.Case = type("Case", (), {})
    peewee_mod.JOIN = SimpleNamespace(LEFT_OUTER=1)
    monkeypatch.setitem(sys.modules, "peewee", peewee_mod)

    # -- rag stubs --
    for pkg in ("rag", "rag.nlp", "rag.utils", "rag.utils.redis_conn"):
        mod = ModuleType(pkg)
        mod.__path__ = []
        monkeypatch.setitem(sys.modules, pkg, mod)
    sys.modules["rag.nlp"].rag_tokenizer = SimpleNamespace()
    sys.modules["rag.nlp"].search = SimpleNamespace()
    sys.modules["rag.utils.redis_conn"].REDIS_CONN = SimpleNamespace()

    # -- api package stubs --
    for pkg in ("api", "api.db", "api.db.services", "api.db.joint_services"):
        mod = ModuleType(pkg)
        mod.__path__ = []
        monkeypatch.setitem(sys.modules, pkg, mod)

    api_constants = ModuleType("api.constants")
    api_constants.IMG_BASE64_PREFIX = "data:image"
    api_constants.FILE_NAME_LEN_LIMIT = 255
    monkeypatch.setitem(sys.modules, "api.constants", api_constants)

    api_db_mod = sys.modules["api.db"]
    api_db_mod.PIPELINE_SPECIAL_PROGRESS_FREEZE_TASK_TYPES = []
    api_db_mod.FileType = SimpleNamespace()
    api_db_mod.UserTenantRole = SimpleNamespace()
    api_db_mod.CanvasCategory = SimpleNamespace()

    # DB: connection_context must be a pass-through so we can call the
    # classmethod directly without a real database connection.
    class _FakeDB:
        @staticmethod
        def connection_context():
            def decorator(fn):
                @functools.wraps(fn)
                def wrapper(*args, **kwargs):
                    return fn(*args, **kwargs)
                return wrapper
            return decorator

    # Knowledgebase: increment_chunk_num updates KB counts as a side-effect;
    # stub it so it doesn't explode.
    class _FakeKBChain:
        @staticmethod
        def where(*_a, **_k):
            return _FakeKBChain()
        @staticmethod
        def execute():
            return 1

    class _FakeKnowledgebase:
        class _F:
            def __add__(self, o): return self
            def __sub__(self, o): return self
        token_num = _F()
        chunk_num = _F()
        id = _F()

        @classmethod
        def update(cls, **_kwargs):
            return _FakeKBChain()

    api_db_models = ModuleType("api.db.db_models")
    api_db_models.DB = _FakeDB()
    # Document is intentionally set to None here; tests patch it via monkeypatch
    api_db_models.Document = None
    api_db_models.Knowledgebase = _FakeKnowledgebase
    for name in ("Task", "Tenant", "UserTenant", "File2Document", "File", "UserCanvas", "User"):
        setattr(api_db_models, name, type(name, (), {}))
    monkeypatch.setitem(sys.modules, "api.db.db_models", api_db_models)

    api_db_utils = ModuleType("api.db.db_utils")
    api_db_utils.bulk_insert_into_db = lambda *a, **k: None
    monkeypatch.setitem(sys.modules, "api.db.db_utils", api_db_utils)

    common_service_mod = ModuleType("api.db.services.common_service")
    common_service_mod.CommonService = type("CommonService", (), {})
    # retry_deadlock_operation is used as @retry_deadlock_operation() on other
    # methods; it must be a callable that returns a pass-through decorator.
    common_service_mod.retry_deadlock_operation = lambda: (lambda fn: fn)
    monkeypatch.setitem(sys.modules, "api.db.services.common_service", common_service_mod)

    kb_svc_mod = ModuleType("api.db.services.knowledgebase_service")
    kb_svc_mod.KnowledgebaseService = type("KnowledgebaseService", (), {})
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_svc_mod)

    doc_meta_mod = ModuleType("api.db.services.doc_metadata_service")
    doc_meta_mod.DocMetadataService = type("DocMetadataService", (), {})
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", doc_meta_mod)

    # -- common stubs --
    for pkg in ("common", "common.doc_store", "common.doc_store.doc_store_base"):
        if pkg not in sys.modules:
            mod = ModuleType(pkg)
            mod.__path__ = []
            monkeypatch.setitem(sys.modules, pkg, mod)

    common_misc = ModuleType("common.misc_utils")
    common_misc.get_uuid = lambda: "test-uuid"
    monkeypatch.setitem(sys.modules, "common.misc_utils", common_misc)

    common_time = ModuleType("common.time_utils")
    common_time.current_timestamp = lambda: 0
    common_time.get_format_time = lambda *a: ""
    monkeypatch.setitem(sys.modules, "common.time_utils", common_time)

    common_constants = ModuleType("common.constants")
    common_constants.LLMType = SimpleNamespace()
    common_constants.ParserType = SimpleNamespace()
    common_constants.StatusEnum = SimpleNamespace()
    common_constants.TaskStatus = SimpleNamespace()
    common_constants.SVR_CONSUMER_GROUP_NAME = "group"
    monkeypatch.setitem(sys.modules, "common.constants", common_constants)

    sys.modules["common.doc_store.doc_store_base"].OrderByExpr = type("OrderByExpr", (), {})

    common_settings = ModuleType("common.settings")
    monkeypatch.setitem(sys.modules, "common.settings", common_settings)

    # Load the actual module
    module_path = repo_root / "api" / "db" / "services" / "document_service.py"
    spec = importlib.util.spec_from_file_location("_test_document_service", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# ---------------------------------------------------------------------------
# Spy model factory
# ---------------------------------------------------------------------------

class _UpdateSpy:
    """Captures keyword arguments passed to Document.update(**update_fields)."""

    def __init__(self):
        self.last_kwargs: dict = {}

    def make_model(self):
        spy = self

        class _Expr:
            """Stand-in for a peewee field expression (supports arithmetic)."""
            def __add__(self, o): return self
            def __sub__(self, o): return self
            def __eq__(self, o): return True  # needed for .where(model.id == doc_id)
            def __hash__(self): return id(self)

        class _QueryChain:
            @staticmethod
            def where(*_a, **_k): return _QueryChain()
            @staticmethod
            def execute(): return 1

        class _FakeDocument:
            token_num = _Expr()
            chunk_num = _Expr()
            process_duration = _Expr()
            llm_token_num = _Expr()
            id = _Expr()

            @classmethod
            def update(cls, **kwargs):
                spy.last_kwargs = dict(kwargs)
                return _QueryChain()

        return _FakeDocument


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_llm_token_num_positive_passed_to_update(monkeypatch):
    """When llm_token_num > 0 the UPDATE must include the llm_token_num field."""
    mod = _load_document_service(monkeypatch)
    spy = _UpdateSpy()
    monkeypatch.setattr(mod.DocumentService, "model", spy.make_model())

    mod.DocumentService.increment_chunk_num("doc-1", "kb-1", 100, 5, 1.0, llm_token_num=200)

    assert "llm_token_num" in spy.last_kwargs


@pytest.mark.p2
def test_llm_token_num_zero_not_passed_to_update(monkeypatch):
    """When llm_token_num == 0 the UPDATE must NOT include the llm_token_num field."""
    mod = _load_document_service(monkeypatch)
    spy = _UpdateSpy()
    monkeypatch.setattr(mod.DocumentService, "model", spy.make_model())

    mod.DocumentService.increment_chunk_num("doc-1", "kb-1", 100, 5, 1.0, llm_token_num=0)

    assert "llm_token_num" not in spy.last_kwargs


@pytest.mark.p2
def test_llm_token_num_default_not_passed_to_update(monkeypatch):
    """When llm_token_num is omitted (default 0) the UPDATE must not include it."""
    mod = _load_document_service(monkeypatch)
    spy = _UpdateSpy()
    monkeypatch.setattr(mod.DocumentService, "model", spy.make_model())

    mod.DocumentService.increment_chunk_num("doc-1", "kb-1", 100, 5, 1.0)

    assert "llm_token_num" not in spy.last_kwargs


@pytest.mark.p2
def test_llm_token_num_negative_not_passed_to_update(monkeypatch):
    """Negative llm_token_num must be rejected by the > 0 guard, not written to DB."""
    mod = _load_document_service(monkeypatch)
    spy = _UpdateSpy()
    monkeypatch.setattr(mod.DocumentService, "model", spy.make_model())

    mod.DocumentService.increment_chunk_num("doc-1", "kb-1", 100, 5, 1.0, llm_token_num=-1)

    assert "llm_token_num" not in spy.last_kwargs


@pytest.mark.p2
def test_standard_fields_always_passed_to_update(monkeypatch):
    """token_num, chunk_num, process_duration are always present in the UPDATE."""
    mod = _load_document_service(monkeypatch)

    for llm_tokens in (0, 50, -1):
        spy = _UpdateSpy()
        monkeypatch.setattr(mod.DocumentService, "model", spy.make_model())
        mod.DocumentService.increment_chunk_num("doc-1", "kb-1", 10, 2, 0.5, llm_token_num=llm_tokens)
        assert "token_num" in spy.last_kwargs
        assert "chunk_num" in spy.last_kwargs
        assert "process_duration" in spy.last_kwargs
