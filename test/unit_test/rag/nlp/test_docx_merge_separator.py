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


def test_merged_paragraphs_keep_a_line_break_with_an_empty_delimiter():
    chunks, _ = naive_merge_docx(SECTIONS, chunk_token_num=128, delimiter="")
    assert [ck["text"] for ck in chunks] == ["First paragraph.\nSecond paragraph.\nThird paragraph."]


@pytest.mark.parametrize("delimiter", ["\n", DEFAULT_DELIMITER, ""])
@pytest.mark.parametrize("between", [IMAGE, TABLE], ids=["image", "table"])
def test_paragraphs_merged_across_an_image_or_table_keep_a_line_break(between, delimiter):
    sections = [SECTIONS[0], between, SECTIONS[1]]
    chunks, _ = naive_merge_docx(sections, chunk_token_num=128, delimiter=delimiter)
    assert [ck["text"] for ck in chunks if ck["ck_type"] == "text"] == ["First paragraph.\nSecond paragraph."]


def test_custom_delimiter_still_keeps_one_chunk_per_paragraph():
    chunks, _ = naive_merge_docx(SECTIONS, chunk_token_num=128, delimiter="`\n`")
    assert [ck["text"] for ck in chunks] == ["First paragraph.", "Second paragraph.", "Third paragraph."]
