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
import functools
import inspect
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import numpy as np
import pytest

from api.db import FileType


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


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


class _DummyFiles(dict):
    def getlist(self, key):
        return self.get(key, [])


class _DummyArgs(dict):
    def getlist(self, key):
        v = self.get(key, [])
        if v is None:
            return []
        if isinstance(v, list):
            return v
        return [v]


class _DummyDoc:
    def __init__(
        self,
        *,
        doc_id="doc-1",
        kb_id="kb-1",
        name="doc.txt",
        chunk_num=1,
        token_num=2,
        progress=0,
        process_duration=0,
        parser_id="naive",
        doc_type=FileType.OTHER,
        status=True,
        run=0,
        progress_msg="",
    ):
        self.id = doc_id
        self.kb_id = kb_id
        self.name = name
        self.chunk_num = chunk_num
        self.token_num = token_num
        self.progress = progress
        self.process_duration = process_duration
        self.parser_id = parser_id
        self.type = doc_type
        self.status = status
        self.run = run
        self.progress_msg = progress_msg

    def to_dict(self):
        return {
            "id": self.id,
            "kb_id": self.kb_id,
            "name": self.name,
            "chunk_num": self.chunk_num,
            "token_num": self.token_num,
            "progress": self.progress,
            "process_duration": self.process_duration,
            "parser_id": self.parser_id,
            "run": self.run,
            "status": self.status,
        }


class _ToggleBoolDocList:
    def __init__(self, value):
        self._calls = 0
        self._value = value

    def __getitem__(self, item):
        return self._value

    def __bool__(self):
        self._calls += 1
        return self._calls == 1


def _run(coro):
    return asyncio.run(coro)


