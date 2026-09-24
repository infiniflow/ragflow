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
"""Markdown Q&A: a heading is a question unless it sits inside a code block."""

from __future__ import annotations

import pytest

from rag.app import qa


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


def _pairs(markdown_text):
    chunks = qa.chunk("faq.md", binary=markdown_text.encode(), lang="English", callback=lambda *a, **k: None)
    pairs = []
    for chunk in chunks:
        question, answer = chunk["content_with_weight"].split("\tAnswer: ", 1)
        pairs.append((question.removeprefix("Question: "), answer))
    return pairs


@pytest.mark.p2
def test_a_hash_line_inside_a_tilde_fence_is_not_a_question():
    pairs = _pairs("# How do I install it?\n\n~~~bash\n# install the dependencies first\npip install ragflow\n~~~\n\n# How do I start it?\n\nRun it.\n")

    assert [question for question, _ in pairs] == ["How do I install it?", "How do I start it?"]
    assert "pip install ragflow" in pairs[0][1]


@pytest.mark.p2
def test_a_shorter_fence_inside_a_longer_one_does_not_close_it():
    """Docs that show a code block in markdown wrap it in a longer fence."""
    pairs = _pairs("# How do I show code?\n\n````markdown\n```\n# not a question\n````\n\n# Next?\n\nYes.\n")

    assert [question for question, _ in pairs] == ["How do I show code?", "Next?"]


@pytest.mark.p2
def test_a_line_that_only_looks_like_a_fence_does_not_hide_the_next_question():
    """A backtick fence cannot carry a backtick after it, so this is inline code."""
    pairs = _pairs("# First?\n\n```inline``` is not a fence\n\n# Second?\n\nAnswer.\n")

    assert [question for question, _ in pairs] == ["First?", "Second?"]


@pytest.mark.p2
def test_a_code_block_in_an_answer_stays_a_code_block():
    pairs = _pairs("# How do I install it?\n\n```bash\n# install the dependencies first\npip install ragflow\n```\n")

    answer = pairs[0][1]
    assert "<pre><code" in answer
    assert "# install the dependencies first" in answer
    assert "<h1>" not in answer
