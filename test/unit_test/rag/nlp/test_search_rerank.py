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

"""Rerank token-assembly tests for ``rag.nlp.search.Dealer``.

All three rerank paths build a per-chunk token list (``ins_tw``) that feeds the
term-similarity score, and ``rerank_by_model`` additionally joins it back into
the ``docs`` strings handed to the reranker model. These tests pin which chunk
fields end up in that list.
"""

from unittest.mock import MagicMock

import numpy as np
import pytest

# `rag.nlp.query` imports `rag.utils.redis_conn`, which imports `common.settings`,
# which imports `rag.utils.redis_conn` back. That cycle only resolves when
# `common.settings` is initialised first -- which is what the API server and task
# executor do at startup, so importing it here reproduces the runtime order.
import common.settings  # noqa: F401

from rag.nlp.search import Dealer


def _dealer(captured):
    """Dealer with ``__init__`` bypassed (it builds a real FulltextQueryer,
    which loads the tokenizer) and ``qryr`` stubbed so we can capture ins_tw."""
    dealer = Dealer.__new__(Dealer)
    qryr = MagicMock()
    qryr.question.return_value = (None, ["apple"])

    def _token_similarity(keywords, ins_tw):
        captured["ins_tw"] = ins_tw
        return [0.5] * len(ins_tw)

    def _hybrid_similarity(ans_embd, ins_embd, ans, inst, tkweight, vtweight):
        captured["ins_tw"] = inst
        n = len(ins_embd)
        return np.array([0.5] * n), np.array([0.5] * n), np.array([0.5] * n)

    qryr.token_similarity.side_effect = _token_similarity
    qryr.hybrid_similarity.side_effect = _hybrid_similarity
    dealer.qryr = qryr
    return dealer


def _sres(**field_overrides):
    field = {
        "content_ltks": "apple sells phones",
        "title_tks": "apple report",
        "question_tks": "what does apple sell",
        "important_kwd": ["revenue"],
        "q_3_vec": [0.1, 0.2, 0.3],
    }
    field.update(field_overrides)
    return Dealer.SearchResult(total=1, ids=["c1"], query_vector=[0.1, 0.2, 0.3], field={"c1": field})


def test_rerank_by_model_includes_question_tks():
    # Regression: rerank_by_model omitted question_tks entirely, so the field
    # reached neither token_similarity nor the docs sent to the reranker --
    # even though rerank()/rerank_with_knn() both weight it highest and the
    # ES query boosts question_tks^20.
    captured = {}
    dealer = _dealer(captured)
    rerank_mdl = MagicMock()
    rerank_mdl.similarity.return_value = (np.array([0.5]), 0)

    dealer.rerank_by_model(rerank_mdl, _sres(), "apple")

    tokens = captured["ins_tw"][0]
    assert "what" in tokens and "sell" in tokens

    # The reranker model must see the questions too: ins_tw is joined into docs.
    docs = rerank_mdl.similarity.call_args[0][1]
    assert "what does apple sell" in docs[0]


def test_rerank_by_model_does_not_repeat_fields():
    # rerank_by_model deliberately drops the *2/*5/*6 multipliers used by the
    # other two paths: its tokens are joined back into a string for a
    # cross-encoder, where duplicating a field would distort the model's score.
    captured = {}
    dealer = _dealer(captured)
    rerank_mdl = MagicMock()
    rerank_mdl.similarity.return_value = (np.array([0.5]), 0)

    dealer.rerank_by_model(rerank_mdl, _sres(), "apple")

    tokens = captured["ins_tw"][0]
    assert tokens.count("revenue") == 1
    assert tokens.count("what") == 1


@pytest.mark.p1
@pytest.mark.parametrize("path", ["rerank", "rerank_with_knn", "rerank_by_model"])
def test_all_rerank_paths_include_question_tks(path):
    # Parity guard: whichever rerank path a deployment takes, the chunk's
    # generated questions must contribute to the score.
    captured = {}
    dealer = _dealer(captured)

    if path == "rerank":
        dealer.rerank(_sres(), "apple")
    elif path == "rerank_with_knn":
        dealer.rerank_with_knn(_sres(), "apple", {"c1": 0.5})
    else:
        rerank_mdl = MagicMock()
        rerank_mdl.similarity.return_value = (np.array([0.5]), 0)
        dealer.rerank_by_model(rerank_mdl, _sres(), "apple")

    assert "what" in captured["ins_tw"][0], f"{path} dropped question_tks"


def test_rerank_by_model_handles_missing_question_tks():
    # Chunks indexed before question generation have no question_tks field.
    captured = {}
    dealer = _dealer(captured)
    rerank_mdl = MagicMock()
    rerank_mdl.similarity.return_value = (np.array([0.5]), 0)

    sres = _sres()
    del sres.field["c1"]["question_tks"]
    dealer.rerank_by_model(rerank_mdl, sres, "apple")

    tokens = captured["ins_tw"][0]
    assert "apple" in tokens
    assert "what" not in tokens
