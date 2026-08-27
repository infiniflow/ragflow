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
from io import BytesIO

with warnings.catch_warnings():
    warnings.filterwarnings("ignore", message=".*pkg_resources is deprecated.*", category=UserWarning)
    import pkg_resources  # noqa: F401 - stabilize xgboost import during collection

import pytest
from openpyxl import Workbook

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
def test_excel_keeps_zero_valued_answers():
    wb = Workbook()
    ws = wb.active
    ws.append(["plain question", "plain answer"])
    ws.append(["how many retries", 0])
    ws.append(["what is the delta", 0.0])
    ws.append(["is it enabled", False])
    ws.append(["count of rows", 7])
    binary = BytesIO()
    wb.save(binary)
    messages = []

    def record_callback(_progress, message):
        messages.append(message)

    chunks = qa.chunk(
        "qa.xlsx",
        binary=binary.getvalue(),
        lang="English",
        callback=record_callback,
    )

    assert len(chunks) == 5
    assert [chunk["content_with_weight"] for chunk in chunks] == [
        "Question: plain question\tAnswer: plain answer",
        "Question: how many retries\tAnswer: 0",
        "Question: what is the delta\tAnswer: 0",
        "Question: is it enabled\tAnswer: False",
        "Question: count of rows\tAnswer: 7",
    ]
    assert not any("failure" in message for message in messages)


@pytest.mark.p2
def test_excel_drops_whitespace_only_cells():
    wb = Workbook()
    ws = wb.active
    ws.append(["valid question", "valid answer"])
    ws.append(["   ", "ignored answer"])
    binary = BytesIO()
    wb.save(binary)
    messages = []

    def record_callback(_progress, message):
        messages.append(message)

    chunks = qa.chunk(
        "qa.xlsx",
        binary=binary.getvalue(),
        lang="English",
        callback=record_callback,
    )

    assert len(chunks) == 1
    assert any("failure" in message for message in messages)
