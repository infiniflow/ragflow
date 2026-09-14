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
"""Regression tests for dynamic model discovery inside `verify_api_key`
(api/apps/services/provider_api_service.py).

Providers with an empty static catalogue discover their models by calling the
base URL the user typed. That call fails for mundane reasons: the host is not
reachable from inside the container, the port is closed, TLS is wrong. The
failure used to be swallowed by a bare `except Exception: pass` and reported as
a flat "No models found for provider 'X'", which reads as a credential or
catalogue problem and sends people hunting for a bad API key.
"""

import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

pytestmark = pytest.mark.p2

PROVIDER = "LM-Studio"
BASE_URL = "http://127.0.0.1:1234/v1"


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


def _load_service(monkeypatch, discovery):
    """Load provider_api_service with `ModelMeta[PROVIDER]` bound to `discovery`.

    `discovery` is a zero-argument coroutine function standing in for
    `get_model_list`, so a test can make discovery raise or come back empty.
    """

    class _Meta:
        def __init__(self, api_key, base_url):
            self.api_key = api_key
            self.base_url = base_url

        async def get_model_list(self):
            return await discovery()

    _stub(monkeypatch, "common.settings", FACTORY_LLM_INFOS=[{"name": PROVIDER, "llm": [], "url": ""}])
    _stub(monkeypatch, "api.db.db_models", DB=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        resolve_model_config=lambda *_a, **_k: {},
        delete_models_by_instance_ids=lambda *_a, **_k: None,
        delete_instances_by_provider_ids=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.tenant_model_provider_service", TenantModelProviderService=SimpleNamespace(get_by_id=lambda _id: (False, None)))
    _stub(monkeypatch, "api.db.services.tenant_model_instance_service", TenantModelInstanceService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.tenant_model_service", TenantModelService=SimpleNamespace())
    _stub(monkeypatch, "api.utils.model_utils", get_model_type_human=lambda *_a, **_k: "", calculate_model_type=lambda *_a, **_k: 0)
    _stub(
        monkeypatch,
        "rag.llm",
        ChatModel={},
        CvModel={},
        EmbeddingModel={},
        ModelMeta={PROVIDER: _Meta},
        OcrModel={},
        RerankModel={},
        Seq2txtModel={},
        TTSModel={},
    )

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "services" / "provider_api_service.py"
    spec = importlib.util.spec_from_file_location("test_provider_api_service_verify_discovery_mod", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_provider_api_service_verify_discovery_mod", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.asyncio
async def test_unreachable_base_url_is_reported_instead_of_swallowed(monkeypatch):
    """The connection error and the URL that was probed both reach the caller."""

    async def _refused():
        raise ConnectionRefusedError("Cannot connect to host 127.0.0.1:1234")

    module = _load_service(monkeypatch, _refused)
    ok, message, _ = await module.verify_api_key(PROVIDER, "sk-test", base_url=BASE_URL)

    assert ok is False
    assert "Cannot connect to host 127.0.0.1:1234" in message
    assert BASE_URL in message


@pytest.mark.asyncio
async def test_discovery_that_simply_finds_nothing_stays_terse(monkeypatch):
    """No exception means no reason to append; the caller keeps the short message."""

    async def _empty():
        return []

    module = _load_service(monkeypatch, _empty)
    ok, message, _ = await module.verify_api_key(PROVIDER, "sk-test", base_url=BASE_URL)

    assert ok is False
    assert f"No models found for provider '{PROVIDER}'" in message
    assert "failed" not in message


@pytest.mark.asyncio
async def test_credentials_in_the_base_url_never_reach_the_caller(monkeypatch, caplog):
    """Users paste endpoints carrying secrets, as userinfo or as a query token.
    Neither the response nor the application log may repeat them, and the client
    echoes the URL it was handed back inside the exception text."""
    secret_url = "https://svc:s3cr3t-token@models.internal:8443/v1?api-key=AKIA-SECRET#frag"

    async def _refused():
        raise ConnectionRefusedError(f"Cannot connect to {secret_url}")

    module = _load_service(monkeypatch, _refused)
    with caplog.at_level(logging.DEBUG):
        _, message, _ = await module.verify_api_key(PROVIDER, "sk-test", base_url=secret_url)

    logged = caplog.text
    for secret in ("s3cr3t-token", "AKIA-SECRET", "svc:"):
        assert secret not in message, message
        assert secret not in logged, logged
    assert "https://***@models.internal:8443/v1" in message


@pytest.mark.asyncio
async def test_authority_less_base_url_is_not_echoed_back(monkeypatch, caplog):
    """`urlparse` happily accepts a URL with no authority, e.g. one the user typed
    without a scheme. There is no host to keep, so nothing may be echoed."""
    secret_url = "models.internal/v1?api-key=AKIA-SECRET"

    async def _refused():
        raise ConnectionRefusedError(f"Cannot connect to {secret_url}")

    module = _load_service(monkeypatch, _refused)
    with caplog.at_level(logging.DEBUG):
        _, message, _ = await module.verify_api_key(PROVIDER, "sk-test", base_url=secret_url)

    assert "AKIA-SECRET" not in message, message
    assert "AKIA-SECRET" not in caplog.text, caplog.text
    assert "<unparsable url>" in message


@pytest.mark.asyncio
async def test_invalid_port_does_not_propagate_out_of_the_error_handler(monkeypatch):
    """`urlparse` defers port validation to attribute access, so a bad port raises
    from inside the `except` block rather than at parse time."""
    secret_url = "https://svc:s3cr3t-token@models.internal:notaport/v1"

    async def _refused():
        raise ConnectionRefusedError("Cannot connect")

    module = _load_service(monkeypatch, _refused)
    ok, message, _ = await module.verify_api_key(PROVIDER, "sk-test", base_url=secret_url)

    assert ok is False
    assert "s3cr3t-token" not in message, message
    assert "<unparsable url>" in message


def test_redact_url_keeps_what_makes_a_failure_diagnosable(monkeypatch):
    async def _unused():
        return []

    redact = _load_service(monkeypatch, _unused)._redact_url

    assert redact("http://127.0.0.1:1234/v1") == "http://127.0.0.1:1234/v1"
    assert redact("https://user:pw@host/v1") == "https://***@host/v1"
    assert redact("https://host/v1?api-key=SECRET") == "https://host/v1"
    assert redact("https://host/v1#tok=SECRET") == "https://host/v1"
    assert redact("host/v1?api-key=SECRET") == "<unparsable url>"
    assert redact("https://host:notaport/v1") == "<unparsable url>"
    assert redact("") == ""
    assert redact(None) == ""


@pytest.mark.asyncio
async def test_discovery_failure_does_not_abort_the_request(monkeypatch):
    """A raising probe still yields a normal (False, message, {}) result rather
    than propagating out of verify_api_key."""

    async def _boom():
        raise RuntimeError("TLS handshake failed")

    module = _load_service(monkeypatch, _boom)
    ok, message, verify_result = await module.verify_api_key(PROVIDER, "sk-test", base_url=BASE_URL)

    assert ok is False
    assert verify_result == {}
    assert "TLS handshake failed" in message