def _load_doc_module(monkeypatch, module_basename="chunk_api"):
    repo_root = Path(__file__).resolve().parents[4]
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    apps_mod = ModuleType("api.apps")

    def _login_required(func=None, **_kwargs):
        # Real login_required is used both bare (chunk_api) and as a factory with
        # auth_types=[...] (document_api); the mock must pass through both forms.
        if func is None:
            return lambda inner: inner
        return func

    apps_mod.login_required = _login_required
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.AUTH_JWT = None
    apps_mod.AUTH_API = None
    apps_mod.AUTH_BETA = None
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    common_settings_mod = ModuleType("common.settings")
    common_settings_mod.retriever = SimpleNamespace()
    common_settings_mod.kg_retriever = SimpleNamespace()
    common_settings_mod.STORAGE_IMPL = SimpleNamespace(get=lambda *_args, **_kwargs: b"", rm=lambda *_args, **_kwargs: None)
    monkeypatch.setitem(sys.modules, "common.settings", common_settings_mod)

    common_misc_utils_mod = ModuleType("common.misc_utils")

    async def _thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    common_misc_utils_mod.thread_pool_exec = _thread_pool_exec
    common_misc_utils_mod.get_uuid = lambda: "uuid-1"
    monkeypatch.setitem(sys.modules, "common.misc_utils", common_misc_utils_mod)

    common_string_utils_mod = ModuleType("common.string_utils")
    common_string_utils_mod.is_content_empty = lambda content: content is None or not str(content).strip()
    common_string_utils_mod.remove_redundant_spaces = lambda text: " ".join(str(text).split())
    monkeypatch.setitem(sys.modules, "common.string_utils", common_string_utils_mod)

    tag_feature_utils_mod = ModuleType("common.tag_feature_utils")
    tag_feature_utils_mod.validate_tag_features = lambda value: value
    monkeypatch.setitem(sys.modules, "common.tag_feature_utils", tag_feature_utils_mod)

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
        # Stands in for Document.select().where(...).for_update().first().
        def where(self, *_args, **_kwargs):
            return self

        def for_update(self):
            return self

        def first(self):
            return _StubDocumentModel.fresh_doc

    class _StubDocumentModel:
        id = _FakeField()
        run = _FakeField()
        # The row re-read under lock by _release_doc_counters; tests that assert on
        # the decrement override this to mirror the document's current counters.
        fresh_doc = _StubFreshDoc()

        @classmethod
        def select(cls, *_args, **_kwargs):
            return _StubDocQuery()

    class _StubTaskModel:
        doc_id = _FakeField()

    class _AnyFieldMeta(type):
        def __getattr__(cls, _name):
            return _FakeField()

    class _StubModel(metaclass=_AnyFieldMeta):
        pass

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.APIToken = SimpleNamespace(query=lambda **_kwargs: [])
    db_models_mod.Document = _StubDocumentModel
    db_models_mod.Task = _StubTaskModel
    db_models_mod.DB = SimpleNamespace(atomic=lambda: contextlib.nullcontext(), connection_context=lambda: lambda fn: fn)
    # Transitively-loaded real services import assorted model classes (File,
    # Knowledgebase, UserTenant, ...); hand them a permissive stub on demand.
    db_models_mod.__getattr__ = lambda _name: _StubModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    services_pkg = ModuleType("api.db.services")
    services_pkg.__path__ = [str(repo_root / "api" / "db" / "services")]
    services_pkg.duplicate_name = lambda _query, name="", **_kwargs: name
    monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

    doc_metadata_service_mod = ModuleType("api.db.services.doc_metadata_service")
    doc_metadata_service_mod.DocMetadataService = SimpleNamespace(
        get_flatted_meta_by_kbs=lambda *_args, **_kwargs: [],
        get_metadata_for_documents=lambda *_args, **_kwargs: {},
    )
    monkeypatch.setitem(sys.modules, "api.db.services.doc_metadata_service", doc_metadata_service_mod)

    document_service_mod = ModuleType("api.db.services.document_service")
    document_service_mod.DocumentService = SimpleNamespace(
        query=lambda **_kwargs: [],
        accessible=lambda *_args, **_kwargs: True,
        filter_update=lambda *_args, **_kwargs: 0,
        get_by_id=lambda *_args, **_kwargs: (False, None),
        update_by_id=lambda *_args, **_kwargs: True,
        decrement_chunk_num=lambda *_args, **_kwargs: None,
        increment_chunk_num=lambda *_args, **_kwargs: True,
        get_embd_id=lambda *_args, **_kwargs: "",
        get_tenant_embd_id=lambda *_args, **_kwargs: None,
    )
    monkeypatch.setitem(sys.modules, "api.db.services.document_service", document_service_mod)

    file2document_service_mod = ModuleType("api.db.services.file2document_service")
    file2document_service_mod.File2DocumentService = SimpleNamespace(
        get_storage_address=lambda **_kwargs: ("", ""),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.file2document_service", file2document_service_mod)

    knowledgebase_service_mod = ModuleType("api.db.services.knowledgebase_service")
    knowledgebase_service_mod.KnowledgebaseService = SimpleNamespace(
        accessible=lambda **_kwargs: False,
        get_by_id=lambda *_args, **_kwargs: (False, None),
        get_by_ids=lambda *_args, **_kwargs: [],
        list_documents_by_ids=lambda *_args, **_kwargs: [],
        query=lambda **_kwargs: [],
    )
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", knowledgebase_service_mod)

    task_service_mod = ModuleType("api.db.services.task_service")
    task_service_mod.TaskService = SimpleNamespace(filter_delete=lambda *_args, **_kwargs: None, query=lambda **_kwargs: [])
    task_service_mod.cancel_all_task_of = lambda *_args, **_kwargs: None
    task_service_mod.queue_tasks = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_mod)

    file_service_mod = ModuleType("api.db.services.file_service")
    file_service_mod.FileService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

    canvas_service_mod = ModuleType("api.db.services.canvas_service")
    canvas_service_mod.UserCanvasService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.canvas_service", canvas_service_mod)

    # document_api imports check_kb_team_permission; stub it so the real module
    # and its user_service/common_service (peewee) import chain stay out.
    check_team_permission_mod = ModuleType("api.common.check_team_permission")
    check_team_permission_mod.check_kb_team_permission = lambda *_args, **_kwargs: True
    check_team_permission_mod.check_file_team_permission = lambda *_args, **_kwargs: True
    monkeypatch.setitem(sys.modules, "api.common.check_team_permission", check_team_permission_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _add_tenant_id_to_kwargs(func):
        # Mirror the real decorator's functools.wraps so tests can reach the raw
        # handler through ``route.__wrapped__`` and pass ``tenant_id`` explicitly.
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            return func(*args, **kwargs)

        return wrapper

    api_utils_mod.add_tenant_id_to_kwargs = _add_tenant_id_to_kwargs
    api_utils_mod.check_duplicate_ids = lambda ids, _kind="item": (ids, [])
    api_utils_mod.construct_json_result = lambda code=0, message="success", data=None: {"code": code, "message": message, "data": data}
    api_utils_mod.get_error_data_result = lambda message="Sorry! Data missing!", code=102: {"code": code, "message": message}
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    api_utils_mod.get_result = lambda code=0, message="", data=None, total=None: {
        key: value for key, value in {"code": code, "message": message, "data": data, "total": total}.items() if value is not None
    }
    api_utils_mod.server_error_response = lambda e: {"code": 500, "message": str(e)}
    api_utils_mod.get_data_error_result = lambda message="Sorry! Data missing!", code=102: {"code": code, "message": message}
    api_utils_mod.get_error_argument_result = lambda message="": {"code": 101, "message": message}
    api_utils_mod.get_json_result = lambda code=0, message="success", data=None, **_kwargs: {"code": code, "message": message, "data": data}
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    image_utils_mod = ModuleType("api.utils.image_utils")
    image_utils_mod.store_chunk_image = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.utils.image_utils", image_utils_mod)

    reference_metadata_utils_mod = ModuleType("api.utils.reference_metadata_utils")
    reference_metadata_utils_mod.resolve_reference_metadata_preferences = lambda req, *_args, **_kwargs: (
        bool((req.get("reference_metadata") or {}).get("include")),
        set((req.get("reference_metadata") or {}).get("fields") or []),
    )

    def _enrich_chunks_with_document_metadata(chunks, metadata_fields=None):
        for chunk in chunks:
            doc_id = chunk.get("doc_id") or chunk.get("document_id")
            if not doc_id:
                continue
            metadata = doc_metadata_service_mod.DocMetadataService.get_metadata_for_documents([doc_id], chunk.get("kb_id"))
            document_metadata = dict(metadata.get(doc_id, {}))
            if metadata_fields:
                document_metadata = {key: value for key, value in document_metadata.items() if key in metadata_fields}
            if document_metadata:
                chunk["document_metadata"] = document_metadata

    reference_metadata_utils_mod.enrich_chunks_with_document_metadata = _enrich_chunks_with_document_metadata
    monkeypatch.setitem(sys.modules, "api.utils.reference_metadata_utils", reference_metadata_utils_mod)

    common_metadata_utils_mod = ModuleType("common.metadata_utils")
    common_metadata_utils_mod.convert_conditions = lambda conditions: conditions
    common_metadata_utils_mod.meta_filter = lambda *_args, **_kwargs: []
    common_metadata_utils_mod.turn2jsonschema = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "common.metadata_utils", common_metadata_utils_mod)

    rag_app_tag_mod = ModuleType("rag.app.tag")
    rag_app_tag_mod.label_question = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "rag.app.tag", rag_app_tag_mod)

    rag_prompts_generator_mod = ModuleType("rag.prompts.generator")
    rag_prompts_generator_mod.cross_languages = lambda *_args, **_kwargs: ""
    rag_prompts_generator_mod.keyword_extraction = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_prompts_generator_mod)

    rag_nlp_mod = ModuleType("rag.nlp")
    rag_nlp_mod.search = SimpleNamespace(index_name=lambda tenant_id: f"idx_{tenant_id}")
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp_mod)
    monkeypatch.setitem(sys.modules, "rag.nlp.search", rag_nlp_mod.search)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubDocxParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_parser_pkg.ExcelParser = _StubExcelParser
    deepdoc_parser_pkg.DocxParser = _StubDocxParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)
    deepdoc_parser_utils = ModuleType("deepdoc.parser.utils")
    deepdoc_parser_utils.get_text = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", deepdoc_parser_utils)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    # Mock tenant_llm_service for TenantLLMService and TenantService
    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _MockModelConfig:
        def __init__(self, tenant_id, model_name):
            self.tenant_id = tenant_id
            self.llm_name = model_name
            self.llm_factory = "Builtin"
            self.api_key = "fake-api-key"
            self.api_base = "https://api.example.com"
            self.model_type = "embedding"
            self.max_tokens = 8192
            self.used_tokens = 0
            self.status = 1
            self.id = 1

        def to_dict(self):
            return {
                "tenant_id": self.tenant_id,
                "llm_name": self.llm_name,
                "llm_factory": self.llm_factory,
                "api_key": self.api_key,
                "api_base": self.api_base,
                "model_type": self.model_type,
                "max_tokens": self.max_tokens,
                "used_tokens": self.used_tokens,
                "status": self.status,
                "id": self.id,
            }

    class _StubTenantService:
        @staticmethod
        def get_by_id(tenant_id):
            return True, SimpleNamespace(id=tenant_id, llm_id="chat-model", embd_id="embd-model", asr_id="asr-model", img2txt_id="img2txt-model", rerank_id="rerank-model", tts_id="tts-model")

    class _StubTenantLLMService:
        @staticmethod
        def get_api_key(tenant_id, model_name):
            return _MockModelConfig(tenant_id, model_name)

        @staticmethod
        def split_model_name_and_factory(model_name):
            if "@" in model_name:
                parts = model_name.split("@")
                return parts[0], parts[1]
            return model_name, None

        @staticmethod
        def get_by_id(tenant_model_id):
            return True, _MockModelConfig("tenant-1", "model-1")

        @staticmethod
        def model_instance(model_config):
            class _EmbedModel:
                def encode(self, texts):
                    import numpy as np

                    return [np.array([0.2, 0.8]), np.array([0.3, 0.7])], 1

            return _EmbedModel()

    tenant_llm_service_mod.TenantService = _StubTenantService
    tenant_llm_service_mod.TenantLLMService = _StubTenantLLMService

    class _StubLLMFactoriesService:
        pass

    tenant_llm_service_mod.LLMFactoriesService = _StubLLMFactoriesService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    # Mock LLMService
    llm_service_mod = ModuleType("api.db.services.llm_service")

    class _StubLLM:
        def __init__(self, llm_name):
            self.llm_name = llm_name
            self.is_tools = False

    class _StubLLMBundle:
        def __init__(self, tenant_id: str, model_config: dict, lang="Chinese", **kwargs):
            self.tenant_id = tenant_id
            self.model_config = model_config
            self.lang = lang

        def encode(self, texts: list):
            import numpy as np

            # Return mock embeddings and token usage
            return [np.array([0.2, 0.8]), np.array([0.3, 0.7])], len(texts) * 10

    llm_service_mod.LLMService = SimpleNamespace(query=lambda llm_name: [_StubLLM(llm_name)] if llm_name else [])
    llm_service_mod.LLMBundle = _StubLLMBundle
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)

    # Mock tenant_model_service to ensure it uses mocked services
    tenant_model_service_mod = ModuleType("api.db.joint_services.tenant_model_service")

    class _MockModelConfig2:
        def __init__(self, tenant_id, model_name):
            self.tenant_id = tenant_id
            self.llm_name = model_name
            self.llm_factory = "Builtin"
            self.api_key = "fake-api-key"
            self.api_base = "https://api.example.com"
            self.model_type = "embedding"
            self.max_tokens = 8192
            self.used_tokens = 0
            self.status = 1
            self.id = 1

        def to_dict(self):
            return {
                "tenant_id": self.tenant_id,
                "llm_name": self.llm_name,
                "llm_factory": self.llm_factory,
                "api_key": self.api_key,
                "api_base": self.api_base,
                "model_type": self.model_type,
                "max_tokens": self.max_tokens,
                "used_tokens": self.used_tokens,
                "status": self.status,
                "id": self.id,
            }

    def _get_model_config_by_id(
        tenant_model_id: str,
        allowed_tenant_ids=None,
        requester_tenant_id=None,
    ) -> dict:
        mock_tenant_id = "tenant-1"
        if allowed_tenant_ids is not None:
            if isinstance(allowed_tenant_ids, str):
                allowed_tenant_ids = {allowed_tenant_ids}
            else:
                allowed_tenant_ids = {str(tenant_id) for tenant_id in allowed_tenant_ids if tenant_id}
            if mock_tenant_id not in allowed_tenant_ids and str(requester_tenant_id) != mock_tenant_id:
                raise LookupError(f"Tenant Model with id {tenant_model_id} not authorized")
        return _MockModelConfig2(mock_tenant_id, "model-1").to_dict()

    def _get_model_config_from_provider_instance(tenant_id: str, model_type: str, model_name: str):
        if not model_name:
            raise Exception("Model Name is required")
        return _MockModelConfig2(tenant_id, model_name).to_dict()

    def _get_tenant_default_model_by_type(tenant_id: str, model_type):
        # Return mock tenant with default model configurations
        return _MockModelConfig2(tenant_id, "chat-model").to_dict()

    def _split_model_name(model_name):
        parts = model_name.rsplit("@", 2)
        if len(parts) == 3:
            return parts[0], parts[1], parts[2]
        if len(parts) == 2:
            return parts[0], "default", parts[1]
        return parts[0], "", ""

    tenant_model_service_mod.get_model_config_by_id = _get_model_config_by_id
    tenant_model_service_mod.get_model_config_from_provider_instance = _get_model_config_from_provider_instance
    tenant_model_service_mod.resolve_model_config = _get_model_config_from_provider_instance
    tenant_model_service_mod.get_tenant_default_model_by_type = _get_tenant_default_model_by_type
    tenant_model_service_mod.split_model_name = _split_model_name
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    if module_basename == "document_api":
        stub_apps_services = ModuleType("api.apps.services")
        stub_apps_services.__path__ = [str(repo_root / "api" / "apps" / "services")]
        monkeypatch.setitem(sys.modules, "api.apps.services", stub_apps_services)

        document_api_service_mod = ModuleType("api.apps.services.document_api_service")
        document_api_service_mod.validate_document_update_fields = lambda *_args, **_kwargs: (None, None)
        document_api_service_mod.map_doc_keys = lambda doc: doc.to_dict() if hasattr(doc, "to_dict") else doc
        document_api_service_mod.map_doc_keys_with_run_status = lambda doc, run_status="0": {
            **(doc if isinstance(doc, dict) else doc.to_dict()),
            "run": run_status,
        }
        document_api_service_mod.update_document_name_only = lambda *_args, **_kwargs: None
        document_api_service_mod.update_chunk_method = lambda *_args, **_kwargs: None
        document_api_service_mod.update_document_status_only = lambda *_args, **_kwargs: None
        document_api_service_mod.reset_document_for_reparse = lambda *_args, **_kwargs: None
        monkeypatch.setitem(sys.modules, "api.apps.services.document_api_service", document_api_service_mod)
    else:
        # chunk_api imports structure_graph_common from api.apps.services; stub
        # it in sys.modules so the real module and its transitive imports are
        # not loaded.
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

    # document_counter_service is a real module (release_reparse_counters); evict
    # any cached copy so it re-imports against this test's freshly-stubbed
    # db_models / document_service rather than a prior test's stubs.
    monkeypatch.delitem(sys.modules, "api.db.services.document_counter_service", raising=False)

    module_path = repo_root / "api" / "apps" / "restful_apis" / f"{module_basename}.py"
    spec = importlib.util.spec_from_file_location("test_doc_sdk_routes_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _load_restful_chunk_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    helper_path = repo_root / "test" / "testcases" / "test_web_api" / "test_chunk_app" / "test_chunk_routes_unit.py"
    spec = importlib.util.spec_from_file_location("test_restful_chunk_route_helpers", helper_path)
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)
    return helper._load_chunk_api_module(monkeypatch)


