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
"""Unit tests for the reranker input built by Dealer.rerank_by_model.

External rerankers must be scored on the natural chunk text
(``content_with_weight``, markup preserved), not the tokenized
``content_ltks``: neural rerankers score stemmed / accent-split tokens far
lower, which collapses relevance scores. The tokenized ``ins_tw`` used for the
keyword-similarity part of the blend must stay unchanged.
"""

import sys
import types

import numpy as np
import pytest

# Stub the heavy / circular-importing dependencies before importing search,
# mirroring test_search_pagination.py so the module imports in isolation.
_fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


_fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", _fake_query)
sys.modules.setdefault("common.settings", types.ModuleType("common.settings"))

from rag.nlp.search import Dealer  # noqa: E402

pytestmark = pytest.mark.p1


class _CapturingRerank:
    """Reranker double that records the documents it is asked to score."""

    def __init__(self):
        self.docs = None

    def similarity(self, query, docs):
        self.docs = list(docs)
        return np.zeros(len(docs)), 0


def _dealer(token_similarity_spy=None):
    dealer = Dealer.__new__(Dealer)

    class _Queryer:
        def question(self, query):
            return None, ["kw"]

        def token_similarity(self, keywords, ins_tw):
            if token_similarity_spy is not None:
                token_similarity_spy["ins_tw"] = ins_tw
            return [0.0] * len(ins_tw)

    dealer.qryr = _Queryer()
    # Tag/pagerank feature scoring is out of scope here.
    dealer._rank_feature_scores = lambda rank_feature, sres: np.zeros(len(sres.ids))
    return dealer


def _sres(field_by_id):
    return Dealer.SearchResult(total=len(field_by_id), ids=list(field_by_id), field=field_by_id)


def test_reranker_receives_natural_text():
    rr = _CapturingRerank()
    sres = _sres({"c1": {"content_with_weight": "Paris is the capital of France.", "content_ltks": "pari capit franc"}})
    _dealer().rerank_by_model(rr, sres, "q")
    assert rr.docs == ["Paris is the capital of France."]


def test_falls_back_to_tokenized_when_natural_absent():
    rr = _CapturingRerank()
    sres = _sres({"c1": {"content_ltks": "pari capit franc", "title_tks": "titre", "important_kwd": ["kw1"]}})
    _dealer().rerank_by_model(rr, sres, "q")
    assert rr.docs == ["pari capit franc titre kw1"]


def test_markup_is_preserved_not_stripped():
    rr = _CapturingRerank()
    html = "<table> <thead> <tr> <th>Contact</th> </tr> </thead> </table>"
    sres = _sres({"c1": {"content_with_weight": html, "content_ltks": "contact"}})
    _dealer().rerank_by_model(rr, sres, "q")
    assert "<table>" in rr.docs[0] and "<th>Contact</th>" in rr.docs[0]


def test_keyword_similarity_still_uses_tokenized_tokens():
    spy = {}
    rr = _CapturingRerank()
    sres = _sres({"c1": {"content_with_weight": "Natural prose.", "content_ltks": "natur pros", "important_kwd": ["kw1"]}})
    _dealer(token_similarity_spy=spy).rerank_by_model(rr, sres, "q")
    # The keyword blend input stays the tokenized tokens, not the natural text.
    assert spy["ins_tw"] == [["natur", "pros", "kw1"]]
