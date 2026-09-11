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
import importlib
import os
import sys
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

def _load_parser_module(monkeypatch):
    for module_name in ("rag.flow.parser.parser", "deepdoc.parser.pdf_parser", "deepdoc.parser", "deepdoc"):
        monkeypatch.delitem(sys.modules, module_name, raising=False)
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")
    return importlib.import_module("rag.flow.parser.parser")


class _FakeProcess:
    def __init__(self):
        self._param = SimpleNamespace(
            setups={
                "pdf": {
                    "parse_method": "docling",
                    "output_format": "markdown",
                    "flatten_media_to_text": False,
                    "remove_toc": False,
                    "remove_header_footer": False,
                }
            }
        )
        self._canvas = SimpleNamespace(_tenant_id="tenant", _language="English")
        self.callback = Mock()
        self.outputs = {}

    def set_output(self, name, value):
        self.outputs[name] = value


def test_pdf_markdown_preserves_figure_text_when_image_is_missing(monkeypatch):
    parser_module = _load_parser_module(monkeypatch)
    pdf_parser = SimpleNamespace(
        outlines=[],
        parse_pdf=Mock(return_value=([("caption survives", "image", "")], [])),
    )
    process = _FakeProcess()
    image2base64 = Mock()

    monkeypatch.setattr(parser_module.TenantModelService, "get_by_id", Mock(return_value=(False, None)))
    monkeypatch.setattr(parser_module, "DoclingParser", Mock(return_value=pdf_parser))
    monkeypatch.setattr(parser_module, "enhance_media_sections_with_vision", Mock())
    monkeypatch.setattr(parser_module.VLM, "image2base64", image2base64)

    parser_module.Parser._pdf(process, "document.pdf", b"pdf")

    assert process.outputs["markdown"] == "caption survives\n"
    image2base64.assert_not_called()


def test_pdf_markdown_consumes_docling_figure_without_image(monkeypatch):
    parser_module = _load_parser_module(monkeypatch)
    pdf_parser = SimpleNamespace(
        outlines=[],
        parse_pdf=Mock(return_value=([], [((None, ["figure OCR text"]), [(0, 1, 2, 3, 4)])])),
    )
    process = _FakeProcess()
    image2base64 = Mock()

    monkeypatch.setattr(parser_module.TenantModelService, "get_by_id", Mock(return_value=(False, None)))
    monkeypatch.setattr(parser_module, "DoclingParser", Mock(return_value=pdf_parser))
    monkeypatch.setattr(parser_module, "enhance_media_sections_with_vision", Mock())
    monkeypatch.setattr(parser_module.VLM, "image2base64", image2base64)

    parser_module.Parser._pdf(process, "document.pdf", b"pdf")

    assert process.outputs["markdown"] == "figure OCR text\n"
    image2base64.assert_not_called()


@pytest.mark.p2
@pytest.mark.skipif(
    not (os.environ.get("DOCLING_SERVER_URL") and os.environ.get("DOCLING_INTEGRATION_PDF")),
    reason="requires DOCLING_SERVER_URL and a media-bearing DOCLING_INTEGRATION_PDF fixture",
)
def test_docling_server_media_reaches_json_and_markdown_outputs(monkeypatch):
    """Manual integration coverage for the complete Docling flow branch.

    The configured fixture must contain at least one table or figure. This test
    intentionally uses the live Docling server so the parser's second result is
    exercised rather than replaced with a mock.
    """
    parser_module = _load_parser_module(monkeypatch)
    fixture = os.environ["DOCLING_INTEGRATION_PDF"]
    with open(fixture, "rb") as file:
        blob = file.read()
    process = _FakeProcess()
    process._canvas = SimpleNamespace(_tenant_id=None, _language="English")
    docling_parser = parser_module.DoclingParser(docling_server_url=os.environ["DOCLING_SERVER_URL"])

    monkeypatch.setattr(parser_module.TenantModelService, "get_by_id", Mock(return_value=(False, None)))
    monkeypatch.setattr(parser_module, "DoclingParser", Mock(return_value=docling_parser))
    monkeypatch.setattr(parser_module.VLM, "image2base64", Mock(return_value="data:image/png;base64,test"))

    process._param.setups["pdf"]["output_format"] = "json"
    parser_module.Parser._pdf(process, fixture, blob)
    media = [item for item in process.outputs["json"] if item["doc_type_kwd"] in {"table", "image"}]
    assert media

    process.outputs.clear()
    process._param.setups["pdf"]["output_format"] = "markdown"
    parser_module.Parser._pdf(process, fixture, blob)
    assert process.outputs["markdown"]
