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

import json
from pathlib import Path

from rag.nlp import rag_tokenizer
from rag.nlp.term_weight import Dealer, _alphabetic_oov_frequency


def test_alphabetic_oov_frequency_matches_shared_fixture():
    """Keep the Python fallback aligned with the fixture consumed by Go tests."""
    fixture_path = Path(__file__).parents[3] / "fixtures" / "term_weight_oov.json"
    cases = json.loads(fixture_path.read_text(encoding="utf-8"))

    for case in cases:
        assert _alphabetic_oov_frequency(case["term"]) == case["frequency"]


def test_oov_weights_favor_longer_content_terms(monkeypatch):
    """Avoid equal weights when no dictionary or tokenizer frequency is available."""
    monkeypatch.setattr(rag_tokenizer, "freq", lambda _term: 0)
    monkeypatch.setattr(rag_tokenizer, "tag", lambda _term: "")
    dealer = Dealer()
    dealer.df = {}

    weights = dict(dealer.weights(["was", "largest", "supplier", "equipment"], preprocess=False))

    assert weights["was"] < weights["largest"] < weights["supplier"] < weights["equipment"]

    dealer.df["equipment"] = 1_000_000
    weights = dict(dealer.weights(["supplier", "equipment"], preprocess=False))
    assert weights["equipment"] < weights["supplier"]
