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

import pytest

from common import settings
from rag.nlp import rag_tokenizer


@pytest.fixture(autouse=True)
def non_infinity_engine(monkeypatch):
    # tokenize() is a passthrough under DOC_ENGINE=infinity (tokenization
    # happens server-side); force the local tokenizer path.
    monkeypatch.setattr(settings, "DOC_ENGINE_INFINITY", False, raising=False)
    yield
    rag_tokenizer.tokenizer.set_language("English")


@pytest.mark.p2
@pytest.mark.parametrize(
    ("text", "folded"),
    [
        ("škola", "skola"),
        ("daňové priznanie", "danove priznanie"),
        ("požiarna bezpečnosť", "poziarna bezpecnost"),
        ("ľĺŕôä", "llroa"),
        ("příliš žluťoučký kůň", "prilis zlutoucky kun"),
        ("ěščřžýáíéúů", "escrzyaieuu"),
        ("Škola", "Skola"),
        ("plain ascii stays", "plain ascii stays"),
        ("", ""),
    ],
)
def test_fold_diacritics(text, folded):
    assert rag_tokenizer.fold_diacritics(text) == folded


@pytest.mark.p2
@pytest.mark.parametrize("language", ["Slovak", "slovak", "SLOVAK", "Czech", "czech"])
@pytest.mark.parametrize(
    ("text", "expected"),
    [
        ("škola", "skola"),
        ("účet", "ucet"),
        ("daňové priznanie", "danove priznanie"),
        ("požiarna bezpečnosť", "poziarna bezpecnost"),
        ("čtvrtek", "ctvrtek"),
    ],
)
def test_folding_languages_keep_words_whole(language, text, expected):
    rag_tokenizer.tokenizer.set_language(language)

    assert rag_tokenizer.tokenize(text) == expected


@pytest.mark.p2
def test_folding_languages_match_unaccented_queries():
    # Users routinely type Slovak without diacritics; index and query
    # tokens must agree either way.
    rag_tokenizer.tokenizer.set_language("Slovak")

    assert rag_tokenizer.tokenize("požiarna bezpečnosť") == rag_tokenizer.tokenize("poziarna bezpecnost")


@pytest.mark.p2
def test_folding_languages_do_not_stem():
    # There is no Snowball stemmer for Slovak/Czech; tokens must not be
    # mangled by another language's stemmer left on the shared tokenizer.
    rag_tokenizer.tokenizer.set_language("English")
    rag_tokenizer.tokenizer.set_language("Slovak")

    assert rag_tokenizer.tokenize("skoly danove") == "skoly danove"


@pytest.mark.p2
def test_english_behavior_unchanged():
    rag_tokenizer.tokenizer.set_language("English")

    # Accented words keep the legacy fragmentation outside folding languages.
    assert rag_tokenizer.tokenize("škola") == "š kola"


@pytest.mark.p2
def test_fold_text_follows_language():
    rag_tokenizer.tokenizer.set_language("English")
    assert rag_tokenizer.tokenizer.fold_text("škola") == "škola"

    rag_tokenizer.tokenizer.set_language("Slovak")
    assert rag_tokenizer.tokenizer.fold_text("škola") == "skola"


@pytest.mark.p2
def test_fine_grained_tokenize_preserves_folded_tokens():
    rag_tokenizer.tokenizer.set_language("Slovak")
    tks = rag_tokenizer.tokenize("daňové priznanie k dani z nehnuteľností")

    assert rag_tokenizer.fine_grained_tokenize(tks).split() == tks.split()


@pytest.mark.p2
def test_switching_back_to_english_restores_stemming():
    rag_tokenizer.tokenizer.set_language("Slovak")
    rag_tokenizer.tokenizer.set_language("English")

    assert rag_tokenizer.tokenize("running") == "run"
    assert rag_tokenizer.tokenizer.fold_text("škola") == "škola"
