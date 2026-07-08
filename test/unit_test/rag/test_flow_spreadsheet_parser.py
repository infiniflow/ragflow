import importlib.util
import logging
import os
import sys
import types
from unittest import mock

import pytest


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


def _ensure_module(name, **attrs):
    module = sys.modules.get(name)
    if module is None:
        module = types.ModuleType(name)
        sys.modules[name] = module
    for key, value in attrs.items():
        setattr(module, key, value)
    if "." in name:
        parent_name, child_name = name.rsplit(".", 1)
        parent = _ensure_module(parent_name)
        setattr(parent, child_name, module)
    return module


class _DummyBase:
    pass


for module_name, attrs in {
    "litellm": {"logging": logging},
    "numpy": {},
    "PIL": {"Image": mock.MagicMock()},
    "api.db.services.file2document_service": {"File2DocumentService": mock.MagicMock()},
    "api.db.services.file_service": {"FileService": mock.MagicMock()},
    "api.db.services.llm_service": {"LLMBundle": mock.MagicMock()},
    "api.db.joint_services.tenant_model_service": {
        "ensure_mineru_from_env": mock.MagicMock(),
        "ensure_opendataloader_from_env": mock.MagicMock(),
        "ensure_paddleocr_from_env": mock.MagicMock(),
        "get_first_provider_model_name": mock.MagicMock(),
        "get_model_config_from_provider_instance": mock.MagicMock(),
        "get_tenant_default_model_by_type": mock.MagicMock(),
    },
    "common": {"settings": mock.MagicMock()},
    "common.constants": {"LLMType": mock.MagicMock()},
    "common.misc_utils": {"get_uuid": mock.MagicMock(), "thread_pool_exec": mock.MagicMock()},
    "deepdoc.parser": {
        "ExcelParser": mock.MagicMock(),
        "HtmlParser": mock.MagicMock(),
        "TxtParser": mock.MagicMock(),
    },
    "deepdoc.parser.docling_parser": {"DoclingParser": mock.MagicMock()},
    "deepdoc.parser.pdf_parser": {
        "PlainParser": mock.MagicMock(),
        "RAGFlowPdfParser": mock.MagicMock(),
        "VisionParser": mock.MagicMock(),
    },
    "deepdoc.parser.tcadp_parser": {"TCADPParser": mock.MagicMock()},
    "rag.app.naive": {"Docx": mock.MagicMock()},
    "rag.flow.base": {"ProcessBase": _DummyBase, "ProcessParamBase": _DummyBase},
    "rag.flow.parser.pdf_chunk_metadata": {
        "extract_pdf_positions": mock.MagicMock(),
        "normalize_pdf_items_metadata": mock.MagicMock(),
        "reorder_multi_column_bboxes": mock.MagicMock(),
    },
    "rag.flow.parser.schema": {"ParserFromUpstream": mock.MagicMock()},
    "rag.flow.parser.utils": {
        "enhance_media_sections_with_vision": mock.MagicMock(),
        "extract_word_outlines": mock.MagicMock(),
        "extract_docx_header_footer_texts": mock.MagicMock(),
        "remove_header_footer_docx_sections": mock.MagicMock(),
        "remove_header_footer_html_blob": mock.MagicMock(),
        "remove_toc": mock.MagicMock(),
        "remove_toc_pdf": mock.MagicMock(),
        "remove_toc_word": mock.MagicMock(),
    },
    "rag.llm.cv_model": {"Base": mock.MagicMock()},
    "rag.utils.base64_image": {"image2id": mock.MagicMock()},
}.items():
    _ensure_module(module_name, **attrs)


_PROJECT_ROOT = _find_project_root()
_SPEC = importlib.util.spec_from_file_location(
    "rag.flow.parser.parser",
    os.path.join(_PROJECT_ROOT, "rag", "flow", "parser", "parser.py"),
)
_MODULE = importlib.util.module_from_spec(_SPEC)
sys.modules["rag.flow.parser.parser"] = _MODULE
_SPEC.loader.exec_module(_MODULE)


class _FakeParam:
    setups = {"spreadsheet": {"output_format": "html", "parse_method": "deepdoc"}}


class _FakeParserProcess:
    def __init__(self):
        self._param = _FakeParam()
        self.outputs = {}
        self.callback = lambda *args, **kwargs: None

    def set_output(self, key, value):
        self.outputs[key] = value


@pytest.mark.p2
def test_spreadsheet_html_keeps_all_sheet_chunks(monkeypatch):
    html_chunks = [
        "<table><caption>Sheet1</caption><tr><td>a</td></tr></table>",
        "<table><caption>Sheet2</caption><tr><td>b</td></tr></table>",
    ]
    fake_excel_parser = mock.MagicMock()
    fake_excel_parser.html.return_value = html_chunks
    monkeypatch.setattr(_MODULE, "ExcelParser", mock.MagicMock(return_value=fake_excel_parser))

    process = _FakeParserProcess()
    _MODULE.Parser._spreadsheet(process, "book.xlsx", b"dummy")

    assert process.outputs["output_format"] == "html"
    assert process.outputs["html"] == "\n".join(html_chunks)
    assert "Sheet1" in process.outputs["html"]
    assert "Sheet2" in process.outputs["html"]
