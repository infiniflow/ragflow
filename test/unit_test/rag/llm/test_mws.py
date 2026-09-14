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
"""Tests for MWS provider registration, discovery, and inference adapters."""

from unittest.mock import AsyncMock, MagicMock, call, patch

import numpy as np
import pytest

from rag.llm import ChatModel, EmbeddingModel, ModelMeta, RerankModel
from rag.llm.chat_model import MWSChat
from rag.llm.embedding_model import MWSEmbed
from rag.llm.model_meta import MWS
from rag.llm.mws_utils import mws_api_url, normalize_mws_project_url
from rag.llm.rerank_model import MWSRerank


PROJECT_URL = "https://gpt.mwsapis.ru/projects/test-project"


def _response(payload, status_code=200):
    """Create a mocked synchronous HTTP response for MWS adapter tests."""
    response = MagicMock()
    response.status_code = status_code
    response.json.return_value = payload
    response.text = str(payload)
    return response


def _async_context(value):
    """Wrap a mocked value in an asynchronous context manager."""
    context = MagicMock()
    context.__aenter__ = AsyncMock(return_value=value)
    context.__aexit__ = AsyncMock(return_value=None)
    return context


@pytest.mark.p1
def test_mws_provider_registration():
    """Register every supported MWS adapter under the public provider name."""
    assert ChatModel["MWS"] is MWSChat
    assert EmbeddingModel["MWS"] is MWSEmbed
    assert RerankModel["MWS"] is MWSRerank
    assert ModelMeta["MWS"] is MWS


@pytest.mark.p1
def test_mws_project_url_validation_and_endpoints():
    """Normalize project roots and construct each supported MWS endpoint."""
    assert normalize_mws_project_url(PROJECT_URL + "/") == PROJECT_URL
    assert mws_api_url(PROJECT_URL, "openai/v1/chat/completions") == PROJECT_URL + "/openai/v1/chat/completions"
    assert mws_api_url(PROJECT_URL, "openai/v1/embeddings") == PROJECT_URL + "/openai/v1/embeddings"
    assert mws_api_url(PROJECT_URL, "cohere/v2/rerank") == PROJECT_URL + "/cohere/v2/rerank"
    with pytest.raises(ValueError, match="project root"):
        normalize_mws_project_url("https://gpt.mwsapis.ru/openai/v1")
    with pytest.raises(ValueError, match="query string"):
        normalize_mws_project_url(PROJECT_URL + "?secret=value")


