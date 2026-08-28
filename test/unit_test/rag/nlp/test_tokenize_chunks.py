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

from rag import nlp


@pytest.fixture(autouse=True)
def stub_tokenize(monkeypatch):
    def tokenize(doc, text, eng, language="English"):
        doc["content_with_weight"] = text

    monkeypatch.setattr(nlp, "tokenize", tokenize)


@pytest.mark.p2
@pytest.mark.parametrize(
    ("chunk", "parent"),
    [
        ("first\nsecond", "first\nsecond"),
        ("\nfirst\nsecond", "first\nsecond"),
        ("\n\nfirst\nsecond", "\nfirst\nsecond"),
    ],
)
def test_tokenize_chunks_removes_one_leading_newline_from_parent(chunk, parent):
    docs = nlp.tokenize_chunks([chunk], {}, True, child_delimiters_pattern="\n")

    assert [doc["mom_with_weight"] for doc in docs] == [parent, parent]
    assert [doc["content_with_weight"] for doc in docs] == ["first\n", "second"]


@pytest.mark.p2
def test_doc_tokenize_chunks_with_images_removes_leading_newline_from_parent():
    docs = nlp.doc_tokenize_chunks_with_images([{"text": "\nfirst\nsecond", "ck_type": "text"}], {}, True, child_delimiters_pattern="\n")

    assert [doc["mom_with_weight"] for doc in docs] == ["first\nsecond", "first\nsecond"]
    assert [doc["content_with_weight"] for doc in docs] == ["first\n", "second"]


@pytest.mark.p2
def test_tokenize_chunks_with_images_removes_leading_newline_from_parent():
    docs = nlp.tokenize_chunks_with_images(["\nfirst\nsecond"], {}, True, [object()], child_delimiters_pattern="\n")

    assert [doc["mom_with_weight"] for doc in docs] == ["first\nsecond", "first\nsecond"]
    assert [doc["content_with_weight"] for doc in docs] == ["first\n", "second"]


@pytest.mark.p2
def test_tokenize_chunks_with_positions_increments_zero_based_sheet():
    # Flow JSON / naive Excel emit 0-based sheet; add_positions stores pn+1.
    docs = nlp.tokenize_chunks_with_positions(
        [("row", (1, 2, 4, 1, 3))],
        {"docnm_kwd": "a.xlsx"},
        True,
    )
    assert docs[0]["position_int"] == [(2, 2, 4, 1, 3)]
    assert docs[0]["page_num_int"] == [2]
    assert docs[0]["content_with_weight"] == "row"


@pytest.mark.p2
def test_tokenize_chunks_with_positions_skips_blank_and_splits_children():
    docs = nlp.tokenize_chunks_with_positions(
        [("  ", (0, 2, 2, 1, 1)), ("first\nsecond", (0, 3, 4, 1, 2))],
        {},
        True,
        child_delimiters_pattern="\n",
    )
    assert [doc["content_with_weight"] for doc in docs] == ["first\n", "second"]
    assert all(doc["position_int"][0] == (1, 3, 4, 1, 2) for doc in docs)


@pytest.mark.p2
def test_flow_json_sheet_is_one_based_after_add_positions():
    # rag/flow/parser/parser.py output_format=json stores 0-based sheet.
    # task_executor / dataflow_service then call add_positions (pn+1).
    json_out = [
        {"text": "s1", "positions": [[0, 2, 2, 1, 1]]},
        {"text": "s2", "positions": [[1, 2, 2, 1, 1]]},
    ]
    sheets = []
    for ck in json_out:
        nlp.add_positions(ck, ck["positions"])
        del ck["positions"]
        sheets.append(ck["position_int"][0][0])
    assert sheets == [1, 2]
