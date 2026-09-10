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

import importlib.util
import logging
from pathlib import Path
import sys
import types

import pytest

from common.doc_store.doc_store_base import MatchTextExpr, FusionExpr


class _DummyFulltextQueryer:
    def question(self, text, min_match=0):
        return MatchTextExpr(fields=["content_ltks"], matching_text=text, topn=10, extra_options={}), ["keyword"]

    def token_similarity(self, keywords, documents):
        return [0.4] * len(documents)


_ROOT = Path(__file__).parents[3]


@pytest.fixture
def search_environment(monkeypatch):
    fake_rag_nlp = types.ModuleType("rag.nlp")
    fake_rag_nlp.__path__ = []

    fake_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
    fake_tokenizer.fine_grained_tokenize = lambda text: text
    fake_rag_nlp.rag_tokenizer = fake_tokenizer

    fake_query = types.ModuleType("rag.nlp.query")
    fake_query.FulltextQueryer = _DummyFulltextQueryer
    fake_rag_nlp.query = fake_query

    fake_settings = types.ModuleType("common.settings")
    fake_settings.DOC_ENGINE_OCEANBASE = False
    fake_settings.DOC_ENGINE_GAUSSDB = False
    fake_settings.DOC_ENGINE_SERENEDB = False
    fake_settings.DOC_ENGINE_INFINITY = True

    fake_token_utils = types.ModuleType("common.token_utils")
    fake_token_utils.num_tokens_from_string = lambda text: len(text.split())

    import common

    monkeypatch.setattr(common, "settings", fake_settings, raising=False)
    monkeypatch.setattr(common, "token_utils", fake_token_utils, raising=False)
    monkeypatch.setitem(sys.modules, "common.settings", fake_settings)
    monkeypatch.setitem(sys.modules, "common.token_utils", fake_token_utils)
    monkeypatch.setitem(sys.modules, "rag.nlp", fake_rag_nlp)
    monkeypatch.setitem(sys.modules, "rag.nlp.rag_tokenizer", fake_tokenizer)
    monkeypatch.setitem(sys.modules, "rag.nlp.query", fake_query)

    search_spec = importlib.util.spec_from_file_location("rag.nlp.search", _ROOT / "rag" / "nlp" / "search.py")
    search_module = importlib.util.module_from_spec(search_spec)
    assert search_spec.loader is not None
    monkeypatch.setitem(sys.modules, "rag.nlp.search", search_module)
    search_spec.loader.exec_module(search_module)

    return search_module


class _FakeEmbeddingModel:
    def encode_queries(self, text):
        return [0.1, 0.2], None


class _CapturingDataStore:
    def __init__(self):
        self.match_expressions = None

    def search(self, *args, **kwargs):
        if len(args[3]) == 3:
            self.match_expressions = args[3]
        return object()

    def get_total(self, result):
        return 1

    def get_doc_ids(self, result):
        return ["chunk-1"]

    def get_fields(self, result, fields):
        return {
            "chunk-1": {
                "_score": 1.0,
                "content_ltks": "test content",
                "content_with_weight": "test content",
                "kb_id": "kb-1",
            }
        }

    def get_scores(self, result):
        return {"chunk-1": 0.5}

    def get_highlight(self, result, keywords, field_name):
        return {}

    def get_aggregation(self, result, field_name):
        return []


@pytest.mark.asyncio
async def test_dealer_retrieval_passes_vector_similarity_weight_to_fusion_expr(search_environment, caplog):
    caplog.set_level(logging.DEBUG)
    data_store = _CapturingDataStore()
    dealer = search_environment.Dealer(data_store)

    await dealer.retrieval(
        question="test question",
        embd_mdl=_FakeEmbeddingModel(),
        tenant_ids=["tenant-1"],
        kb_ids=["kb-1"],
        page=1,
        page_size=10,
        similarity_threshold=0.0,
        vector_similarity_weight=0.8,
        knn_top_k=10,
        aggs=False,
    )

    assert data_store.match_expressions is not None
    assert len(data_store.match_expressions) == 3
    fusion_expr = data_store.match_expressions[2]
    assert isinstance(fusion_expr, FusionExpr)
    assert fusion_expr.fusion_params["weights"] == "0.2,0.8"
    assert "Dealer.search fusion" in caplog.text
    assert "vector_similarity_weight=0.8" in caplog.text


@pytest.mark.asyncio
async def test_dealer_retrieval_keeps_elasticsearch_fusion_weight(search_environment):
    data_store = _CapturingDataStore()
    dealer = search_environment.Dealer(data_store)

    import common

    common.settings.DOC_ENGINE_INFINITY = False

    await dealer.retrieval(
        question="test question",
        embd_mdl=_FakeEmbeddingModel(),
        tenant_ids=["tenant-1"],
        kb_ids=["kb-1"],
        page=1,
        page_size=10,
        similarity_threshold=0.0,
        vector_similarity_weight=0.8,
        knn_top_k=10,
        aggs=False,
    )

    assert data_store.match_expressions is not None
    fusion_expr = data_store.match_expressions[2]
    assert isinstance(fusion_expr, FusionExpr)
    assert fusion_expr.fusion_params["weights"] == "0.001,1"


@pytest.mark.parametrize(
    ("vector_similarity_weight", "expected_weights"),
    [
        (0.0, "1,0"),
        (0.3, "0.7,0.3"),
        (0.5, "0.5,0.5"),
        (1.0, "0,1"),
    ],
)
def test_build_fusion_expr_uses_vector_similarity_weight(search_environment, vector_similarity_weight, expected_weights):
    fusion_expr = search_environment.build_fusion_expr(topn=10, vector_similarity_weight=vector_similarity_weight)

    assert fusion_expr.method == "weighted_sum"
    assert fusion_expr.topn == 10
    assert fusion_expr.fusion_params["weights"] == expected_weights
