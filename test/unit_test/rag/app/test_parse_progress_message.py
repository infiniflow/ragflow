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

"""Parse progress text keeps the parsed count and the failure count apart.

Each parser appends "<n> failure, line: ..." to a count it has just formatted, and
the result is shown on the dataset page. Without a separator one parsed pair and one
failed row read as "Extract Q&A: 11 failure, line: 1...".
"""

from __future__ import annotations

import importlib
import re

import pytest

# One malformed line followed by one valid pair.
QA_BINARY = b"malformed row\nWhat is X?,X is a thing\n"
# A header, one row with too few fields, one complete row.
TABLE_CSV = b"a,b\n1\n2,3\n"
TABLE_TXT = b"a\tb\n1\n2\t3"


def _load(module_name: str):
    """Import a `rag.app` parser inside the test, after collection has finished."""
    return importlib.import_module(f"rag.app.{module_name}")


class _ProgressRecorder:
    """Collects the progress text each parser hands to its callback."""

    def __init__(self):
        self.messages = []

    def __call__(self, prog=None, msg=""):
        if msg:
            self.messages.append(msg)

    @property
    def last(self):
        return self.messages[-1]


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


@pytest.mark.p2
def test_qa_csv_progress_separates_the_two_counts():
    callback = _ProgressRecorder()
    _load("qa").chunk("qa.csv", binary=QA_BINARY, lang="English", callback=callback)

    assert re.search(r"\b1\D+1 failure\b", callback.last)


@pytest.mark.p2
def test_qa_txt_progress_separates_the_two_counts():
    callback = _ProgressRecorder()
    _load("qa").chunk("qa.txt", binary=QA_BINARY, lang="English", callback=callback)

    assert re.search(r"\b1\D+1 failure\b", callback.last)


@pytest.mark.p2
def test_table_csv_progress_separates_the_two_counts():
    callback = _ProgressRecorder()
    _load("table").chunk("rows.csv", binary=TABLE_CSV, lang="English", callback=callback)

    assert re.search(r"0~1\D+1 failure\b", callback.last)


@pytest.mark.p2
def test_table_txt_progress_separates_the_two_counts():
    callback = _ProgressRecorder()
    _load("table").chunk("rows.txt", binary=TABLE_TXT, lang="English", callback=callback)

    assert re.search(r"0~3\D+1 failure\b", callback.last)
