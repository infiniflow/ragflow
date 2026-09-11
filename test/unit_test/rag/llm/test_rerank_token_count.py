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
"""Regression tests for reranker token accounting.

``JinaRerank`` and every provider that inherits it (NovitaAI, GiteeAI,
Jiekou.AI, GreenPT) report ``token_count`` from the HTTP response. The
providers disagree on the usage field names: Jina emits
``usage.total_tokens`` while GiteeAI/Moark emits OpenAI-style
``usage.prompt_tokens``/``completion_tokens`` in camelCase
(``promptTokens``/``completionTokens``/``totalTokens``). Reading the response
through the legacy ``total_token_count_from_response`` helper silently returned
0 for those shapes, so ``_compute_rank`` now goes through the shared
``usage_from_response`` helper and falls back to a local count when the
provider reports nothing usable. These tests pin every supported shape.
"""

import logging
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from rag.llm.rerank_model import (
    GiteeRerank,
    GreenPTRerank,
    JiekouAIRerank,
    JinaRerank,
    NovitaRerank,
)

pytestmark = pytest.mark.p1


def _mock_post(payload):
    """Patch ``requests.post`` so ``response.json()`` returns ``payload``."""
    response = MagicMock()
    response.raise_for_status.return_value = None
    response.json.return_value = payload
    return patch("rag.llm.rerank_model.requests.post", return_value=response)


def _results(*scores):
    return {"results": [{"index": index, "relevance_score": score} for index, score in enumerate(scores)]}


def _similarity(reranker, payload):
    """Run ``similarity`` against a mocked response, bypassing real tokenization."""
    with _mock_post(payload), patch("rag.llm.rerank_model.num_tokens_from_string", return_value=1):
        return reranker.similarity("query", ["first", "second"])


def _jina(**kwargs):
    return JinaRerank("key", base_url="http://example.test/rerank", **kwargs)


# --- Supported usage shapes -------------------------------------------------


def test_jina_native_total_tokens_is_used():
    rank, tokens = _similarity(_jina(), {**_results(0.9, 0.1), "usage": {"total_tokens": 42}})
    assert tokens == 42
    assert np.allclose(rank, [0.9, 0.1])


def test_openai_style_prompt_and_completion_tokens_are_summed():
    """GiteeAI/Moark omits ``total_tokens``; the count must still be reported."""
    payload = {**_results(0.9, 0.1), "usage": {"prompt_tokens": 7, "completion_tokens": 3}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 10


def test_openai_style_total_tokens_takes_precedence():
    payload = {**_results(0.5), "usage": {"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 12}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 12


def test_anthropic_style_input_and_output_tokens_are_summed():
    payload = {**_results(0.5), "usage": {"input_tokens": 5, "output_tokens": 2}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 7


def test_camel_case_prompt_and_completion_tokens_are_summed():
    """Moark/GiteeAI spell the OpenAI fields in camelCase."""
    payload = {**_results(0.5), "usage": {"promptTokens": 7, "completionTokens": 3}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 10


def test_camel_case_total_tokens_takes_precedence():
    payload = {**_results(0.5), "usage": {"promptTokens": 7, "completionTokens": 3, "totalTokens": 12}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 12


# --- Local fallback when the provider reports nothing usable ----------------
#
# ``_similarity`` patches ``num_tokens_from_string`` to return 1, so the local
# fallback counts the query plus the two documents as 3.


def test_missing_usage_falls_back_to_local_count():
    _, tokens = _similarity(_jina(), _results(0.5))
    assert tokens == 3


def test_zero_valued_usage_falls_back_to_local_count():
    """The exact Moark shape: camelCase keys that still carry zeros."""
    payload = {**_results(0.5), "usage": {"totalTokens": 0, "promptTokens": 0}}
    _, tokens = _similarity(_jina(), payload)
    assert tokens == 3


# --- Diagnostics ------------------------------------------------------------


def test_unmapped_usage_falls_back_and_logs_response_shape(caplog):
    """An unrecognized usage layout must be traceable without dumping content."""
    payload = {**_results(0.5), "meta": {"billed_units": {"search_units": 1}}}
    with caplog.at_level(logging.DEBUG):
        _, tokens = _similarity(_jina(), payload)
    assert tokens == 3
    assert any("fell back to local count=3" in record.getMessage() for record in caplog.records)
    assert any("meta=['billed_units']" in record.getMessage() for record in caplog.records)


def test_mapped_usage_logs_resolved_split(caplog):
    payload = {**_results(0.5), "usage": {"prompt_tokens": 7, "completion_tokens": 3}}
    with caplog.at_level(logging.DEBUG):
        _, tokens = _similarity(_jina(), payload)
    assert tokens == 10
    assert any("total_tokens=10 prompt_tokens=7 completion_tokens=3" in record.getMessage() for record in caplog.records)


# --- Every Jina-family provider inherits the fix ----------------------------


@pytest.mark.parametrize(
    ("factory", "kwargs"),
    [
        (JinaRerank, {"base_url": "http://example.test/rerank"}),
        (NovitaRerank, {}),
        (GiteeRerank, {}),
        (JiekouAIRerank, {}),
        (GreenPTRerank, {}),
    ],
)
def test_jina_family_providers_count_openai_style_usage(factory, kwargs):
    payload = {**_results(0.4, 0.6), "usage": {"prompt_tokens": 11, "completion_tokens": 4}}
    _, tokens = _similarity(factory("key", "test-model", **kwargs), payload)
    assert tokens == 15
