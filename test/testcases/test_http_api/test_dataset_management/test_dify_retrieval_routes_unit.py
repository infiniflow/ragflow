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
import importlib.util
import inspect
import sys
from copy import deepcopy
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


class _DummyKB:
    def __init__(self, tenant_id="tenant-1", embd_id="embd-1", tenant_embd_id=1):
        self.tenant_id = tenant_id
        self.embd_id = embd_id
        self.tenant_embd_id = tenant_embd_id


class _DummyRetriever:
    async def retrieval(self, *_args, **_kwargs):
        return {
            "chunks": [
                {"doc_id": "doc-1", "content_with_weight": "chunk-content", "similarity": 0.8, "docnm_kwd": "doc-title", "vector": [0.1]}
            ]
        }

    def retrieval_by_children(self, chunks, _tenant_ids):
        return chunks


def _run(coro):
    return asyncio.run(coro)


def _load_dify_retrieval_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

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
            self.model_type = "chat"
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
                "id": self.id
            }
    
    class _StubTenantService:
        @staticmethod
        def get_by_id(tenant_id):
            # Return a mock tenant with default model configurations
            return True, SimpleNamespace(
                id=tenant_id,
                llm_id="chat-model",
                embd_id="embd-model",
                asr_id="asr-model",
                img2txt_id="img2txt-model",
                rerank_id="rerank-model",
                tts_id="tts-model"
            )
    
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
    
    tenant_llm_service_mod.TenantService = _StubTenantService
    tenant_llm_service_mod.TenantLLMService = _StubTenantLLMService

    class _StubLLMFactoriesService:
        pass

    tenant_llm_service_mod.LLMFactoriesService = _StubLLMFactoriesService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    # Mock llm_service for LLMService
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
            return [np.array([0.1, 0.2, 0.3]) for _ in texts], len(texts) * 10
    
    llm_service_mod.LLMService = SimpleNamespace(
        query=lambda llm_name: [_StubLLM(llm_name)] if llm_name else []
    )
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
            self.model_type = "chat"
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
                "id": self.id
            }
    
    def _get_model_config_by_id(tenant_model_id: int) -> dict:
        return _MockModelConfig2("tenant-1", "model-1").to_dict()
    
    def _get_model_config_by_type_and_name(tenant_id: str, model_type: str, model_name: str):
        if not model_name:
            raise Exception("Model Name is required")
        return _MockModelConfig2(tenant_id, model_name).to_dict()
    
    def _get_tenant_default_model_by_type(tenant_id: str, model_type):
        # Return mock tenant with default model configurations
        return _MockModelConfig2(tenant_id, "chat-model").to_dict()
    
    tenant_model_service_mod.get_model_config_by_id = _get_model_config_by_id
    tenant_model_service_mod.get_model_config_by_type_and_name = _get_model_config_by_type_and_name
    tenant_model_service_mod.get_tenant_default_model_by_type = _get_tenant_default_model_by_type
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    module_name = "test_dify_retrieval_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "sdk" / "dify_retrieval.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


@pytest.mark.p2
def test_retrieval_success_with_metadata_and_kg(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    _set_request_json(
        monkeypatch,
        module,
        {
            "knowledge_id": "kb-1",
            "query": "hello",
            "use_kg": True,
            "retrieval_setting": {"score_threshold": 0.1, "top_k": 3},
            "metadata_condition": {"conditions": [{"name": "author", "comparison_operator": "is", "value": "alice"}], "logic": "and"},
        },
    )

    monkeypatch.setattr(module, "jsonify", lambda payload: payload)
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [{"doc_id": "doc-1"}])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _DummyKB()))
    monkeypatch.setattr(module, "convert_conditions", lambda cond: cond.get("conditions", []))
    monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])

    retriever = _DummyRetriever()
    monkeypatch.setattr(module.settings, "retriever", retriever)

    class _DummyKgRetriever:
        async def retrieval(self, *_args, **_kwargs):
            return {
                "doc_id": "doc-2",
                "content_with_weight": "kg-content",
                "similarity": 0.9,
                "docnm_kwd": "kg-title",
            }

    monkeypatch.setattr(module.settings, "kg_retriever", _DummyKgRetriever())
    monkeypatch.setattr(
        module.DocumentService,
        "get_by_id",
        lambda doc_id: (True, SimpleNamespace(meta_fields={"origin": f"meta-{doc_id}"})),
    )
    monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: [])

    res = _run(inspect.unwrap(module.retrieval)("tenant-1"))
    assert "records" in res, res
    assert len(res["records"]) == 2, res
    top = res["records"][0]
    assert top["title"] == "kg-title", res
    assert top["metadata"]["doc_id"] == "doc-2", res
    assert "score" in top, res


@pytest.mark.p2
def test_retrieval_kb_not_found(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"knowledge_id": "kb-missing", "query": "hello"})
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))

    res = _run(inspect.unwrap(module.retrieval)("tenant-1"))
    assert res["code"] == module.RetCode.NOT_FOUND, res
    assert "Knowledgebase not found" in res["message"], res


@pytest.mark.p2
def test_retrieval_not_found_exception_mapping(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"knowledge_id": "kb-1", "query": "hello"})
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _DummyKB()))
    monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: [])

    class _BrokenRetriever:
        async def retrieval(self, *_args, **_kwargs):
            raise RuntimeError("chunk_not_found_error")

    monkeypatch.setattr(module.settings, "retriever", _BrokenRetriever())

    res = _run(inspect.unwrap(module.retrieval)("tenant-1"))
    assert res["code"] == module.RetCode.NOT_FOUND, res
    assert "No chunk found" in res["message"], res


@pytest.mark.p2
def test_retrieval_generic_exception_mapping(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    _set_request_json(monkeypatch, module, {"knowledge_id": "kb-1", "query": "hello"})
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kbs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, _DummyKB()))
    monkeypatch.setattr(module, "label_question", lambda *_args, **_kwargs: [])

    class _BrokenRetriever:
        async def retrieval(self, *_args, **_kwargs):
            raise RuntimeError("boom")

    monkeypatch.setattr(module.settings, "retriever", _BrokenRetriever())

    res = _run(inspect.unwrap(module.retrieval)("tenant-1"))
    assert res["code"] == module.RetCode.SERVER_ERROR, res
    assert "boom" in res["message"], res


@pytest.mark.p2
def test_read_retrieval_request_from_get_args(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            method="GET",
            args={
                "knowledge_id": "kb-1",
                "query": "hello",
                "use_kg": "true",
                "top_k": "12",
                "score_threshold": "0.66",
            },
        ),
    )

    req = _run(module._read_retrieval_request())
    assert req["knowledge_id"] == "kb-1", req
    assert req["query"] == "hello", req
    assert req["use_kg"] is True, req
    assert req["retrieval_setting"]["top_k"] == 12, req
    assert req["retrieval_setting"]["score_threshold"] == 0.66, req


@pytest.mark.p2
def test_read_retrieval_request_from_post_json(monkeypatch):
    module = _load_dify_retrieval_module(monkeypatch)
    payload = {"knowledge_id": "kb-1", "query": "hello"}
    monkeypatch.setattr(module, "request", SimpleNamespace(method="POST", args={}))
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(payload))

    req = _run(module._read_retrieval_request())
    assert req == payload, req