@pytest.mark.p1
def test_mws_model_discovery_logs_validation_failure_without_credentials():
    """Log the failed validation target without exposing rejected URL data."""
    with patch("rag.llm.model_meta.logging.warning") as warning:
        with pytest.raises(ValueError, match="query string"):
            MWS("super-secret-token", PROJECT_URL + "?secret=do-not-log")

    warning.assert_called_once_with(
        "mws_model_discovery_validation_failed",
        extra={
            "provider": "MWS",
            "operation": "model_discovery",
            "validation_target": "api_url",
            "error_type": "ValueError",
        },
    )
    logged = repr(warning.call_args)
    assert "super-secret-token" not in logged
    assert "do-not-log" not in logged
    assert "Bearer" not in logged


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_model_list_uses_exact_url_and_bearer_header():
    """Load and classify models with the required URL and bearer token."""
    response = MagicMock(status=200)
    response.json = AsyncMock(
        return_value={
            "data": [
                {"id": "qwen3-32b"},
                {"id": "qwen-vl"},
                {"id": "bge-m3"},
                {"id": "bge-reranker-v2-m3"},
            ]
        }
    )
    session = MagicMock()
    session.get.return_value = _async_context(response)
    session_context = _async_context(session)

    with (
        patch(
            "rag.llm.model_meta.aiohttp.ClientSession",
            return_value=session_context,
        ) as client_session,
        patch("rag.llm.model_meta.logging.info") as info,
    ):
        models = await MWS("token", PROJECT_URL + "/").get_model_list()

    assert client_session.call_args.kwargs["timeout"].total == 30
    session.get.assert_called_once_with(
        PROJECT_URL + "/openai/v1/models",
        headers={"Authorization": "Bearer token"},
    )
    assert [model["model_types"] for model in models] == [
        ["chat"],
        ["embedding"],
        ["rerank"],
    ]
    log_context = {
        "provider": "MWS",
        "operation": "model_discovery",
        "url": PROJECT_URL + "/openai/v1/models",
    }
    assert info.call_args_list == [
        call("mws_model_discovery_request", extra=log_context),
        call(
            "mws_model_discovery_completed",
            extra={**log_context, "result_count": 3},
        ),
    ]
    assert "token" not in repr(info.call_args_list)
    assert "Bearer" not in repr(info.call_args_list)


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_model_discovery_logs_http_failure_and_zero_results():
    """Log a failed HTTP response and the resulting empty model list."""
    response = MagicMock(status=503)
    session = MagicMock()
    session.get.return_value = _async_context(response)
    log_context = {
        "provider": "MWS",
        "operation": "model_discovery",
        "url": PROJECT_URL + "/openai/v1/models",
    }

    with (
        patch(
            "rag.llm.model_meta.aiohttp.ClientSession",
            return_value=_async_context(session),
        ),
        patch("rag.llm.model_meta.logging.info") as info,
        patch("rag.llm.model_meta.logging.warning") as warning,
    ):
        models = await MWS("super-secret-token", PROJECT_URL).get_model_list()

    assert models == []
    warning.assert_called_once_with(
        "mws_model_discovery_request_failed",
        extra={
            **log_context,
            "failure_stage": "http_response",
            "http_status": 503,
        },
    )
    assert info.call_args_list == [
        call("mws_model_discovery_request", extra=log_context),
        call(
            "mws_model_discovery_completed",
            extra={**log_context, "result_count": 0},
        ),
    ]
    logged = repr([info.call_args_list, warning.call_args_list])
    assert "super-secret-token" not in logged
    assert "Bearer" not in logged


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_model_discovery_logs_request_exception_without_credentials():
    """Log a discovery transport exception without exposing authentication."""
    session = MagicMock()
    session.get.side_effect = RuntimeError("connection failed")

    with (
        patch(
            "rag.llm.model_meta.aiohttp.ClientSession",
            return_value=_async_context(session),
        ),
        patch("rag.llm.model_meta.logging.info"),
        patch("rag.llm.model_meta.logging.warning") as warning,
    ):
        with pytest.raises(RuntimeError, match="connection failed"):
            await MWS("super-secret-token", PROJECT_URL).get_model_list()

    warning.assert_called_once_with(
        "mws_model_discovery_request_failed",
        extra={
            "provider": "MWS",
            "operation": "model_discovery",
            "url": PROJECT_URL + "/openai/v1/models",
            "failure_stage": "request",
            "error_type": "RuntimeError",
        },
    )
    logged = repr(warning.call_args)
    assert "super-secret-token" not in logged
    assert "Bearer" not in logged


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_chat_uses_exact_url_bearer_header_and_documented_fields():
    """Send only documented fields in a non-streaming MWS chat request."""
    chat = MWSChat("token", "qwen3-32b", PROJECT_URL + "/")
    response = MagicMock(status=200)
    response.json = AsyncMock(
        return_value={
            "id": "chat-1",
            "model": "qwen3-32b",
            "choices": [
                {
                    "index": 0,
                    "finish_reason": "stop",
                    "message": {"role": "assistant", "content": "Hello"},
                }
            ],
            "usage": {
                "prompt_tokens": 4,
                "completion_tokens": 1,
                "total_tokens": 5,
            },
        }
    )
    session = MagicMock()
    session.post.return_value = _async_context(response)

    with patch(
        "rag.llm.chat_model.aiohttp.ClientSession",
        return_value=_async_context(session),
    ):
        answer, tokens = await chat._async_chat(
            [
                {"role": "system", "content": "Be concise."},
                {
                    "role": "user",
                    "content": "Hi",
                    "tool_call_id": "must-not-be-forwarded",
                },
            ],
            {
                "temperature": 0.25,
                "max_tokens": 128,
                "top_p": 0.9,
                "tools": [{"must": "not be forwarded"}],
            },
        )

    assert answer == "Hello"
    assert tokens == 5
    assert chat.last_usage == {
        "prompt_tokens": 4,
        "completion_tokens": 1,
        "total_tokens": 5,
    }
    session.post.assert_called_once_with(
        PROJECT_URL + "/openai/v1/chat/completions",
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer token",
        },
        json={
            "model": "qwen3-32b",
            "messages": [
                {"role": "system", "content": "Be concise."},
                {"role": "user", "content": "Hi"},
            ],
            "temperature": 0.25,
            "max_completion_tokens": 128,
        },
    )


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_chat_streaming_uses_documented_fields_and_usage():
    """Parse streaming MWS chat chunks and report final token usage."""
    chat = MWSChat("token", "qwen3-32b", PROJECT_URL)
    response = MagicMock(status=200)
    response.content = MagicMock()
    response.content.__aiter__.return_value = iter(
        [
            b'data: {"model":"qwen3-32b","choices":[{"index":0,"delta":{"content":"Hel"}}]}\n',
            b'data: {"model":"qwen3-32b","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}\n',
            b"data: [DONE]\n",
        ]
    )
    session = MagicMock()
    session.post.return_value = _async_context(response)

    with patch(
        "rag.llm.chat_model.aiohttp.ClientSession",
        return_value=_async_context(session),
    ):
        chunks = [
            chunk
            async for chunk in chat._async_chat_streamly(
                [{"role": "user", "content": "Hi"}],
                {"top_p": 0.9},
            )
        ]

    assert chunks == [("Hel", 0), ("lo", 5)]
    assert chat.last_usage == {
        "prompt_tokens": 4,
        "completion_tokens": 1,
        "total_tokens": 5,
    }
    session.post.assert_called_once_with(
        PROJECT_URL + "/openai/v1/chat/completions",
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer token",
        },
        json={
            "model": "qwen3-32b",
            "messages": [{"role": "user", "content": "Hi"}],
            "stream": True,
            "stream_options": {"include_usage": True},
        },
    )


