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

"""Unit tests for rag.app.external_loader."""

from __future__ import annotations

import sys
from unittest.mock import MagicMock, patch

# Stub rag.app.naive before importing external_loader to prevent loading the full
# naive module at collection time (it pulls in xgboost, deepdoc, PIL, etc.).
# Individual tests that exercise the external-loader path patch
# rag.app.external_loader.naive directly.
if "rag.app.naive" not in sys.modules:
    sys.modules["rag.app.naive"] = MagicMock()

import requests
import pytest

# ---------------------------------------------------------------------------
# Import the module under test.  get_base_config reads service_conf.yaml at
# call-time (not import-time), so no special import-time patching is needed.
# ---------------------------------------------------------------------------
from common.constants import MAXIMUM_PAGE_NUMBER
from rag.app.external_loader import _call_loader, _find_route, _resolve, chunk


# ---------------------------------------------------------------------------
# _find_route
# ---------------------------------------------------------------------------

class TestFindRoute:
    ROUTES = [
        {"extensions": [".docx", ".doc"], "url": "http://doc-loader/"},
        {"extensions": [".pdf"], "url": "http://pdf-loader/"},
    ]

    def test_exact_match(self):
        assert _find_route(".pdf", self.ROUTES)["url"] == "http://pdf-loader/"

    def test_case_insensitive(self):
        assert _find_route(".PDF", self.ROUTES)["url"] == "http://pdf-loader/"
        assert _find_route(".DOCX", self.ROUTES)["url"] == "http://doc-loader/"

    def test_no_match_returns_none(self):
        assert _find_route(".xlsx", self.ROUTES) is None

    def test_first_match_wins(self):
        routes = [
            {"extensions": [".pdf"], "url": "http://first/"},
            {"extensions": [".pdf"], "url": "http://second/"},
        ]
        assert _find_route(".pdf", routes)["url"] == "http://first/"


# ---------------------------------------------------------------------------
# _resolve
# ---------------------------------------------------------------------------

YAML_WITH_ROUTES = {
    "routes": [
        {"extensions": [".pdf"], "url": "http://pdf-loader/", "api_key": "tok"},
    ],
    "fallback": {"url": "http://fallback-loader/"},
}

YAML_WITH_BUILTIN_FALLBACK = {
    "routes": [],
    "fallback": {"parser": "naive"},
}

YAML_EMPTY = {}


class TestResolve:
    def test_route_match_returns_loader_cfg(self):
        with patch("rag.app.external_loader.get_base_config", return_value=YAML_WITH_ROUTES):
            result = _resolve(".pdf", {})
        assert isinstance(result, dict)
        assert result["url"] == "http://pdf-loader/"
        assert result["api_key"] == "tok"

    def test_fallback_used_when_no_route_matches(self):
        with patch("rag.app.external_loader.get_base_config", return_value=YAML_WITH_ROUTES):
            result = _resolve(".docx", {})
        assert isinstance(result, dict)
        assert result["url"] == "http://fallback-loader/"

    def test_builtin_parser_fallback_returns_string(self):
        with patch("rag.app.external_loader.get_base_config", return_value=YAML_WITH_BUILTIN_FALLBACK):
            result = _resolve(".whatever", {})
        assert result == "naive"

    def test_parser_config_override_wins(self):
        override = {"external_loader": {"url": "http://override/"}}
        with patch("rag.app.external_loader.get_base_config", return_value=YAML_EMPTY):
            result = _resolve(".pdf", override)
        assert result["url"] == "http://override/"

    def test_raises_when_nothing_configured(self):
        with patch("rag.app.external_loader.get_base_config", return_value=YAML_EMPTY):
            with pytest.raises(ValueError, match="No loader configured"):
                _resolve(".pdf", {})


# ---------------------------------------------------------------------------
# _call_loader
# ---------------------------------------------------------------------------