def _route_core(func):
    return inspect.unwrap(func)


def _patch_send_file(monkeypatch, module):
    async def _fake_send_file(file_obj, **kwargs):
        return {
            "file": file_obj,
            "filename": kwargs.get("attachment_filename"),
            "mimetype": kwargs.get("mimetype"),
        }

    monkeypatch.setattr(module, "send_file", _fake_send_file)


def _patch_storage(monkeypatch, module, *, file_stream=b"abc"):
    storage = SimpleNamespace(get=lambda *_args, **_kwargs: file_stream, rm=lambda *_args, **_kwargs: None)
    monkeypatch.setattr(module.settings, "STORAGE_IMPL", storage)


def _patch_docstore(monkeypatch, module, **kwargs):
    defaults = {
        "delete": lambda *_args, **_kwargs: 0,
        "update": lambda *_args, **_kwargs: None,
        "get": lambda *_args, **_kwargs: {},
        "insert": lambda *_args, **_kwargs: None,
        "index_exist": lambda *_args, **_kwargs: False,
    }
    defaults.update(kwargs)
    monkeypatch.setattr(module.settings, "docStoreConn", SimpleNamespace(**defaults), raising=False)


@pytest.mark.p2
class TestDocRoutesUnit:
    def test_chunk_positions_validation_error(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        with pytest.raises(ValueError) as exc_info:
            module.Chunk(positions=[[1, 2, 3, 4]])
        assert "length of 5" in str(exc_info.value)

    def test_download_and_download_doc_errors(self, monkeypatch):
        module = _load_doc_module(monkeypatch, module_basename="document_api")
        _patch_send_file(monkeypatch, module)
        _patch_storage(monkeypatch, module, file_stream=b"")

        # download(dataset_id, document_id)
        res = _run(module.download("ds-1", ""))
        assert res["message"] == "Specify document_id please."

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.download("ds-1", "doc-1"))
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
        res = _run(module.download("ds-1", "doc-1"))
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.download("ds-1", "doc-1"))
        assert "not own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        res = _run(module.download("ds-1", "doc-1"))
        assert res["message"] == "This file is empty."

        # download_document(document_id)
        res = _run(module.download_document(""))
        assert res["message"] == "Specify document_id please."

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
        res = _run(module.download_document("doc-1"))
        assert res["message"] == "Document not found!"

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.download_document("doc-1"))
        assert "not own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        res = _run(module.download_document("doc-1"))
        assert res["message"] == "This file is empty."

        _patch_storage(monkeypatch, module, file_stream=b"abc")
        res = _run(module.download_document("doc-1"))
        assert res["filename"] == "doc.txt"

    def test_download_mimetype_from_filename(self, monkeypatch):
        module = _load_doc_module(monkeypatch, module_basename="document_api")
        _patch_send_file(monkeypatch, module)
        _patch_storage(monkeypatch, module, file_stream=b"pdf-bytes")
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(name="report.pdf", doc_type=FileType.PDF)])
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        res = _run(module.download("ds-1", "doc-1"))
        assert res["filename"] == "report.pdf"
        assert res["mimetype"] == "application/pdf"

    def test_parse_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", pipeline_id=None)))
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        toggle_doc = _ToggleBoolDocList(_DummyDoc(progress=0))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: toggle_doc)
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        monkeypatch.setattr(
            module.DocumentService,
            "filter_update",
            lambda *_args, **_kwargs: 0,
        )
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert "currently being processed" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(progress=0)])
        monkeypatch.setattr(module.DocumentService, "filter_update", lambda *_args, **_kwargs: 1)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, _DummyDoc()))
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.TaskService, "filter_delete", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "queue_tasks", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, ["Duplicate document ids: doc-1"]))
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0
        assert res["data"]["success_count"] == 1
        assert "Duplicate document ids" in res["data"]["errors"][0]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: ([], ["Duplicate document ids: doc-1"]))
        res = _run(module.parse.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Duplicate document ids" in res["message"]

    def test_parse_and_stop_decrement_kb_counters(self, monkeypatch):
        # Both routes delete the document's chunks, so the knowledgebase aggregate
        # must drop by the document's current counters. Zeroing only the document
        # row leaves that amount stranded in the KB total on every re-parse.
        module = _load_doc_module(monkeypatch)
        # _release_doc_counters re-reads the row under lock; mirror the document's
        # current counters so the decrement is driven by that fresh read.
        monkeypatch.setattr(module.Document, "fresh_doc", SimpleNamespace(id="doc-1", kb_id="kb-1", token_num=70, chunk_num=7, process_duration=1.5))
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", pipeline_id=None)))
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, _DummyDoc()))
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        monkeypatch.setattr(module.TaskService, "filter_delete", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "queue_tasks", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "cancel_all_task_of", lambda *_args, **_kwargs: None)
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)

        decrements = []
        updates = []
        update_by_id_payloads = []

        def _capture_filter_update(_conditions, info):
            updates.append(info)
            return 1

        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda _id, info: update_by_id_payloads.append(info) or True)

        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *args: decrements.append(args))
        monkeypatch.setattr(module.DocumentService, "filter_update", _capture_filter_update)

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(token_num=70, chunk_num=7, process_duration=1.5)])
        assert _run(module.parse.__wrapped__("tenant-1", "ds-1"))["code"] == 0
        assert decrements == [("doc-1", "kb-1", -70, -7, -1.5)]
        # The document update must not zero the counters itself; that is exactly
        # what strands the difference in the KB aggregate.
        assert "chunk_num" not in updates[0]
        assert "token_num" not in updates[0]

        decrements.clear()
        monkeypatch.setattr(
            module.DocumentService,
            "query",
            lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value, token_num=70, chunk_num=7, process_duration=1.5)],
        )
        assert _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))["code"] == 0
        assert decrements == [("doc-1", "kb-1", -70, -7, -1.5)]
        # stop_parsing must not zero the counters in its own document update either.
        assert "chunk_num" not in update_by_id_payloads[0]
        assert "token_num" not in update_by_id_payloads[0]

    def test_stop_parsing_branches(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", pipeline_id=None)))
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "`document_ids` is required" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.DONE.value)])
        monkeypatch.setattr(
            module,
            "cancel_all_task_of",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("cancel_all_task_of must not be called for non-running docs")),
        )
        monkeypatch.setattr(
            module.DocumentService,
            "update_by_id",
            lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("update_by_id must not be called for non-running docs")),
        )
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert res["data"]["error_code"] == module.DOC_STOP_PARSING_INVALID_STATE_ERROR_CODE
        assert res["message"] == module.DOC_STOP_PARSING_INVALID_STATE_MESSAGE

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        monkeypatch.setattr(module, "cancel_all_task_of", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, ["Duplicate document ids: doc-1"]))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0
        assert res["data"]["success_count"] == 1
        assert "Duplicate document ids" in res["data"]["errors"][0]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: ([], ["Duplicate document ids: doc-1"]))
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Duplicate document ids" in res["message"]

        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(run=module.TaskStatus.RUNNING.value)])
        res = _run(module.stop_parsing.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0

    def test_legacy_chunks_parse_uses_dataset_owner_tenant_for_delete(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        deleted = []
        requester_tenant = "team-member"
        owner_tenant = "dataset-owner"

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(
            module.KnowledgebaseService,
            "get_by_id",
            lambda _id: (True, SimpleNamespace(tenant_id=owner_tenant, pipeline_id=None)),
        )
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(
            module.DocumentService,
            "query",
            lambda **_kwargs: [_DummyDoc(doc_id="doc-1", run=module.TaskStatus.UNSTART.value)],
        )
        monkeypatch.setattr(module.DocumentService, "filter_update", lambda *_args, **_kwargs: 1)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _id: (True, _DummyDoc(doc_id="doc-1")))
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("b", "n"))
        monkeypatch.setattr(module.TaskService, "filter_delete", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "queue_tasks", lambda *_args, **_kwargs: None)
        _patch_docstore(
            monkeypatch,
            module,
            index_exist=lambda *_args, **_kwargs: True,
            delete=lambda condition, index, kb_id: deleted.append((condition, index, kb_id)),
        )

        res = _run(module.parse.__wrapped__(requester_tenant, "ds-1"))

        assert res["code"] == 0
        assert deleted == [({"doc_id": "doc-1"}, module.search.index_name(owner_tenant), "kb-1")]

    def test_legacy_chunks_stop_uses_dataset_owner_tenant_for_delete(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        deleted = []
        requester_tenant = "team-member"
        owner_tenant = "dataset-owner"

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(
            module.KnowledgebaseService,
            "get_by_id",
            lambda _id: (True, SimpleNamespace(tenant_id=owner_tenant, pipeline_id=None)),
        )
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(
            module.DocumentService,
            "query",
            lambda **_kwargs: [_DummyDoc(doc_id="doc-1", run=module.TaskStatus.RUNNING.value)],
        )
        monkeypatch.setattr(module, "cancel_all_task_of", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        _patch_docstore(
            monkeypatch,
            module,
            index_exist=lambda *_args, **_kwargs: True,
            delete=lambda condition, index, kb_id: deleted.append((condition, index, kb_id)),
        )

        res = _run(module.stop_parsing.__wrapped__(requester_tenant, "ds-1"))

        assert res["code"] == 0
        assert deleted == [({"doc_id": "doc-1"}, module.search.index_name(owner_tenant), "kb-1")]

    def test_stop_parse_documents_cleans_partial_chunks(self, monkeypatch):
        module = _load_doc_module(monkeypatch, module_basename="document_api")
        updated = []
        deleted = []
        decrements = []

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"document_ids": ["doc-1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [object()])
        monkeypatch.setattr(
            module.DocumentService,
            "get_by_id",
            lambda _id: (True, _DummyDoc(doc_id="doc-1", run=module.TaskStatus.RUNNING.value)),
        )
        monkeypatch.setattr(module.TaskService, "query", lambda **_kwargs: [SimpleNamespace(progress=0.5)])
        monkeypatch.setattr(module, "cancel_all_task_of", lambda *_args, **_kwargs: None)
        # release_reparse_counters re-reads the row under lock and decrements the
        # KB by the document's partial counts before chunk_num is zeroed below.
        monkeypatch.setattr(
            sys.modules["api.db.db_models"].Document,
            "fresh_doc",
            SimpleNamespace(id="doc-1", kb_id="kb-1", token_num=70, chunk_num=7, process_duration=1.5),
        )
        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *args: decrements.append(args))
        monkeypatch.setattr(
            module.DocumentService,
            "update_by_id",
            lambda doc_id, info: updated.append((doc_id, info)) or True,
        )
        _patch_docstore(
            monkeypatch,
            module,
            index_exist=lambda *_args, **_kwargs: True,
            delete=lambda condition, index, kb_id: deleted.append((condition, index, kb_id)),
        )

        res = _run(module.stop_parse_documents.__wrapped__("tenant-1", "ds-1"))

        assert res["code"] == 0
        assert res["data"]["success_count"] == 1
        assert len(updated) == 1
        updated_doc_id, updated_info = updated[0]
        assert updated_doc_id == "doc-1"
        assert updated_info["run"] == str(module.TaskStatus.CANCEL.value)
        assert updated_info["progress"] == 0
        # release_reparse_counters is the sole counter adjustment; the status
        # update must not set chunk_num itself (that would strand KB counts).
        assert "chunk_num" not in updated_info
        # progress_msg carries a timestamped cancellation marker, so match loosely.
        assert "Task stopped by user." in updated_info["progress_msg"]
        # The partial chunk/token counts are released from the KB aggregate via the
        # row re-read under lock.
        assert decrements == [("doc-1", "kb-1", -70, -7, -1.5)]
        assert deleted == [({"doc_id": "doc-1"}, module.search.index_name("tenant-1"), "kb-1")]

        deleted.clear()
        _patch_docstore(monkeypatch, module, index_exist=lambda *_args, **_kwargs: False)
        res = _run(module.stop_parse_documents.__wrapped__("tenant-1", "ds-1"))
        assert res["code"] == 0
        assert deleted == []

    def test_list_chunks_branches(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({})))
        _patch_docstore(monkeypatch, module, index_exist=lambda *_args, **_kwargs: False)
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0
        assert res["data"]["total"] == 0
        assert res["data"]["chunks"] == []

        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({"id": "chunk-1"})))
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: None)
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Chunk not found" in res["message"]

        _patch_docstore(
            monkeypatch,
            module,
            get=lambda *_args, **_kwargs: {
                "chunk_id": "chunk-1",
                "content_with_weight": "x",
                "doc_id": "other-doc",
                "docnm_kwd": "doc",
                "position_int": [[1, 2, 3, 4, 5]],
            },
        )
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Chunk not found" in res["message"]

        _patch_docstore(
            monkeypatch,
            module,
            get=lambda *_args, **_kwargs: {
                "chunk_id": "chunk-1",
                "content_with_weight": "x",
                "doc_id": "doc-1",
                "docnm_kwd": "doc",
                "position_int": [[1, 2, 3, 4, 5]],
            },
        )
        res = _run(_route_core(module.list_chunks)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0
        assert res["data"]["total"] == 1
        assert res["data"]["chunks"][0]["id"] == "chunk-1"

    def test_list_chunks_uses_dataset_owner_index_for_team_dataset(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        seen = {}
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(
            module.KnowledgebaseService,
            "get_by_id",
            lambda _dataset_id: (True, SimpleNamespace(tenant_id="owner-tenant")),
        )
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc(kb_id="ds-1")])
        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs({})))

        def _index_exist(index_name, dataset_id):
            seen["index_exist"] = (index_name, dataset_id)
            return True

        class _Retriever:
            async def search(self, _query, index_name, dataset_ids, *_args, **_kwargs):
                seen["search"] = (index_name, dataset_ids)
                return SimpleNamespace(total=0, ids=[], field={}, highlight={})

        _patch_docstore(monkeypatch, module, index_exist=_index_exist)
        monkeypatch.setattr(module.settings, "retriever", _Retriever())

        res = _run(_route_core(module.list_chunks)("member-tenant", "ds-1", "doc-1"))

        assert res["code"] == 0
        assert seen["index_exist"] == ("idx-owner-tenant", "ds-1")
        assert seen["search"] == ("idx-owner-tenant", ["ds-1"])

    def test_add_chunk_access_guard(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(_route_core(module.add_chunk)("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

    def test_rm_chunk_branches(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(_route_core(module.rm_chunk)("tenant-1", "ds-1", "doc-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(_route_core(module.rm_chunk)("tenant-1", "ds-1", "doc-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
        _patch_docstore(
            monkeypatch,
            module,
            delete=lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("delete must not run for empty chunk ids")),
        )
        monkeypatch.setattr(module.DocumentService, "decrement_chunk_num", lambda *_args, **_kwargs: None)
        res = _run(_route_core(module.rm_chunk)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"chunk_ids": ["c1", "c1"]}))
        monkeypatch.setattr(module, "check_duplicate_ids", lambda _ids, _kind: (["c1"], ["Duplicate chunk ids: c1"]))
        _patch_docstore(monkeypatch, module, delete=lambda *_args, **_kwargs: 1)
        res = _run(_route_core(module.rm_chunk)("tenant-1", "ds-1", "doc-1"))
        assert res["code"] == 0
        assert res["data"]["errors"] == ["Duplicate chunk ids: c1"]

    def test_update_chunk_branches(self, monkeypatch):
        module = _load_restful_chunk_module(monkeypatch)
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("chunk lookup must not run before access check")))
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [])
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [_DummyDoc()])
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: None)
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "Can't find this chunk" in res["message"]

        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"doc_id": "other-doc", "content_with_weight": "q\na"})
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "Can't find this chunk" in res["message"]

        doc = _DummyDoc(parser_id="naive")
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [doc])
        monkeypatch.setattr(module.rag_tokenizer, "tokenize", lambda text: text or "")
        monkeypatch.setattr(module.rag_tokenizer, "fine_grained_tokenize", lambda text: text or "")
        monkeypatch.setattr(module.rag_tokenizer, "is_chinese", lambda _text: False)
        monkeypatch.setattr(module.DocumentService, "get_embd_id", lambda _doc_id: "embd")
        monkeypatch.setattr(module.DocumentService, "get_tenant_embd_id", lambda _doc_id: "tm-embd-1")

        class _EmbedModel:
            def encode(self, _texts):
                return [np.array([0.2, 0.8]), np.array([0.3, 0.7])], 1

        monkeypatch.setattr(module.TenantLLMService, "model_instance", lambda *_args, **_kwargs: _EmbedModel())
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"doc_id": "doc-1", "content_with_weight": "x"}, update=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"positions": "bad"}))
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "`positions` should be a list" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"positions": [[1, 2, 3, 4, 5]]}))
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert res["code"] == 0

        qa_doc = _DummyDoc(parser_id=module.ParserType.QA)
        monkeypatch.setattr(module.DocumentService, "query", lambda **_kwargs: [qa_doc])
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"content": "no-separator"}))
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert "Q&A must be separated" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"content": "Q?\nA!"}))
        _patch_docstore(monkeypatch, module, get=lambda *_args, **_kwargs: {"doc_id": "doc-1", "content_with_weight": "Q?\nA!"}, update=lambda *_args, **_kwargs: None)
        monkeypatch.setattr(module, "beAdoc", lambda d, *_args, **_kwargs: d)
        res = _run(_route_core(module.update_chunk)("tenant-1", "ds-1", "doc-1", "chunk-1"))
        assert res["code"] == 0

    def test_retrieval_metadata_validation_matrix(self, monkeypatch):
        module = _load_doc_module(monkeypatch)
        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"dataset_ids": "bad"}))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`dataset_ids` should be a list" in res["message"]

        monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"dataset_ids": ["ds-1"]}))
        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: False)
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "don't own the dataset" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: True)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1"), SimpleNamespace(embd_id="m2")])
        monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda embd_id: (embd_id, "f"))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "different embedding models" in res["message"]

        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1", tenant_id="tenant-1")])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`question` is required." in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "   "}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0
        assert res["data"]["chunks"] == []

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "document_ids": "bad"}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`documents` should be a list" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "document_ids": ["not-owned"]}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "list_documents_by_ids", lambda _ids: ["doc-1"])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "don't own the document" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "metadata_condition": {"logic": "and"}}),
        )
        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [])
        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "code" in res

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": "True"}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [SimpleNamespace(embd_id="m1", tenant_id="tenant-1", tenant_embd_id="tm-embd-1")])
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", embd_id="m1", tenant_embd_id="tm-embd-1")))

        class _Retriever:
            async def retrieval(self, *_args, **_kwargs):
                return {"chunks": [], "total": 0}

            def retrieval_by_children(self, chunks, *_args, **_kwargs):
                return chunks

        monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: SimpleNamespace())
        monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: {})
        monkeypatch.setattr(module.settings, "retriever", _Retriever())
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0, res["message"]
        assert res["data"]["chunks"] == []

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": True}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": "yes"}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`highlight` should be a boolean" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q", "highlight": 1}),
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "`highlight` should be a boolean" in res["message"]

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q"}),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (False, None))
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert "Dataset not found!" in res["message"]

        feature_calls = {"cross": None, "keyword": None, "retrieval_question": None}

        async def _cross_languages(_tenant_id, _dialog, question, langs):
            feature_calls["cross"] = tuple(langs)
            return f"{question}-xl"

        async def _keyword_extraction(_chat_mdl, question):
            feature_calls["keyword"] = question
            return "-kw"

        class _FeatureRetriever:
            async def retrieval(self, question, *_args, **_kwargs):
                feature_calls["retrieval_question"] = question
                return {
                    "chunks": [
                        {
                            "chunk_id": "c1",
                            "content_with_weight": "content",
                            "doc_id": "doc-1",
                            "kb_id": "ds-1",
                            "vector": [1, 2],
                        }
                    ],
                    "total": 1,
                }

            async def retrieval_by_toc(self, question, chunks, tenant_ids, _chat_mdl, size):
                assert question == "q-xl-kw"
                assert chunks and tenant_ids
                assert size == 30
                return [
                    {
                        "chunk_id": "toc-1",
                        "content_with_weight": "toc content",
                        "doc_id": "doc-toc",
                        "kb_id": "ds-1",
                    }
                ]

            def retrieval_by_children(self, chunks, _tenant_ids):
                return chunks + [
                    {
                        "chunk_id": "child-1",
                        "content_with_weight": "child content",
                        "doc_id": "doc-child",
                        "kb_id": "ds-1",
                    }
                ]

        class _FeatureKgRetriever:
            async def retrieval(self, *_args, **_kwargs):
                return {
                    "chunk_id": "kg-1",
                    "content_with_weight": "kg content",
                    "doc_id": "doc-kg",
                    "kb_id": "ds-1",
                }

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue(
                {
                    "dataset_ids": ["ds-1"],
                    "question": "q",
                    "rerank_id": "rerank-1",
                    "cross_languages": ["fr"],
                    "keyword": True,
                    "toc_enhance": True,
                    "use_kg": True,
                    "reference_metadata": {"include": True, "fields": ["author"]},
                }
            ),
        )
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, SimpleNamespace(tenant_id="tenant-1", embd_id="m1", tenant_embd_id="tm-embd-1")))
        monkeypatch.setattr(module, "cross_languages", _cross_languages)
        monkeypatch.setattr(module, "keyword_extraction", _keyword_extraction)
        monkeypatch.setattr(module.settings, "retriever", _FeatureRetriever())
        monkeypatch.setattr(module.settings, "kg_retriever", _FeatureKgRetriever())
        monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: {})
        monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: SimpleNamespace())
        monkeypatch.setattr(
            module.DocMetadataService,
            "get_metadata_for_documents",
            lambda _doc_ids, _kb_id: {
                "doc-1": {"author": "alice", "year": "2025"},
                "doc-toc": {"author": "bob"},
                "doc-child": {"author": "carol"},
                "doc-kg": {"author": "kg-author"},
            },
        )
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == 0, res["message"]
        assert feature_calls["cross"] == ("fr",)
        assert feature_calls["keyword"] == "q-xl"
        assert feature_calls["retrieval_question"] == "q-xl-kw"
        assert res["data"]["chunks"][0]["id"] == "kg-1"
        assert res["data"]["chunks"][0]["content"] == "kg content"
        assert res["data"]["chunks"][0]["document_metadata"]["author"] == "kg-author"
        assert any(chunk["id"] == "toc-1" for chunk in res["data"]["chunks"])
        assert any(chunk["id"] == "child-1" for chunk in res["data"]["chunks"])

        class _NotFoundRetriever:
            async def retrieval(self, *_args, **_kwargs):
                raise Exception("boom not_found boom")

            def retrieval_by_children(self, chunks, *_args, **_kwargs):
                return chunks

        monkeypatch.setattr(
            module,
            "get_request_json",
            lambda: _AwaitableValue({"dataset_ids": ["ds-1"], "question": "q"}),
        )
        monkeypatch.setattr(module.settings, "retriever", _NotFoundRetriever())
        res = _run(module.retrieval_test.__wrapped__("tenant-1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "No chunk found! Check the chunk status please!" in res["message"]
