"""Unit tests for deepdoc/parser/mineru_parser.py."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path
from unittest import mock

import pytest
import requests

import types as _types


for _m in ("pdfplumber", "PIL", "PIL.Image"):
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()

_pdf_parser_mod = _types.ModuleType("deepdoc.parser.pdf_parser")


class _RAGFlowPdfParserStub:
    pass


_pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParserStub
sys.modules.setdefault("deepdoc.parser.pdf_parser", _pdf_parser_mod)
sys.modules.setdefault("deepdoc", mock.MagicMock())
sys.modules.setdefault("deepdoc.parser", mock.MagicMock())

_utils_mod = _types.ModuleType("deepdoc.parser.utils")
_utils_mod.extract_pdf_outlines = mock.MagicMock(return_value=[])
sys.modules.setdefault("deepdoc.parser.utils", _utils_mod)

_REPO = Path(__file__).parents[4]
_spec = importlib.util.spec_from_file_location(
    "mineru_parser",
    _REPO / "deepdoc" / "parser" / "mineru_parser.py",
)
_mod = importlib.util.module_from_spec(_spec)
sys.modules["mineru_parser"] = _mod
_spec.loader.exec_module(_mod)

MinerUAccessMode = _mod.MinerUAccessMode
MinerUParseOptions = _mod.MinerUParseOptions
MinerUParser = _mod.MinerUParser


def _make_parser() -> MinerUParser:
    parser = MinerUParser(
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
    def test_repr_redacts_api_token(self):
        options = MinerUParseOptions(api_token="secret-token")

        assert "secret-token" not in repr(options)
        assert "api_token" not in repr(options)


class TestReadOutput:
    def test_direct_fallback_ignores_unrelated_content_list(self, tmp_path: Path):
        parser = _make_parser()
        (tmp_path / "other_doc_content_list.json").write_text(json.dumps([]), encoding="utf-8")

        with pytest.raises(FileNotFoundError, match="Missing output file"):
            parser._read_output(tmp_path, "current_doc")


class TestPollOfficialBatchResult:
    def test_retries_transient_poll_failure_then_succeeds(self):
        parser = _make_parser()
        options = MinerUParseOptions(api_base_url="https://mineru.net", api_token="token", poll_interval=1)

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

        with mock.patch.object(_mod.requests, "get", side_effect=[requests.Timeout("timeout"), ok_resp]) as mock_get, \
             mock.patch.object(_mod.time, "sleep", return_value=None):
            full_zip_url = parser._poll_official_batch_result("batch-1", "demo.pdf", options)

        assert full_zip_url == "https://files/demo.zip"
        assert mock_get.call_count == 2


class TestParsePdf:
    def test_official_v4_settings_can_come_from_parser_config(self):
        parser = _make_parser()
        parser.mineru_access_mode = MinerUAccessMode.SELF_HOSTED
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
        assert options.access_mode == MinerUAccessMode.OFFICIAL_V4
        assert options.api_base_url == "https://mineru.custom"
        assert options.api_token == "parser-config-token"
        assert options.model_version == "MinerU-HTML"
        assert options.poll_interval == 5
        assert options.poll_timeout == 120
        assert sections == []
        assert tables == []