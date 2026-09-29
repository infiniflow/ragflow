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

from io import BytesIO
from types import SimpleNamespace
from zipfile import BadZipFile

import mammoth
import pytest

from rag.app import naive


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


@pytest.mark.p2
def test_docx_empty_binary_does_not_open_display_name():
    with pytest.raises(BadZipFile, match="File is not a zip file"):
        naive.Docx()("report.docx", b"")


@pytest.mark.p2
def test_docx_to_markdown_empty_binary_does_not_open_display_name(monkeypatch):
    opened = []
    converted = []

    def fail_open(filename, *args, **kwargs):
        opened.append(filename)
        raise AssertionError(f"unexpected open: {filename}")

    def fake_convert_to_html(docx_file):
        converted.append(docx_file)
        assert isinstance(docx_file, BytesIO)
        assert docx_file.getvalue() == b""
        return SimpleNamespace(value="")

    monkeypatch.setattr("builtins.open", fail_open)
    monkeypatch.setattr(mammoth, "convert_to_html", fake_convert_to_html)

    assert naive.Docx().to_markdown("report.docx", binary=b"", inline_images=False) == ""
    assert opened == []
    assert not converted[0].closed


@pytest.mark.p2
def test_deepdoc_empty_binary_passes_bytes_to_pdf_parser(monkeypatch):
    captured = []

    class CapturingPdf:
        def __call__(self, source, **kwargs):
            captured.append(source)
            return [], []

    monkeypatch.setattr(naive, "vision_figure_parser_pdf_wrapper", lambda **kwargs: kwargs["tbls"])

    naive.by_deepdoc("report.pdf", binary=b"", pdf_cls=CapturingPdf)

    assert captured == [b""]
