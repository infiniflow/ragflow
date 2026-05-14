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
"""Unit tests for LLMBundle.cumulated_tokens tracking.

Covers the fix that ensures async_chat_streamly and async_chat_streamly_delta
accumulate tokens into self.cumulated_tokens (used to populate llm_token_num on
documents after ingestion parsing phases).
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


# ---------------------------------------------------------------------------
# Module loader helpers
# ---------------------------------------------------------------------------

def _load_llm_service(monkeypatch):
    """Load api/db/services/llm_service.py with all heavy deps stubbed out."""
    repo_root = Path(__file__).resolve().parents[5]

    # --- common ---
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    common_constants = ModuleType("common.constants")
    common_constants.LLMType = SimpleNamespace(
        CHAT="chat", EMBEDDING="embedding", RERANK="rerank",
        IMAGE2TEXT="image2text", SPEECH2TEXT="speech2text",
    )
    monkeypatch.setitem(sys.modules, "common.constants", common_constants)

    common_token_utils = ModuleType("common.token_utils")
    common_token_utils.num_tokens_from_string = lambda text: len(text.split())
    monkeypatch.setitem(sys.modules, "common.token_utils", common_token_utils)

    # --- api.db ---
    api_pkg = ModuleType("api")
    api_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    api_db_pkg = ModuleType("api.db")
    api_db_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db", api_db_pkg)

    api_db_db_models = ModuleType("api.db.db_models")

    class _FakeLLM:
        pass

    api_db_db_models.LLM = _FakeLLM
    monkeypatch.setitem(sys.modules, "api.db.db_models", api_db_db_models)

    api_db_services_pkg = ModuleType("api.db.services")
    api_db_services_pkg.__path__ = []
    monkeypatch.setitem(sys.modules, "api.db.services", api_db_services_pkg)

    common_service_mod = ModuleType("api.db.services.common_service")

    class _FakeCommonService:
        pass

    common_service_mod.CommonService = _FakeCommonService
    monkeypatch.setitem(sys.modules, "api.db.services.common_service", common_service_mod)

    # --- tenant_llm_service (provides LLM4Tenant + TenantLLMService) ---
    tenant_llm_mod = ModuleType("api.db.services.tenant_llm_service")

    class _FakeLLM4Tenant:
        def __init__(self, tenant_id: str, model_config: dict, lang="Chinese", **kwargs):
            self.tenant_id = tenant_id
            self.model_config = model_config
            self.mdl = model_config.get("_mdl_instance")
            assert self.mdl is not None, "Pass _mdl_instance in model_config for tests"
            self.max_length = model_config.get("max_tokens", 8192)
            self.is_tools = model_config.get("is_tools", False)
            self.verbose_tool_use = kwargs.get("verbose_tool_use")
            self.langfuse = None

    class _FakeTenantLLMService:
        @staticmethod
        def increase_usage_by_id(model_id, tokens):
            return True

    tenant_llm_mod.LLM4Tenant = _FakeLLM4Tenant
    tenant_llm_mod.TenantLLMService = _FakeTenantLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_mod)

    # --- common.settings (needed by get_init_tenant_llm) ---
    common_settings_mod = ModuleType("common.settings")
    common_settings_mod.CHAT_CFG = {"factory": "openai", "api_key": "", "base_url": ""}
    common_settings_mod.EMBEDDING_CFG = {"factory": "openai", "api_key": "", "base_url": ""}
    common_settings_mod.ASR_CFG = {"factory": "openai", "api_key": "", "base_url": ""}
    common_settings_mod.IMAGE2TEXT_CFG = {"factory": "openai", "api_key": "", "base_url": ""}
    common_settings_mod.RERANK_CFG = {"factory": "openai", "api_key": "", "base_url": ""}
    monkeypatch.setitem(sys.modules, "common.settings", common_settings_mod)

    # Load the actual module
    module_path = repo_root / "api" / "db" / "services" / "llm_service.py"
    spec = importlib.util.spec_from_file_location("test_llm_service_unit", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# ---------------------------------------------------------------------------
# Fake LLM model
# ---------------------------------------------------------------------------

class _FakeChatModel:
    """Minimal async LLM model stub."""

    def __init__(self, tokens_to_return: int = 42):
        self._tokens = tokens_to_return
        self.is_tools = False

    async def async_chat(self, system, history, gen_conf, **kwargs):
        return "response text", self._tokens

    async def async_chat_streamly(self, system, history, gen_conf, **kwargs):
        yield "chunk1"
        yield "chunk2"
        yield self._tokens  # final int = token count


def _make_bundle(llm_module, tokens: int = 50):
    fake_model = _FakeChatModel(tokens_to_return=tokens)
    model_config = {
        "llm_name": "test-model",
        "llm_type": "chat",
        "llm_factory": "TestFactory",
        "id": 1,
        "max_tokens": 4096,
        "_mdl_instance": fake_model,
    }
    return llm_module.LLMBundle("tenant-test", model_config)


def _run(coro):
    return asyncio.run(coro)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_cumulated_tokens_initialized_to_zero(monkeypatch):
    """LLMBundle sets cumulated_tokens = 0 on init."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod)
    assert bundle.cumulated_tokens == 0


