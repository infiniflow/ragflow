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
"""SSRF-guard regression tests for provider base_url handling (#22870).

A user-supplied provider ``base_url`` is dialed server-side during key
verification and model listing, so verify_api_key and list_provider_models
must reject URLs that resolve to non-public addresses before any network use,
while leaving factory-default flows (no base_url) untouched.

``api.apps.services.provider_api_service`` is loaded in isolation with the DB
services and model registries stubbed so only the real SSRF guard runs.
"""

import asyncio
import importlib.util
import socket
import sys
from contextlib import contextmanager
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import patch

_REPO_ROOT = Path(__file__).resolve().parents[4]
_FACTORY_NAME = "TestFactory"


def _stub_module(name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    sys.modules[name] = mod
    return mod


def _load_service_module():
    for pkg in ("api", "api.db", "api.db.joint_services", "api.db.services", "api.utils", "rag"):
        sys.modules.setdefault(pkg, ModuleType(pkg))

    _stub_module("common.settings", FACTORY_LLM_INFOS=[{"name": _FACTORY_NAME, "llm": [], "url": ""}])
    _stub_module(
        "common.constants",
        LLMType=SimpleNamespace(
            IMAGE=SimpleNamespace(value="image"),
            CHAT=SimpleNamespace(value="chat"),
            EMBEDDING=SimpleNamespace(value="embedding"),
            RERANK=SimpleNamespace(value="rerank"),
            TTS=SimpleNamespace(value="tts"),
            SPEECH2TEXT=SimpleNamespace(value="speech2text"),
            MODERATION=SimpleNamespace(value="moderation"),
        ),
        ActiveStatusEnum=SimpleNamespace(ACTIVE=SimpleNamespace(value="1")),
        ModelVerifyStatusEnum=SimpleNamespace(UNKNOWN=SimpleNamespace(value="unknown"), SUCCESS=SimpleNamespace(value="success"), FAIL=SimpleNamespace(value="fail")),
    )
    _stub_module("api.db.db_models", DB=SimpleNamespace())
    _stub_module(
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *_a, **_k: {},
        delete_models_by_instance_ids=lambda *_a, **_k: None,
        delete_instances_by_provider_ids=lambda *_a, **_k: None,
    )
    _stub_module(
        "api.db.services.tenant_model_provider_service",
        TenantModelProviderService=SimpleNamespace(get_by_id=lambda _id: (False, None)),
    )
    _stub_module("api.db.services.tenant_model_instance_service", TenantModelInstanceService=SimpleNamespace())
    _stub_module("api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub_module(
        "api.utils.model_utils",
        get_model_type_human=lambda *_a, **_k: "",
        calculate_model_type=lambda *_a, **_k: 0,
    )
    _stub_module(
        "rag.llm",
        ChatModel={},
        CvModel={},
        EmbeddingModel={},
        ModelMeta={},
        OcrModel={},
        RerankModel={},
        Seq2txtModel={},
        TTSModel={},
    )

    spec = importlib.util.spec_from_file_location(
        "provider_api_service_ssrf_uut", _REPO_ROOT / "api" / "apps" / "services" / "provider_api_service.py"
    )
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


_service = _load_service_module()


@contextmanager
def _resolve_to(*ips):
    addrs = list(ips)

    def fake_getaddrinfo(host, port, *args, **kwargs):
        ip = addrs.pop(0) if len(addrs) > 1 else addrs[0]
        return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (ip, 0))]

    with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=fake_getaddrinfo):
        yield


def test_verify_api_key_rejects_private_base_url():
    with _resolve_to("10.0.0.5"):
        success, msg, _ = asyncio.run(_service.verify_api_key(_FACTORY_NAME, "sk-probe", "http://internal-host/v1"))
    assert success is False
    assert "rejected" in msg


def test_verify_api_key_rejects_metadata_base_url():
    with _resolve_to("169.254.169.254"):
        success, msg, _ = asyncio.run(
            _service.verify_api_key(_FACTORY_NAME, "sk-probe", "http://metadata.internal/v1")
        )
    assert success is False
    assert "rejected" in msg


def test_verify_api_key_rejects_loopback_base_url():
    with _resolve_to("127.0.0.1"):
        success, msg, _ = asyncio.run(_service.verify_api_key(_FACTORY_NAME, "sk-probe", "http://127.0.0.1:11434/v1"))
    assert success is False
    assert "rejected" in msg


def test_verify_api_key_public_base_url_passes_guard():
    with _resolve_to("93.184.216.34"):
        _success, msg, _ = asyncio.run(
            _service.verify_api_key(_FACTORY_NAME, "sk-probe", "https://example.com/v1")
        )
    assert "rejected" not in msg


def test_verify_api_key_without_base_url_untouched():
    with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=AssertionError("resolver called")):
        _success, msg, _ = asyncio.run(_service.verify_api_key(_FACTORY_NAME, "sk-probe", ""))
    assert "rejected" not in msg


def test_list_provider_models_rejects_private_base_url():
    with _resolve_to("10.0.0.5"):
        success, msg = asyncio.run(_service.list_provider_models(_FACTORY_NAME, "sk-probe", "http://internal-host/v1"))
    assert success is False
    assert "rejected" in msg


def test_list_provider_models_without_base_url_uses_factory_default():
    with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=AssertionError("resolver called")):
        success, models = asyncio.run(_service.list_provider_models(_FACTORY_NAME, "sk-probe"))
    assert success is True
    assert models == []
