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

from io import BytesIO
from unittest.mock import MagicMock

import pytest

from common.pdf_auto_layout import (
    is_auto_layout_recognize,
    pdf_has_usable_text_layer,
    resolve_auto_layout_recognize,
    resolve_pdf_layout_recognize_raw,
    resolve_pipeline_auto_parse_method,
    _layout_to_pipeline_parse_method,
)


def test_is_auto_layout_recognize():
    assert is_auto_layout_recognize("Auto")
    assert is_auto_layout_recognize(" auto ")
    assert not is_auto_layout_recognize("DeepDOC")


def test_resolve_pdf_layout_recognize_raw_passthrough():
    assert (
        resolve_pdf_layout_recognize_raw(
            "DeepDOC",
            {},
            filename="x.pdf",
            binary=b"%PDF",
        )
        == "DeepDOC"
    )


def test_resolve_auto_layout_recognize_uses_text_parser(monkeypatch):
    monkeypatch.setattr(
        "common.pdf_auto_layout.pdf_has_usable_text_layer",
        lambda *args, **kwargs: True,
    )
    chosen = resolve_auto_layout_recognize(
        {
            "layout_recognize_auto_text": "DeepDOC",
            "layout_recognize_auto_scanned": "Plain Text",
        },
        filename="doc.pdf",
        binary=b"%PDF",
    )
    assert chosen == "DeepDOC"


def test_resolve_auto_layout_recognize_uses_scanned_parser(monkeypatch):
    monkeypatch.setattr(
        "common.pdf_auto_layout.pdf_has_usable_text_layer",
        lambda *args, **kwargs: False,
    )
    chosen = resolve_auto_layout_recognize(
        {
            "layout_recognize_auto_text": "DeepDOC",
            "layout_recognize_auto_scanned": "Plain Text",
        },
        filename="scan.pdf",
        binary=b"%PDF",
    )
    assert chosen == "Plain Text"


def test_resolve_pdf_layout_recognize_raw_auto(monkeypatch):
    monkeypatch.setattr(
        "common.pdf_auto_layout.resolve_auto_layout_recognize",
        lambda *args, **kwargs: "MinerU",
    )
    assert (
        resolve_pdf_layout_recognize_raw(
            "Auto",
            {},
            filename="x.pdf",
            binary=b"%PDF",
        )
        == "MinerU"
    )


def test_layout_to_pipeline_parse_method_aliases():
    assert _layout_to_pipeline_parse_method("DeepDOC") == "deepdoc"
    assert _layout_to_pipeline_parse_method("Plain Text") == "plain_text"


def test_pdf_has_usable_text_layer_with_pdfplumber(monkeypatch):
    class _Page:
        def __init__(self, text):
            self._text = text

        def extract_text(self):
            return self._text

    class _Pdf:
        def __init__(self, pages):
            self.pages = pages

        def __enter__(self):
            return self

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(
        "pdfplumber.open",
        lambda src: _Pdf([_Page("hello world " * 20), _Page("more text " * 20)]),
    )
    assert pdf_has_usable_text_layer("t.pdf", binary=b"%PDF", min_chars_per_page=50)


def test_resolve_pipeline_auto_parse_method(monkeypatch):
    monkeypatch.setattr(
        "common.pdf_auto_layout.resolve_auto_layout_recognize",
        lambda *args, **kwargs: "DeepDOC",
    )
    assert resolve_pipeline_auto_parse_method({}, b"%PDF") == "deepdoc"
