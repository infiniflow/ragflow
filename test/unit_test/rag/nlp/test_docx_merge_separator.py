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

"""Merged DOCX paragraphs keep a line break between them, as naive_merge does."""

import pytest

from rag.nlp import DEFAULT_DELIMITER, naive_merge_docx

SECTIONS = [
    ("First paragraph.", None, None),
    ("Second paragraph.", None, None),
    ("Third paragraph.", None, None),
]
IMAGE = ("", "IMG", None)
TABLE = ("", None, "<table><tr><td>x</td></tr></table>")
DELIMITERS = ["\n", DEFAULT_DELIMITER, ""]


def _texts(chunks):
    return [ck["text"] for ck in chunks if ck["ck_type"] == "text"]


@pytest.mark.parametrize("delimiter", DELIMITERS)
def test_merged_paragraphs_keep_a_line_break(delimiter):
    chunks, _ = naive_merge_docx(SECTIONS, chunk_token_num=128, delimiter=delimiter)
    assert _texts(chunks) == ["First paragraph.\nSecond paragraph.\nThird paragraph."]


@pytest.mark.parametrize("delimiter", DELIMITERS)
@pytest.mark.parametrize("between", [IMAGE, TABLE], ids=["image", "table"])
def test_paragraphs_merged_across_an_image_or_table_keep_a_line_break(between, delimiter):
    sections = [SECTIONS[0], between, SECTIONS[1]]
    chunks, _ = naive_merge_docx(sections, chunk_token_num=128, delimiter=delimiter)
    assert _texts(chunks) == ["First paragraph.\nSecond paragraph."]


def test_paragraph_that_ends_on_a_delimiter_keeps_a_line_break():
    sections = [("Did revenue grow?", None, None), ("It grew in March.", None, None)]
    chunks, _ = naive_merge_docx(sections, chunk_token_num=128, delimiter=DEFAULT_DELIMITER)
    assert _texts(chunks) == ["Did revenue grow?\nIt grew in March."]


def test_line_break_inside_a_paragraph_is_kept():
    chunks, _ = naive_merge_docx([("Line one\nLine two", None, None)], chunk_token_num=128, delimiter="\n")
    assert _texts(chunks) == ["Line one\nLine two"]


def test_sentences_of_one_paragraph_get_no_line_break():
    chunks, _ = naive_merge_docx([("Did revenue grow? It grew.", None, None)], chunk_token_num=128, delimiter=DEFAULT_DELIMITER)
    assert [text.count("\n") for text in _texts(chunks)] == [0]


def test_merged_chunks_carry_no_line_break_key():
    chunks, _ = naive_merge_docx(SECTIONS, chunk_token_num=128, delimiter="\n")
    assert all("line_break" not in ck for ck in chunks)


def test_custom_delimiter_still_keeps_one_chunk_per_paragraph():
    chunks, _ = naive_merge_docx(SECTIONS, chunk_token_num=128, delimiter="`\n`")
    assert _texts(chunks) == ["First paragraph.", "Second paragraph.", "Third paragraph."]
