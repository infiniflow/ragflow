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
"""Unit tests for PaddleOCRParser.check_installation().

The check verifies real service reachability on top of the configuration
checks, mirroring the Mistral/MinerU parser checks. The probe follows the
wire protocol of the configured deployment: the hosted service gets a
minimal model-only POST to the jobs endpoint (which also validates the
token via 401/403), while a self-hosted PaddleX deployment only serves the
synchronous layout-parsing endpoint and is probed with a plain GET. No
live service or heavy deepdoc dependency chain is required.
"""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest import mock

import requests

_REPO = Path(__file__).resolve().parents[4]

# The hosted address recognized by _is_hosted_service (matches the module's
# _DEFAULT_BASE_URL); any other hostname counts as a self-hosted deployment.
_HOSTED_BASE = "https://paddleocr.aistudio-app.com"


def _load_paddleocr_parser(monkeypatch) -> ModuleType:
    """Load paddleocr_parser.py directly, stubbing heavy dependencies."""
    # Heavy / optional third-party imports used by the parser at runtime.
    for name in ("numpy", "pdfplumber", "PIL", "PIL.Image"):
        monkeypatch.setitem(sys.modules, name, mock.MagicMock())

    # deepdoc package tree: provide real modules so class inheritance works.
    deepdoc_mod = ModuleType("deepdoc")
    deepdoc_mod.__path__ = [str(_REPO / "deepdoc")]
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_mod)

    parser_mod = ModuleType("deepdoc.parser")
    parser_mod.__path__ = [str(_REPO / "deepdoc" / "parser")]
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_mod)

    pdf_parser_mod = ModuleType("deepdoc.parser.pdf_parser")

    class _RAGFlowPdfParser:
        pass

    pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = mock.MagicMock(return_value=[])
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    constants_mod = ModuleType("common.constants")
    constants_mod.MAXIMUM_PAGE_NUMBER = 100000
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)

    module_name = "test_paddleocr_parser_unit_module"
    spec = importlib.util.spec_from_file_location(module_name, _REPO / "deepdoc" / "parser" / "paddleocr_parser.py")
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


class _Resp:
    def __init__(self, status_code: int, text: str = ""):
        self.status_code = status_code
        self.text = text


def test_missing_access_token_returns_false_without_request(monkeypatch):
    """The hosted service requires a token; the check fails before any HTTP
    call."""
    m = _load_paddleocr_parser(monkeypatch)
    monkeypatch.delenv("PADDLEOCR_ACCESS_TOKEN", raising=False)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE, access_token=None)
    with (
        mock.patch.object(m.requests, "post") as post,
        mock.patch.object(m.requests, "get") as get,
    ):
        ok, reason = p.check_installation()
    assert ok is False
    assert "token" in reason.lower()
    post.assert_not_called()
    get.assert_not_called()


def test_missing_base_url_returns_false_without_request(monkeypatch):
    m = _load_paddleocr_parser(monkeypatch)
    monkeypatch.setenv("PADDLEOCR_BASE_URL", "")
    p = m.PaddleOCRParser(base_url=None, access_token="tok")
    with (
        mock.patch.object(m.requests, "post") as post,
        mock.patch.object(m.requests, "get") as get,
    ):
        ok, reason = p.check_installation()
    assert ok is False
    assert "base url" in reason.lower()
    post.assert_not_called()
    get.assert_not_called()


def test_access_token_rejected_returns_false(monkeypatch):
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE + "/", access_token="bad-token")
    with mock.patch.object(m.requests, "post", return_value=_Resp(401, '{"code": 401, "msg": "Unauthorized"}')) as post:
        ok, reason = p.check_installation()
    assert ok is False
    assert "rejected" in reason.lower()
    post.assert_called_once()
    args, kwargs = post.call_args
    assert args[0] == _HOSTED_BASE + "/api/v2/ocr/jobs"
    assert kwargs["headers"]["Authorization"] == "Bearer bad-token"
    assert kwargs["data"] == {"model": "PaddleOCR-VL"}


def test_access_token_rejected_403_returns_false(monkeypatch):
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE, access_token="bad-token")
    with mock.patch.object(m.requests, "post", return_value=_Resp(403)):
        ok, reason = p.check_installation()
    assert ok is False
    assert "403" in reason


