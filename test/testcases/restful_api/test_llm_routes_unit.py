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
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest
import requests

from configs import HOST_ADDRESS, VERSION
from test.testcases.conftest import login
from test.testcases.libs.auth import RAGFlowWebApiAuth


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


class _StrEnum(str):
    @property
    def value(self):
        return str(self)


class _DummyTenantLLMModel:
    tenant_id = _ExprField("tenant_id")
    llm_factory = _ExprField("llm_factory")
    llm_name = _ExprField("llm_name")

    def __init__(self, id=None, **kwargs):
        self.id = id
        self.api_key = None
        self.status = None
        for key, value in kwargs.items():
            setattr(self, key, value)


class _TenantLLMRow:
    def __init__(
        self,
        *,
        id,
        llm_name,
        llm_factory,
        model_type,
        api_key="key",
        status="1",
        used_tokens=0,
        api_base="",
        max_tokens=8192,
    ):
        self.id = id
        self.llm_name = llm_name
        self.llm_factory = llm_factory
        self.model_type = model_type
        self.api_key = api_key
        self.status = status
        self.used_tokens = used_tokens
        self.api_base = api_base
        self.max_tokens = max_tokens

    def to_dict(self):
        return {
            "id": self.id,
            "llm_name": self.llm_name,
            "llm_factory": self.llm_factory,
            "model_type": self.model_type,
            "status": self.status,
            "used_tokens": self.used_tokens,
            "api_base": self.api_base,
            "max_tokens": self.max_tokens,
        }


class _LLMRow:
    def __init__(self, *, llm_name, fid, model_type, status="1", max_tokens=2048):
        self.llm_name = llm_name
        self.fid = fid
        self.model_type = model_type
        self.status = status
        self.max_tokens = max_tokens

    def to_dict(self):
        return {
            "llm_name": self.llm_name,
            "fid": self.fid,
            "model_type": self.model_type,
            "status": self.status,
            "max_tokens": self.max_tokens,
        }


def _run(coro):
    return asyncio.run(coro)


def _set_request_json(monkeypatch, module, payload):
    async def _get_request_json():
        return dict(payload)

    monkeypatch.setattr(module, "get_request_json", _get_request_json)


