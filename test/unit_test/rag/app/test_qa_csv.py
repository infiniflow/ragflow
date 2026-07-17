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

from __future__ import annotations

import warnings

with warnings.catch_warnings():
    warnings.filterwarnings("ignore", message=".*pkg_resources is deprecated.*", category=UserWarning)
    import pkg_resources  # noqa: F401 - stabilize xgboost import during collection

import pytest

from rag.app import qa


def _noop_callback(*_args, **_kwargs):
    pass


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


@pytest.mark.p2
def test_csv_final_pair_uses_last_line_number():
    chunks = qa.chunk(
        "qa.csv",
        binary=b"Question 1,Answer 1\nQuestion 2,Answer 2",
        lang="English",
        callback=_noop_callback,
    )

    assert len(chunks) == 2
    assert chunks[0]["top_int"] == [1]
    assert chunks[1]["top_int"] == [2]


@pytest.mark.p2
def test_csv_multiline_quoted_answer_preserves_pairs_and_source_lines():
    chunks = qa.chunk(
        "qa.csv",
        binary=b'First prompt,"first line\nsecond line"\nSecond prompt,second answer',
        lang="English",
        callback=_noop_callback,
    )

    assert len(chunks) == 2
    assert chunks[0]["content_with_weight"] == "Question: First prompt\tAnswer: first line\nsecond line"
    assert chunks[0]["top_int"] == [2]
    assert chunks[1]["content_with_weight"] == "Question: Second prompt\tAnswer: second answer"
    assert chunks[1]["top_int"] == [3]


@pytest.mark.p2
def test_txt_multiline_quoted_answer_uses_csv_reader_flow():
    chunks = qa.chunk(
        "qa.txt",
        binary=b'First prompt\t"first line\nsecond line"\nSecond prompt\tsecond answer',
        lang="English",
        callback=_noop_callback,
    )

    assert len(chunks) == 2
    assert chunks[0]["content_with_weight"] == "Question: First prompt\tAnswer: first line\nsecond line"
    assert chunks[0]["top_int"] == [2]
    assert chunks[1]["content_with_weight"] == "Question: Second prompt\tAnswer: second answer"
    assert chunks[1]["top_int"] == [3]


@pytest.mark.p2
def test_csv_progress_callback_does_not_repeat_when_no_chunks_are_added():
    callbacks = []

    def collect_callback(*args):
        callbacks.append(args)

    chunks = qa.chunk(
        "qa.csv",
        binary=b"malformed row\nanother malformed row\nQuestion,Answer",
        lang="English",
        callback=collect_callback,
    )

    assert len(chunks) == 1
    assert [args[0] for args in callbacks] == [0.1, 0.6]
