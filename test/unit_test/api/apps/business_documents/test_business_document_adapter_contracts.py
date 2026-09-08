#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#

"""Behavioral contracts for the narrow Business Documents upstream adapters."""

from __future__ import annotations

import asyncio
import json
from pathlib import Path
import sys
from types import ModuleType

import pytest


if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(Path(__file__).resolve().parents[5] / "api" / "apps")]
    sys.modules["api.apps"] = api_apps

from api.apps.business_documents import ai as ai_module
from api.apps.business_documents.ai import BusinessDocumentAI, RAGFlowLLMAdapter
from api.apps.business_documents.evidence import BusinessDocumentEvidence, RAGFlowDatasetSearchAdapter


@pytest.mark.p1
@pytest.mark.parametrize(("task_type", "expected_limit"), [("GENERATE_DRAFT", 8192), ("ASSESS_INTAKE", 4096)])
def test_llm_adapter_preserves_tenant_prompt_payload_and_retry_contract(monkeypatch, task_type, expected_limit):
    lookup_calls = []
    bundle_calls = []
    chat_calls = []
    drain_calls = []
    model_config = {"model_name": "configured-chat"}

    tenant_model_module = ModuleType("api.db.joint_services.tenant_model_service")

    def get_tenant_default_model_by_type(tenant_id, model_type):
        lookup_calls.append((tenant_id, model_type))
        return model_config

    tenant_model_module.get_tenant_default_model_by_type = get_tenant_default_model_by_type
    monkeypatch.setitem(sys.modules, tenant_model_module.__name__, tenant_model_module)

    llm_service_module = ModuleType("api.db.services.llm_service")

    class FakeBundle:
        def __init__(self, *args, **kwargs):
            bundle_calls.append((args, kwargs))

        def __enter__(self):
            return self

        def __exit__(self, *_args):
            return False

        async def async_chat(self, system_prompt, messages, options):
            chat_calls.append((system_prompt, messages, options))
            return "adapter-result"

    llm_service_module.LLMBundle = FakeBundle
    monkeypatch.setitem(sys.modules, llm_service_module.__name__, llm_service_module)

    async def drain_litellm_callbacks():
        drain_calls.append(True)

    monkeypatch.setattr(ai_module, "_drain_litellm_callbacks", drain_litellm_callbacks)

    payload = {"job_input": {"task_type": task_type}, "document": {"title": "Регламент"}}
    result = RAGFlowLLMAdapter().generate("tenant-1", "system contract", payload)

    assert result == "adapter-result"
    assert lookup_calls[0][0] == "tenant-1"
    assert getattr(lookup_calls[0][1], "value", lookup_calls[0][1]) == "chat"
    assert bundle_calls == [(("tenant-1", model_config), {"lang": "Russian", "max_retries": 0})]
    assert len(chat_calls) == 1
    system_prompt, messages, options = chat_calls[0]
    assert system_prompt == "system contract"
    assert messages[0]["role"] == "user"
    assert json.loads(messages[0]["content"]) == payload
    assert options == {"temperature": 0, "top_p": 0.1, "max_completion_tokens": expected_limit}
    assert drain_calls == [True]


@pytest.mark.p1
def test_dataset_search_adapter_forwards_actor_request_and_result(monkeypatch):
    calls = []
    expected = (True, {"chunks": [{"id": "chunk-1"}]})
    service_module = ModuleType("api.apps.services.dataset_api_service")

    async def search_datasets(actor_id, request):
        calls.append((actor_id, request))
        return expected

    service_module.search_datasets = search_datasets
    monkeypatch.setitem(sys.modules, service_module.__name__, service_module)
    request = {"dataset_ids": ["dataset-1"], "question": "Как?"}

    assert RAGFlowDatasetSearchAdapter().search("actor-1", request) == expected
    assert calls == [("actor-1", request)]


@pytest.mark.p1
def test_default_and_injected_adapters_keep_one_production_boundary():
    custom_llm = object()
    custom_search = object()

    assert isinstance(BusinessDocumentAI()._adapter, RAGFlowLLMAdapter)
    assert BusinessDocumentAI(custom_llm)._adapter is custom_llm
    assert isinstance(BusinessDocumentEvidence()._search_adapter, RAGFlowDatasetSearchAdapter)
    assert BusinessDocumentEvidence(custom_search)._search_adapter is custom_search


@pytest.mark.p1
def test_litellm_callback_drain_does_not_stop_or_clear_the_global_worker(monkeypatch):
    calls = []

    class FakeWorker:
        _running_tasks = set()

        async def flush(self):
            calls.append("flush")

        async def stop(self):
            raise AssertionError("the process-global worker must not be stopped")

        async def clear_queue(self):
            raise AssertionError("callbacks from other requests must not be cleared")

    logging_worker = ModuleType("litellm.litellm_core_utils.logging_worker")
    logging_worker.GLOBAL_LOGGING_WORKER = FakeWorker()
    monkeypatch.setitem(sys.modules, logging_worker.__name__, logging_worker)

    asyncio.run(ai_module._drain_litellm_callbacks())

    assert calls == ["flush"]
