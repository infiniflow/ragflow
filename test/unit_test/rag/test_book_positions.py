#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Tests for the book chunk policy's page-position handling (rag/app/book.py).

The DeepDoc position tag is ``@@page\tx0\tx1\ttop\tbottom##`` (a double ``@@``).
``_sections_with_positions`` splits on ``@@`` so ``naive_merge`` re-attaches the
tag and page + bbox survive into the chunk.
"""

import re

import pytest

import rag.nlp as nlp
from rag.nlp import naive_merge
from rag.app.book import _sections_with_positions

DELIMITER = "\n。；！？"
POS = "@@1\t72.0\t523.0\t88.0\t101.0##"


@pytest.fixture(autouse=True)
def word_count_tokens(monkeypatch):
    """Count tokens as whitespace-delimited words, ignoring the ``@@..##`` tag."""

    def fake_num_tokens(s):
        s = re.sub(r"@@[0-9]+\t[^#]*##", "", s or "")
        return len(s.split())

    monkeypatch.setattr(nlp, "num_tokens_from_string", fake_num_tokens)


@pytest.mark.p2
def test_position_tag_is_preserved_through_merge():
    normalized = _sections_with_positions([("Some passage of a book about the common field mice" + POS, "")])
    assert normalized[0][1] == POS
    # naive_merge re-appends the position, so page + bbox reach the chunk.
    chunks = naive_merge(normalized, 512, DELIMITER)
    assert any(POS in c for c in chunks)


@pytest.mark.p2
def test_untagged_section_has_no_position():
    assert _sections_with_positions([("A plain section", "")]) == [("A plain section", "")]


@pytest.mark.p2
def test_each_section_keeps_its_own_tag():
    normalized = _sections_with_positions([("first@@1\t0\t0\t0\t0##", ""), ("second@@2\t0\t0\t0\t0##", "")])
    assert normalized[0][1] == "@@1\t0\t0\t0\t0##"
    assert normalized[1][1] == "@@2\t0\t0\t0\t0##"


@pytest.mark.p2
def test_natural_double_at_without_tag_is_kept_as_content():
    # A "@@" in the text with no ##-terminated tag is content, not a position.
    assert _sections_with_positions([("see a@@b for details", "")]) == [("see a@@b for details", "")]


@pytest.mark.p2
def test_only_the_trailing_tag_is_detached():
    # Content may itself contain "@@"; only the last "@@..##" suffix is the tag.
    normalized = _sections_with_positions([("text a@@b more" + POS, "")])
    assert normalized[0] == ("text a@@b more", POS)
