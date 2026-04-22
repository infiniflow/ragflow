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

import asyncio
import inspect
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _Vec(list):
    def __mul__(self, scalar):
        return _Vec([scalar * x for x in self])

    __rmul__ = __mul__

    def __add__(self, other):
        return _Vec([a + b for a, b in zip(self, other)])

    def tolist(self):
        return list(self)


class _DummyDoc:
    def __init__(self, *, doc_id="doc-1", kb_id="kb-1", name="Doc", parser_id="naive"):
        self.id = doc_id
        self.kb_id = kb_id
        self.name = name
        self.parser_id = parser_id

    def to_dict(self):
        return {"id": self.id, "kb_id": self.kb_id, "name": self.name}


class _DummyRetCode:
    SUCCESS = 0
    DATA_ERROR = 102
    EXCEPTION_ERROR = 100
    OPERATING_ERROR = 103
    NOT_FOUND = 404


class _DummyParserType:
    QA = "qa"
    NAIVE = "naive"


class _DummyRetriever:
    async def search(self, query, _index_name, _kb_ids, *args, highlight=None, **kwargs):
        class _SRes:
            total = 1
            ids = ["chunk-1"]
            field = {
                "chunk-1": {
                    "content_with_weight": "chunk content",
                    "doc_id": "doc-1",
                    "docnm_kwd": "Doc",
                    "important_kwd": ["k1"],
                    "question_kwd": ["q1"],
                    "img_id": "img-1",
                    "available_int": 1,
                    "position_int": [],
                    "doc_type_kwd": "text",
                }
            }
            highlight = {"chunk-1": " highlighted  content "}

        _ = (query, highlight)
        return _SRes()


class _DummyDocStore:
    def __init__(self):
        self.updated = []
        self.inserted = []
        self.deleted_inputs = []
        self.to_delete = [1]
        self.chunk = {
            "id": "chunk-1",
            "doc_id": "doc-1",
            "kb_id": "kb-1",
            "content_with_weight": "chunk content",
            "docnm_kwd": "Doc",
            "q_2_vec": [0.1, 0.2],
            "content_tks": ["a"],
            "content_ltks": ["b"],
            "content_sm_ltks": ["c"],
        }

    def get(self, *_args, **_kwargs):
        return dict(self.chunk) if self.chunk is not None else None

    def update(self, condition, payload, *_args, **_kwargs):
        self.updated.append((condition, payload))
        return True

    def delete(self, condition, *_args, **_kwargs):
        self.deleted_inputs.append(condition)
        if not self.to_delete:
            return 0
        return self.to_delete.pop(0)

    def insert(self, docs, *_args, **_kwargs):
        self.inserted.extend(docs)

    def index_exist(self, *_args, **_kwargs):
        return True


class _DummyStorage:
    def __init__(self):
        self.put_calls = []
        self.rm_calls = []

    def put(self, bucket, name, binary):
        self.put_calls.append((bucket, name, binary))

    def obj_exist(self, _bucket, _name):
        return True

    def rm(self, bucket, name):
        self.rm_calls.append((bucket, name))


class _DummyTenant:
    def __init__(self, tenant_id="tenant-1"):
        self.tenant_id = tenant_id


class _DummyLLMBundle:
    def __init__(self, *_args, **_kwargs):
        pass

    def encode(self, _inputs):
        return [_Vec([1.0, 2.0]), _Vec([3.0, 4.0])], 9


class _DummyXXHash:
    def __init__(self, data):
        self._data = data

    def hexdigest(self):
        return f"chunk-{len(self._data)}"


def _run(coro):
    return asyncio.run(coro)


def _route_core(func):
    return inspect.unwrap(func)


