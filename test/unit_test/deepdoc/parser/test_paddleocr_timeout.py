"""Regression tests for PaddleOCR timeout propagation."""

from __future__ import annotations

import importlib.util
import json
import sys
import types
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[4]


def _make_module(name: str, **attrs):
    module = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    return module


def _load_module(module_name: str, path: Path, injected_modules: dict[str, object]):
    saved_modules = {name: sys.modules.get(name) for name in injected_modules}
    try:
        sys.modules.update(injected_modules)
        spec = importlib.util.spec_from_file_location(module_name, path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"Unable to load module from {path}")

        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
        return module
    finally:
        for name, original in saved_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


def _base_injected_modules():
    deepdoc_pkg = _make_module("deepdoc")
    deepdoc_pkg.__path__ = [str(PROJECT_ROOT / "deepdoc")]

    parser_pkg = _make_module("deepdoc.parser")
    parser_pkg.__path__ = [str(PROJECT_ROOT / "deepdoc" / "parser")]

    pil_image_pkg = _make_module("PIL.Image")
    pil_image_pkg.Image = object
    pil_pkg = _make_module("PIL", Image=pil_image_pkg)

    return {
        "numpy": _make_module("numpy"),
        "pdfplumber": _make_module("pdfplumber"),
        "PIL": pil_pkg,
        "PIL.Image": pil_image_pkg,
        "deepdoc": deepdoc_pkg,
        "deepdoc.parser": parser_pkg,
        "deepdoc.parser.utils": _make_module(
            "deepdoc.parser.utils",
            extract_pdf_outlines=lambda *_args, **_kwargs: [],
        ),
        "deepdoc.parser.pdf_parser": _make_module(
            "deepdoc.parser.pdf_parser",
            RAGFlowPdfParser=type("RAGFlowPdfParser", (), {}),
        ),
        "deepdoc.parser.mineru_parser": _make_module(
            "deepdoc.parser.mineru_parser",
            MinerUParser=type("MinerUParser", (), {}),
        ),
    }


PADDLEOCR_MODULE = _load_module(
    "_paddleocr_parser_test",
    PROJECT_ROOT / "deepdoc" / "parser" / "paddleocr_parser.py",
    _base_injected_modules(),
)

OCR_MODEL_MODULE = _load_module(
    "_ocr_model_test",
    PROJECT_ROOT / "rag" / "llm" / "ocr_model.py",
    {
        **_base_injected_modules(),
        "deepdoc.parser.paddleocr_parser": PADDLEOCR_MODULE,
    },
)

PaddleOCRParser = PADDLEOCR_MODULE.PaddleOCRParser
PaddleOCRConfig = PADDLEOCR_MODULE.PaddleOCRConfig
PaddleOCROcrModel = OCR_MODEL_MODULE.PaddleOCROcrModel


def test_paddleocr_model_reads_request_timeout_from_json_config(monkeypatch):
    captured = {}

    def fake_init(self, api_url=None, access_token=None, algorithm="PaddleOCR-VL", *, request_timeout=600):
        captured["api_url"] = api_url
        captured["access_token"] = access_token
        captured["algorithm"] = algorithm
        captured["request_timeout"] = request_timeout
        self.request_timeout = request_timeout

    monkeypatch.setattr(PaddleOCRParser, "__init__", fake_init)

    model = PaddleOCROcrModel(
        json.dumps(
            {
                "api_key": {
                    "paddleocr_api_url": "https://paddleocr.example.com",
                    "paddleocr_access_token": "secret-token",
                    "paddleocr_algorithm": "PaddleOCR-VL",
                    "paddleocr_request_timeout": "1800",
                }
            }
        ),
        "paddleocr-model",
    )

    assert captured["api_url"] == "https://paddleocr.example.com"
    assert captured["access_token"] == "secret-token"
    assert captured["algorithm"] == "PaddleOCR-VL"
    assert captured["request_timeout"] == 1800
    assert model.paddleocr_request_timeout == 1800


def test_paddleocr_parse_pdf_forwards_request_timeout_to_http_call(monkeypatch):
    parser = PaddleOCRParser(api_url="https://paddleocr.example.com", request_timeout=600)

    sent = {}

    def fake_send_request(data, config, callback):
        sent["data"] = data
        sent["timeout"] = config.request_timeout
        sent["callback"] = callback
        return {}

    monkeypatch.setattr(parser, "__images__", lambda *args, **kwargs: None)
    monkeypatch.setattr(parser, "_send_request", fake_send_request)
    monkeypatch.setattr(parser, "_transfer_to_sections", lambda *args, **kwargs: [])
    monkeypatch.setattr(parser, "_transfer_to_tables", lambda *args, **kwargs: [])

    sections, tables = parser.parse_pdf(
        filepath="dummy.pdf",
        binary=b"dummy-bytes",
        request_timeout=1800,
    )

    assert sent["timeout"] == 1800
    assert isinstance(sent["data"], bytes)
    assert sections == []
    assert tables == []


def test_paddleocr_send_request_uses_configured_timeout(monkeypatch):
    parser = PaddleOCRParser(api_url="https://paddleocr.example.com", request_timeout=600)

    response = types.SimpleNamespace(
        raise_for_status=lambda: None,
        json=lambda: {"errorCode": 0, "result": {"ok": True}},
    )
    post_calls = {}

    def fake_post(url, json=None, headers=None, timeout=None):
        post_calls["url"] = url
        post_calls["json"] = json
        post_calls["headers"] = headers
        post_calls["timeout"] = timeout
        return response

    monkeypatch.setattr(PADDLEOCR_MODULE.requests, "post", fake_post)

    result = parser._send_request(
        b"dummy-bytes",
        PaddleOCRConfig(api_url="https://paddleocr.example.com", request_timeout=1800),
        callback=None,
    )

    assert post_calls["url"] == "https://paddleocr.example.com"
    assert post_calls["timeout"] == 1800
    assert result == {"ok": True}
