from __future__ import annotations

import importlib.util
import logging
import sys
import types
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[4]


class _Response:
    status_code = 200
    text = ""

    def __init__(self, payload, status_code: int = 200):
        self._payload = payload
        self.status_code = status_code

    def json(self):
        return self._payload


class _FakeImage:
    """Stands in for a rendered page; only its height is ever read."""

    def __init__(self, height: int, width: int = 600):
        self.size = (width, height)


def _load_docling_parser(monkeypatch):
    common_pkg = types.ModuleType("common")
    constants_mod = types.ModuleType("common.constants")
    constants_mod.MAXIMUM_PAGE_NUMBER = 1000

    deepdoc_pkg = types.ModuleType("deepdoc")
    parser_pkg = types.ModuleType("deepdoc.parser")
    parser_pkg.__path__ = []
    utils_mod = types.ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda _source: []

    pil_pkg = types.ModuleType("PIL")
    image_mod = types.ModuleType("PIL.Image")
    image_mod.Image = object
    pil_pkg.Image = image_mod

    monkeypatch.setitem(sys.modules, "common", common_pkg)
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)
    monkeypatch.setitem(sys.modules, "pdfplumber", types.ModuleType("pdfplumber"))
    monkeypatch.setitem(sys.modules, "PIL", pil_pkg)
    monkeypatch.setitem(sys.modules, "PIL.Image", image_mod)

    spec = importlib.util.spec_from_file_location(
        "_docling_parser_under_test",
        ROOT / "deepdoc" / "parser" / "docling_parser.py",
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_remote_chunked_200_standard_payload_falls_back(monkeypatch):
    module = _load_docling_parser(monkeypatch)
    calls = []

    def fake_post(_url, json, timeout):
        calls.append((json, timeout))
        return _Response({"document": {"md_content": "# Parsed\n\nbody"}})

    monkeypatch.setattr(module.requests, "post", fake_post)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    sections, tables = parser._parse_pdf_remote("sample.pdf", binary=b"%PDF", parse_method="raw")

    assert sections == [("# Parsed\n\nbody", "")]
    assert tables == []
    assert calls[0][0]["options"]["do_chunking"] is True


@pytest.mark.p2
def test_chunk_shape_helper_recognises_chunk_payloads(monkeypatch):
    """A response that is chunk-shaped (list, or dict with non-empty results/chunks)
    is classified as chunked regardless of which payload was sent."""
    module = _load_docling_parser(monkeypatch)
    assert module.DoclingParser._looks_like_chunk_response([{"text": "chunk-1"}]) is True
    assert module.DoclingParser._looks_like_chunk_response({"results": [{"text": "chunk-1"}, {"text": "chunk-2"}]}) is True
    assert module.DoclingParser._looks_like_chunk_response({"chunks": [{"text": "chunk-1"}]}) is True


@pytest.mark.p2
def test_chunk_shape_helper_rejects_standard_payloads(monkeypatch):
    """A standard conversion response, empty containers, and non-payload types
    are correctly classified as not-chunked."""
    module = _load_docling_parser(monkeypatch)
    standard = {"document": {"md_content": "body"}, "status": "success"}
    assert module.DoclingParser._looks_like_chunk_response(standard) is False
    assert module.DoclingParser._looks_like_chunk_response({}) is False
    assert module.DoclingParser._looks_like_chunk_response({"results": []}) is False
    assert module.DoclingParser._looks_like_chunk_response({"chunks": []}) is False
    assert module.DoclingParser._looks_like_chunk_response([]) is False
    assert module.DoclingParser._looks_like_chunk_response("not-a-payload") is False
    assert module.DoclingParser._looks_like_chunk_response(None) is False
    assert module.DoclingParser._looks_like_chunk_response(42) is False


@pytest.mark.p2
def test_remote_chunked_request_with_results_list_is_treated_as_chunked(monkeypatch):
    """A server that returns a ``results`` list (Docling Serve's native chunk
    shape) is treated as chunked and each chunk becomes a section."""
    module = _load_docling_parser(monkeypatch)

    def fake_post(_url, json, timeout):
        return _Response({"results": [{"text": "alpha"}, {"text": "beta"}]})

    monkeypatch.setattr(module.requests, "post", fake_post)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    sections, tables = parser._parse_pdf_remote("sample.pdf", binary=b"%PDF", parse_method="raw")

    assert sections == [("alpha", ""), ("beta", "")]
    assert tables == []


@pytest.mark.p2
def test_remote_top_level_list_response_is_treated_as_chunked(monkeypatch):
    """A server that returns a top-level JSON array of chunks is treated
    as chunked (matches the existing implicit assumption in the code)."""
    module = _load_docling_parser(monkeypatch)

    def fake_post(_url, json, timeout):
        return _Response([{"text": "first"}, {"text": "second"}])

    monkeypatch.setattr(module.requests, "post", fake_post)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    sections, _ = parser._parse_pdf_remote("sample.pdf", binary=b"%PDF", parse_method="raw")

    assert sections == [("first", ""), ("second", "")]


@pytest.mark.p2
def test_remote_chunked_request_with_ignored_flag_does_not_log_success(monkeypatch, caplog):
    """When Docling Serve silently drops the ``do_chunking`` flag and returns
    a standard conversion response, RAGFlow must not log a chunking-success
    message and must log a warning instead."""
    module = _load_docling_parser(monkeypatch)

    def fake_post(_url, json, timeout):
        return _Response({"document": {"md_content": "real content"}, "status": "success"})

    monkeypatch.setattr(module.requests, "post", fake_post)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    with caplog.at_level(logging.DEBUG, logger="DoclingParser"):
        sections, _ = parser._parse_pdf_remote("sample.pdf", binary=b"%PDF", parse_method="raw")

    assert sections == [("real content", "")]
    flat = " ".join(record.getMessage() for record in caplog.records)
    assert "Successfully used native chunking" not in flat
    assert "Server ignored chunking request" in flat


def _capture_remote_payloads(monkeypatch, module, reject_chunking: bool = False):
    """Fake out the remote server and the installation probe so ``parse_pdf`` can
    be driven end to end. Returns the list of payloads it posts; with
    ``reject_chunking`` the chunked attempts 422 so the standard fallback
    payloads are exercised too."""
    payloads = []

    def fake_post(_url, json, timeout):
        payloads.append(json)
        if reject_chunking and json["options"].get("do_chunking"):
            return _Response(None, status_code=422)
        return _Response({"document": {"md_content": "body"}})

    monkeypatch.setattr(module.requests, "post", fake_post)
    monkeypatch.setattr(module.DoclingParser, "check_installation", lambda _self, **_kw: True)
    return payloads


@pytest.mark.p2
def test_resolve_page_range_translates_ragflow_convention(monkeypatch):
    """RAGFlow's 0-based/exclusive task range becomes Docling's 1-based/inclusive
    one; an un-narrowed or empty range asks for the whole document instead."""
    module = _load_docling_parser(monkeypatch)
    resolve = module.DoclingParser._resolve_page_range

    assert resolve(0, 13) == (1, 13)
    assert resolve(144, 157) == (145, 157)
    assert resolve(0, module.MAXIMUM_PAGE_NUMBER) is None
    # Docling rejects end < start, so a degenerate range must not be sent.
    assert resolve(12, 12) is None


@pytest.mark.p2
def test_resolve_page_range_clamps_out_of_bounds_input(monkeypatch):
    """``parse_pdf`` is public, so out-of-range bounds must be clamped rather
    than passed on: Docling rejects a start below 1 outright."""
    module = _load_docling_parser(monkeypatch)
    resolve = module.DoclingParser._resolve_page_range
    maximum = module.MAXIMUM_PAGE_NUMBER

    assert resolve(-1, 5) == (1, 5)
    assert resolve(-10, maximum + 100) is None
    assert resolve(5, maximum + 100) == (6, maximum)

    for page_from, page_to in ((-1, 5), (5, maximum + 100), (0, 13), (144, 157)):
        start, end = resolve(page_from, page_to)
        assert start >= 1
        assert end >= start


@pytest.mark.p2
def test_line_tag_is_relative_to_the_rendered_window(monkeypatch):
    """Docling numbers pages from the start of the document, but page_images only
    holds the rendered window and ``crop`` adds ``page_from`` back — so a tag must
    name the page relative to that window."""
    module = _load_docling_parser(monkeypatch)
    parser = module.DoclingParser()
    parser.page_from = 144
    parser.page_images = [_FakeImage(800), _FakeImage(800)]

    # absolute page 145 is the first page of a window starting at page_from=144
    bbox = module._BBox(page_no=145, x0=1.0, y0=10.0, x1=2.0, y1=20.0)
    tag = parser._make_line_tag(bbox)

    assert tag.startswith("@@1\t")
    # y coordinates are flipped against the height of the page actually rendered
    assert "790.0\t780.0" in tag
    # and crop() maps it back onto the absolute page it came from
    pages, *_ = module.DoclingParser.extract_positions(tag)[0]
    assert pages[0] + parser.page_from == 144


@pytest.mark.p2
def test_page_range_is_threaded_into_remote_payload(monkeypatch):
    """A task that owns pages 145-157 must ask Docling Serve for exactly that
    window instead of converting the whole document — on every payload variant
    the fallback chain can reach, not just the first one."""
    module = _load_docling_parser(monkeypatch)
    payloads = _capture_remote_payloads(monkeypatch, module, reject_chunking=True)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    parser.parse_pdf("sample.pdf", binary=b"%PDF", page_from=144, page_to=157)

    # two rejected chunked attempts, then the standard payload that succeeds
    assert len(payloads) == 3
    assert all(payload["options"]["page_range"] == [145, 157] for payload in payloads)


@pytest.mark.p2
def test_full_document_request_omits_page_range(monkeypatch):
    """A task covering the whole document omits ``page_range`` entirely, so the
    server converts everything."""
    module = _load_docling_parser(monkeypatch)
    payloads = _capture_remote_payloads(monkeypatch, module, reject_chunking=True)

    parser = module.DoclingParser(docling_server_url="http://docling.local")
    parser.parse_pdf("sample.pdf", binary=b"%PDF")

    assert len(payloads) == 3
    assert all("page_range" not in payload["options"] for payload in payloads)


def _drive_local_conversion(monkeypatch, module, tmp_path, **parse_kwargs):
    """Run ``parse_pdf``'s local branch against stubbed docling classes. Returns
    the page bounds ``__images__`` was rendered with and the range handed to
    ``DocumentConverter.convert``."""
    captured = {}

    class _FakeConverter:
        def __init__(self, *_a, **_kw):
            pass

        def convert(self, _source, page_range=None):
            captured["convert_range"] = page_range
            return types.SimpleNamespace(document=types.SimpleNamespace(texts=[], tables=[], pictures=[]))

    def fake_images(_self, _fnm, zoomin=1, page_from=0, page_to=module.MAXIMUM_PAGE_NUMBER, callback=None):
        captured["rendered"] = (page_from, page_to)

    monkeypatch.setattr(module, "DocumentConverter", _FakeConverter)
    monkeypatch.setattr(module, "PdfPipelineOptions", lambda: types.SimpleNamespace(do_formula_enrichment=False))
    monkeypatch.setattr(module, "PdfFormatOption", lambda **_kw: object())
    monkeypatch.setattr(module, "InputFormat", types.SimpleNamespace(PDF="pdf"))
    monkeypatch.setattr(module.DoclingParser, "__images__", fake_images)
    monkeypatch.setattr(module.DoclingParser, "check_installation", lambda _self, **_kw: True)
    monkeypatch.setattr(module.DoclingParser, "_effective_server_url", lambda _self, *_a, **_kw: "")

    pdf = tmp_path / "sample.pdf"
    pdf.write_bytes(b"%PDF-1.4 fake")

    parser = module.DoclingParser()
    parser.parse_pdf(str(pdf), **parse_kwargs)
    return captured


@pytest.mark.p2
def test_local_conversion_renders_only_the_selected_pages(monkeypatch, tmp_path):
    """Rasterising is as expensive as converting, so a page-split task must render
    its own window only — and page_from must match it, since the position logic
    indexes page_images relative to the window."""
    module = _load_docling_parser(monkeypatch)
    captured = _drive_local_conversion(monkeypatch, module, tmp_path, page_from=144, page_to=157)

    assert captured["convert_range"] == (145, 157)
    # __images__ is 0-based with an exclusive stop, so the same 13 pages
    assert captured["rendered"] == (144, 157)


@pytest.mark.p2
def test_local_conversion_of_whole_document_renders_every_page(monkeypatch, tmp_path):
    """A task that owns the whole document renders it whole and converts it whole."""
    module = _load_docling_parser(monkeypatch)
    captured = _drive_local_conversion(monkeypatch, module, tmp_path)

    assert captured["convert_range"] is None
    assert captured["rendered"] == (0, module.MAXIMUM_PAGE_NUMBER)


@pytest.mark.p2
def test_crop_without_page_images_returns_none(monkeypatch):
    """Position tags are emitted even when page rendering failed and left
    ``page_images`` empty, so ``crop`` must bail out instead of raising
    IndexError while indexing the empty list."""
    module = _load_docling_parser(monkeypatch)
    parser = module.DoclingParser(docling_server_url="http://docling.local")
    parser.page_images = []

    tag = "@@1\t1.0\t2.0\t3.0\t4.0##"
    assert parser.crop(tag, need_position=True) == (None, None)
    assert parser.crop(tag) is None


@pytest.mark.p2
def test_crop_drops_positions_beyond_rendered_pages(monkeypatch):
    """A tag naming a page past the rendered range must be filtered out rather
    than indexed, mirroring the range check in cropout_docling_table."""
    module = _load_docling_parser(monkeypatch)
    parser = module.DoclingParser(docling_server_url="http://docling.local")
    # a single rendered page; the sentinel is never indexed because the
    # out-of-range position is dropped first.
    parser.page_images = [object()]

    tag = "@@5\t1.0\t2.0\t3.0\t4.0##"
    assert parser.crop(tag, need_position=True) == (None, None)
    assert parser.crop(tag) is None
