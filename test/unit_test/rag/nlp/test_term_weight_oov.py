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
import logging
from pathlib import Path

from rag.nlp import rag_tokenizer, term_weight
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


def test_missing_frequency_dictionary_is_silent(tmp_path, monkeypatch, caplog):
    """Treat an absent optional frequency dictionary as an expected fallback."""
    resource_dir = tmp_path / "rag" / "res"
    resource_dir.mkdir(parents=True)
    (resource_dir / "ner.json").write_text("{}", encoding="utf-8")
    monkeypatch.setattr(term_weight, "get_project_base_directory", lambda: str(tmp_path))

    with caplog.at_level(logging.WARNING):
        Dealer()

    assert "Load term.freq FAIL!" not in caplog.text


def test_malformed_frequency_dictionary_logs_warning(tmp_path, monkeypatch, caplog):
    """Expose malformed dictionaries instead of silently enabling the fallback."""
    resource_dir = tmp_path / "rag" / "res"
    resource_dir.mkdir(parents=True)
    (resource_dir / "ner.json").write_text("{}", encoding="utf-8")
    (resource_dir / "term.freq").write_text("term\tnot-a-number\n", encoding="utf-8")
    monkeypatch.setattr(term_weight, "get_project_base_directory", lambda: str(tmp_path))

    with caplog.at_level(logging.WARNING):
        Dealer()

    assert "Load term.freq FAIL!" in caplog.text
