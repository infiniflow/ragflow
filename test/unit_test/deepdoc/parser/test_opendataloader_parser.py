"""
Unit tests for deepdoc/parser/opendataloader_parser.py

Tests cover the HTTP-client refactoring: check_installation(), parse_pdf(),
and the crop() bounds guard — without requiring a live OpenDataLoader service,
opendataloader_pdf package, or Java runtime.
"""

from __future__ import annotations

import importlib.util
import io
import sys
from pathlib import Path
from unittest import mock

import pytest
import requests

# ---------------------------------------------------------------------------
# Bootstrap: stub out heavy imports the module pulls in so tests run anywhere
# ---------------------------------------------------------------------------
import types as _types

# PIL — used only at runtime for image ops, mock the whole package
for _m in ("pdfplumber", "PIL", "PIL.Image"):
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()

# deepdoc.parser.pdf_parser — provide a real base class so OpenDataLoaderParser
# inherits a proper Python class, not a MagicMock (which breaks __init__).
_pdf_parser_mod = _types.ModuleType("deepdoc.parser.pdf_parser")
class _RAGFlowPdfParserStub:  # noqa: E302
    pass
_pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParserStub
sys.modules.setdefault("deepdoc.parser.pdf_parser", _pdf_parser_mod)
sys.modules.setdefault("deepdoc", mock.MagicMock())
sys.modules.setdefault("deepdoc.parser", mock.MagicMock())

# deepdoc.parser.utils — extract_pdf_outlines must be a real callable
_utils_mod = _types.ModuleType("deepdoc.parser.utils")
_utils_mod.extract_pdf_outlines = mock.MagicMock(return_value=[])
sys.modules.setdefault("deepdoc.parser.utils", _utils_mod)

# Load the module under test
_REPO = Path(__file__).parents[4]
_spec = importlib.util.spec_from_file_location(
    "opendataloader_parser",
    _REPO / "deepdoc" / "parser" / "opendataloader_parser.py",
)
_mod = importlib.util.module_from_spec(_spec)
# Register before exec so @dataclass can resolve __module__
sys.modules["opendataloader_parser"] = _mod
_spec.loader.exec_module(_mod)

OpenDataLoaderParser = _mod.OpenDataLoaderParser


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_parser(api_url: str = "http://odl:9383") -> OpenDataLoaderParser:
    p = OpenDataLoaderParser()
    p.api_url = api_url
    return p


def _fake_page_image(width: int = 600, height: int = 800):
    img = mock.MagicMock()
    img.size = (width, height)
    img.crop = mock.MagicMock(return_value=img)
    img.convert = mock.MagicMock(return_value=img)
    return img


# ---------------------------------------------------------------------------
# check_installation()
# ---------------------------------------------------------------------------

class TestCheckInstallation:
    def test_no_api_url_returns_false(self):
        p = OpenDataLoaderParser()
        p.api_url = ""
        assert p.check_installation() is False

    def test_health_200_returns_true(self):
        p = _make_parser()
        resp = mock.MagicMock(status_code=200)
        with mock.patch("requests.get", return_value=resp):
            assert p.check_installation() is True

    def test_health_503_returns_false(self):
        p = _make_parser()
        resp = mock.MagicMock(status_code=503, text="unavailable")
        with mock.patch("requests.get", return_value=resp):
            assert p.check_installation() is False

    def test_connection_error_returns_false(self):
        p = _make_parser()
        with mock.patch("requests.get", side_effect=requests.ConnectionError("refused")):
            assert p.check_installation() is False


# ---------------------------------------------------------------------------
# parse_pdf()
# ---------------------------------------------------------------------------

