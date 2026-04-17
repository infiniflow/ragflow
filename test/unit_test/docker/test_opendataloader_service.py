"""
Unit tests for docker/opendataloader-service/app.py

Tests cover the FastAPI endpoint logic by calling endpoint functions
directly with mocked UploadFile objects — no live service or Java runtime
required.
"""

from __future__ import annotations

import importlib.util
import io
import json
import sys
from pathlib import Path
from unittest import mock

import pytest

# ---------------------------------------------------------------------------
# Stub out opendataloader_pdf and fastapi before loading app.py
# ---------------------------------------------------------------------------
_FAKE_ODL = mock.MagicMock()
sys.modules.setdefault("opendataloader_pdf", _FAKE_ODL)

# We need real fastapi for the endpoint signatures; skip if unavailable.
try:
    import fastapi  # noqa: F401
    _FASTAPI_AVAILABLE = True
except ImportError:
    _FASTAPI_AVAILABLE = False

pytestmark = pytest.mark.skipif(
    not _FASTAPI_AVAILABLE,
    reason="fastapi not installed in test environment",
)

# Load app module
_REPO = Path(__file__).parents[3]
_APP_PATH = _REPO / "docker" / "opendataloader-service" / "app.py"
_spec = importlib.util.spec_from_file_location("odl_service_app", _APP_PATH)
_app_mod = importlib.util.module_from_spec(_spec)
sys.modules["odl_service_app"] = _app_mod
_spec.loader.exec_module(_app_mod)

health = _app_mod.health
file_parse = _app_mod.file_parse


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_upload_file(content: bytes = b"%PDF-1.4 fake", filename: str = "test.pdf"):
    uf = mock.MagicMock()
    uf.filename = filename
    uf.file = io.BytesIO(content)
    return uf


# ---------------------------------------------------------------------------
# /health
# ---------------------------------------------------------------------------

class TestHealthEndpoint:
    def test_returns_ok_when_package_available(self):
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL):
            result = health()
        assert result == {"status": "ok"}

    def test_returns_503_when_package_missing(self):
        from fastapi.responses import JSONResponse
        with mock.patch.object(_app_mod, "opendataloader_pdf", None):
            result = health()
        assert isinstance(result, JSONResponse)
        assert result.status_code == 503


# ---------------------------------------------------------------------------
# /file_parse
# ---------------------------------------------------------------------------

class TestFileParse:
    def test_returns_503_when_package_missing(self):
        from fastapi.responses import JSONResponse
        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", None):
            result = file_parse(file=uf)
        assert isinstance(result, JSONResponse)
        assert result.status_code == 503

    def test_successful_parse_returns_json_doc_and_md(self, tmp_path):
        fake_json = {"type": "paragraph", "content": "Hello"}
        fake_md = "# Hello\n\nWorld"

        def fake_convert(**kwargs):
            out = Path(kwargs["output_dir"])
            (out / "result.json").write_text(json.dumps(fake_json))
            (out / "result.md").write_text(fake_md)

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            result = file_parse(file=uf, hybrid=None, image_output=None, sanitize=None)

        assert result["json_doc"] == fake_json
        assert result["md_text"] == fake_md

    def test_convert_failure_returns_500(self):
        from fastapi.responses import JSONResponse

        def boom(**kwargs):
            raise RuntimeError("Java crashed")

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=boom):
            result = file_parse(file=uf, hybrid=None, image_output=None, sanitize=None)

        assert isinstance(result, JSONResponse)
        assert result.status_code == 500

    def test_no_output_files_returns_nulls(self):
        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert"):  # no files written
            result = file_parse(file=uf, hybrid=None, image_output=None, sanitize=None)

        assert result["json_doc"] is None
        assert result["md_text"] is None

    def test_sanitize_true_strings_parsed_correctly(self, tmp_path):
        def fake_convert(**kwargs):
            assert kwargs.get("sanitize") is True
            Path(kwargs["output_dir"]).mkdir(exist_ok=True)

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, sanitize="true")
            file_parse(file=uf, sanitize="1")
            file_parse(file=uf, sanitize="yes")

    def test_sanitize_false_strings_parsed_correctly(self):
        def fake_convert(**kwargs):
            assert kwargs.get("sanitize") is False

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, sanitize="false")
            file_parse(file=uf, sanitize="0")

    def test_sanitize_none_not_passed_to_convert(self):
        def fake_convert(**kwargs):
            assert "sanitize" not in kwargs

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, sanitize=None)

    def test_hybrid_forwarded_to_convert(self):
        def fake_convert(**kwargs):
            assert kwargs.get("hybrid") == "docling-fast"

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, hybrid="docling-fast", image_output=None, sanitize=None)

    def test_image_output_forwarded_to_convert(self):
        def fake_convert(**kwargs):
            assert kwargs.get("image_output") == "embedded"

        uf = _make_upload_file()
        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, hybrid=None, image_output="embedded", sanitize=None)

    def test_default_filename_when_none(self):
        uf = _make_upload_file()
        uf.filename = None

        captured = {}

        def fake_convert(**kwargs):
            captured["input"] = kwargs["input_path"][0]

        with mock.patch.object(_app_mod, "opendataloader_pdf", _FAKE_ODL), \
             mock.patch.object(_FAKE_ODL, "convert", side_effect=fake_convert):
            file_parse(file=uf, hybrid=None, image_output=None, sanitize=None)

        assert captured["input"].endswith("input.pdf")
