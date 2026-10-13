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

from rag.app import tag


def _noop_callback(*_args, **_kwargs):
    pass


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


def _chunk(binary):
    chunks = tag.chunk("tags.csv", binary=binary, lang="English", callback=_noop_callback)
    return [(c["content_with_weight"].strip(), c["tag_kwd"]) for c in chunks]


@pytest.mark.p2
@pytest.mark.parametrize(
    "binary",
    [
        b'first line,alpha\n"press\tTab",keys\nlast line,omega',
        b"first line,alpha\npress\tTab,keys\nlast line,omega",
        b'first line,alpha,\n"press\tTab",keys,\nlast line,omega,',
    ],
)
def test_csv_tab_inside_comma_separated_content(binary):
    assert _chunk(binary) == [
        ("first line", ["alpha"]),
        ("press\tTab", ["keys"]),
        ("last line", ["omega"]),
    ]


@pytest.mark.p2
@pytest.mark.parametrize(
    ("binary", "expected"),
    [
        (b"the quick, brown fox\tanimal,fox", [("the quick, brown fox", ["animal", "fox"])]),
        (b"hello world\tgreeting,demo", [("hello world", ["greeting", "demo"])]),
        (b"hello world\tgreeting,demo\t", [("hello world", ["greeting", "demo"])]),
        (
            b"Animal tags\ncat on a mat\tanimal,cat\ndog in a park\tanimal,dog",
            [("Animal tags\ncat on a mat", ["animal", "cat"]), ("dog in a park", ["animal", "dog"])],
        ),
        (b"Animal tags, part one\ncat on a mat\tanimal,cat", [("Animal tags, part one\ncat on a mat", ["animal", "cat"])]),
    ],
)
def test_csv_tab_separated(binary, expected):
    assert _chunk(binary) == expected
