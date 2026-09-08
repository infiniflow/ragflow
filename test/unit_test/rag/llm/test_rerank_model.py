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

import sys
from http import HTTPStatus
from types import SimpleNamespace
from unittest.mock import Mock, patch

import pytest

from rag.llm.rerank_model import QWenRerank


def _dashscope_stub():
    response = SimpleNamespace(
        status_code=HTTPStatus.OK,
        output=SimpleNamespace(results=[]),
    )
    call = Mock(return_value=response)
    dashscope = SimpleNamespace(
        TextReRank=SimpleNamespace(
            Models=SimpleNamespace(gte_rerank="gte-rerank-v2"),
            call=call,
        )
    )
    return dashscope, call


@pytest.mark.parametrize(
    ("model_name", "expected_model", "expects_return_documents"),
    [
        ("qwen3-rerank", "qwen3-rerank", False),
        ("gte-rerank-v2", "gte-rerank-v2", True),
        ("", "gte-rerank-v2", True),
        (None, "gte-rerank-v2", True),
    ],
)
def test_qwen_rerank_builds_provider_specific_call_kwargs(model_name, expected_model, expects_return_documents):
    dashscope, call = _dashscope_stub()

    with (
        patch.dict(sys.modules, {"dashscope": dashscope}),
        patch("rag.llm.rerank_model.total_token_count_from_response", return_value=0),
    ):
        reranker = QWenRerank("test-key", model_name=model_name)
        reranker.similarity("query", ["document"])

    call.assert_called_once()
    call_kwargs = call.call_args.kwargs
    assert call_kwargs["api_key"] == "test-key"
    assert call_kwargs["model"] == expected_model
    assert call_kwargs["query"] == "query"
    assert call_kwargs["documents"] == ["document"]
    assert call_kwargs["top_n"] == 1
    assert call_kwargs["request_timeout"] == 30.0
    if expects_return_documents:
        assert call_kwargs["return_documents"] is False
    else:
        assert "return_documents" not in call_kwargs