class TestCallLoader:
    LOADER_CFG = {"url": "http://loader/process", "api_key": "secret", "method": "POST"}

    def _make_response(self, json_data=None, status=200, text=""):
        resp = MagicMock()
        resp.status_code = status
        resp.text = text
        if json_data is not None:
            resp.json.return_value = json_data
        else:
            resp.json.side_effect = ValueError("not json")
        return resp

    def test_returns_page_content(self):
        from pathlib import Path
        resp = self._make_response({"page_content": "# Hello"})
        with patch("requests.request", return_value=resp) as mock_req:
            result = _call_loader(Path("doc.pdf"), b"binary", self.LOADER_CFG)
        assert result == "# Hello"
        args, kwargs = mock_req.call_args
        assert args[0] == "POST"
        assert args[1] == "http://loader/process"
        assert kwargs["headers"]["Authorization"] == "Bearer secret"
        assert kwargs["headers"]["X-Filename"] == "doc.pdf"

    def test_invalid_json_raises(self):
        from pathlib import Path
        resp = self._make_response(json_data=None, text="not json at all")
        with patch("requests.request", return_value=resp):
            with pytest.raises(ValueError, match="invalid JSON"):
                _call_loader(Path("doc.pdf"), b"binary", self.LOADER_CFG)

    def test_missing_page_content_raises(self):
        from pathlib import Path
        resp = self._make_response({"other_key": "value"})
        with patch("requests.request", return_value=resp):
            with pytest.raises(ValueError, match="missing 'page_content'"):
                _call_loader(Path("doc.pdf"), b"binary", self.LOADER_CFG)

    def test_http_error_raises(self):
        from pathlib import Path
        resp = self._make_response(status=500, text="Internal Server Error")
        resp.raise_for_status.side_effect = requests.exceptions.HTTPError("500 Server Error")
        with patch("requests.request", return_value=resp):
            with pytest.raises(requests.exceptions.HTTPError):
                _call_loader(Path("doc.pdf"), b"binary", self.LOADER_CFG)

    def test_timeout_propagates(self):
        from pathlib import Path
        with patch("requests.request", side_effect=requests.exceptions.Timeout):
            with pytest.raises(requests.exceptions.Timeout):
                _call_loader(Path("doc.pdf"), b"binary", self.LOADER_CFG)


# ---------------------------------------------------------------------------
# chunk() — built-in delegation path
# ---------------------------------------------------------------------------

class TestChunkBuiltinDelegation:
    def test_delegates_to_builtin_parser(self):
        fake_module = MagicMock()
        fake_module.chunk.return_value = [{"content": "chunk1"}]

        with patch("rag.app.external_loader.get_base_config", return_value=YAML_WITH_BUILTIN_FALLBACK):
            with patch("importlib.import_module", return_value=fake_module) as mock_import:
                result = chunk("report.pdf", binary=b"data")

        mock_import.assert_called_once_with("rag.app.naive")
        fake_module.chunk.assert_called_once_with(
            "report.pdf",
            binary=b"data",
            from_page=0,
            to_page=MAXIMUM_PAGE_NUMBER,
            lang="Chinese",
            callback=None,
        )
        assert result == [{"content": "chunk1"}]


# ---------------------------------------------------------------------------
# chunk() — external loader path
# ---------------------------------------------------------------------------

class TestChunkExternalLoader:
    def test_calls_loader_and_chunks_markdown(self):
        resp = MagicMock()
        resp.status_code = 200
        resp.json.return_value = {"page_content": "# Title\n\nBody text."}

        fake_naive = MagicMock()
        fake_naive.chunk.return_value = [{"docnm_kwd": "report.pdf.md", "content": "Body text."}]

        yaml_cfg = {"routes": [{"extensions": [".pdf"], "url": "http://loader/"}], "fallback": {}}

        with patch("rag.app.external_loader.get_base_config", return_value=yaml_cfg):
            with patch("requests.request", return_value=resp):
                with patch("rag.app.external_loader.naive", fake_naive):
                    result = chunk("report.pdf", binary=b"pdfbinary")

        fake_naive.chunk.assert_called_once_with(
            filename="report.pdf.md",
            binary=b"# Title\n\nBody text.",
            from_page=0,
            to_page=MAXIMUM_PAGE_NUMBER,
            lang="Chinese",
            callback=None,
        )
        # docnm_kwd must be restored to the original filename
        assert result[0]["docnm_kwd"] == "report.pdf"
