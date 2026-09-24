import importlib.util
import sys
import types
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
PARSER_DIR = REPO_ROOT / "deepdoc" / "parser"

# A tag whose coordinate slot holds a page-range-looking value: the "@@" pattern
# accepts it, but float() cannot consume it.
MALFORMED = "@@1\t1-2\t3\t4\t5##"


@pytest.fixture(autouse=True)
def _isolate_stubs():
    """This file fabricates sys.modules entries; keep them out of the other files."""
    saved = dict(sys.modules)
    yield
    for name in [key for key in sys.modules if key not in saved]:
        del sys.modules[name]


class _AnyMeta(type):
    def __getattr__(cls, _item):
        return _Any


class _Any(metaclass=_AnyMeta):
    """Stand-in for any attribute of a stubbed module: callable, subclassable, nestable."""

    def __init__(self, *_args, **_kwargs):
        pass

    def __call__(self, *_args, **_kwargs):
        return _Any()

    def __getattr__(self, _item):
        return _Any()


def _stub(name, search_path=None):
    module = types.ModuleType(name)
    module.__path__ = [str(REPO_ROOT / search_path)] if search_path else []
    module.__getattr__ = lambda attr, _n=name: _Any
    sys.modules[name] = module
    return module


_CACHE: dict[str, object] = {}


def _load(parser_name):
    """Import a deepdoc parser for real, stubbing only what is not a parser module.

    Only ``extract_positions`` is exercised, which is a pure staticmethod, so the
    stubs never participate in the assertion. Missing modules are discovered from the
    import errors themselves, which also covers the conditional imports the parsers
    wrap in ``try`` blocks. A parser imported by another parser (``pdf_parser``) is
    loaded for real and registered under its dotted name: stubbing it passes in an
    isolated run but breaks in CI, where a sibling file has already imported the real
    module under that name.
    """
    for pkg in ("common", "deepdoc", "rag", "api"):
        if not getattr(sys.modules.get(pkg), "__path__", None):
            _stub(pkg, pkg)
    # Register the parsers under their dotted names, but never let the real
    # deepdoc/parser/__init__.py run: it imports every parser, and pdf_parser is
    # already mid-import here.
    if "deepdoc.parser" not in sys.modules:
        parser_pkg = _stub("deepdoc.parser")
        parser_pkg.__path__ = [str(PARSER_DIR)]

    dotted = f"deepdoc.parser.{parser_name}"
    if parser_name in _CACHE:
        sys.modules[dotted] = _CACHE[parser_name]
        return _CACHE[parser_name]

    path = PARSER_DIR / f"{parser_name}.py"
    existing = sys.modules.get(dotted)
    if existing is not None and str(getattr(existing, "__file__", "") or "") == str(path):
        # Already the real module (a sibling file imported it): reuse it.
        _CACHE[parser_name] = existing
        return existing

    if parser_name != "pdf_parser":
        # A sibling test file may have left a stub here; every parser except
        # pdf_parser itself imports names from it.
        sys.modules["deepdoc.parser.pdf_parser"] = _load("pdf_parser")

    for _attempt in range(200):
        spec = importlib.util.spec_from_file_location(dotted, path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[dotted] = module
        try:
            spec.loader.exec_module(module)
        except ModuleNotFoundError as exc:
            missing = exc.name or ""
            del sys.modules[dotted]
            if missing == dotted:
                raise
            leaf = missing.rsplit(".", 1)[-1]
            if missing.startswith("deepdoc.parser.") and (PARSER_DIR / f"{leaf}.py").exists():
                _load(leaf)
                continue
            if missing in sys.modules:
                raise
            sys.modules[missing] = _stub(missing)
        else:
            _CACHE[parser_name] = module
            return module
    raise AssertionError(f"could not import {parser_name}.py")


def _extractor(parser_name):
    module = _load(parser_name)
    for value in vars(module).values():
        if isinstance(value, type) and "extract_positions" in vars(value):
            return value.extract_positions
    raise AssertionError(f"{parser_name}.py exposes no extract_positions")


# (parser, a well-formed tag it must still parse, the expected tuple)
PARSERS = [
    pytest.param("docling_parser", "@@50\t45.0\t549.7\t-3.0\t737.9##", ([49], 45.0, 549.7, -3.0, 737.9), id="docling"),
    pytest.param("mineru_parser", "@@50\t45.0\t549.7\t-3.0\t737.9##", ([49], 45.0, 549.7, -3.0, 737.9), id="mineru"),
    pytest.param("mistral_parser", "@@3-4\t1.5\t2.5\t-0.5\t3.5##", ([2, 3], 1.5, 2.5, -0.5, 3.5), id="mistral"),
    pytest.param("opendataloader_parser", "@@50\t45.0\t549.7\t-3.0\t737.9##", ([49], 45.0, 549.7, -3.0, 737.9), id="opendataloader"),
    pytest.param("paddleocr_parser", "@@50\t45.0\t549.7\t-3.0\t737.9##", ([49], 45.0, 549.7, -3.0, 737.9), id="paddleocr"),
    pytest.param("pdf_parser", "@@3-4\t1.5\t2.5\t-0.5\t3.5##", ([2, 3], 1.5, 2.5, -0.5, 3.5), id="pdf"),
    pytest.param("somark_parser", "@@3-4\t1.5\t2.5\t-0.5\t3.5##", ([2, 3], 1.5, 2.5, -0.5, 3.5), id="somark"),
    # monkeyocrv2 stores one page number, not a page list.
    pytest.param("monkeyocrv2_parser", "@@50\t45.0\t549.7\t-3.0\t737.9##", (49, 45.0, 549.7, -3.0, 737.9), id="monkeyocrv2"),
]


@pytest.mark.parametrize(("parser_name", "valid_tag", "expected"), PARSERS)
def test_extract_positions_still_parses_signed_coordinates(parser_name, valid_tag, expected):
    extract = _extractor(parser_name)
    assert extract(valid_tag) == [expected]


@pytest.mark.parametrize(("parser_name", "valid_tag", "expected"), PARSERS)
def test_a_malformed_tag_is_skipped_and_not_fatal(parser_name, valid_tag, expected):
    """One unparsable tag must not abort extraction for the rest of the document.

    The pattern widened for negative coordinates also accepts a hyphen inside a
    coordinate slot, so ``float`` can still be handed something like ``1-2``. The
    Go port (ExtractPositions in
    internal/deepdoc/parser/pdf/util/position.go) answers that by warning and
    continuing; the Python side has to keep up, or a single bad tag costs every
    section image on the page.
    """
    extract = _extractor(parser_name)
    assert extract(MALFORMED) == []
    assert extract(f"{valid_tag} then {MALFORMED}") == [expected]
    assert extract(f"{MALFORMED} then {valid_tag}") == [expected]
