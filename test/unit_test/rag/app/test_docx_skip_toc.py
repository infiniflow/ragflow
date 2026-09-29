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

"""Pin the DOCX table-of-contents skip added for #19491.

``Docx.__call__`` drops a paragraph that ``_is_toc_paragraph`` recognises as a
TOC entry. Indexing those entries is pure navigation noise: an entry is a short
string that literally matches its section title, so it wastes ``top_k`` slots
and can outrank the real content block in vector search.

Two signals decide it:

* Word's built-in TOC styles (``toc 1`` / ``TOC 1`` / ``TOC Heading`` / ...);
* a text-shape fallback for documents whose TOC styles were renamed or dropped
  (e.g. converted from PDF). The shape alone cannot tell an entry
  (``1.1 Introduction<TAB>1``) from a tab-separated data row
  (``1. Price<TAB>100``), so the fallback additionally requires the ``PAGEREF``
  field Word writes into every real TOC entry.

The tests below pin both signals, the false-positive guard (a data row that only
matches the shape must be kept), and the end-to-end behaviour of the parser.
"""

from __future__ import annotations

from io import BytesIO
from types import SimpleNamespace

import pytest
from docx import Document
from docx.enum.style import WD_STYLE_TYPE

from rag.app import naive


class _Paragraph:
    """Stand-in exposing the only attribute ``_is_toc_paragraph`` reads."""

    def __init__(self, xml: str):
        self._element = SimpleNamespace(xml=xml)


# The PAGEREF field Word writes into every TOC entry, and a plain body run.
TOC_XML = "<w:p><w:r><w:instrText> PAGEREF _Toc12345 \\h </w:instrText></w:r></w:p>"
BODY_XML = "<w:p><w:r><w:t>row</w:t></w:r></w:p>"


def _ensure_style(document, style_name):
    """Return ``style_name``, adding it to ``document`` when absent."""
    for style in document.styles:
        if style.name == style_name:
            return style
    return document.styles.add_style(style_name, WD_STYLE_TYPE.PARAGRAPH)


def _docx_bytes(paragraphs):
    """Build an in-memory ``.docx`` from ``(text, style_name)`` pairs."""
    document = Document()
    for text, style_name in paragraphs:
        paragraph = document.add_paragraph(text)
        paragraph.style = _ensure_style(document, style_name)
    buffer = BytesIO()
    document.save(buffer)
    return buffer.getvalue()


@pytest.mark.p2
@pytest.mark.parametrize("style_name", ["toc 1", "TOC 1", "Toc 2", "TOC Heading"])
def test_word_toc_style_is_skipped(style_name):
    assert naive._is_toc_paragraph("1.1 Introduction\t1", style_name) is True


@pytest.mark.p2
def test_style_that_only_starts_with_toc_is_kept():
    # "tocustom" is not a Word TOC style; the pattern must not swallow it.
    assert naive._is_toc_paragraph("Body text", "tocustom") is False


@pytest.mark.p2
def test_plain_body_paragraph_is_kept():
    assert naive._is_toc_paragraph("The quick brown fox jumps.", "Normal") is False


@pytest.mark.p2
def test_empty_text_is_never_a_toc_entry():
    assert naive._is_toc_paragraph("", "toc 1") is False


@pytest.mark.p2
def test_styleless_toc_entry_with_pageref_is_skipped():
    # The TOC style was dropped, but the PAGEREF field Word writes survives.
    assert naive._is_toc_paragraph("1.1 Introduction\t1", "Normal", _Paragraph(TOC_XML)) is True


@pytest.mark.p2
@pytest.mark.parametrize("row", ["1. Price\t100", "2024\t2025", "3.5 mg dose\t250"])
def test_tab_separated_data_row_without_pageref_is_kept(row):
    # Regression guard: these rows match the TOC text shape but are body
    # content. Requiring PAGEREF is what keeps them.
    assert naive._is_toc_paragraph(row, "Normal", _Paragraph(BODY_XML)) is False


@pytest.mark.p2
def test_toc_shaped_text_without_paragraph_object_is_kept():
    # Without a paragraph to inspect, the fallback cannot confirm PAGEREF.
    assert naive._is_toc_paragraph("1.1 Introduction\t1", "Normal", None) is False


@pytest.mark.p2
def test_docx_parser_skips_toc_entry_and_keeps_body():
    binary = _docx_bytes(
        [
            ("1.1 Introduction\t1", "TOC 1"),
            ("Body paragraph.", "Normal"),
        ]
    )
    lines = naive.Docx()("report.docx", binary)
    texts = [text for text, _image, _table in lines if text]
    assert "Body paragraph." in texts
    assert all("Introduction" not in text for text in texts)
