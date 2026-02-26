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
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _ExprField:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return (self.name, other)


class _DummyTenantLLMModel:
    tenant_id = _ExprField("tenant_id")
    llm_factory = _ExprField("llm_factory")


class _TenantLLMRow:
    def __init__(self, *, llm_name, llm_factory, model_type, api_key="key", status="1"):
        self.llm_name = llm_name
        self.llm_factory = llm_factory
        self.model_type = model_type
        self.api_key = api_key
        self.status = status

    def to_dict(self):
        return {
            "llm_name": self.llm_name,
            "llm_factory": self.llm_factory,
            "model_type": self.model_type,
            "status": self.status,
        }


class _LLMRow:
    def __init__(self, *, llm_name, fid, model_type, status="1"):
        self.llm_name = llm_name
        self.fid = fid
        self.model_type = model_type
        self.status = status

    def to_dict(self):
        return {
            "llm_name": self.llm_name,
            "fid": self.fid,
            "model_type": self.model_type,
            "status": self.status,
        }


def _run(coro):
    return asyncio.run(coro)


def _load_llm_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args={})
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.login_required = lambda fn: fn
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    tenant_llm_mod = ModuleType("api.db.services.tenant_llm_service")

    class _StubLLMFactoriesService:
        @staticmethod
        def query(**_kwargs):
            return []

    class _StubTenantLLMService:
        @staticmethod
        def ensure_mineru_from_env(_tenant_id):
            return None

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_my_llms(_tenant_id):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def filter_delete(_filters):
            return True

    tenant_llm_mod.LLMFactoriesService = _StubLLMFactoriesService
    tenant_llm_mod.TenantLLMService = _StubTenantLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_mod)

    llm_service_mod = ModuleType("api.db.services.llm_service")

    class _StubLLMService:
        @staticmethod
        def get_all():
            return []

        @staticmethod
        def query(**_kwargs):
            return []

    llm_service_mod.LLMService = _StubLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")
    api_utils_mod.get_allowed_llm_factories = lambda: []
    api_utils_mod.get_data_error_result = lambda message="", code=400, data=None: {
        "code": code,
        "message": message,
        "data": data,
    }
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {
        "code": code,
        "message": message,
        "data": data,
    }

    async def _get_request_json():
        return {}

    api_utils_mod.get_request_json = _get_request_json
    api_utils_mod.server_error_response = lambda exc: {"code": 500, "message": str(exc), "data": None}
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda fn: fn)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.StatusEnum = SimpleNamespace(VALID=SimpleNamespace(value="1"), INVALID=SimpleNamespace(value="0"))
    constants_mod.LLMType = SimpleNamespace(
        CHAT="chat",
        EMBEDDING="embedding",
        SPEECH2TEXT="speech2text",
        IMAGE2TEXT="image2text",
        RERANK="rerank",
        TTS="tts",
        OCR="ocr",
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.TenantLLM = _DummyTenantLLMModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    base64_mod = ModuleType("rag.utils.base64_image")
    base64_mod.test_image = lambda _s: _s
    monkeypatch.setitem(sys.modules, "rag.utils.base64_image", base64_mod)

    rag_llm_mod = ModuleType("rag.llm")
    rag_llm_mod.EmbeddingModel = {}
    rag_llm_mod.ChatModel = {}
    rag_llm_mod.RerankModel = {}
    rag_llm_mod.CvModel = {}
    rag_llm_mod.TTSModel = {}
    rag_llm_mod.OcrModel = {}
    rag_llm_mod.Seq2txtModel = {}
    monkeypatch.setitem(sys.modules, "rag.llm", rag_llm_mod)

    module_path = repo_root / "api" / "apps" / "llm_app.py"
    spec = importlib.util.spec_from_file_location("test_llm_list_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_list_app_grouping_availability_and_merge(monkeypatch):
    module = _load_llm_app(monkeypatch)

    ensure_calls = []
    monkeypatch.setattr(module.TenantLLMService, "ensure_mineru_from_env", lambda tenant_id: ensure_calls.append(tenant_id))

    tenant_rows = [
        _TenantLLMRow(llm_name="fast-emb", llm_factory="FastEmbed", model_type="embedding", api_key="k1", status="1"),
        _TenantLLMRow(llm_name="tenant-only", llm_factory="CustomFactory", model_type="chat", api_key="k2", status="1"),
    ]
    monkeypatch.setattr(module.TenantLLMService, "query", lambda **_kwargs: tenant_rows)

    all_llms = [
        _LLMRow(llm_name="tei-embed", fid="Builtin", model_type="embedding", status="1"),
        _LLMRow(llm_name="fast-emb", fid="FastEmbed", model_type="embedding", status="1"),
        _LLMRow(llm_name="not-in-status", fid="Other", model_type="chat", status="1"),
    ]
    monkeypatch.setattr(module.LLMService, "get_all", lambda: all_llms)

    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    monkeypatch.setenv("COMPOSE_PROFILES", "tei-cpu")
    monkeypatch.setenv("TEI_MODEL", "tei-embed")

    res = _run(module.list_app())
    assert res["code"] == 0
    assert ensure_calls == ["tenant-1"]

    data = res["data"]
    assert {"Builtin", "FastEmbed", "CustomFactory"}.issubset(set(data.keys()))

    builtin = data["Builtin"][0]
    assert builtin["llm_name"] == "tei-embed"
    assert builtin["available"] is True

    fastembed = data["FastEmbed"][0]
    assert fastembed["llm_name"] == "fast-emb"
    assert fastembed["available"] is True

    tenant_only = data["CustomFactory"][0]
    assert tenant_only["llm_name"] == "tenant-only"
    assert tenant_only["available"] is True


@pytest.mark.p2
def test_list_app_model_type_filter(monkeypatch):
    module = _load_llm_app(monkeypatch)

    monkeypatch.setattr(module.TenantLLMService, "ensure_mineru_from_env", lambda _tenant_id: None)
    monkeypatch.setattr(
        module.TenantLLMService,
        "query",
        lambda **_kwargs: [
            _TenantLLMRow(llm_name="fast-emb", llm_factory="FastEmbed", model_type="embedding", api_key="k1", status="1"),
            _TenantLLMRow(llm_name="tenant-only", llm_factory="CustomFactory", model_type="chat", api_key="k2", status="1"),
        ],
    )
    monkeypatch.setattr(
        module.LLMService,
        "get_all",
        lambda: [
            _LLMRow(llm_name="tei-embed", fid="Builtin", model_type="embedding", status="1"),
            _LLMRow(llm_name="fast-emb", fid="FastEmbed", model_type="embedding", status="1"),
        ],
    )

    monkeypatch.setattr(module, "request", SimpleNamespace(args={"model_type": "chat"}))
    res = _run(module.list_app())
    assert res["code"] == 0
    assert list(res["data"].keys()) == ["CustomFactory"]
    assert res["data"]["CustomFactory"][0]["model_type"] == "chat"


@pytest.mark.p2
def test_list_app_exception_path(monkeypatch):
    module = _load_llm_app(monkeypatch)

    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    monkeypatch.setattr(module.TenantLLMService, "ensure_mineru_from_env", lambda _tenant_id: None)
    monkeypatch.setattr(
        module.TenantLLMService,
        "query",
        lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("query boom")),
    )

    res = _run(module.list_app())
    assert res["code"] == 500
    assert "query boom" in res["message"]
