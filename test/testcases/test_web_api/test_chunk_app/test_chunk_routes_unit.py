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
import contextlib
import inspect
import importlib.util
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
        ASR = SimpleNamespace(value="asr")
        VISION = SimpleNamespace(value="vision")
        TTS = SimpleNamespace(value="tts")
        OCR = SimpleNamespace(value="ocr")

    constants_mod.RetCode = _DummyRetCode
    constants_mod.LLMType = _DummyLLMType
    constants_mod.ParserType = _DummyParserType
    constants_mod.PAGERANK_FLD = "pagerank_flt"
    constants_mod.TaskStatus = SimpleNamespace(
        UNSTART=SimpleNamespace(value="0"),
        RUNNING=SimpleNamespace(value="1"),
        CANCEL=SimpleNamespace(value="2"),
        DONE=SimpleNamespace(value="3"),
        FAIL=SimpleNamespace(value="4"),
        SCHEDULE=SimpleNamespace(value="5"),
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    string_utils_mod = ModuleType("common.string_utils")
    string_utils_mod.remove_redundant_spaces = lambda text: " ".join(str(text).split())
    string_utils_mod.is_content_empty = lambda content: content is None or not str(content).strip()
    monkeypatch.setitem(sys.modules, "common.string_utils", string_utils_mod)

    metadata_utils_mod = ModuleType("common.metadata_utils")
    metadata_utils_mod.apply_meta_data_filter = lambda *_args, **_kwargs: {}
    metadata_utils_mod.convert_conditions = lambda *_args, **_kwargs: {}
    metadata_utils_mod.meta_filter = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "common.metadata_utils", metadata_utils_mod)

    doc_store_base_mod = ModuleType("common.doc_store.doc_store_base")
    doc_store_base_mod.OrderByExpr = type("OrderByExpr", (), {})
    monkeypatch.setitem(sys.modules, "common.doc_store", ModuleType("common.doc_store"))
    monkeypatch.setitem(sys.modules, "common.doc_store.doc_store_base", doc_store_base_mod)

    tag_feature_utils_mod = ModuleType("common.tag_feature_utils")
    tag_feature_utils_mod.validate_tag_features = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "common.tag_feature_utils", tag_feature_utils_mod)

    pagination_utils_mod = ModuleType("api.utils.pagination_utils")
    pagination_utils_mod.validate_rest_api_page_size = lambda *_args, **_kwargs: (1, 30)
    monkeypatch.setitem(sys.modules, "api.utils.pagination_utils", pagination_utils_mod)

    reference_metadata_utils_mod = ModuleType("api.utils.reference_metadata_utils")
    reference_metadata_utils_mod.enrich_chunks_with_document_metadata = lambda chunks, *_args, **_kwargs: chunks
    reference_metadata_utils_mod.resolve_reference_metadata_preferences = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "api.utils.reference_metadata_utils", reference_metadata_utils_mod)

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
    api_utils_mod.validate_request = lambda *_args, **_kwargs: lambda fn: fn
    api_utils_mod.add_tenant_id_to_kwargs = lambda func: func
    api_utils_mod.check_duplicate_ids = lambda ids, _kind: (list(dict.fromkeys(ids)), [] if len(ids) == len(set(ids)) else [f"Duplicate {_kind} ids"])
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    api_utils_mod.construct_json_result = lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    image_utils_mod = ModuleType("api.utils.image_utils")
    image_utils_mod.store_chunk_image = lambda *_args, **_kwargs: None
    image_utils_mod.remove_chunk_image = lambda *_args, **_kwargs: None
    image_utils_mod.IMAGE_UPDATE_MODE_REMOVE = "remove"
    image_utils_mod.IMAGE_UPDATE_MODES = frozenset({"append", "replace", "remove"})
    monkeypatch.setitem(sys.modules, "api.utils.image_utils", image_utils_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    joint_services_pkg = ModuleType("api.db.joint_services")
    joint_services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.joint_services", joint_services_pkg)

    tenant_model_service_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service_mod.get_model_config_by_id = lambda *_args, **_kwargs: {"llm_name": "embed", "model_type": "embedding"}
    tenant_model_service_mod.get_model_config_from_provider_instance = lambda *_args, **_kwargs: {"llm_name": "embed", "model_type": "embedding"}
    tenant_model_service_mod.resolve_model_config = lambda *_args, **_kwargs: {"llm_name": "embed", "model_type": "embedding"}
    tenant_model_service_mod.get_tenant_default_model_by_type = lambda *_args, **_kwargs: {"llm_name": "chat", "model_type": "chat"}
    tenant_model_service_mod.split_model_name = lambda model_name: (model_name.rsplit("@", 2) + ["", ""])[:3]
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    # chunk_api imports structure_graph_common from api.apps.services; stub it in
    # sys.modules so the real module and its transitive imports are not loaded.
    stub_apps_services = ModuleType("api.apps.services")
    monkeypatch.setitem(sys.modules, "api.apps.services", stub_apps_services)

    sgc_mod = ModuleType("api.apps.services.structure_graph_common")

    async def _sgc_keyword_subgraph(*_args, **_kwargs):
        return {}, [], []

    async def _sgc_build_bucket(*_args, **_kwargs):
        return [], []

    sgc_mod.keyword_subgraph = _sgc_keyword_subgraph
    sgc_mod.build_bucket = _sgc_build_bucket
    monkeypatch.setitem(sys.modules, "api.apps.services.structure_graph_common", sgc_mod)

    # chunk_api imports DB, Document, Task from db_models; stub them so the real
    # module (which pulls in quart_auth) is never loaded.
    class _FakeExpr:
        def __or__(self, other):
            return self

        def __and__(self, other):
            return self

    class _FakeField:
        def __eq__(self, other):
            return _FakeExpr()

        def __ne__(self, other):
            return _FakeExpr()

        def is_null(self, value=True):
            return _FakeExpr()

    class _StubFreshDoc:
        id = "doc-1"
        kb_id = "kb-1"
        token_num = 2
        chunk_num = 1
        process_duration = 0.0

    class _StubDocQuery:
        def where(self, *_args, **_kwargs):
            return self

        def for_update(self):
            return self

        def first(self):
            return _StubDocumentModel.fresh_doc

    class _StubDocumentModel:
        id = _FakeField()
        run = _FakeField()
        fresh_doc = _StubFreshDoc()

        @classmethod
        def select(cls, *_args, **_kwargs):
            return _StubDocQuery()

    class _StubTaskModel:
        doc_id = _FakeField()

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.Document = _StubDocumentModel
    db_models_mod.Task = _StubTaskModel
    db_models_mod.DB = SimpleNamespace(atomic=lambda: contextlib.nullcontext())
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    file2document_service_mod = ModuleType("api.db.services.file2document_service")
    file2document_service_mod.File2DocumentService = SimpleNamespace(get_storage_address=lambda **_kwargs: ("", ""))
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_service_mod)

    task_service_mod = ModuleType("api.db.services.task_service")
    task_service_mod.TaskService = SimpleNamespace(filter_delete=lambda *_args, **_kwargs: None)
    task_service_mod.cancel_all_task_of = lambda *_args, **_kwargs: None
    task_service_mod.queue_tasks = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)

    document_counter_service_mod = ModuleType("api.db.services.document_counter_service")
    document_counter_service_mod.release_reparse_counters = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.document_counter_service", document_counter_service_mod)

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
            return "tm-embd-1"

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
            return True, SimpleNamespace(pagerank=0.6, tenant_id="tenant-1", tenant_embd_id="tm-embd-2", tenant_llm_id="tm-llm-1")

    kb_service_mod.KnowledgebaseService = _KnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)
    services_pkg.knowledgebase_service = kb_service_mod

    class _DummyLLMService:
        @staticmethod
        def query(**_kwargs):
            return [SimpleNamespace(llm_name="gpt-3.5-turbo", model_type="chat", max_tokens=8192, is_tools=True)]

    llm_service_mod = ModuleType("api.db.services.llm_service")
    llm_service_mod.LLMService = _DummyLLMService
    llm_service_mod.LLMBundle = _DummyLLMBundle
    llm_service_mod.resolve_llm_setting = lambda *_args, **_kwargs: {}
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
                status=1,
            )

        @staticmethod
        def get_api_key(tenant_id, model_name):
            return _MockTableObject(
                id=1, tenant_id=tenant_id, llm_factory="", model_type="chat", llm_name=model_name, api_key="fake-api-key", api_base="https://api.example.com", max_tokens=8192, used_tokens=0, status=1
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
                tenant_llm_id="tm-llm-1",
                embd_id="text-embedding-ada-002",
                tenant_embd_id="tm-embd-2",
                asr_id="whisper-1",
                img2txt_id="gpt-4-vision-preview",
                rerank_id="bge-reranker",
                tts_id="tts-1",
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

    module_path = repo_root / "api" / "apps" / "chunk_app.py"
    module = None
    if module_path.exists():
        module_name = "test_chunk_routes_unit_module"
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
    # chunk_api imports these inside the chunk-write helpers; re-expose the shared
    # stubs so tests can reach them via module.<name> (setattr / method patching).
    module.rag_tokenizer = sys.modules["rag.nlp"].rag_tokenizer
    module.beAdoc = sys.modules["rag.app.qa"].beAdoc
    module.rmPrefix = sys.modules["rag.app.qa"].rmPrefix
    module.label_question = sys.modules["rag.app.tag"].label_question
    module.cross_languages = sys.modules["rag.prompts.generator"].cross_languages
    module.keyword_extraction = sys.modules["rag.prompts.generator"].keyword_extraction
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
    assert res["data"]["chunks"][0]["doc_type_kwd"] == "text", res

    module.request = SimpleNamespace(args={"id": "chunk-1"}, headers={})
    module.settings.docStoreConn.chunk["doc_type_kwd"] = "image"
    res = _run(_route_core(module.list_chunks)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert res["data"]["chunks"][0]["doc_type_kwd"] == "image", res

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
    assert res["message"] == "you don't own the document doc-1", res

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
def test_restful_add_chunk_invalid_image_base64_does_not_index_chunk(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})
    module.settings.docStoreConn.inserted.clear()

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"content": "chunk with bad image", "image_base64": "not-valid-base64!!!"}),
    )
    res = _run(_route_core(module.add_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert res["message"] == "Invalid `image_base64`", res
    assert module.settings.docStoreConn.inserted == [], res
    assert module.DocumentService.increment_calls == [], res


@pytest.mark.p2
def test_restful_add_chunk_empty_image_base64_does_not_index_chunk(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})
    module.settings.docStoreConn.inserted.clear()

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"content": "chunk with empty image", "image_base64": ""}),
    )
    res = _run(_route_core(module.add_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert res["message"] == "`image_base64` must be a non-empty string", res
    assert module.settings.docStoreConn.inserted == [], res
    assert module.DocumentService.increment_calls == [], res


@pytest.mark.p2
def test_restful_update_chunk_invalid_image_base64_does_not_update_chunk(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})
    module.settings.docStoreConn.updated.clear()

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"content": "updated chunk", "image_base64": "not-valid-base64!!!"}),
    )
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert res["message"] == "Invalid `image_base64`", res
    assert module.settings.docStoreConn.updated == [], res


