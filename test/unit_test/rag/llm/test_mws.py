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
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

from rag.llm import EmbeddingModel, ModelMeta, RerankModel
from rag.llm.embedding_model import MWSEmbed
from rag.llm.model_meta import MWS
from rag.llm.mws_utils import mws_api_url, normalize_mws_project_url
from rag.llm.rerank_model import MWSRerank


PROJECT_URL = "https://gpt.mwsapis.ru/projects/test-project"


def _response(payload, status_code=200):
    response = MagicMock()
    response.status_code = status_code
    response.json.return_value = payload
    response.text = str(payload)
    return response


def _async_context(value):
    context = MagicMock()
    context.__aenter__ = AsyncMock(return_value=value)
    context.__aexit__ = AsyncMock(return_value=None)
    return context


@pytest.mark.p1
def test_mws_provider_registration():
    assert EmbeddingModel["MWS"] is MWSEmbed
    assert RerankModel["MWS"] is MWSRerank
    assert ModelMeta["MWS"] is MWS


@pytest.mark.p1
def test_mws_project_url_validation_and_endpoints():
    assert normalize_mws_project_url(PROJECT_URL + "/") == PROJECT_URL
    assert (
        mws_api_url(PROJECT_URL, "openai/v1/embeddings")
        == PROJECT_URL + "/openai/v1/embeddings"
    )
    assert (
        mws_api_url(PROJECT_URL, "cohere/v2/rerank")
        == PROJECT_URL + "/cohere/v2/rerank"
    )
    with pytest.raises(ValueError, match="project root"):
        normalize_mws_project_url("https://gpt.mwsapis.ru/openai/v1")
    with pytest.raises(ValueError, match="query string"):
        normalize_mws_project_url(PROJECT_URL + "?secret=value")


@pytest.mark.p1
@pytest.mark.asyncio
async def test_mws_model_list_uses_exact_url_and_bearer_header():
    response = MagicMock(status=200)
    response.json = AsyncMock(
        return_value={"data": [{"id": "bge-m3"}, {"id": "bge-reranker-v2-m3"}]}
    )
    session = MagicMock()
    session.get.return_value = _async_context(response)
    session_context = _async_context(session)

    with patch("rag.llm.model_meta.aiohttp.ClientSession", return_value=session_context):
        models = await MWS("token", PROJECT_URL + "/").get_model_list()

    session.get.assert_called_once_with(
        PROJECT_URL + "/openai/v1/models",
        headers={"Authorization": "Bearer token"},
    )
    assert [model["model_types"] for model in models] == [
        ["embedding"],
        ["rerank"],
    ]


@pytest.mark.p1
def test_mws_embedding_sends_only_documented_fields_and_orders_results():
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
    with patch("rag.llm.embedding_model.requests.post") as post:
        with pytest.raises(ValueError, match="Token is required"):
            MWSEmbed(" ", "bge-m3", PROJECT_URL)
    post.assert_not_called()


@pytest.mark.p1
def test_mws_rerank_uses_cohere_path_and_exact_payload():
    reranker = MWSRerank("token", "bge-reranker-v2-m3", PROJECT_URL)
    response = _response(
        {
            "results": [
                {"index": 1, "relevance_score": 0.9},
                {"index": 0, "relevance_score": 0.2},
            ]
        }
    )


@pytest.mark.p1
def test_mws_empty_inputs_do_not_send_requests():
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

    with patch("rag.llm.rerank_model.requests.post", return_value=response) as post:
        scores, _ = reranker.similarity("query", ["first", "second"])

    assert np.array_equal(scores, np.array([0.2, 0.9]))
    post.assert_called_once_with(
        PROJECT_URL + "/cohere/v2/rerank",
        headers={"Content-Type": "application/json", "Authorization": "Bearer token"},
        json={
            "model": "bge-reranker-v2-m3",
            "query": "query",
            "documents": ["first", "second"],
            "top_n": 2,
        },
        timeout=30,
    )


@pytest.mark.p1
def test_mws_model_list_keeps_only_embedding_and_rerank():
    meta = MWS("token", PROJECT_URL)
    models = meta._format_model_list(
        {
            "data": [
                {"id": "bge-m3"},
                {"id": "bge-reranker-v2-m3"},
                {"id": "qwen3-32b"},
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
    ]
