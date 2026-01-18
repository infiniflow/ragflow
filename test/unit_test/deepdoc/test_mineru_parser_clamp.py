import importlib.util
import importlib.machinery
import sys
import types
from pathlib import Path

# Provide a minimal dummy RAGFlowPdfParser to avoid importing heavy submodules during tests
pdf_parser_module = types.ModuleType("deepdoc.parser.pdf_parser")
class DummyRAGFlowPdfParser:
    pass
pdf_parser_module.RAGFlowPdfParser = DummyRAGFlowPdfParser
sys.modules["deepdoc.parser.pdf_parser"] = pdf_parser_module

# Load mineru_parser module directly from source file to avoid package import side-effects
spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.mineru_parser",
    Path(__file__).resolve().parent.parent.parent.parent / "deepdoc" / "parser" / "mineru_parser.py",
)
module = importlib.util.module_from_spec(spec)
loader = spec.loader
assert loader is not None
loader.exec_module(module)
MinerUParser = module.MinerUParser


def test_clamp_accepts_ints_and_floats():
    parser = MinerUParser()
    # image size 100x200
    x0, y0, x1, y1 = parser._clamp_coordinates_to_image(10, 20, 90, 180, 100, 200)
    assert (x0, y0, x1, y1) == (10, 20, 90, 180)

    x0, y0, x1, y1 = parser._clamp_coordinates_to_image(10.5, 20.2, 90.9, 199.9, 100, 200)
    assert (x0, y0, x1, y1) == (10, 20, 90, 199)


def test_clamp_handles_out_of_bounds_and_zero_size():
    parser = MinerUParser()
    # right <= left -> expand to ensure at least 1px
    x0, y0, x1, y1 = parser._clamp_coordinates_to_image(95, 10, 95, 10, 100, 50)
    assert x1 - x0 >= 1
    assert y1 - y0 >= 1

    # negative coordinates become 0
    x0, y0, x1, y1 = parser._clamp_coordinates_to_image(-10, -5, 10, 5, 100, 50)
    assert x0 == 0 and y0 == 0