@pytest.mark.p2
def test_async_chat_updates_cumulated_tokens(monkeypatch):
    """async_chat adds returned token count to cumulated_tokens."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=77)

    async def _run_chat():
        return await bundle.async_chat("system", [], {})

    txt = _run(_run_chat())
    assert txt == "response text"
    assert bundle.cumulated_tokens == 77


@pytest.mark.p2
def test_async_chat_accumulates_across_calls(monkeypatch):
    """Multiple async_chat calls accumulate into cumulated_tokens."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=10)

    async def _run_two_calls():
        await bundle.async_chat("system", [], {})
        await bundle.async_chat("system", [], {})

    _run(_run_two_calls())
    assert bundle.cumulated_tokens == 20


@pytest.mark.p2
def test_async_chat_streamly_updates_cumulated_tokens(monkeypatch):
    """async_chat_streamly adds the final int token yield to cumulated_tokens."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=33)

    async def _consume():
        chunks = []
        async for chunk in bundle.async_chat_streamly("system", [], {}):
            chunks.append(chunk)
        return chunks

    chunks = _run(_consume())
    # Yields accumulated text (not individual chunks), last int is consumed internally
    assert len(chunks) > 0
    assert bundle.cumulated_tokens == 33


@pytest.mark.p2
def test_async_chat_streamly_zero_tokens_not_accumulated(monkeypatch):
    """async_chat_streamly skips cumulated_tokens update when token count is 0."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=0)

    async def _consume():
        async for _ in bundle.async_chat_streamly("system", [], {}):
            pass

    _run(_consume())
    assert bundle.cumulated_tokens == 0


@pytest.mark.p2
def test_async_chat_streamly_delta_updates_cumulated_tokens(monkeypatch):
    """async_chat_streamly_delta adds the final int token yield to cumulated_tokens."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=55)

    async def _consume():
        deltas = []
        async for delta in bundle.async_chat_streamly_delta("system", [], {}):
            deltas.append(delta)
        return deltas

    deltas = _run(_consume())
    # Yields individual text deltas (not accumulated), final int consumed internally
    assert len(deltas) > 0
    assert bundle.cumulated_tokens == 55


@pytest.mark.p2
def test_async_chat_streamly_delta_zero_tokens_not_accumulated(monkeypatch):
    """async_chat_streamly_delta skips cumulated_tokens update when token count is 0."""
    mod = _load_llm_service(monkeypatch)
    bundle = _make_bundle(mod, tokens=0)

    async def _consume():
        async for _ in bundle.async_chat_streamly_delta("system", [], {}):
            pass

    _run(_consume())
    assert bundle.cumulated_tokens == 0