@pytest.mark.p2
def test_restful_add_chunk_valid_image_base64_stores_before_insert(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.request = SimpleNamespace(args={}, headers={})
    module.settings.docStoreConn.inserted.clear()
    store_calls = []
    monkeypatch.setattr(module, "store_chunk_image", lambda bucket, name, binary: store_calls.append((bucket, name, binary)))

    valid_b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"content": "chunk with image", "image_base64": valid_b64}),
    )
    res = _run(_route_core(module.add_chunk)("tenant-1", "kb-1", "doc-1"))
    assert res["code"] == 0, res
    assert store_calls, "store_chunk_image should run before doc-store insert"
    assert module.settings.docStoreConn.inserted, "chunk should be indexed after image stored"
    inserted = module.settings.docStoreConn.inserted[-1]
    assert inserted.get("img_id"), inserted
    assert inserted.get("doc_type_kwd") == "image", inserted
    assert res["data"]["chunk"]["doc_type_kwd"] == "image", res


@pytest.mark.p2
def test_restful_update_chunk_replace_image_mode(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.settings.docStoreConn.chunk = {
        "id": "chunk-1",
        "doc_id": "doc-1",
        "content_with_weight": "chunk",
        "img_id": "kb-1-chunk-1",
    }
    store_calls = []
    module.store_chunk_image = lambda bucket, name, binary, mode="append": store_calls.append((bucket, name, mode))

    valid_b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"image_base64": valid_b64, "image_update_mode": "replace"}),
    )
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == 0, res
    assert store_calls == [("kb-1", "chunk-1", "replace")], store_calls
    updated = module.settings.docStoreConn.updated[-1][1]
    assert updated["img_id"] == "kb-1-chunk-1", updated
    assert updated["doc_type_kwd"] == "image", updated


