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
import types
import importlib.util
from pathlib import Path

import pytest

from common.doc_store.doc_store_base import MatchTextExpr, FusionExpr


fake_rag_nlp = types.ModuleType("rag.nlp")
fake_rag_nlp.__path__ = []

fake_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
fake_tokenizer.fine_grained_tokenize = lambda text: text
fake_rag_nlp.rag_tokenizer = fake_tokenizer
sys.modules["rag.nlp"] = fake_rag_nlp
sys.modules["rag.nlp.rag_tokenizer"] = fake_tokenizer

fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    def question(self, text, min_match):
        return MatchTextExpr(fields=["content_ltks"], matching_text=text, topn=10, extra_options={}), ["keyword"]


fake_query.FulltextQueryer = _DummyFulltextQueryer
fake_rag_nlp.query = fake_query
sys.modules["rag.nlp.query"] = fake_query

fake_settings = types.ModuleType("common.settings")
fake_settings.DOC_ENGINE_OCEANBASE = False
sys.modules.setdefault("common.settings", fake_settings)

fake_token_utils = types.ModuleType("common.token_utils")
fake_token_utils.num_tokens_from_string = lambda text: len(text.split())
sys.modules.setdefault("common.token_utils", fake_token_utils)

from common import settings as rag_settings  # noqa: E402

_ROOT = Path(__file__).parents[3]
_FUSION_SPEC = importlib.util.spec_from_file_location("rag.nlp.fusion", _ROOT / "rag" / "nlp" / "fusion.py")
_FUSION_MODULE = importlib.util.module_from_spec(_FUSION_SPEC)
assert _FUSION_SPEC.loader is not None
sys.modules["rag.nlp.fusion"] = _FUSION_MODULE
_FUSION_SPEC.loader.exec_module(_FUSION_MODULE)
fake_rag_nlp.fusion = _FUSION_MODULE

_SEARCH_SPEC = importlib.util.spec_from_file_location("rag.nlp.search", _ROOT / "rag" / "nlp" / "search.py")
_SEARCH_MODULE = importlib.util.module_from_spec(_SEARCH_SPEC)
assert _SEARCH_SPEC.loader is not None
sys.modules["rag.nlp.search"] = _SEARCH_MODULE
_SEARCH_SPEC.loader.exec_module(_SEARCH_MODULE)
Dealer = _SEARCH_MODULE.Dealer

rag_settings.DOC_ENGINE_OCEANBASE = False
rag_settings.DOC_ENGINE_INFINITY = True


class _FakeEmbeddingModel:
    def encode_queries(self, text):
        return [0.1, 0.2], None


class _CapturingDataStore:
    def __init__(self):
        self.match_expressions = None

    def search(self, *args, **kwargs):
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

    def get_highlight(self, result, keywords, field_name):
        return {}

    def get_aggregation(self, result, field_name):
        return []


@pytest.mark.asyncio
async def test_dealer_retrieval_passes_vector_similarity_weight_to_fusion_expr():
    data_store = _CapturingDataStore()
    dealer = Dealer(data_store)

    await dealer.retrieval(
        question="test question",
        embd_mdl=_FakeEmbeddingModel(),
        tenant_ids=["tenant-1"],
        kb_ids=["kb-1"],
        page=1,
        page_size=10,
        similarity_threshold=0.0,
        vector_similarity_weight=0.8,
        top=10,
        aggs=False,
    )

    assert data_store.match_expressions is not None
    assert len(data_store.match_expressions) == 3
    fusion_expr = data_store.match_expressions[2]
    assert isinstance(fusion_expr, FusionExpr)
    assert fusion_expr.fusion_params["weights"] == "0.2,0.8"