@pytest.mark.p1
def test_mws_embedding_sends_only_documented_fields_and_orders_results():
    """Send a strict embedding request and restore response index order."""
    embed = MWSEmbed("token", "bge-m3", PROJECT_URL + "/")
    response = _response(
        {
            "data": [
                {"index": 1, "embedding": [0.3, 0.4]},
                {"index": 0, "embedding": [0.1, 0.2]},
            ],
            "usage": {"prompt_tokens": 7, "total_tokens": 7},
        }
    )

    with patch("rag.llm.embedding_model.requests.post", return_value=response) as post:
        vectors, tokens = embed.encode(["first", "second"])

    assert np.array_equal(vectors, np.array([[0.1, 0.2], [0.3, 0.4]]))
    assert tokens == 7
    post.assert_called_once_with(
        PROJECT_URL + "/openai/v1/embeddings",
        headers={"Content-Type": "application/json", "Authorization": "Bearer token"},
        json={"model": "bge-m3", "input": ["first", "second"]},
        timeout=30,
    )


@pytest.mark.p1
def test_mws_embedding_rejects_empty_token_without_request():
    """Reject an empty MWS token before attempting an embedding request."""
    with patch("rag.llm.embedding_model.requests.post") as post:
        with pytest.raises(ValueError, match="Token is required"):
            MWSEmbed(" ", "bge-m3", PROJECT_URL)
    post.assert_not_called()


@pytest.mark.p1
def test_mws_rerank_uses_cohere_path_and_exact_payload():
    """Use the MWS Cohere endpoint with the exact reranking payload."""
    reranker = MWSRerank("super-secret-token", "bge-reranker-v2-m3", PROJECT_URL)
    response = _response(
        {
            "results": [
                {"index": 1, "relevance_score": 0.9},
                {"index": 0, "relevance_score": 0.2},
            ]
        }
    )

    token_counts = {"query": 5, "first": 2, "second": 3}
    with (
        patch("rag.llm.rerank_model.requests.post", return_value=response) as post,
        patch(
            "rag.llm.rerank_model.num_tokens_from_string",
            side_effect=lambda text: token_counts[text],
        ),
        patch("rag.llm.rerank_model.logging.info") as info,
    ):
        scores, tokens = reranker.similarity("query", ["first", "second"])

    assert np.array_equal(scores, np.array([0.2, 0.9]))
    assert tokens == 10
    post.assert_called_once_with(
        PROJECT_URL + "/cohere/v2/rerank",
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer super-secret-token",
        },
        json={
            "model": "bge-reranker-v2-m3",
            "query": "query",
            "documents": ["first", "second"],
            "top_n": 2,
        },
        timeout=30,
    )
    info.assert_called_once_with(
        "mws_rerank_request",
        extra={
            "provider": "MWS",
            "operation": "rerank",
            "endpoint": PROJECT_URL + "/cohere/v2/rerank",
            "model": "bge-reranker-v2-m3",
            "document_count": 2,
        },
    )
    logged = repr(info.call_args)
    assert "super-secret-token" not in logged
    assert "Bearer" not in logged
    assert "query" not in logged
    assert "first" not in logged
    assert "second" not in logged


@pytest.mark.p1
@pytest.mark.parametrize(
    "result",
    [
        {"index": 0},
        {"index": 0, "relevance_score": "invalid"},
        {"index": 0, "relevance_score": True},
        {"index": 0, "relevance_score": float("nan")},
        {"index": 0, "relevance_score": float("inf")},
    ],
)
def test_mws_rerank_rejects_invalid_relevance_scores(result):
    """Reject missing, non-numeric, boolean, and non-finite scores."""
    reranker = MWSRerank("super-secret-token", "bge-reranker-v2-m3", PROJECT_URL)
    response = _response({"results": [result]})
    log_context = {
        "provider": "MWS",
        "operation": "rerank",
        "endpoint": PROJECT_URL + "/cohere/v2/rerank",
        "model": "bge-reranker-v2-m3",
        "document_count": 1,
    }

    with (
        patch("rag.llm.rerank_model.requests.post", return_value=response),
        patch("rag.llm.rerank_model.logging.info"),
        patch("rag.llm.rerank_model.logging.warning") as warning,
    ):
        with pytest.raises(ValueError, match="relevance_score"):
            reranker.similarity("query", ["first"])

    warning.assert_called_once_with(
        "mws_rerank_failed",
        extra={
            **log_context,
            "failure_stage": "response_validation",
            "error_type": "ValueError",
        },
    )
    logged = repr(warning.call_args)
    assert "super-secret-token" not in logged
    assert "query" not in logged
    assert "first" not in logged


