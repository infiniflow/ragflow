#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Unit tests for the AWS Bedrock reranker connector.

``BedrockRerank`` talks to the ``bedrock-agent-runtime`` Rerank API via boto3.
These tests patch ``boto3.client`` so no AWS call is made, and verify the
score-by-index mapping, per-model document truncation and key/ARN handling.
"""

import json
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from common.token_utils import num_tokens_from_string
from rag.llm.rerank_model import BedrockRerank

pytestmark = pytest.mark.p1

KEY = json.dumps(
    {
        "auth_mode": "access_key_secret",
        "bedrock_region": "eu-central-1",
        "bedrock_ak": "AKIA_TEST",
        "bedrock_sk": "secret_test",
    }
)


def _rerank_response(scores):
    """Bedrock returns results out of order; ``index`` maps back to the source doc."""
    return {"results": [{"index": i, "relevanceScore": s} for i, s in scores]}


def _make(model_name="amazon.rerank-v1:0", key=KEY):
    """Instantiate the connector with ``boto3.client`` patched; return (model, client)."""
    with patch("boto3.client") as client_factory:
        client = MagicMock()
        client_factory.return_value = client
        mdl = BedrockRerank(key, model_name)
    return mdl, client


def test_scores_are_mapped_back_by_index():
    mdl, client = _make()
    # Response deliberately out of source order.
    client.rerank.return_value = _rerank_response([(2, 0.9), (0, 0.1), (1, 0.5)])
    rank, _ = mdl.similarity("q", ["a", "b", "c"])
    assert np.allclose(rank, [0.1, 0.5, 0.9])


def test_model_arn_is_built_from_region_and_name():
    mdl, _ = _make(model_name="cohere.rerank-v3-5:0")
    assert mdl.model_arn == "arn:aws:bedrock:eu-central-1::foundation-model/cohere.rerank-v3-5:0"


def test_doc_window_depends_on_model():
    cohere, _ = _make(model_name="cohere.rerank-v3-5:0")
    amazon, _ = _make(model_name="amazon.rerank-v1:0")
    assert cohere.doc_max_tokens == 2048
    assert amazon.doc_max_tokens == 8192


def test_documents_are_truncated_before_send():
    mdl, client = _make(model_name="cohere.rerank-v3-5:0")  # cap 2048
    client.rerank.return_value = _rerank_response([(0, 0.5)])
    mdl.similarity("q", ["mot " * 5000])  # > 2048 tokens
    sent = client.rerank.call_args.kwargs["sources"][0]["inlineDocumentSource"]["textDocument"]["text"]
    assert num_tokens_from_string(sent) <= 2048


def test_number_of_results_covers_all_documents():
    mdl, client = _make()
    client.rerank.return_value = _rerank_response([(0, 0.3), (1, 0.6)])
    mdl.similarity("q", ["a", "b"])
    cfg = client.rerank.call_args.kwargs["rerankingConfiguration"]["bedrockRerankingConfiguration"]
    assert cfg["numberOfResults"] == 2
    assert cfg["modelConfiguration"]["modelArn"].endswith("amazon.rerank-v1:0")


def test_missing_auth_mode_raises():
    with patch("boto3.client"):
        with pytest.raises(ValueError):
            BedrockRerank(json.dumps({"bedrock_region": "eu-central-1"}), "amazon.rerank-v1:0")


def test_access_key_secret_mode_wires_the_client():
    with patch("boto3.client") as client_factory:
        client_factory.return_value = MagicMock()
        BedrockRerank(KEY, "amazon.rerank-v1:0")
    kwargs = client_factory.call_args.kwargs
    assert kwargs["service_name"] == "bedrock-agent-runtime"
    assert kwargs["region_name"] == "eu-central-1"
    assert kwargs["aws_access_key_id"] == "AKIA_TEST"
    assert kwargs["aws_secret_access_key"] == "secret_test"


@pytest.mark.parametrize("query, texts", [("", ["a"]), ("q", [])])
def test_empty_input_short_circuits_without_calling_bedrock(query, texts):
    mdl, client = _make()
    rank, tokens = mdl.similarity(query, texts)
    assert tokens == 0
    assert rank.size == len(texts)
    client.rerank.assert_not_called()
