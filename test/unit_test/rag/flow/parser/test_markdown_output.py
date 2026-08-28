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
import sys
from types import SimpleNamespace
from unittest.mock import Mock


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