@pytest.mark.p1
@pytest.mark.parametrize("failure_stage", ["http_request", "json_parsing"])
def test_mws_rerank_logs_failure_stage_and_reraises(failure_stage):
    """Log the failing rerank stage and re-raise the original exception."""
    reranker = MWSRerank("super-secret-token", "bge-reranker-v2-m3", PROJECT_URL)
    response = _response({"results": []})
    error = RuntimeError(failure_stage)
    if failure_stage == "http_request":
        response.raise_for_status.side_effect = error
    else:
        response.json.side_effect = error
    log_context = {
        "provider": "MWS",
        "operation": "rerank",
        "endpoint": PROJECT_URL + "/cohere/v2/rerank",
        "model": "bge-reranker-v2-m3",
        "document_count": 1,
    }

    with (
        patch("rag.llm.rerank_model.requests.post", return_value=response),
        patch("rag.llm.rerank_model.logging.info"),
        patch("rag.llm.rerank_model.logging.warning") as warning,
    ):
        with pytest.raises(RuntimeError) as caught:
            reranker.similarity("query", ["first"])

    assert caught.value is error
    warning.assert_called_once_with(
        "mws_rerank_failed",
        extra={
            **log_context,
            "failure_stage": failure_stage,
            "error_type": "RuntimeError",
        },
    )
    logged = repr(warning.call_args)
    assert "super-secret-token" not in logged
    assert "query" not in logged
    assert "first" not in logged


@pytest.mark.p1
def test_mws_empty_inputs_do_not_send_requests():
    """Return empty results without calling MWS for empty input lists."""
    embed = MWSEmbed("token", "bge-m3", PROJECT_URL)
    reranker = MWSRerank("token", "bge-reranker-v2-m3", PROJECT_URL)

    with (
        patch("rag.llm.embedding_model.requests.post") as embed_post,
        patch("rag.llm.rerank_model.requests.post") as rerank_post,
    ):
        vectors, embedding_tokens = embed.encode([])
        scores, rerank_tokens = reranker.similarity("query", [])

    assert vectors.size == 0
    assert scores.size == 0
    assert embedding_tokens == 0
    assert rerank_tokens == 0
    embed_post.assert_not_called()
    rerank_post.assert_not_called()


@pytest.mark.p1
def test_mws_rejects_incomplete_indexed_responses():
    """Reject incomplete embedding and reranking response collections."""
    embed = MWSEmbed("token", "bge-m3", PROJECT_URL)
    reranker = MWSRerank("token", "bge-reranker-v2-m3", PROJECT_URL)

    with patch(
        "rag.llm.embedding_model.requests.post",
        return_value=_response({"data": [{"index": 0, "embedding": [0.1]}]}),
    ):
        with pytest.raises(Exception, match="1 embeddings for 2 inputs"):
            embed.encode(["first", "second"])

    with patch(
        "rag.llm.rerank_model.requests.post",
        return_value=_response({"results": [{"index": 0, "relevance_score": 0.1}]}),
    ):
        with pytest.raises(ValueError, match="1 rerank results for 2 documents"):
            reranker.similarity("query", ["first", "second"])


@pytest.mark.p1
def test_mws_model_list_keeps_chat_embedding_and_rerank():
    """Expose only the three model types implemented by the MWS provider."""
    meta = MWS("token", PROJECT_URL)
    models = meta._format_model_list(
        {
            "data": [
                {"id": "bge-m3"},
                {"id": "bge-reranker-v2-m3"},
                {"id": "qwen3-32b"},
                {"id": "qwen-vl"},
            ]
        }
    )

    assert meta._get_model_list_url() == PROJECT_URL + "/openai/v1/models"
    assert models == [
        {
            "name": "bge-m3",
            "model_types": ["embedding"],
            "features": [],
            "max_tokens": 8192,
        },
        {
            "name": "bge-reranker-v2-m3",
            "model_types": ["rerank"],
            "features": [],
            "max_tokens": 8192,
        },
        {
            "name": "qwen3-32b",
            "model_types": ["chat"],
            "features": [],
            "max_tokens": 8192,
        },
    ]