def test_validation_error_means_token_and_service_ok(monkeypatch):
    """A 4xx other than 401/403 proves the token is accepted (e.g. 422 for a
    missing file), so connectivity is confirmed."""
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE, access_token="valid-token")
    with mock.patch.object(m.requests, "post", return_value=_Resp(422, "missing file")):
        ok, reason = p.check_installation()
    assert ok is True
    assert reason == ""


def test_http_error_other_than_auth_confirms_reachability(monkeypatch):
    """405/404/500 responses still prove the service is reachable."""
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE, access_token="tok")
    with mock.patch.object(m.requests, "post", return_value=_Resp(405)):
        ok, _ = p.check_installation()
    assert ok is True


def test_network_error_returns_false(monkeypatch):
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url=_HOSTED_BASE, access_token="tok")
    with mock.patch.object(
        m.requests,
        "post",
        side_effect=requests.ConnectionError("connection refused"),
    ):
        ok, reason = p.check_installation()
    assert ok is False
    assert "unreachable" in reason.lower()


def test_hosted_non_https_base_url_rejected_before_probe(monkeypatch):
    """A hosted hostname over plain HTTP is a misconfiguration: fail with a
    configuration reason instead of probing without authentication."""
    m = _load_paddleocr_parser(monkeypatch)
    p = m.PaddleOCRParser(base_url="http://paddleocr.aistudio-app.com", access_token="tok")
    with (
        mock.patch.object(m.requests, "post") as post,
        mock.patch.object(m.requests, "get") as get,
    ):
        ok, reason = p.check_installation()
    assert ok is False
    assert "https" in reason.lower()
    post.assert_not_called()
    get.assert_not_called()


def test_self_hosted_probe_uses_layout_parsing_get(monkeypatch):
    """A self-hosted PaddleX deployment only serves the synchronous
    layout-parsing endpoint, so the probe is a GET there — no token is
    required and any HTTP response counts as reachable."""
    m = _load_paddleocr_parser(monkeypatch)
    monkeypatch.delenv("PADDLEOCR_ACCESS_TOKEN", raising=False)
    p = m.PaddleOCRParser(base_url="http://127.0.0.1:8080", access_token=None)
    with (
        mock.patch.object(m.requests, "get", return_value=_Resp(404)) as get,
        mock.patch.object(m.requests, "post") as post,
    ):
        ok, reason = p.check_installation()
    assert ok is True
    assert reason == ""
    get.assert_called_once()
    args, kwargs = get.call_args
    assert args[0] == "http://127.0.0.1:8080/layout-parsing"
    assert "Authorization" not in kwargs["headers"]
    post.assert_not_called()


def test_self_hosted_token_not_sent_over_http(monkeypatch):
    """Mirroring the parse path, the token is not sent to a non-HTTPS
    endpoint."""
    m = _load_paddleocr_parser(monkeypatch)
    monkeypatch.delenv("PADDLEOCR_ACCESS_TOKEN", raising=False)
    p = m.PaddleOCRParser(base_url="http://127.0.0.1:8080", access_token="tok")
    with mock.patch.object(m.requests, "get", return_value=_Resp(200)) as get:
        ok, _ = p.check_installation()
    assert ok is True
    _, kwargs = get.call_args
    assert "Authorization" not in kwargs["headers"]


def test_self_hosted_base_url_carrying_endpoint_path_not_duplicated(monkeypatch):
    """Pasting the full pipeline URL as the base URL is the likelier user
    mistake; the probe must reuse the address as-is instead of appending the
    endpoint path a second time."""
    m = _load_paddleocr_parser(monkeypatch)
    monkeypatch.delenv("PADDLEOCR_ACCESS_TOKEN", raising=False)
    p = m.PaddleOCRParser(base_url="http://127.0.0.1:8080/layout-parsing", access_token=None)
    with mock.patch.object(m.requests, "get", return_value=_Resp(200)) as get:
        ok, _ = p.check_installation()
    assert ok is True
    args, _ = get.call_args
    assert args[0] == "http://127.0.0.1:8080/layout-parsing"
