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

"""Unit tests for LocalAI.get_model_list() — verify Ollama-style tag suffix
(e.g. ``:latest``) is stripped from model names returned by ``/api/tags``
so that names match what the OpenAI-compatible ``/v1/chat/completions``
endpoint expects."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from rag.llm.model_meta import LocalAI

pytestmark = pytest.mark.p2


def _build_tags_response(models):
    return {"models": models}


def _build_show_response(capabilities=None, context_length=8192):
    return {
        "model_info": {"general.context_length": context_length},
        "capabilities": capabilities or ["completion"],
    }


def _make_async_cm(response_mock):
    """Wrap a mock so ``async with obj as x:`` binds x = response_mock."""
    wrapper = MagicMock()
    wrapper.__aenter__ = AsyncMock(return_value=response_mock)
    wrapper.__aexit__ = AsyncMock(return_value=None)
    return wrapper


def _make_mock_resp(json_payload, status=200):
    """Build a response mock usable inside ``async with resp:``."""
    resp = MagicMock()
    resp.status = status
    resp.json = AsyncMock(return_value=json_payload)
    return resp


async def _run_test(models_in, expected_names_out):
    """Core helper: mock /api/tags + /api/show, call get_model_list, assert names."""
    tags_payload = _build_tags_response(models_in)
    show_payload = _build_show_response()

    tags_resp = _make_mock_resp(tags_payload)
    show_resps = [_make_mock_resp(show_payload) for _ in models_in]

    # session.get returns a context-manager wrapper over tags_resp
    # session.post returns a list of context-manager wrappers (one per model)
    session = MagicMock()
    session.get.return_value = _make_async_cm(tags_resp)
    session.post.side_effect = [_make_async_cm(r) for r in show_resps]

    # aiohttp.ClientSession() is used as async context manager, so its
    # __aenter__ must return `session`.
    session_cls = MagicMock()
    session_cls.__aenter__ = AsyncMock(return_value=session)
    session_cls.__aexit__ = AsyncMock(return_value=None)

    with patch("aiohttp.ClientSession", return_value=session_cls):
        result = await LocalAI(api_key="", base_url="http://127.0.0.1:8080").get_model_list()

    assert [m["name"] for m in result] == expected_names_out


@pytest.mark.asyncio
async def test_get_model_list_strips_latest_tag():
    await _run_test(
        [
            {"name": "lfm2.5-8b-a1b:latest", "model": "lfm2.5-8b-a1b:latest"},
            {"name": "qwen_qwen3.5-2b:latest", "model": "qwen_qwen3.5-2b:latest"},
        ],
        ["lfm2.5-8b-a1b", "qwen_qwen3.5-2b"],
    )


@pytest.mark.asyncio
async def test_get_model_list_strips_custom_tag():
    await _run_test(
        [
            {"name": "llama3:7b", "model": "llama3:7b"},
            {"name": "mistral:q4_K_M", "model": "mistral:q4_K_M"},
        ],
        ["llama3", "mistral"],
    )


@pytest.mark.asyncio
async def test_get_model_list_no_tag_unchanged():
    await _run_test(
        [
            {"name": "gemma2-9b", "model": "gemma2-9b"},
            {"name": "phi3-mini", "model": "phi3-mini"},
        ],
        ["gemma2-9b", "phi3-mini"],
    )


@pytest.mark.asyncio
async def test_get_model_list_name_with_multiple_colons():
    await _run_test(
        [{"name": "registry.example.com/team/model:v1.0", "model": "registry.example.com/team/model:v1.0"}],
        ["registry.example.com/team/model"],
    )


@pytest.mark.asyncio
async def test_get_model_list_no_base_url_returns_empty():
    result = await LocalAI(api_key="", base_url=None).get_model_list()
    assert result == []


@pytest.mark.asyncio
async def test_get_model_list_tags_endpoint_non_200_returns_empty():
    tags_resp = _make_mock_resp({}, status=500)
    session = MagicMock()
    session.get.return_value = _make_async_cm(tags_resp)

    session_cls = MagicMock()
    session_cls.__aenter__ = AsyncMock(return_value=session)
    session_cls.__aexit__ = AsyncMock(return_value=None)

    with patch("aiohttp.ClientSession", return_value=session_cls):
        result = await LocalAI(api_key="", base_url="http://127.0.0.1:8080").get_model_list()

    assert result == []
