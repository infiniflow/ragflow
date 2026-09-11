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

import logging
from unittest.mock import MagicMock, patch

import pytest

from rag.svr.task_executor_refactor.task_handler import TaskHandler


@pytest.mark.asyncio
async def test_bind_embedding_model_reresolves_empty_stale_tenant_model(task_context):
    task_context.raw_task["tenant_embd_id"] = "stale-instance"
    task_context.raw_task["embd_id"] = "current-embedding@provider"
    handler = TaskHandler(task_context)
    embedding_model = MagicMock()
    embedding_model.encode.return_value = ([[1.0, 2.0]], 1)

    with (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            return_value={"llm_factory": "OpenAI-API-Compatible", "api_key": ""},
        ) as get_model_config_by_id,
        patch(
            "rag.svr.task_executor_refactor.task_handler.resolve_model_config",
            return_value={"llm_factory": "OpenAI-API-Compatible", "api_key": "current-key"},
        ) as resolve_model_config,
        patch(
            "rag.svr.task_executor_refactor.task_handler.LLMBundle",
            return_value=embedding_model,
        ) as llm_bundle,
    ):
        result = await handler._bind_embedding_model()

    get_model_config_by_id.assert_called_once_with(
        task_context.tenant_id,
        "embedding",
        "stale-instance",
    )
    resolve_model_config.assert_called_once_with(
        task_context.tenant_id,
        "embedding",
        "current-embedding@provider",
    )
    llm_bundle.assert_called_once_with(
        task_context.tenant_id,
        {"llm_factory": "OpenAI-API-Compatible", "api_key": "current-key"},
        lang=task_context.language,
    )
    assert result == (embedding_model, 2)


@pytest.mark.asyncio
async def test_bind_embedding_model_keeps_nonempty_tenant_model(task_context):
    task_context.raw_task["tenant_embd_id"] = "current-instance"
    handler = TaskHandler(task_context)
    embedding_model = MagicMock()
    embedding_model.encode.return_value = ([[1.0]], 1)

    with (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            return_value={"llm_factory": "OpenAI", "api_key": "current-key"},
        ),
        patch("rag.svr.task_executor_refactor.task_handler.resolve_model_config") as resolve_model_config,
        patch(
            "rag.svr.task_executor_refactor.task_handler.LLMBundle",
            return_value=embedding_model,
        ),
    ):
        result = await handler._bind_embedding_model()

    resolve_model_config.assert_not_called()
    assert result == (embedding_model, 1)


@pytest.mark.asyncio
async def test_bind_embedding_model_does_not_retry_failed_stale_model_fallback(task_context):
    task_context.raw_task["tenant_embd_id"] = "stale-instance"
    task_context.raw_task["embd_id"] = "current-embedding@provider"
    handler = TaskHandler(task_context)

    with (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            return_value={"llm_factory": "OpenAI-API-Compatible", "api_key": ""},
        ) as get_model_config_by_id,
        patch(
            "rag.svr.task_executor_refactor.task_handler.resolve_model_config",
            side_effect=LookupError("model unavailable"),
        ) as resolve_model_config,
        pytest.raises(LookupError, match="model unavailable"),
    ):
        await handler._bind_embedding_model()

    get_model_config_by_id.assert_called_once_with(
        task_context.tenant_id,
        "embedding",
        "stale-instance",
    )
    resolve_model_config.assert_called_once_with(
        task_context.tenant_id,
        "embedding",
        "current-embedding@provider",
    )


@pytest.mark.asyncio
async def test_bind_embedding_model_keeps_keyless_local_model(task_context):
    """A keyless local embedding model must not be mistaken for stale credentials."""
    task_context.raw_task["tenant_embd_id"] = "local-instance"
    task_context.raw_task["embd_id"] = "other-embedding@provider"
    handler = TaskHandler(task_context)
    embedding_model = MagicMock()
    embedding_model.encode.return_value = ([[1.0, 2.0, 3.0]], 1)
    local_config = {"llm_factory": "Builtin", "api_key": "", "api_base": "http://tei:8080"}

    with (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            return_value=local_config,
        ),
        patch("rag.svr.task_executor_refactor.task_handler.resolve_model_config") as resolve_model_config,
        patch("rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type") as get_default_model,
        patch(
            "rag.svr.task_executor_refactor.task_handler.LLMBundle",
            return_value=embedding_model,
        ) as llm_bundle,
    ):
        result = await handler._bind_embedding_model()

    resolve_model_config.assert_not_called()
    get_default_model.assert_not_called()
    llm_bundle.assert_called_once_with(task_context.tenant_id, local_config, lang=task_context.language)
    assert result == (embedding_model, 3)


@pytest.mark.asyncio
async def test_bind_embedding_model_uses_default_for_stale_model_without_name(task_context, caplog):
    """A stale cached model without embd_id must resolve the current tenant default."""
    task_context.raw_task["tenant_embd_id"] = "stale-instance"
    task_context.raw_task["embd_id"] = ""
    handler = TaskHandler(task_context)
    embedding_model = MagicMock()
    embedding_model.encode.return_value = ([[1.0, 2.0]], 1)
    default_config = {"llm_factory": "OpenAI", "api_key": "current-key"}

    with (
        caplog.at_level(logging.INFO),
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            side_effect=LookupError("stale model binding"),
        ),
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_tenant_default_model_by_type",
            return_value=default_config,
        ) as get_default_model,
        patch(
            "rag.svr.task_executor_refactor.task_handler.LLMBundle",
            return_value=embedding_model,
        ) as llm_bundle,
    ):
        result = await handler._bind_embedding_model()

    get_default_model.assert_called_once_with(task_context.tenant_id, "embedding")
    llm_bundle.assert_called_once_with(task_context.tenant_id, default_config, lang=task_context.language)
    assert (f"Recovering stale embedding model binding for task {task_context.id} in tenant {task_context.tenant_id} with model tenant-default") in caplog.messages
    assert result == (embedding_model, 2)


@pytest.mark.asyncio
async def test_bind_embedding_model_rejects_fallback_with_missing_credentials(task_context):
    """A fallback that still lacks required credentials must fail before model construction."""
    task_context.raw_task["tenant_embd_id"] = "stale-instance"
    task_context.raw_task["embd_id"] = "current-embedding@provider"
    handler = TaskHandler(task_context)

    with (
        patch(
            "rag.svr.task_executor_refactor.task_handler.get_model_config_by_id",
            return_value={"llm_factory": "OpenAI-API-Compatible", "api_key": ""},
        ),
        patch(
            "rag.svr.task_executor_refactor.task_handler.resolve_model_config",
            return_value={"llm_factory": "OpenAI-API-Compatible", "api_key": ""},
        ),
        patch("rag.svr.task_executor_refactor.task_handler.LLMBundle") as llm_bundle,
        pytest.raises(LookupError, match="credentials are missing"),
    ):
        await handler._bind_embedding_model()

    llm_bundle.assert_not_called()
