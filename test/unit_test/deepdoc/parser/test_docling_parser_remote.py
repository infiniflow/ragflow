from __future__ import annotations

import importlib.util
import sys
import types
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[4]


class _Response:
    status_code = 200
    text = ""

    def __init__(self, payload):
        self._payload = payload

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