@pytest.mark.p2
def test_restful_update_chunk_remove_image_mode(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.settings.docStoreConn.chunk = {
        "id": "chunk-1",
        "doc_id": "doc-1",
        "content_with_weight": "chunk",
        "img_id": "kb-1-chunk-1",
    }
    call_order = []
    original_update = module.settings.docStoreConn.update

    def tracked_update(*args, **kwargs):
        call_order.append("update")
        return original_update(*args, **kwargs)

    module.settings.docStoreConn.update = tracked_update
    module.remove_chunk_image = lambda bucket, name: call_order.append(("remove", bucket, name))

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"image_update_mode": "remove"}))
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == 0, res
    assert call_order == ["update", ("remove", "kb-1", "chunk-1")], call_order
    updated = module.settings.docStoreConn.updated[-1][1]
    assert updated["img_id"] == "", updated
    assert updated["doc_type_kwd"] == "text", updated


@pytest.mark.p2
def test_restful_update_chunk_remove_image_skips_delete_when_encode_fails(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.settings.docStoreConn.chunk = {
        "id": "chunk-1",
        "doc_id": "doc-1",
        "content_with_weight": "chunk",
        "img_id": "kb-1-chunk-1",
    }
    remove_calls = []

    class _FailingLLMBundle(_DummyLLMBundle):
        def encode(self, _inputs):
            raise RuntimeError("embedding failed")

    module.TenantLLMService.model_instance = staticmethod(lambda _model_config: _FailingLLMBundle())
    module.remove_chunk_image = lambda bucket, name: remove_calls.append((bucket, name))

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"image_update_mode": "remove"}))
    with pytest.raises(RuntimeError, match="embedding failed"):
        _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert remove_calls == [], remove_calls
    assert module.settings.docStoreConn.updated == [], module.settings.docStoreConn.updated


@pytest.mark.p2
def test_restful_update_chunk_remove_image_skips_delete_when_update_fails(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.settings.docStoreConn.chunk = {
        "id": "chunk-1",
        "doc_id": "doc-1",
        "content_with_weight": "chunk",
        "img_id": "kb-1-chunk-1",
    }
    remove_calls = []
    module.settings.docStoreConn.update = lambda *_args, **_kwargs: False
    module.remove_chunk_image = lambda bucket, name: remove_calls.append((bucket, name))

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"image_update_mode": "remove"}))
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "Index updating failure" in res["message"], res
    assert remove_calls == [], remove_calls


@pytest.mark.p2
def test_restful_update_chunk_replace_requires_image_base64(monkeypatch):
    module = _load_chunk_api_module(monkeypatch)
    module.settings.docStoreConn.chunk = {"id": "chunk-1", "doc_id": "doc-1", "content_with_weight": "chunk"}
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"image_update_mode": "replace"}))
    res = _run(_route_core(module.update_chunk)("tenant-1", "kb-1", "doc-1", "chunk-1"))
    assert res["code"] == module.RetCode.DATA_ERROR, res
    assert "`image_base64` is required" in res["message"], res
    assert module.settings.docStoreConn.updated == [], res