def _load_chunk_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args={}, headers={})
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    xxhash_mod = ModuleType("xxhash")
    xxhash_mod.xxh64 = lambda data: _DummyXXHash(data)
    monkeypatch.setitem(sys.modules, "xxhash", xxhash_mod)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    settings_mod = ModuleType("common.settings")
    settings_mod.retriever = _DummyRetriever()
    settings_mod.docStoreConn = _DummyDocStore()
    settings_mod.STORAGE_IMPL = _DummyStorage()
    monkeypatch.setitem(sys.modules, "common.settings", settings_mod)
    common_pkg.settings = settings_mod

    constants_mod = ModuleType("common.constants")

    class _DummyLLMType:
        EMBEDDING = SimpleNamespace(value="embedding")
        CHAT = SimpleNamespace(value="chat")
        RERANK = SimpleNamespace(value="rerank")
        SPEECH2TEXT = SimpleNamespace(value="speech2text")
        IMAGE2TEXT = SimpleNamespace(value="image2text")
        TTS = SimpleNamespace(value="tts")
        OCR = SimpleNamespace(value="ocr")

    constants_mod.RetCode = _DummyRetCode
    constants_mod.LLMType = _DummyLLMType
    constants_mod.ParserType = _DummyParserType
    constants_mod.PAGERANK_FLD = "pagerank_flt"
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    string_utils_mod = ModuleType("common.string_utils")
    string_utils_mod.remove_redundant_spaces = lambda text: " ".join(str(text).split())
    string_utils_mod.is_content_empty = lambda content: content is None or not str(content).strip()
    monkeypatch.setitem(sys.modules, "common.string_utils", string_utils_mod)

    metadata_utils_mod = ModuleType("common.metadata_utils")
    metadata_utils_mod.apply_meta_data_filter = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "common.metadata_utils", metadata_utils_mod)

    misc_utils_mod = ModuleType("common.misc_utils")

    async def _thread_pool_exec(func):
        return func()

    misc_utils_mod.thread_pool_exec = _thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_app_pkg = ModuleType("rag.app")
    rag_app_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.app", rag_app_pkg)

    rag_qa_mod = ModuleType("rag.app.qa")
    rag_qa_mod.rmPrefix = lambda text: str(text).strip("Q: ").strip("A: ")
    rag_qa_mod.beAdoc = lambda d, q, a, _latin: {**d, "question_kwd": [q], "content_with_weight": f"{q}\n{a}"}
    monkeypatch.setitem(sys.modules, "rag.app.qa", rag_qa_mod)

    rag_tag_mod = ModuleType("rag.app.tag")
    rag_tag_mod.label_question = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "rag.app.tag", rag_tag_mod)

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.rag_tokenizer = SimpleNamespace(
        tokenize=lambda text: [str(text)],
        fine_grained_tokenize=lambda toks: [f"fg:{t}" for t in toks],
        is_chinese=lambda _text: False,
    )
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"idx-{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)

    rag_prompts_pkg = ModuleType("rag.prompts")
    rag_prompts_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "rag.prompts", rag_prompts_pkg)

    rag_generator_mod = ModuleType("rag.prompts.generator")
    rag_generator_mod.cross_languages = lambda *_args, **_kwargs: []
    rag_generator_mod.keyword_extraction = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_generator_mod)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="user-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {"code": code, "message": message, "data": data}
    api_utils_mod.get_data_error_result = lambda message="": {"code": _DummyRetCode.DATA_ERROR, "message": message, "data": False}
    api_utils_mod.get_result = lambda data=None, message="", code=0: {"code": code, "message": message, "data": data}
    api_utils_mod.get_error_data_result = lambda message="": {"code": _DummyRetCode.DATA_ERROR, "message": message, "data": False}
    api_utils_mod.server_error_response = lambda exc: {"code": _DummyRetCode.EXCEPTION_ERROR, "message": repr(exc), "data": False}
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda fn: fn)
    api_utils_mod.add_tenant_id_to_kwargs = lambda func: func
    api_utils_mod.check_duplicate_ids = lambda ids, _kind: (list(dict.fromkeys(ids)), [] if len(ids) == len(set(ids)) else [f"Duplicate {_kind} ids"])
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    image_utils_mod = ModuleType("api.utils.image_utils")
    image_utils_mod.store_chunk_image = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.utils.image_utils", image_utils_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    joint_services_pkg = ModuleType("api.db.joint_services")
    joint_services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.joint_services", joint_services_pkg)

    tenant_model_service_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service_mod.get_model_config_by_id = lambda *_args, **_kwargs: {"llm_name": "embed", "model_type": "embedding"}
    tenant_model_service_mod.get_model_config_by_type_and_name = lambda *_args, **_kwargs: {"llm_name": "embed", "model_type": "embedding"}
    tenant_model_service_mod.get_tenant_default_model_by_type = lambda *_args, **_kwargs: {"llm_name": "chat", "model_type": "chat"}
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    document_service_mod = ModuleType("api.db.services.document_service")

    class _DocumentService:
        decrement_calls = []
        increment_calls = []

        @staticmethod
        def get_tenant_id(_doc_id):
            return "tenant-1"

        @staticmethod
        def get_by_id(doc_id):
            return True, _DummyDoc(doc_id=doc_id, parser_id=_DummyParserType.NAIVE)

        @staticmethod
        def query(**kwargs):
            return [_DummyDoc(doc_id=kwargs.get("id", "doc-1"), kb_id=kwargs.get("kb_id", "kb-1"))]

        @staticmethod
        def get_by_ids(ids):
            return [_DummyDoc(doc_id=ids[0] if ids else "doc-1")]

        @staticmethod
        def delete_chunk_images(*_args, **_kwargs):
            return None

        @staticmethod
        def get_embd_id(_doc_id):
            return "embed-1"

        @staticmethod
        def get_tenant_embd_id(_doc_id):
            return 1

        @staticmethod
        def decrement_chunk_num(*args):
            _DocumentService.decrement_calls.append(args)

        @staticmethod
        def increment_chunk_num(*args):
            _DocumentService.increment_calls.append(args)

    document_service_mod.DocumentService = _DocumentService
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)
    services_pkg.document_service = document_service_mod

    doc_metadata_service_mod = ModuleType("api.db.services.doc_metadata_service")
    doc_metadata_service_mod.DocMetadataService = type("DocMetadataService", (), {})
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", doc_metadata_service_mod)
    services_pkg.doc_metadata_service = doc_metadata_service_mod

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _KnowledgebaseService:
        @staticmethod
        def get_kb_ids(_tenant_id):
            return ["kb-1"]

        @staticmethod
        def accessible(**_kwargs):
            return True

        @staticmethod
        def get_by_id(_kb_id):
            return True, SimpleNamespace(pagerank=0.6, tenant_embd_id=2, tenant_llm_id=1)

    kb_service_mod.KnowledgebaseService = _KnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)
    services_pkg.knowledgebase_service = kb_service_mod

    class _DummyLLMService:
        @staticmethod
        def query(**_kwargs):
            return [SimpleNamespace(
                llm_name="gpt-3.5-turbo",
                model_type="chat",
                max_tokens=8192,
                is_tools=True
            )]

    llm_service_mod = ModuleType("api.db.services.llm_service")
    llm_service_mod.LLMService = _DummyLLMService
    llm_service_mod.LLMBundle = _DummyLLMBundle
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)
    services_pkg.llm_service = llm_service_mod

    search_service_mod = ModuleType("api.db.services.search_service")
    search_service_mod.SearchService = type("SearchService", (), {})
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", search_service_mod)
    services_pkg.search_service = search_service_mod

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _MockTableObject:
        def __init__(self, **kwargs):
            for key, value in kwargs.items():
                setattr(self, key, value)

        def to_dict(self):
            return {k: v for k, v in self.__dict__.items()}

    class _TenantLLMService:
        @staticmethod
        def get_by_id(tenant_model_id):
            return True, _MockTableObject(
                id=tenant_model_id,
                tenant_id="tenant-1",
                llm_factory="",
                model_type="chat",
                llm_name="gpt-3.5-turbo",
                api_key="fake-api-key",
                api_base="https://api.example.com",
                max_tokens=8192,
                used_tokens=0,
                status=1
            )

        @staticmethod
        def get_api_key(tenant_id, model_name):
            return _MockTableObject(
                id=1,
                tenant_id=tenant_id,
                llm_factory="",
                model_type="chat",
                llm_name=model_name,
                api_key="fake-api-key",
                api_base="https://api.example.com",
                max_tokens=8192,
                used_tokens=0,
                status=1
            )

        @staticmethod
        def split_model_name_and_factory(model_name):
            if "@" in model_name:
                parts = model_name.rsplit("@", 1)
                return parts[0], parts[1]
            return model_name, None

        @staticmethod
        def increase_usage_by_id(model_id, used_tokens):
            return True

        @staticmethod
        def model_instance(_model_config):
            return _DummyLLMBundle()

    class _TenantService:
        @staticmethod
        def get_by_id(tenant_id):
            return True, SimpleNamespace(
                llm_id="gpt-3.5-turbo",
                tenant_llm_id=1,
                embd_id="text-embedding-ada-002",
                tenant_embd_id=2,
                asr_id="whisper-1",
                img2txt_id="gpt-4-vision-preview",
                rerank_id="bge-reranker",
                tts_id="tts-1"
            )

    tenant_llm_service_mod.TenantLLMService = _TenantLLMService
    tenant_llm_service_mod.TenantService = _TenantService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)
    services_pkg.tenant_llm_service = tenant_llm_service_mod

    user_service_mod = ModuleType("api.db.services.user_service")

    class _UserTenantService:
        @staticmethod
        def query(**_kwargs):
            return [_DummyTenant("tenant-1")]

    user_service_mod.UserTenantService = _UserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)
    services_pkg.user_service = user_service_mod

    module_name = "test_chunk_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "chunk_app.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _load_chunk_api_module(monkeypatch):
    _load_chunk_module(monkeypatch)
    repo_root = Path(__file__).resolve().parents[4]
    module_name = "test_chunk_api_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chunk_api.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(payload))


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.mark.p2
def test_restful_chunk_list_get_and_delete_unit(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={"keywords": "chunk", "available": "true"}, headers={})

    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert res["data"]["total"] == 1, res
    assert res["data"]["chunks"][0]["id"] == "chunk-1", res
    assert res["data"]["chunks"][0]["available"] is True, res

    res = _run(_route_core(module.get_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == 0, res
    assert "q_2_vec" not in res["data"], res
    assert "content_tks" not in res["data"], res
    assert "content_ltks" not in res["data"], res
    assert "content_sm_ltks" not in res["data"], res

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": ["chunk-1"]}))
    res = _run(_route_core(module.rm_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert module.settings.docStoreConn.deleted_inputs[-1]["doc_id"] == "doc-1"


@pytest.mark.p2
def test_restful_chunk_add_update_and_switch_unit(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "content": "chunk",
                "important_keywords": ["i1"],
                "questions": ["q1"],
                "tag_kwd": ["tag"],
                "tag_feas": {"tag": 0.2},
            }
        ),
    )
    res = _run(_route_core(module.add_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert res["data"]["chunk"]["content"] == "chunk", res
    assert module.settings.docStoreConn.inserted, "insert should be called"
    assert module.DocumentService.increment_calls, "increment_chunk_num should be called"

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "content": "updated chunk",
                "important_keywords": ["i2"],
                "questions": ["q2"],
                "tag_kwd": ["tag2"],
                "positions": [[1, 2, 3, 4, 5]],
                "available": False,
            }
        ),
    )
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == 0, res
    updated = module.settings.docStoreConn.updated[-1][1]
    assert updated["content_with_weight"] == "updated chunk"
    assert updated["available_int"] == 0
    assert updated["position_int"] == [[1, 2, 3, 4, 5]]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": ["chunk-1"], "available": True}))
    res = _run(_route_core(module.switch_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert res["data"] is True, res


@pytest.mark.p2
def test_restful_chunk_guard_branches_unit(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["message"] == "You don't own the dataset kb-1.", res

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
    monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["message"] == "You don't own the document doc-1.", res

    monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
    module.request = SimpleNamespace(args={"id": "chunk-1"}, headers={})
    module.settings.docStoreConn.chunk = None
    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Chunk not found" in res["message"], res

    module.settings.docStoreConn.chunk = {
        "id": "chunk-1",
        "doc_id": "other-doc",
        "content_with_weight": "chunk",
        "docnm_kwd": "Doc",
    }
    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Chunk not found" in res["message"], res

    module.settings.docStoreConn.chunk = None
    module.request = SimpleNamespace(args={}, headers={})
    res = _run(_route_core(module.get_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Chunk not found" in res["message"], res

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"content": ""}))
    res = _run(_route_core(module.add_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["message"] == "`content` is required", res

    module.settings.docStoreConn.chunk = {"id": "chunk-1", "doc_id": "doc-1", "content_with_weight": "chunk"}
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"important_keywords": "bad"}))
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["message"] == "`important_keywords` should be a list", res

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": []}))
    res = _run(_route_core(module.switch_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["message"] == "`chunk_ids` is required.", res

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": ["chunk-1"]}))
    res = _run(_route_core(module.switch_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["message"] == "`available_int` or `available` is required.", res


@pytest.mark.p2
def test_retrieval_test_branch_matrix_unit(monkeypatch):
    module = _load_chunk_module(monkeypatch)
    module.request = SimpleNamespace(headers={"X-Request-ID": "req-r"}, args={})

    applied_filters = []
    llm_calls = []
    cross_calls = []
    keyword_calls = []

    async def _apply_filter(meta_data_filter, metas, question, chat_mdl, local_doc_ids):
        applied_filters.append(
            {
                "meta_data_filter": meta_data_filter,
                "metas": metas,
                "question": question,
                "chat_mdl": chat_mdl,
                "local_doc_ids": list(local_doc_ids),
            }
        )
        return ["doc-filtered"]

    async def _cross_languages(_tenant_id, _dialog, question, langs):
        cross_calls.append((question, tuple(langs)))
        return f"{question}-xl"

    async def _keyword_extraction(_chat_mdl, question):
        keyword_calls.append(question)
        return "-kw"

    class _Retriever:
        def __init__(self, mode="ok"):
            self.mode = mode
            self.retrieval_questions = []

        async def retrieval(self, question, *_args, **_kwargs):
            if self.mode == "not_found":
                raise Exception("boom not_found boom")
            if self.mode == "explode":
                raise RuntimeError("retrieval boom")
            self.retrieval_questions.append(question)
            return {"chunks": [{"id": "c1", "vector": [0.1], "content_with_weight": "chunk-content"}]}

        def retrieval_by_children(self, chunks, _tenant_ids):
            return list(chunks)

    class _KgRetriever:
        async def retrieval(self, *_args, **_kwargs):
            return {"id": "kg-1", "content_with_weight": "kg-content"}

    class _NoContentKgRetriever:
        async def retrieval(self, *_args, **_kwargs):
            return {"id": "kg-2", "content_with_weight": ""}

    monkeypatch.setattr(module, "LLMBundle", lambda *args, **kwargs: llm_calls.append((args, kwargs)) or SimpleNamespace())
    monkeypatch.setattr(module, "get_model_config_by_type_and_name", lambda *_args, **_kwargs: {"llm_name": "stub-model", "model_type": "chat"})
    monkeypatch.setattr(module, "get_tenant_default_model_by_type", lambda *_args, **_kwargs: {"llm_name": "stub-model", "model_type": "chat"})
    monkeypatch.setattr(module, "get_model_config_by_id", lambda *_args, **_kwargs: {"llm_name": "stub-model", "model_type": "embedding"})
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: [{"meta": "v"}], raising=False)
    monkeypatch.setattr(module, "apply_meta_data_filter", _apply_filter)
    monkeypatch.setattr(module.SearchService, "get_detail", lambda _sid: {"search_config": {"meta_data_filter": {"method": "auto"}, "chat_id": "chat-1"}}, raising=False)
    monkeypatch.setattr(module, "cross_languages", _cross_languages)
    monkeypatch.setattr(module, "keyword_extraction", _keyword_extraction)
    monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: ["lbl"])
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [_DummyTenant("tenant-1")])

    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: False, raising=False)
    _set_request_json(monkeypatch, module, {"kb_id": "kb-1", "question": "q", "search_id": "search-1"})
    res = _run(module.retrieval_test())
    assert res["code"] == module.RetCode.OPERATING_ERROR, res
    assert "Only owner of dataset authorized for this operation." in res["message"], res
    assert applied_filters and applied_filters[-1]["meta_data_filter"]["method"] == "auto"
    assert llm_calls, "search_id metadata auto branch should instantiate chat model"

    _set_request_json(monkeypatch, module, {"kb_id": [], "question": "q"})
    res = _run(module.retrieval_test())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Please specify dataset firstly." in res["message"], res

    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: True, raising=False)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None), raising=False)
    _set_request_json(
        monkeypatch,
        module,
        {"kb_id": ["kb-1"], "question": "q", "meta_data_filter": {"method": "semi_auto"}},
    )
    res = _run(module.retrieval_test())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Knowledgebase not found!" in res["message"], res

    retriever = _Retriever(mode="ok")
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, SimpleNamespace(tenant_id="tenant-kb", embd_id="embd-1", tenant_embd_id=2)), raising=False)
    monkeypatch.setattr(module.settings, "retriever", retriever)
    monkeypatch.setattr(module.settings, "kg_retriever", _KgRetriever(), raising=False)
    _set_request_json(
        monkeypatch,
        module,
        {
            "kb_id": ["kb-1"],
            "question": "q",
            "cross_languages": ["fr"],
            "rerank_id": "rerank-1",
            "keyword": True,
            "use_kg": True,
        },
    )
    res = _run(module.retrieval_test())
    assert res["code"] == 0, res
    assert cross_calls[-1] == ("q", ("fr",))
    assert keyword_calls[-1] == "q-xl"
    assert retriever.retrieval_questions[-1] == "q-xl-kw"
    assert res["data"]["chunks"][0]["id"] == "kg-1", res
    assert all("vector" not in chunk for chunk in res["data"]["chunks"])

    monkeypatch.setattr(module.settings, "kg_retriever", _NoContentKgRetriever(), raising=False)
    _set_request_json(monkeypatch, module, {"kb_id": ["kb-1"], "question": "q", "use_kg": True})
    res = _run(module.retrieval_test())
    assert res["code"] == 0, res
    assert res["data"]["chunks"][0]["id"] == "c1", res

    monkeypatch.setattr(module.settings, "retriever", _Retriever(mode="not_found"))
    _set_request_json(monkeypatch, module, {"kb_id": ["kb-1"], "question": "q"})
    res = _run(module.retrieval_test())
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "No chunk found! Check the chunk status please!" in res["message"], res

    monkeypatch.setattr(module.settings, "retriever", _Retriever(mode="explode"))
    _set_request_json(monkeypatch, module, {"kb_id": ["kb-1"], "question": "q"})
    res = _run(module.retrieval_test())
    assert res["code"] == module.RetCode.EXCEPTION_ERROR, res
    assert "retrieval boom" in res["message"], res


@pytest.mark.p2
def test_knowledge_graph_repeat_deal_matrix_unit(monkeypatch):
    module = _load_chunk_module(monkeypatch)
    module.request = SimpleNamespace(args={"doc_id": "doc-1"}, headers={})

    payload = {
        "id": "root",
        "children": [
            {"id": "dup"},
            {"id": "dup", "children": [{"id": "dup"}]},
        ],
    }

    class _SRes:
        ids = ["bad-json", "mind-map"]
        field = {
            "bad-json": {"knowledge_graph_kwd": "graph", "content_with_weight": "{bad json"},
            "mind-map": {"knowledge_graph_kwd": "mind_map", "content_with_weight": json.dumps(payload)},
        }

    async def _search(*_args, **_kwargs):
        return _SRes()

    monkeypatch.setattr(module.settings.retriever, "search", _search)
    res = _run(module.knowledge_graph())
    assert res["code"] == 0, res
    assert res["data"]["graph"] == {}, res
    mind_map = res["data"]["mind_map"]
    assert mind_map["children"][0]["id"] == "dup", res
    assert mind_map["children"][1]["id"] == "dup(1)", res
    assert mind_map["children"][1]["children"][0]["id"] == "dup(2)", res
