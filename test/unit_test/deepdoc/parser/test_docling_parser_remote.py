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