def _load_llm_app(monkeypatch):
    repo_root = Path(__file__).resolve().parents[3]

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
        def ensure_opendataloader_from_env(_tenant_id):
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

        @staticmethod
        def filter_update(_filters, _payload):
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
        CHAT=_StrEnum("chat"),
        EMBEDDING=_StrEnum("embedding"),
        SPEECH2TEXT=_StrEnum("speech2text"),
        IMAGE2TEXT=_StrEnum("image2text"),
        RERANK=_StrEnum("rerank"),
        TTS=_StrEnum("tts"),
        OCR=_StrEnum("ocr"),
    )
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    db_models_mod = ModuleType("api.db.db_models")
    db_models_mod.TenantLLM = _DummyTenantLLMModel
    monkeypatch.setitem(sys.modules, "api.db.db_models", db_models_mod)

    base64_mod = ModuleType("rag.utils.base64_image")
    base64_mod.test_image = b"image-bytes"
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
    spec = importlib.util.spec_from_file_location("test_llm_routes_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    spec.loader.exec_module(module)
    return module


def _rag_llm_module():
    return sys.modules["rag.llm"]


@pytest.mark.p2
def test_llm_list_app_grouping_availability_and_merge(monkeypatch):
    module = _load_llm_app(monkeypatch)

    ensure_calls = []
    monkeypatch.setattr(module.TenantLLMService, "ensure_mineru_from_env", lambda tenant_id: ensure_calls.append(tenant_id))

    tenant_rows = [
        _TenantLLMRow(id=1, llm_name="fast-emb", llm_factory="FastEmbed", model_type="embedding", api_key="k1", status="1"),
        _TenantLLMRow(id=2, llm_name="tenant-only", llm_factory="CustomFactory", model_type="chat", api_key="k2", status="1"),
        _TenantLLMRow(id=3, llm_name="gpt-5.5", llm_factory="OpenAI", model_type="chat", api_key="k3", status="1"),
        _TenantLLMRow(id=4, llm_name="gpt-5.4", llm_factory="OpenAI", model_type="chat", api_key="k4", status="1"),
    ]
    monkeypatch.setattr(module.TenantLLMService, "query", lambda **_kwargs: tenant_rows)

    all_llms = [
        _LLMRow(llm_name="tei-embed", fid="Builtin", model_type="embedding", status="1"),
        _LLMRow(llm_name="fast-emb", fid="FastEmbed", model_type="embedding", status="1"),
        _LLMRow(llm_name="gpt-5.5", fid="OpenAI", model_type="chat", status="1"),
        _LLMRow(llm_name="gpt-5.4", fid="OpenAI", model_type="chat", status="1"),
        _LLMRow(llm_name="not-in-status", fid="Other", model_type="chat", status="1"),
    ]
    monkeypatch.setattr(module.LLMService, "get_all", lambda: all_llms)

    monkeypatch.setattr(module, "request", SimpleNamespace(args={}))
    monkeypatch.setenv("COMPOSE_PROFILES", "tei-cpu")
    monkeypatch.setenv("TEI_MODEL", "tei-embed")

    res = _run(module.list_app())
    assert res["code"] == 0, res["message"]
    assert ensure_calls == ["tenant-1"]

    data = res["data"]
    assert {"Builtin", "FastEmbed", "CustomFactory", "OpenAI"}.issubset(set(data.keys()))
    assert data["Builtin"][0]["llm_name"] == "tei-embed"
    assert data["Builtin"][0]["available"] is True
    assert data["FastEmbed"][0]["llm_name"] == "fast-emb"
    assert data["FastEmbed"][0]["available"] is True
    assert data["CustomFactory"][0]["llm_name"] == "tenant-only"
    assert data["CustomFactory"][0]["available"] is True
    openai_names = {item["llm_name"] for item in data["OpenAI"]}
    assert {"gpt-5.5", "gpt-5.4"}.issubset(openai_names)


@pytest.mark.p2
def test_llm_list_app_model_type_filter(monkeypatch):
    module = _load_llm_app(monkeypatch)

    monkeypatch.setattr(module.TenantLLMService, "ensure_mineru_from_env", lambda _tenant_id: None)
    monkeypatch.setattr(
        module.TenantLLMService,
        "query",
        lambda **_kwargs: [
            _TenantLLMRow(id=1, llm_name="fast-emb", llm_factory="FastEmbed", model_type="embedding", api_key="k1", status="1"),
            _TenantLLMRow(id=2, llm_name="tenant-only", llm_factory="CustomFactory", model_type="chat", api_key="k2", status="1"),
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
    assert res["code"] == 0, res["message"]
    assert list(res["data"].keys()) == ["CustomFactory"]
    assert res["data"]["CustomFactory"][0]["model_type"] == "chat"


@pytest.mark.p2
def test_llm_list_app_exception_path(monkeypatch):
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


@pytest.mark.p2
def test_llm_factories_route_success_and_exception_unit(monkeypatch):
    module = _load_llm_app(monkeypatch)

    def _factory(name):
        return SimpleNamespace(name=name, to_dict=lambda n=name: {"name": n})

    monkeypatch.setattr(
        module,
        "get_allowed_llm_factories",
        lambda: [
            _factory("OpenAI"),
            _factory("CustomFactory"),
            _factory("FastEmbed"),
            _factory("Builtin"),
        ],
    )
    monkeypatch.setattr(
        module.LLMService,
        "get_all",
        lambda: [
            _LLMRow(llm_name="m1", fid="OpenAI", model_type="chat", status="1"),
            _LLMRow(llm_name="m2", fid="OpenAI", model_type="embedding", status="1"),
            _LLMRow(llm_name="m3", fid="OpenAI", model_type="rerank", status="0"),
        ],
    )
    res = module.factories()
    assert res["code"] == 0
    names = [item["name"] for item in res["data"]]
    assert "FastEmbed" not in names
    assert "Builtin" not in names
    assert {"OpenAI", "CustomFactory"} == set(names)
    openai = next(item for item in res["data"] if item["name"] == "OpenAI")
    assert {"chat", "embedding"} == set(openai["model_types"])

    monkeypatch.setattr(module, "get_allowed_llm_factories", lambda: (_ for _ in ()).throw(RuntimeError("factories boom")))
    res = module.factories()
    assert res["code"] == 500
    assert "factories boom" in res["message"]


@pytest.mark.p2
def test_add_llm_factory_specific_key_assembly_unit(monkeypatch):
    module = _load_llm_app(monkeypatch)

    async def _wait_for(coro, *_args, **_kwargs):
        return await coro

    async def _to_thread(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    monkeypatch.setattr(module.asyncio, "wait_for", _wait_for)
    monkeypatch.setattr(module.asyncio, "to_thread", _to_thread)

    allowed = [
        "VolcEngine",
        "Tencent Cloud",
        "Bedrock",
        "LocalAI",
        "HuggingFace",
        "OpenAI-API-Compatible",
        "VLLM",
        "XunFei Spark",
        "BaiduYiyan",
        "Fish Audio",
        "Google Cloud",
        "Azure-OpenAI",
        "OpenRouter",
        "MinerU",
        "PaddleOCR",
    ]
    monkeypatch.setattr(module, "get_allowed_llm_factories", lambda: [SimpleNamespace(name=name) for name in allowed])

    captured = {"filter_payloads": []}

    class _ChatOK:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, *_args, **_kwargs):
            return "ok", 1

        async def async_chat_streamly(self, *_args, **_kwargs):
            yield "ok"
            yield 1

    class _TTSOK:
        def __init__(self, *_args, **_kwargs):
            pass

        def tts(self, _text):
            yield b"ok"

    monkeypatch.setattr(_rag_llm_module(), "ChatModel", {name: _ChatOK for name in allowed})
    monkeypatch.setattr(_rag_llm_module(), "TTSModel", {"XunFei Spark": _TTSOK})
    monkeypatch.setattr(module.TenantLLMService, "filter_update", lambda _filters, payload: captured["filter_payloads"].append(dict(payload)) or True)

    reject_req = {"llm_factory": "NotAllowed", "llm_name": "x", "model_type": module.LLMType.CHAT.value}
    _set_request_json(monkeypatch, module, reject_req)
    res = _run(module.add_llm())
    assert res["code"] == 400
    assert "is not allowed" in res["message"]

    def _run_case(factory, *, model_type=module.LLMType.CHAT.value, extra=None):
        req = {"llm_factory": factory, "llm_name": "model", "model_type": model_type, "api_key": "k", "api_base": "http://api"}
        if extra:
            req.update(extra)
        _set_request_json(monkeypatch, module, req)
        out = _run(module.add_llm())
        assert out["code"] == 0
        assert out["data"] is True
        return captured["filter_payloads"][-1]

    volc = _run_case("VolcEngine", extra={"ark_api_key": "ak", "endpoint_id": "eid"})
    assert json.loads(volc["api_key"]) == {"ark_api_key": "ak", "endpoint_id": "eid"}

    bedrock = _run_case(
        "Bedrock",
        extra={"auth_mode": "iam", "bedrock_ak": "ak", "bedrock_sk": "sk", "bedrock_region": "r", "aws_role_arn": "arn"},
    )
    assert json.loads(bedrock["api_key"]) == {
        "auth_mode": "iam",
        "bedrock_ak": "ak",
        "bedrock_sk": "sk",
        "bedrock_region": "r",
        "aws_role_arn": "arn",
    }

    assert _run_case("LocalAI")["llm_name"] == "model___LocalAI"
    assert _run_case("HuggingFace")["llm_name"] == "model___HuggingFace"
    assert _run_case("OpenAI-API-Compatible")["llm_name"] == "model___OpenAI-API"
    assert _run_case("VLLM")["llm_name"] == "model___VLLM"

    spark_chat = _run_case("XunFei Spark", extra={"spark_api_password": "spark-pass"})
    assert spark_chat["api_key"] == "spark-pass"
    spark_tts = _run_case(
        "XunFei Spark",
        model_type=module.LLMType.TTS.value,
        extra={"spark_app_id": "app", "spark_api_secret": "secret", "spark_api_key": "key"},
    )
    assert json.loads(spark_tts["api_key"]) == {
        "spark_app_id": "app",
        "spark_api_secret": "secret",
        "spark_api_key": "key",
    }

    assert json.loads(_run_case("BaiduYiyan", extra={"yiyan_ak": "ak", "yiyan_sk": "sk"})["api_key"]) == {"yiyan_ak": "ak", "yiyan_sk": "sk"}
    assert json.loads(_run_case("Fish Audio", extra={"fish_audio_ak": "ak", "fish_audio_refid": "rid"})["api_key"]) == {"fish_audio_ak": "ak", "fish_audio_refid": "rid"}
    assert json.loads(
        _run_case("Google Cloud", extra={"google_project_id": "pid", "google_region": "us", "google_service_account_key": "sak"})["api_key"]
    ) == {
        "google_project_id": "pid",
        "google_region": "us",
        "google_service_account_key": "sak",
    }
    assert json.loads(_run_case("Azure-OpenAI", extra={"api_key": "real-key", "api_version": "2024-01-01"})["api_key"]) == {
        "api_key": "real-key",
        "api_version": "2024-01-01",
    }
    assert json.loads(_run_case("OpenRouter", extra={"api_key": "or-key", "provider_order": "a,b"})["api_key"]) == {
        "api_key": "or-key",
        "provider_order": "a,b",
    }
    assert json.loads(_run_case("MinerU", extra={"api_key": "m-key", "provider_order": "p1"})["api_key"]) == {
        "api_key": "m-key",
        "provider_order": "p1",
    }
    assert json.loads(_run_case("PaddleOCR", extra={"api_key": "p-key", "provider_order": "p2"})["api_key"]) == {
        "api_key": "p-key",
        "provider_order": "p2",
    }

    tencent_req = {
        "llm_factory": "Tencent Cloud",
        "llm_name": "model",
        "model_type": module.LLMType.CHAT.value,
        "tencent_cloud_sid": "sid",
        "tencent_cloud_sk": "sk",
    }

    async def _tencent_request_json():
        return tencent_req

    monkeypatch.setattr(module, "get_request_json", _tencent_request_json)
    delegated = {}

    async def _fake_set_api_key():
        delegated["api_key"] = tencent_req.get("api_key")
        return {"code": 0, "data": "delegated"}

    monkeypatch.setattr(module, "set_api_key", _fake_set_api_key)
    res = _run(module.add_llm())
    assert res["code"] == 0
    assert res["data"] == "delegated"
    assert json.loads(delegated["api_key"]) == {"tencent_cloud_sid": "sid", "tencent_cloud_sk": "sk"}


@pytest.mark.p2
def test_add_llm_model_type_probe_and_persistence_matrix_unit(monkeypatch):
    module = _load_llm_app(monkeypatch)

    async def _wait_for(coro, *_args, **_kwargs):
        return await coro

    async def _to_thread(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    monkeypatch.setattr(module.asyncio, "wait_for", _wait_for)
    monkeypatch.setattr(module.asyncio, "to_thread", _to_thread)
    monkeypatch.setattr(
        module,
        "get_allowed_llm_factories",
        lambda: [
            SimpleNamespace(name=name)
            for name in [
                "FEmbFail",
                "FEmbPass",
                "FChatFail",
                "FChatPass",
                "FRKey",
                "FRFail",
                "FImgFail",
                "FTTSFail",
                "FOcrFail",
                "FSttFail",
                "FUnknown",
            ]
        ],
    )

    class _EmbeddingFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def encode(self, _texts):
            return [[]], 1

    class _EmbeddingPass:
        def __init__(self, *_args, **_kwargs):
            pass

        def encode(self, _texts):
            return [[0.5]], 1

    class _ChatFail:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, *_args, **_kwargs):
            return "**ERROR**: chat failed", 0

        async def async_chat_streamly(self, *_args, **_kwargs):
            yield "**ERROR**: chat failed"
            yield 0

    class _ChatPass:
        def __init__(self, *_args, **_kwargs):
            pass

        async def async_chat(self, *_args, **_kwargs):
            return "ok", 1

        async def async_chat_streamly(self, *_args, **_kwargs):
            yield "ok"
            yield 1

    class _RerankFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def similarity(self, *_args, **_kwargs):
            return [], 1

    class _CvFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def describe(self, _image_data):
            return "**ERROR**: image failed", 0

    class _TTSFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def tts(self, _text):
            raise RuntimeError("tts failed")

    class _OcrFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def __call__(self, _img):
            return None

    class _SttFail:
        def __init__(self, *_args, **_kwargs):
            pass

        def transcribe(self, _audio):
            return "", 0

    rag_llm_mod = _rag_llm_module()
    monkeypatch.setattr(rag_llm_mod, "EmbeddingModel", {"FEmbFail": _EmbeddingFail, "FEmbPass": _EmbeddingPass})
    monkeypatch.setattr(rag_llm_mod, "ChatModel", {"FChatFail": _ChatFail, "FChatPass": _ChatPass})
    monkeypatch.setattr(rag_llm_mod, "RerankModel", {"FRFail": _RerankFail, "FRKey": _RerankFail})
    monkeypatch.setattr(rag_llm_mod, "CvModel", {"FImgFail": _CvFail})
    monkeypatch.setattr(rag_llm_mod, "TTSModel", {"FTTSFail": _TTSFail})
    monkeypatch.setattr(rag_llm_mod, "OcrModel", {"FOcrFail": _OcrFail})
    monkeypatch.setattr(rag_llm_mod, "Seq2txtModel", {"FSttFail": _SttFail})

    saves = []
    monkeypatch.setattr(module.TenantLLMService, "filter_update", lambda _filters, _payload: False)
    monkeypatch.setattr(module.TenantLLMService, "save", lambda **kwargs: saves.append(kwargs) or True)

    monkeypatch.setattr(
        module.LLMService,
        "query",
        lambda **kwargs: [] if kwargs.get("llm_factory") == "FUnknown" else [
            _LLMRow(llm_name="m", fid=kwargs.get("llm_factory"), model_type=kwargs.get("model_type", module.LLMType.CHAT.value), max_tokens=4096)
        ],
    )

    _set_request_json(monkeypatch, module, {"llm_factory": "FUnknown", "llm_name": "m", "model_type": "unknown"})
    with pytest.raises(RuntimeError, match="Unknown model type: unknown"):
        _run(module.add_llm())

    cases = [
        ("FEmbFail", module.LLMType.EMBEDDING.value, 400, None, "embedding model"),
        ("FEmbPass", module.LLMType.EMBEDDING.value, 0, True, ""),
        ("FChatFail", module.LLMType.CHAT.value, 400, None, "No valid response received"),
        ("FChatPass", module.LLMType.CHAT.value, 0, True, ""),
        ("FRFail", module.LLMType.RERANK.value, 400, None, "Not known"),
        ("FImgFail", module.LLMType.IMAGE2TEXT.value, 400, None, "image failed"),
        ("FTTSFail", module.LLMType.TTS.value, 400, None, "tts"),
        ("FOcrFail", module.LLMType.OCR.value, 400, None, "ocr"),
        ("FSttFail", module.LLMType.SPEECH2TEXT.value, 0, True, ""),
    ]
    for factory, model_type, expected_code, expected_data, expected_fragment in cases:
        _set_request_json(monkeypatch, module, {"llm_factory": factory, "llm_name": "m", "model_type": model_type, "api_key": "key"})
        res = _run(module.add_llm())
        assert res["code"] == expected_code
        if expected_data is not None:
            assert res["data"] is expected_data
        if expected_fragment:
            assert expected_fragment.lower() in res["message"].lower()

    assert any(item["llm_factory"] == "FEmbPass" for item in saves)


@pytest.mark.p2
def test_llm_factories_live_auth_contract():
    llm_url = f"{HOST_ADDRESS}/{VERSION}/llm/factories"
    invalid_auth_cases = [
        (None, 401, "<Unauthorized '401: Unauthorized'>"),
        (RAGFlowWebApiAuth("invalid-token"), 401, "<Unauthorized '401: Unauthorized'>"),
    ]
    for auth_obj, expected_code, expected_message in invalid_auth_cases:
        res = requests.get(llm_url, auth=auth_obj, timeout=30)
        assert res.status_code == 401
        payload = res.json()
        assert payload["code"] == expected_code, payload
        assert payload["message"] == expected_message, payload

    ok_res = requests.get(llm_url, auth=RAGFlowWebApiAuth(login()), timeout=30)
    assert ok_res.status_code == 200
    ok_payload = ok_res.json()
    assert ok_payload["code"] == 0, ok_payload
    assert isinstance(ok_payload["data"], list), ok_payload


@pytest.mark.p2
def test_llm_list_live_auth_contract():
    llm_url = f"{HOST_ADDRESS}/{VERSION}/llm/list"
    invalid_auth_cases = [
        (None, 401, "<Unauthorized '401: Unauthorized'>"),
        (RAGFlowWebApiAuth("invalid-token"), 401, "<Unauthorized '401: Unauthorized'>"),
    ]
    for auth_obj, expected_code, expected_message in invalid_auth_cases:
        res = requests.get(llm_url, auth=auth_obj, timeout=30)
        assert res.status_code == 401
        payload = res.json()
        assert payload["code"] == expected_code, payload
        assert payload["message"] == expected_message, payload

    ok_res = requests.get(llm_url, auth=RAGFlowWebApiAuth(login()), timeout=30)
    assert ok_res.status_code == 200
    ok_payload = ok_res.json()
    assert ok_payload["code"] == 0, ok_payload
    assert isinstance(ok_payload["data"], dict), ok_payload