class TestParsePdf:
    def _mock_response(self, json_doc=None, md_text=None) -> mock.MagicMock:
        resp = mock.MagicMock()
        resp.raise_for_status = mock.MagicMock()
        resp.json.return_value = {"json_doc": json_doc, "md_text": md_text}
        return resp

    def test_raises_when_api_url_not_set(self, tmp_path):
        p = OpenDataLoaderParser()
        p.api_url = ""
        pdf = tmp_path / "doc.pdf"
        pdf.write_bytes(b"%PDF-dummy")
        with pytest.raises(RuntimeError, match="OPENDATALOADER_APISERVER"):
            p.parse_pdf(filepath=str(pdf))

    def test_posts_to_file_parse_endpoint(self, tmp_path):
        p = _make_parser()
        pdf = tmp_path / "doc.pdf"
        pdf.write_bytes(b"%PDF-dummy")
        resp = self._mock_response(md_text="hello world")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath=str(pdf))

        mock_post.assert_called_once()
        call_kwargs = mock_post.call_args
        assert "/file_parse" in call_kwargs.kwargs.get("url", call_kwargs.args[0] if call_kwargs.args else "")

    def test_binary_bytes_sent_as_multipart(self, tmp_path):
        p = _make_parser()
        pdf_bytes = b"%PDF-binary"
        resp = self._mock_response(md_text="section text")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="file.pdf", binary=pdf_bytes)

        files_arg = mock_post.call_args.kwargs.get("files", {})
        assert "file" in files_arg
        _, sent_bytes, mime = files_arg["file"]
        assert sent_bytes == pdf_bytes
        assert mime == "application/pdf"

    def test_bytesio_binary_sent_correctly(self, tmp_path):
        p = _make_parser()
        pdf_bytes = b"%PDF-bytesio"
        resp = self._mock_response(md_text="text from bytesio")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="file.pdf", binary=io.BytesIO(pdf_bytes))

        files_arg = mock_post.call_args.kwargs.get("files", {})
        _, sent_bytes, _ = files_arg["file"]
        assert sent_bytes == pdf_bytes

    def test_json_doc_response_returns_sections(self, tmp_path):
        p = _make_parser()
        json_doc = {
            "type": "paragraph",
            "content": "Hello from JSON",
            "page_number": 1,
            "bounding_box": [0, 0, 100, 20],
        }
        resp = self._mock_response(json_doc=json_doc)

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp):
            sections, tables = p.parse_pdf(filepath="doc.pdf", binary=b"%PDF", parse_method="pipeline")

        assert any("Hello from JSON" in s[0] for s in sections)

    def test_md_text_fallback_when_no_json(self, tmp_path):
        p = _make_parser()
        resp = self._mock_response(json_doc=None, md_text="# Markdown heading\n\nBody text.")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp):
            sections, tables = p.parse_pdf(filepath="doc.pdf", binary=b"%PDF", parse_method="pipeline")

        assert len(sections) > 0
        assert tables == []

    def test_sanitize_true_sends_string_true(self):
        p = _make_parser()
        resp = self._mock_response(md_text="ok")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="doc.pdf", binary=b"%PDF", sanitize=True)

        data_arg = mock_post.call_args.kwargs.get("data", {})
        assert data_arg.get("sanitize") == "true"

    def test_sanitize_false_sends_string_false(self):
        p = _make_parser()
        resp = self._mock_response(md_text="ok")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="doc.pdf", binary=b"%PDF", sanitize=False)

        data_arg = mock_post.call_args.kwargs.get("data", {})
        assert data_arg.get("sanitize") == "false"

    def test_hybrid_and_image_output_forwarded(self):
        p = _make_parser()
        resp = self._mock_response(md_text="ok")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="doc.pdf", binary=b"%PDF",
                        hybrid="docling-fast", image_output="embedded")

        data_arg = mock_post.call_args.kwargs.get("data", {})
        assert data_arg.get("hybrid") == "docling-fast"
        assert data_arg.get("image_output") == "embedded"

    def test_optional_params_omitted_when_none(self):
        p = _make_parser()
        resp = self._mock_response(md_text="ok")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp) as mock_post:
            p.parse_pdf(filepath="doc.pdf", binary=b"%PDF")

        data_arg = mock_post.call_args.kwargs.get("data", {})
        assert "hybrid" not in data_arg
        assert "image_output" not in data_arg
        assert "sanitize" not in data_arg

    def test_callback_called_at_progress_points(self):
        p = _make_parser()
        resp = self._mock_response(md_text="text")
        cb = mock.MagicMock()

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp):
            p.parse_pdf(filepath="doc.pdf", binary=b"%PDF", callback=cb)

        progress_values = [call.args[0] for call in cb.call_args_list]
        assert 0.1 in progress_values
        assert 1.0 in progress_values

    def test_http_error_raises_runtime_error(self):
        p = _make_parser()

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", side_effect=requests.ConnectionError("down")):
            with pytest.raises(RuntimeError, match="service call failed"):
                p.parse_pdf(filepath="doc.pdf", binary=b"%PDF")

    def test_non_200_status_raises_runtime_error(self):
        p = _make_parser()
        resp = mock.MagicMock()
        resp.raise_for_status.side_effect = requests.HTTPError("500 Server Error")

        with mock.patch.object(p, "__images__"), \
             mock.patch("requests.post", return_value=resp):
            with pytest.raises(RuntimeError, match="service call failed"):
                p.parse_pdf(filepath="doc.pdf", binary=b"%PDF")


# ---------------------------------------------------------------------------
# crop() — bounds guard
# ---------------------------------------------------------------------------

class TestCrop:
    def test_returns_none_when_no_page_images(self):
        p = _make_parser()
        p.page_images = []
        result = p.crop("@@1\t10.0\t100.0\t20.0\t80.0##")
        assert result is None

    def test_returns_none_when_no_position_tags(self):
        p = _make_parser()
        p.page_images = [_fake_page_image()]
        result = p.crop("no tags here")
        assert result is None

    def test_out_of_range_page_index_filtered_returns_none(self):
        p = _make_parser()
        # Only 1 page rendered (index 0), but tag references page 5 (index 4)
        p.page_images = [_fake_page_image()]
        # Tag: page 5 → extract_positions returns pn=[4]
        tag = "@@5\t10.0\t100.0\t20.0\t80.0##"
        result = p.crop(tag)
        assert result is None

    def test_valid_page_index_does_not_raise(self):
        p = _make_parser()
        img = _fake_page_image(width=200, height=300)
        p.page_images = [img, img, img]
        # Tag references page 2 (index 1) — within rendered range.
        # Patch Image.new and alpha_composite at the module level to avoid
        # real ImagingCore requirements from mocked PIL images.
        tag = "@@2\t10.0\t100.0\t20.0\t80.0##"
        canvas = mock.MagicMock()
        canvas.paste = mock.MagicMock()
        try:
            with mock.patch.object(_mod.Image, "new", return_value=canvas), \
                 mock.patch.object(_mod.Image, "alpha_composite", return_value=img):
                p.crop(tag)
        except IndexError:
            pytest.fail("crop() raised IndexError for a valid page index")

    def test_need_position_false_returns_image_or_none(self):
        p = _make_parser()
        p.page_images = []
        result = p.crop("@@1\t10.0\t100.0\t20.0\t80.0##", need_position=False)
        assert result is None

    def test_need_position_true_returns_tuple_when_no_images(self):
        p = _make_parser()
        p.page_images = []
        result = p.crop("@@1\t10.0\t100.0\t20.0\t80.0##", need_position=True)
        assert result == (None, None)
