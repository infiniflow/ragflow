"""Unit tests for deepdoc/parser/mineru_parser.py."""

from __future__ import annotations

import importlib.util
import json
import logging
import sys
from pathlib import Path
from types import ModuleType
from unittest import mock

import pytest
import requests


def _load_mineru_parser(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    monkeypatch.setitem(sys.modules, "pdfplumber", ModuleType("pdfplumber"))
    pil_mod = ModuleType("PIL")
    image_mod = ModuleType("PIL.Image")
    pil_mod.Image = image_mod
    monkeypatch.setitem(sys.modules, "PIL", pil_mod)
    monkeypatch.setitem(sys.modules, "PIL.Image", image_mod)

    deepdoc_mod = ModuleType("deepdoc")
    deepdoc_mod.__path__ = [str(repo_root / "deepdoc")]
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_mod)

    parser_mod = ModuleType("deepdoc.parser")
    parser_mod.__path__ = [str(repo_root / "deepdoc" / "parser")]
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_mod)

    pdf_parser_mod = ModuleType("deepdoc.parser.pdf_parser")

    class _RAGFlowPdfParser:
        pass

    pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    module_name = "test_mineru_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "mineru_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


@pytest.fixture()
def mineru_module(monkeypatch):
    return _load_mineru_parser(monkeypatch)


def _make_parser(module):
    parser = module.MinerUParser(
        mineru_api="http://mineru:9987",
        access_mode="self_hosted",
        api_base_url="https://mineru.net",
        api_token="default-token",
        model_version="vlm",
        poll_interval=3,
        poll_timeout=300,
    )
    parser.logger = mock.MagicMock()
    return parser


class TestMinerUParseOptions:
    def test_repr_redacts_api_token(self, mineru_module):
        options = mineru_module.MinerUParseOptions(api_token="secret-token")

        assert "secret-token" not in repr(options)
        assert "api_token" not in repr(options)


class TestReadOutput:
    def test_direct_fallback_ignores_unrelated_content_list(self, mineru_module, tmp_path: Path):
        parser = _make_parser(mineru_module)
        (tmp_path / "other_doc_content_list.json").write_text(json.dumps([]), encoding="utf-8")

        with pytest.raises(FileNotFoundError, match="Missing output file"):
            parser._read_output(tmp_path, "current_doc")


class TestPollOfficialBatchResult:
    def test_retries_transient_poll_failure_then_succeeds(self, mineru_module):
        parser = _make_parser(mineru_module)
        options = mineru_module.MinerUParseOptions(api_base_url="https://mineru.net", api_token="token", poll_interval=1)

        ok_resp = mock.Mock()
        ok_resp.raise_for_status.return_value = None
        ok_resp.content = b"{}"
        ok_resp.json.return_value = {
            "code": 0,
            "data": {
                "extract_result": [
                    {"file_name": "demo.pdf", "state": "done", "full_zip_url": "https://files/demo.zip"}
                ]
            },
        }

        with mock.patch.object(mineru_module.requests, "get", side_effect=[requests.Timeout("timeout"), ok_resp]) as mock_get, \
             mock.patch.object(mineru_module.time, "sleep", return_value=None):
            full_zip_url = parser._poll_official_batch_result("batch-1", "demo.pdf", options)

        assert full_zip_url == "https://files/demo.zip"
        assert mock_get.call_count == 2


class TestParsePdf:
    def test_official_v4_settings_can_come_from_parser_config(self, mineru_module):
        parser = _make_parser(mineru_module)
        parser.mineru_access_mode = mineru_module.MinerUAccessMode.SELF_HOSTED
        parser.mineru_api_base_url = "https://default.example"
        parser.mineru_api_token = "default-token"
        parser.mineru_model_version = "pipeline"
        parser.mineru_poll_interval = 9
        parser.mineru_poll_timeout = 99

        captured_options = {}

        def _capture_run(_pdf, _out_dir, options, callback=None):
            captured_options["options"] = options
            return Path("/tmp/mineru-output")

        with mock.patch.object(parser, "__images__", return_value=None), \
             mock.patch.object(parser, "_run_mineru", side_effect=_capture_run), \
             mock.patch.object(parser, "_read_output", return_value=[]):
            sections, tables = parser.parse_pdf(
                filepath="demo.pdf",
                binary=b"%PDF-1.4",
                parse_method="pipeline",
                parser_config={
                    "mineru_access_mode": "official_v4",
                    "mineru_api_base_url": "https://mineru.custom",
                    "mineru_api_token": "parser-config-token",
                    "mineru_model_version": "MinerU-HTML",
                    "mineru_poll_interval": 5,
                    "mineru_poll_timeout": 120,
                },
            )

        options = captured_options["options"]
        assert options.access_mode == mineru_module.MinerUAccessMode.OFFICIAL_V4
        assert options.api_base_url == "https://mineru.custom"
        assert options.api_token == "parser-config-token"
        assert options.model_version == "MinerU-HTML"
        assert options.poll_interval == 5
        assert options.poll_timeout == 120
        assert sections == []
        assert tables == []


def test_sanitize_section_text_removes_escaped_html_tags(mineru_module):
    text = "&lt;table&gt;&lt;tr&gt;&lt;td&gt;Alpha&lt;/td&gt;&lt;td&gt;Beta&lt;/td&gt;&lt;/tr&gt;&lt;/table&gt;"

    sanitized = mineru_module.MinerUParser._sanitize_section_text(text)

    assert sanitized == "AlphaBeta"
    assert "<td>" not in sanitized
    assert "</td>" not in sanitized


def test_transfer_to_sections_logs_sections_dropped_after_sanitization(mineru_module, caplog):
    parser = mineru_module.MinerUParser()
    outputs = [
        {
            "type": mineru_module.MinerUContentType.TEXT,
            "text": "&lt;td&gt;&lt;/td&gt;",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        }
    ]

    with caplog.at_level(logging.DEBUG, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="pipeline")

    assert sections == []
    assert "Skip section after sanitization" in caplog.text
    assert f"type={mineru_module.MinerUContentType.TEXT}" in caplog.text


def test_transfer_to_sections_skips_page_chrome_without_duplicating_text(mineru_module):
    parser = mineru_module.MinerUParser()
    fixture_path = Path(__file__).resolve().parents[3] / "fixtures" / "mineru" / "bmw_page_chrome_content_list.json"
    outputs = json.loads(fixture_path.read_text(encoding="utf-8"))

    sections = parser._transfer_to_sections(outputs, parse_method="raw")
    texts = [section[0] for section in sections]

    assert texts == ["打开和关闭", "车辆装备", "车辆钥匙", "概述", "安全提示"]
    assert texts.count("打开和关闭") == 1
    assert texts.count("概述") == 1
    assert "77" not in texts
    assert "Online Edition for Part no." not in " ".join(texts)


def test_transfer_to_sections_skips_unknown_types_without_duplicating_text(mineru_module, caplog):
    parser = mineru_module.MinerUParser()
    outputs = [
        {
            "type": mineru_module.MinerUContentType.TEXT,
            "text": "Primary content",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
        {
            "type": "sidebar",
            "text": "Should not repeat previous section",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
        {
            "type": mineru_module.MinerUContentType.TEXT,
            "text": "Next content",
            "page_idx": 0,
            "bbox": (0, 0, 1, 1),
        },
    ]

    with caplog.at_level(logging.DEBUG, logger=parser.logger.name):
        sections = parser._transfer_to_sections(outputs, parse_method="raw")

    assert [section[0] for section in sections] == ["Primary content", "Next content"]
    assert "Skip unsupported section type=sidebar" in caplog.text
