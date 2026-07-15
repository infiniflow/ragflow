import importlib.util
import sys
from pathlib import Path
from types import ModuleType


def _load_mistral_parser(monkeypatch):
    """Load mistral_parser.py directly, bypassing deepdoc/__init__.py's
    beartype_this_package() and the heavy deepdoc dependency chain.
    Mirrors test_somark_parser.py."""
    repo_root = Path(__file__).resolve().parents[4]

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
    pdf_parser_mod.MAXIMUM_PAGE_NUMBER = 100000
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda *_a, **_k: []
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    module_name = "test_mistral_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "mistral_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _make_parser(m, **kwargs):
    """Build a MistralParser without any network call. __init__ only sets attributes."""
    return m.MistralParser(base_url="https://api.mistral.ai/v1", api_key="", **kwargs)


def test_resolve_internal_type_maps_known_types(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("text") == "text"
    assert p._resolve_internal_type("title") == "text"
    assert p._resolve_internal_type("list") == "text"
    assert p._resolve_internal_type("image") == "image"
    assert p._resolve_internal_type("table") == "table"


def test_resolve_internal_type_drops_header_footer_by_default(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("header") is None
    assert p._resolve_internal_type("footer") is None


def test_resolve_internal_type_keeps_header_footer_when_flagged(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m, keep_header_footer=True)
    assert p._resolve_internal_type("header") == "text"
    assert p._resolve_internal_type("footer") == "text"


def test_resolve_internal_type_unknown_falls_back_to_text(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    p = _make_parser(m)
    assert p._resolve_internal_type("brand_new_type") == "text"


def test_block_text_image_returns_empty(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    assert m.MistralParser._block_text({"content": "x"}, "image") == ""


def test_block_text_strips_and_returns_content(monkeypatch):
    m = _load_mistral_parser(monkeypatch)
    assert m.MistralParser._block_text({"content": "  hi  "}, "text") == "hi"
